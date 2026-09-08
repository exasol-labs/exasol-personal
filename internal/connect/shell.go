// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/internal/connect/readline"
	"github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"github.com/exasol/exasol-personal/internal/util"
)

const (
	historyFileName = "exasol_history"
	historyDirPerm  = 0o700
)

const exitCommand = "exit"

// ProcessInputFunc defines a way to process a shell input.
type ProcessInputFunc func(input string) error

type ShellOpts struct {
	ExecuteOnSemicolon bool
}

type shell struct {
	lineReader          types.LineReader
	processInput        ProcessInputFunc
	executeOnSemicolon  bool
	pendingStatementBuf string
}

func newShell(lineReader types.LineReader, processInput ProcessInputFunc, opts ShellOpts) *shell {
	return &shell{
		lineReader:         lineReader,
		processInput:       processInput,
		executeOnSemicolon: opts.ExecuteOnSemicolon,
	}
}

func (sh *shell) close() error {
	return sh.lineReader.Close()
}

func (sh *shell) execStatement(stmt string) {
	if err := sh.processInput(stmt); err != nil {
		slog.Error(err.Error())
	}
}

func (sh *shell) handleEOF() {
	remaining := strings.TrimSpace(sh.pendingStatementBuf)
	if remaining == "" {
		return
	}

	sh.execStatement(remaining)
}

func (sh *shell) run() error {
	for {
		slog.Debug("reading next line")

		line, err := sh.lineReader.Readline()
		if err != nil {
			if errors.Is(err, types.ErrInterrupt) {
				continue
			} else if errors.Is(err, io.EOF) {
				sh.handleEOF()

				return nil
			}

			return err
		}

		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == exitCommand && strings.TrimSpace(sh.pendingStatementBuf) == "" {
			slog.Debug("got the exit command, exiting")
			return nil
		}

		if !sh.executeOnSemicolon {
			sh.execStatement(trimmedLine)
			continue
		}

		sh.processInputSemicolonMode(line)
	}
}

func (sh *shell) processInputSemicolonMode(line string) {
	if sh.pendingStatementBuf != "" {
		sh.pendingStatementBuf += "\n"
	}
	sh.pendingStatementBuf += line

	statements, remainder := splitStatements(sh.pendingStatementBuf)
	sh.pendingStatementBuf = remainder

	for _, statement := range statements {
		sh.execStatement(strings.TrimSpace(statement))
	}
}

// CREATE ... SCRIPT / FUNCTION definitions terminate on a lone '/' rather than ';' (EXAplus
// rule), so semicolons inside a script body are not statement terminators.
func splitStatements(sql string) ([]string, string) {
	var statements []string
	start := 0

	for start < len(sql) {
		term, ok := findStatementTerminator(sql, start)
		if !ok {
			break
		}
		if statement := strings.TrimSpace(sql[start:term.statementEnd]); statement != "" {
			statements = append(statements, statement)
		}
		start = term.nextStart
	}

	return statements, sql[start:]
}

func findStatementTerminator(sql string, from int) (terminator, bool) {
	if looksLikeScriptDDL(sql[from:]) {
		return findScriptTerminator(sql, from)
	}

	return findSemicolonTerminator(sql, from)
}

// quoteCommentState tracks which quoted-string or comment span the scanner is
// currently inside, so ';' terminators found inside them are ignored.
type quoteCommentState struct {
	inSingleQuotes bool
	inDoubleQuotes bool
	inLineComment  bool
	inBlockComment bool
}

func findSemicolonTerminator(sql string, from int) (terminator, bool) {
	var state quoteCommentState

	for charIndex := from; charIndex < len(sql); charIndex++ {
		switch {
		case state.inLineComment:
			charIndex += consumeLineComment(&state, sql, charIndex)
		case state.inBlockComment:
			charIndex += consumeBlockComment(&state, sql, charIndex)
		case state.inSingleQuotes:
			charIndex += consumeSingleQuote(&state, sql, charIndex)
		case state.inDoubleQuotes:
			charIndex += consumeDoubleQuote(&state, sql, charIndex)
		default:
			term, found, skip := consumeDefault(&state, sql, charIndex)
			if found {
				return term, true
			}
			charIndex += skip
		}
	}

	return terminator{}, false
}

func consumeLineComment(state *quoteCommentState, sql string, charIndex int) int {
	if sql[charIndex] == '\n' {
		state.inLineComment = false
	}

	return 0
}

func consumeBlockComment(state *quoteCommentState, sql string, charIndex int) int {
	if sql[charIndex] == '*' && charIndex+1 < len(sql) && sql[charIndex+1] == '/' {
		state.inBlockComment = false

		return 1
	}

	return 0
}

func consumeSingleQuote(state *quoteCommentState, sql string, charIndex int) int {
	if sql[charIndex] == '\'' && charIndex+1 < len(sql) && sql[charIndex+1] == '\'' {
		return 1
	}
	if sql[charIndex] == '\'' {
		state.inSingleQuotes = false
	}

	return 0
}

func consumeDoubleQuote(state *quoteCommentState, sql string, charIndex int) int {
	if sql[charIndex] == '"' && charIndex+1 < len(sql) && sql[charIndex+1] == '"' {
		return 1
	}
	if sql[charIndex] == '"' {
		state.inDoubleQuotes = false
	}

	return 0
}

func consumeDefault(state *quoteCommentState, sql string, charIndex int) (terminator, bool, int) {
	switch sql[charIndex] {
	case '\'':
		state.inSingleQuotes = true
	case '"':
		state.inDoubleQuotes = true
	case '-':
		if charIndex+1 < len(sql) && sql[charIndex+1] == '-' {
			state.inLineComment = true

			return terminator{}, false, 1
		}
	case '/':
		if charIndex+1 < len(sql) && sql[charIndex+1] == '*' {
			state.inBlockComment = true

			return terminator{}, false, 1
		}
	case ';':
		return terminator{statementEnd: charIndex, nextStart: charIndex + 1}, true, 0
	default:
	}

	return terminator{}, false, 0
}

// SCRIPT and FUNCTION bodies may contain ';', so they terminate on '/' instead.
var scriptBodyKeywords = []string{"SCRIPT", "FUNCTION"}

func looksLikeScriptDDL(sql string) bool {
	token, pos := nextToken(sql, 0)
	if token != "CREATE" {
		return false
	}

	for {
		token, pos = nextToken(sql, pos)
		if token == "" || token == "AS" {
			return false
		}
		if slices.Contains(scriptBodyKeywords, token) {
			return true
		}
	}
}

func nextToken(sql string, from int) (string, int) {
	start := skipSpaceAndComments(sql, from)
	end := start
	for end < len(sql) && !isTokenSeparator(sql, end) {
		if sql[end] == '\'' || sql[end] == '"' {
			end = skipQuoted(sql, end)

			continue
		}
		end++
	}
	if end == start {
		return "", end
	}

	return strings.ToUpper(sql[start:end]), end
}

func isTokenSeparator(sql string, pos int) bool {
	return isSpaceByte(sql[pos]) || sql[pos] == '(' || sql[pos] == ';' ||
		strings.HasPrefix(sql[pos:], "--") || strings.HasPrefix(sql[pos:], "/*")
}

func skipQuoted(sql string, pos int) int {
	quote := sql[pos]
	for cursor := pos + 1; cursor < len(sql); cursor++ {
		if sql[cursor] != quote {
			continue
		}
		if cursor+1 < len(sql) && sql[cursor+1] == quote {
			cursor++

			continue
		}

		return cursor + 1
	}

	return len(sql)
}

func skipSpaceAndComments(sql string, from int) int {
	for pos := from; pos < len(sql); {
		switch {
		case isSpaceByte(sql[pos]):
			pos++
		case strings.HasPrefix(sql[pos:], "--"):
			newline := strings.IndexByte(sql[pos:], '\n')
			if newline < 0 {
				return len(sql)
			}
			pos += newline + 1
		case strings.HasPrefix(sql[pos:], "/*"):
			end := strings.Index(sql[pos:], "*/")
			if end < 0 {
				return len(sql)
			}
			pos += end + len("*/")
		default:
			return pos
		}
	}

	return len(sql)
}

type terminator struct {
	statementEnd int
	nextStart    int
}

func findScriptTerminator(sql string, from int) (terminator, bool) {
	for lineStart := from; lineStart < len(sql); {
		newline := strings.IndexByte(sql[lineStart:], '\n')
		line := sql[lineStart:]
		if newline >= 0 {
			line = sql[lineStart : lineStart+newline]
		}
		if strings.TrimSpace(line) == "/" {
			if newline < 0 {
				return terminator{
					statementEnd: lineStart,
					nextStart:    len(sql),
				}, true
			}

			return terminator{
				statementEnd: lineStart,
				nextStart:    lineStart + newline + 1,
			}, true
		}
		if newline < 0 {
			break
		}
		lineStart += newline + 1
	}

	return terminator{}, false
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

func HistoryFilePath() (string, error) {
	historyDir, err := launcherpaths.HistoryRootPath()
	if err != nil {
		return "", err
	}

	historyFilePath := filepath.Join(historyDir, historyFileName)

	slog.Debug("obtained history file path", "path", historyFilePath)

	return historyFilePath, nil
}

func runShellImpl(
	lineReader types.LineReader,
	processInput ProcessInputFunc,
	opts ShellOpts,
) error {
	shell := newShell(lineReader, processInput, opts)

	defer shell.close()

	return shell.run()
}

// runStatements executes the ;-separated statements in sql non-interactively,
// in order, stopping at and returning the first error. It uses the same
// quote- and comment-aware splitting as the interactive shell. Any non-empty
// trailing remainder after the final terminator is executed as a final
// statement, mirroring the shell's end-of-input handling.
func runStatements(sql string, processInput ProcessInputFunc) error {
	for _, statement := range nonInteractiveStatements(sql) {
		if err := processInput(strings.TrimSpace(statement)); err != nil {
			return err
		}
	}

	return nil
}

func nonInteractiveStatements(sql string) []string {
	statements, remainder := splitStatements(sql)
	if trailing := strings.TrimSpace(remainder); trailing != "" {
		statements = append(statements, trailing)
	}

	return statements
}

// RunShell runs the shell, processing incoming input
// with the passed callback. Blocks until the shell exits.
func RunShell(processInput ProcessInputFunc) error {
	return RunShellWithOpts(processInput, ShellOpts{
		ExecuteOnSemicolon: true,
	})
}

func RunShellWithOpts(processInput ProcessInputFunc, opts ShellOpts) error {
	if !util.IsInteractiveStdin() {
		return runShellImpl(readline.NewBuffered(os.Stdin), processInput, opts)
	}

	lineReader, err := newInteractiveLineReader()
	if err != nil {
		return err
	}

	return runShellImpl(lineReader, processInput, opts)
}

func newInteractiveLineReader() (types.LineReader, error) {
	historyFilePath, err := ensureHistoryFileDir()
	if err != nil {
		return nil, err
	}

	lineReader, err := readline.New(historyFilePath)
	if err != nil {
		return nil, err
	}

	return lineReader, nil
}

// ensureHistoryFileDir resolves the history file path and creates the
// directory holding it. readline responds to a history file it cannot open by
// silently keeping no history at all, so on a fresh install nothing would
// ever be persisted unless the directory is in place first.
func ensureHistoryFileDir() (string, error) {
	historyFilePath, err := HistoryFilePath()
	if err != nil {
		return "", fmt.Errorf("couldn't get the history file path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(historyFilePath), historyDirPerm); err != nil {
		return "", fmt.Errorf("couldn't create the history directory: %w", err)
	}

	return historyFilePath, nil
}
