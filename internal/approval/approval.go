// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package approval describes how a command may obtain approval for an action
// that requires it, so each command decides its own response instead of
// receiving a single pre-collapsed yes or no.
package approval

// Mode reports the approval channel available for the current invocation.
type Mode string

const (
	// ModeApprove means approval was granted up front and nothing is asked.
	ModeApprove Mode = "approve"
	// ModePrompt means the user is reachable and can be asked.
	ModePrompt Mode = "prompt"
	// ModeNonInteractive means approval was not granted and cannot be requested.
	// Callers decide what that means for their action; treating it as consent
	// lets a scripted run mutate state its author never approved.
	ModeNonInteractive Mode = "non-interactive"
)

// Resolve maps the state of the --auto-approve flag and the availability of an
// interactive terminal onto a Mode. Terminal detection stays with the caller so
// this stays a pure decision.
func Resolve(autoApprove, interactive bool) Mode {
	if autoApprove {
		return ModeApprove
	}
	if interactive {
		return ModePrompt
	}

	return ModeNonInteractive
}
