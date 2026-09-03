// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
)

var (
	terminalNotices       []string
	terminalOutputs       []string
	terminalCallsToAction []string
)

func resetTerminalMessages() {
	terminalNotices = nil
	terminalOutputs = nil
	terminalCallsToAction = nil
}

func addTerminalNotice(message string) {
	if message == "" {
		return
	}

	terminalNotices = append(terminalNotices, message)
}

func addTerminalOutput(message string) {
	if message == "" {
		return
	}

	terminalOutputs = append(terminalOutputs, message)
}

// addTerminalCallToAction queues next-step guidance. Calls to action are
// suppressed under --json (see callsToActionVisible).
func addTerminalCallToAction(message string) {
	if message == "" {
		return
	}

	terminalCallsToAction = append(terminalCallsToAction, message)
}

func printTerminalMessages() {
	writeTerminalMessages(terminalConfig{
		stdout:            os.Stdout,
		stderr:            os.Stderr,
		showCallsToAction: callsToActionVisible(commonFlags.OutputJson),
	})
}

func printTerminalCallsToActionAfterError() {
	writeTerminalCallsToAction(
		os.Stderr,
		callsToActionVisible(commonFlags.OutputJson),
		true,
	)
}

// Calls to action help any reader, including a non-interactive agent driving
// the CLI, so they are suppressed only for JSON output.
func callsToActionVisible(jsonOutput bool) bool {
	return !jsonOutput
}

type terminalConfig struct {
	stdout            io.Writer
	stderr            io.Writer
	showCallsToAction bool
}

func writeTerminalMessages(cfg terminalConfig) {
	hasPrecedingMessages := len(terminalNotices) > 0 || len(terminalOutputs) > 0
	for _, message := range terminalNotices {
		_, _ = fmt.Fprintln(cfg.stderr, message)
	}
	terminalNotices = nil
	for _, message := range terminalOutputs {
		_, _ = fmt.Fprintln(cfg.stdout, message)
	}
	terminalOutputs = nil
	writeTerminalCallsToAction(cfg.stderr, cfg.showCallsToAction, hasPrecedingMessages)
}

func writeTerminalCallsToAction(writer io.Writer, visible, separate bool) {
	if visible {
		if len(terminalCallsToAction) > 0 && separate {
			_, _ = fmt.Fprintln(writer)
		}
		for _, message := range terminalCallsToAction {
			_, _ = fmt.Fprintln(writer, message)
		}
	}
	terminalCallsToAction = nil
}
