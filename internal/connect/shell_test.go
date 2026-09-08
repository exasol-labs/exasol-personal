// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/connect/readline"
	"github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/connect/types/typesfakes"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"github.com/stretchr/testify/require"
)

var (
	errTest            = errors.New("error")
	errInputsProcessor = errors.New("inputs processor error")
)

type mockInputsProcessor struct {
	callCount int
	inputs    []string
	retErr    bool
	failOn    map[string]error
}

func (mp *mockInputsProcessor) processInput(input string) error {
	mp.callCount++
	if err := mp.failOn[input]; err != nil {
		return err
	}
	if mp.retErr {
		return errInputsProcessor
	}

	mp.inputs = append(mp.inputs, input)

	return nil
}

func (mp *mockInputsProcessor) returnError() { mp.retErr = true }

func (mp *mockInputsProcessor) failOnInput(input string, err error) {
	if mp.failOn == nil {
		mp.failOn = map[string]error{}
	}
	mp.failOn[input] = err
}

// nolint: revive
func TestRunShell(t *testing.T) {
	t.Parallel()

	type mocks struct {
		shell           *typesfakes.FakeLineReader
		inputsProcessor *mockInputsProcessor
	}

	for _, test := range []struct {
		name  string
		opts  ShellOpts
		given func(*mocks)
		then  func(*testing.T, *mocks, error)
	}{
		{
			name: "single query without semicolon is buffered",
			opts: ShellOpts{ExecuteOnSemicolon: true},
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "SELECT 1 FROM Dual", nil)
				mocks.shell.ReadlineReturnsOnCall(1, "", errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTest)
				require.Equal(t, 2, mocks.shell.ReadlineCallCount())
				require.Equal(t, 0, mocks.inputsProcessor.callCount)
			},
		},
		{
			name: "EOF flushes buffered statement without semicolon",
			opts: ShellOpts{ExecuteOnSemicolon: true},
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "SELECT 1 FROM Dual", nil)
				mocks.shell.ReadlineReturnsOnCall(1, "", io.EOF)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.NoError(t, err)
				require.Equal(t, 2, mocks.shell.ReadlineCallCount())
				require.Equal(t, []string{"SELECT 1 FROM Dual"}, mocks.inputsProcessor.inputs)
			},
		},
		{
			name: "interrupt",
			opts: ShellOpts{ExecuteOnSemicolon: true},
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "", types.ErrInterrupt)
				mocks.shell.ReadlineReturnsOnCall(1, "", errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTest)
				require.Equal(t, 2, mocks.shell.ReadlineCallCount())
				require.Equal(t, 0, mocks.inputsProcessor.callCount)
			},
		},
		{
			name: "multiline query executes on semicolon",
			opts: ShellOpts{ExecuteOnSemicolon: true},
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "OPEN SCHEMA dummy  ", nil)
				mocks.shell.ReadlineReturnsOnCall(1, "SELECT 1 FROM Dual;", nil)
				mocks.shell.ReadlineReturnsOnCall(2, "", errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTest)
				require.Equal(t, []string{"OPEN SCHEMA dummy  \nSELECT 1 FROM Dual"}, mocks.inputsProcessor.inputs)
			},
		},
		{
			name: "multiple queries same line execute separately",
			opts: ShellOpts{ExecuteOnSemicolon: true},
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "OPEN SCHEMA dummy;SELECT 1 FROM Dual;", nil)
				mocks.shell.ReadlineReturnsOnCall(1, "", errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTest)
				require.Equal(t, []string{
					"OPEN SCHEMA dummy",
					"SELECT 1 FROM Dual",
				}, mocks.inputsProcessor.inputs)
			},
		},
		{
			name: "input processor error",
			opts: ShellOpts{ExecuteOnSemicolon: true},
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "SELECT 1 FROM Dual;", nil)
				mocks.shell.ReadlineReturnsOnCall(1, "", errTest)
				mocks.inputsProcessor.returnError()
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				// Inputs processor error shouldn't stop it.
				require.ErrorIs(t, err, errTest)
				require.Equal(t, 2, mocks.shell.ReadlineCallCount())
				require.Equal(t, 1, mocks.inputsProcessor.callCount)
			},
		},
		{
			name: "semicolon mode continues after a failed statement on same line",
			opts: ShellOpts{ExecuteOnSemicolon: true},
			given: func(mocks *mocks) {
				mocks.inputsProcessor.failOnInput("INVALID SQL", errInputsProcessor)
				mocks.shell.ReadlineReturnsOnCall(
					0,
					"SELECT 1;INVALID SQL;SELECT 2;",
					nil,
				)
				mocks.shell.ReadlineReturnsOnCall(1, "", errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTest)
				require.Equal(t, 3, mocks.inputsProcessor.callCount)
				require.Equal(t, []string{"SELECT 1", "SELECT 2"}, mocks.inputsProcessor.inputs)
			},
		},
		{
			name: "line mode executes per line",
			opts: ShellOpts{ExecuteOnSemicolon: false},
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "OPEN SCHEMA dummy  ", nil)
				mocks.shell.ReadlineReturnsOnCall(1, "  SELECT 1 FROM exa", nil)
				mocks.shell.ReadlineReturnsOnCall(2, "SELECT 1 FROM Dual", nil)
				mocks.shell.ReadlineReturnsOnCall(3, "", errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTest)
				require.Equal(t, []string{
					"OPEN SCHEMA dummy",
					"SELECT 1 FROM exa",
					"SELECT 1 FROM Dual",
				}, mocks.inputsProcessor.inputs)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mocks := &mocks{&typesfakes.FakeLineReader{}, &mockInputsProcessor{}}
			test.given(mocks)

			err := runShellImpl(mocks.shell, mocks.inputsProcessor.processInput, test.opts)

			test.then(t, mocks, err)
		})
	}
}

func TestNonInteractiveLineReader(t *testing.T) {
	t.Parallel()

	// Given
	processor := &mockInputsProcessor{}

	// When
	err := runShellImpl(readline.NewBuffered(
		strings.NewReader("SELECT 1 FROM Dual;\nexit\n"),
	), processor.processInput, ShellOpts{
		ExecuteOnSemicolon: true,
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, []string{"SELECT 1 FROM Dual"}, processor.inputs)
}

func TestRunStatements(t *testing.T) {
	t.Parallel()

	t.Run("executes a single statement without a terminator", func(t *testing.T) {
		t.Parallel()

		processor := &mockInputsProcessor{}
		err := runStatements("SELECT 1", processor.processInput)

		require.NoError(t, err)
		require.Equal(t, []string{"SELECT 1"}, processor.inputs)
	})

	t.Run("executes semicolon-separated statements in order", func(t *testing.T) {
		t.Parallel()

		processor := &mockInputsProcessor{}
		err := runStatements("SELECT 1; SELECT 2; SELECT 3", processor.processInput)

		require.NoError(t, err)
		require.Equal(t, []string{"SELECT 1", "SELECT 2", "SELECT 3"}, processor.inputs)
	})

	t.Run("skips empty and trailing segments", func(t *testing.T) {
		t.Parallel()

		processor := &mockInputsProcessor{}
		err := runStatements("SELECT 1;; SELECT 2;\n  \n", processor.processInput)

		require.NoError(t, err)
		require.Equal(t, []string{"SELECT 1", "SELECT 2"}, processor.inputs)
	})

	t.Run("stops at and returns the first error", func(t *testing.T) {
		t.Parallel()

		processor := &mockInputsProcessor{}
		processor.failOnInput("INVALID SQL", errInputsProcessor)

		err := runStatements("SELECT 1; INVALID SQL; SELECT 2", processor.processInput)

		require.ErrorIs(t, err, errInputsProcessor)
		// SELECT 1 ran, INVALID SQL failed and aborted, SELECT 2 never ran.
		require.Equal(t, 2, processor.callCount)
		require.Equal(t, []string{"SELECT 1"}, processor.inputs)
	})
}

func TestSplitSemicolonTerminatedStatements(t *testing.T) {
	t.Parallel()

	t.Run("keeps semicolons inside single quotes", func(t *testing.T) {
		t.Parallel()

		statements, remainder := splitStatements("SELECT 'a;b';")
		require.Equal(t, []string{"SELECT 'a;b'"}, statements)
		require.Empty(t, remainder)
	})

	t.Run("returns remaining unterminated statement", func(t *testing.T) {
		t.Parallel()

		sql := "OPEN SCHEMA foo;" +
			"SELECT 1 FROM dual"
		statements, remainder := splitStatements(sql)
		require.Equal(t, []string{"OPEN SCHEMA foo"}, statements)
		require.Equal(t, "SELECT 1 FROM dual", remainder)
	})

	t.Run("keeps semicolons inside double quotes", func(t *testing.T) {
		t.Parallel()

		statements, remainder := splitStatements(`SELECT "a;b";`)
		require.Equal(t, []string{`SELECT "a;b"`}, statements)
		require.Empty(t, remainder)
	})

	t.Run("ignores semicolons in sql line comments", func(t *testing.T) {
		t.Parallel()

		sql := "SELECT 1 -- ; in comment\n" +
			";SELECT 2;"
		statements, remainder := splitStatements(sql)
		require.Equal(t, []string{"SELECT 1 -- ; in comment", "SELECT 2"}, statements)
		require.Empty(t, remainder)
	})

	t.Run("ignores semicolons in sql block comments", func(t *testing.T) {
		t.Parallel()

		sql := "SELECT 1 /* ; in comment */;" +
			"SELECT 2;"
		statements, remainder := splitStatements(sql)
		require.Equal(
			t,
			[]string{"SELECT 1 /* ; in comment */", "SELECT 2"},
			statements,
		)
		require.Empty(t, remainder)
	})
}

func TestLooksLikeScriptDDL(t *testing.T) {
	t.Parallel()

	scripts := []string{
		"CREATE SCRIPT foo AS",
		"CREATE PYTHON3 SCALAR SCRIPT foo(x INT) RETURNS INT AS",
		"CREATE OR REPLACE JAVA ADAPTER SCRIPT foo AS",
		"CREATE FUNCTION bar (x INT) RETURN INT AS",
		"create or replace python3 scalar script foo as",
		"CREATE SOMEFUTURELANG SCALAR SCRIPT foo AS",
		"-- comment\nCREATE JAVA SCALAR SCRIPT foo AS",
		"CREATE /* lang */ JAVA SCALAR SCRIPT foo AS",
		"CREATE/* lang */JAVA SCALAR SCRIPT foo AS",
		"CREATE--x\nJAVA SCALAR SCRIPT foo AS",
	}
	for _, sql := range scripts {
		require.Truef(t, looksLikeScriptDDL(sql), "expected script DDL: %q", sql)
	}

	notScripts := []string{
		"SELECT 1",
		"CREATE TABLE t (a INT)",
		"CREATE TABLE script_log (a INT)",
		"CREATE VIEW v AS SELECT 1",
		"CREATE TABLE report AS SELECT id FROM script",
		"CREATE SCHEMA s",
		"ALTER TABLE t ADD COLUMN c INT",
		"DROP SCRIPT foo",
		"TRUNCATE TABLE t",
		"CREATE /* a script */ TABLE t (id INT)",
		"CREATE TABLE t -- function\nAS SELECT 1",
		"CREATE CONNECTION c TO 'a script b'",
		"CREATE USER u IDENTIFIED BY 'my function pw'",
		"CREATE SCHEMA s; CREATE PYTHON3 SCALAR SCRIPT foo AS",
		// A ';' inside a literal must not end the token scan early either.
		"CREATE CONNECTION c TO 'a;b'; CREATE PYTHON3 SCALAR SCRIPT foo AS",
	}
	for _, sql := range notScripts {
		require.Falsef(t, looksLikeScriptDDL(sql), "expected not script DDL: %q", sql)
	}
}

func TestSplitStatementsScriptDelimiter(t *testing.T) {
	t.Parallel()

	t.Run("java body with semicolons and slashes terminated by slash line", func(t *testing.T) {
		t.Parallel()

		sql := "CREATE JAVA SCALAR SCRIPT m() RETURNS INT AS\n" +
			"class M { int run() { return 6 / 2; } }\n/\n"
		statements, remainder := splitStatements(sql)
		require.Equal(t, []string{
			"CREATE JAVA SCALAR SCRIPT m() RETURNS INT AS\n" +
				"class M { int run() { return 6 / 2; } }",
		}, statements)
		require.Empty(t, strings.TrimSpace(remainder))
	})

	t.Run("script word inside a string literal splits on semicolon", func(t *testing.T) {
		t.Parallel()

		sql := "CREATE CONNECTION c TO 'a script b';\nSELECT 1;"
		statements, _ := splitStatements(sql)
		require.Equal(t, []string{"CREATE CONNECTION c TO 'a script b'", "SELECT 1"}, statements)
	})

	t.Run("comment glued to CREATE still detects the script", func(t *testing.T) {
		t.Parallel()

		sql := "CREATE/* c */JAVA SCALAR SCRIPT m() RETURNS INT AS\n" +
			"class M { int run() { return 1; } }\n/\n"
		statements, _ := splitStatements(sql)
		require.Len(t, statements, 1)
		require.Contains(t, statements[0], "return 1;")
	})

	t.Run("create table with script word in a comment splits on semicolon", func(t *testing.T) {
		t.Parallel()

		sql := "CREATE /* a script */ TABLE t (id INT);\nSELECT 1;"
		statements, _ := splitStatements(sql)
		require.Equal(t, []string{"CREATE /* a script */ TABLE t (id INT)", "SELECT 1"}, statements)
	})

	t.Run("mixed normal and script statements", func(t *testing.T) {
		t.Parallel()

		sql := "OPEN SCHEMA s;\n" +
			"CREATE PYTHON3 SCALAR SCRIPT add1(x INT) RETURNS INT AS\n" +
			"def run(c):\n return c.x + 1\n/\n" +
			"SELECT add1(41) FROM dual;\n"
		statements, _ := splitStatements(sql)
		require.Len(t, statements, 3)
	})

	t.Run("create statement before a script splits correctly", func(t *testing.T) {
		t.Parallel()

		// Given
		sql := "CREATE SCHEMA IF NOT EXISTS s;\n" +
			"OPEN SCHEMA s;\n" +
			"CREATE PYTHON3 SCALAR SCRIPT hello() RETURNS INT AS\n" +
			"def run(c):\n return 1\n/\n" +
			"SELECT hello();\n"

		// When
		statements, _ := splitStatements(sql)

		// Then
		require.Equal(t, []string{
			"CREATE SCHEMA IF NOT EXISTS s",
			"OPEN SCHEMA s",
			"CREATE PYTHON3 SCALAR SCRIPT hello() RETURNS INT AS\ndef run(c):\n return 1",
			"SELECT hello()",
		}, statements)
	})

	t.Run("a quoted semicolon before a script does not confuse the split", func(t *testing.T) {
		t.Parallel()

		// Given
		sql := "CREATE CONNECTION c TO 'host;port';\n" +
			"CREATE PYTHON3 SCALAR SCRIPT hello() RETURNS INT AS\n" +
			"def run(c):\n return 1\n/\n"

		// When
		statements, _ := splitStatements(sql)

		// Then
		require.Equal(t, []string{
			"CREATE CONNECTION c TO 'host;port'",
			"CREATE PYTHON3 SCALAR SCRIPT hello() RETURNS INT AS\ndef run(c):\n return 1",
		}, statements)
	})

	t.Run("create statement before a java script keeps the body intact", func(t *testing.T) {
		t.Parallel()

		// Given
		sql := "CREATE SCHEMA s;\n" +
			"CREATE JAVA SCALAR SCRIPT m(x DOUBLE) RETURNS DOUBLE AS\n" +
			"class M { double run() { return x * 2.0; } }\n/\n"

		// When
		statements, _ := splitStatements(sql)

		// Then
		require.Len(t, statements, 2)
		require.Equal(t, "CREATE SCHEMA s", statements[0])
		require.Contains(t, statements[1], "return x * 2.0;")
	})

	t.Run("script without slash is buffered, not split on body semicolons", func(t *testing.T) {
		t.Parallel()

		sql := "CREATE JAVA SCALAR SCRIPT m(x DOUBLE) RETURNS DOUBLE AS\n" +
			"class M { double run() { return x * 2.0; } }\n"
		statements, remainder := splitStatements(sql)
		require.Empty(t, statements)
		require.Equal(t, sql, remainder)
	})

	t.Run("buffered script without slash flushes whole at end of input", func(t *testing.T) {
		t.Parallel()

		sql := "CREATE JAVA SCALAR SCRIPT m(x DOUBLE) RETURNS DOUBLE AS\n" +
			"class M { double run() { return x * 2.0; } }\n"
		statements := nonInteractiveStatements(sql)
		require.Len(t, statements, 1)
		require.Contains(t, statements[0], "return x * 2.0;")
	})
}

func TestScriptBufferedAcrossLinesUntilSlash(t *testing.T) {
	t.Parallel()

	processor := &mockInputsProcessor{}
	sql := "CREATE JAVA SCALAR SCRIPT m(x DOUBLE) RETURNS DOUBLE AS\n" +
		"class M { double run() { return x * 2.0; } }\n" +
		"/\n" +
		"SELECT m(5) FROM dual;\n"
	err := runShellImpl(
		readline.NewBuffered(strings.NewReader(sql)),
		processor.processInput,
		ShellOpts{ExecuteOnSemicolon: true},
	)

	require.NoError(t, err)
	require.Len(t, processor.inputs, 2)
	require.Contains(t, processor.inputs[0], "return x * 2.0;")
	require.Equal(t, "SELECT m(5) FROM dual", processor.inputs[1])
}

// An unterminated script is intentionally flushed whole at EOF (client flushes, DB validates).
func TestScriptWithoutSlashFlushedWholeAtEOF(t *testing.T) {
	t.Parallel()

	processor := &mockInputsProcessor{}
	sql := "CREATE JAVA SCALAR SCRIPT m(x DOUBLE) RETURNS DOUBLE AS\n" +
		"class M { double run() { return x * 2.0; } }\n"
	err := runShellImpl(
		readline.NewBuffered(strings.NewReader(sql)),
		processor.processInput,
		ShellOpts{ExecuteOnSemicolon: true},
	)

	require.NoError(t, err)
	require.Len(t, processor.inputs, 1)
	require.Equal(
		t,
		"CREATE JAVA SCALAR SCRIPT m(x DOUBLE) RETURNS DOUBLE AS\n"+
			"class M { double run() { return x * 2.0; } }",
		processor.inputs[0],
	)
}

func TestHistoryFilePath_ResolvesUnderTheLauncherHistoryRoot(t *testing.T) {
	t.Parallel()

	// Given
	want, err := launcherpaths.HistoryRootPath()
	require.NoError(t, err)

	// When
	got, err := HistoryFilePath()

	// Then
	require.NoError(t, err)
	require.Equal(t, filepath.Join(want, historyFileName), got)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestEnsureHistoryFileDir_CreatesTheDirectoryOnAFreshInstall(t *testing.T) {
	// Given a home with no history directory, as after a fresh install
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	// When
	path, err := ensureHistoryFileDir()

	// Then the file can be created where readline will look for it
	require.NoError(t, err)
	require.DirExists(t, filepath.Dir(path))
}
