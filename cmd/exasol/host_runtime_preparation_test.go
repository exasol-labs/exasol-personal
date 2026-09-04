// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/spf13/cobra"
)

func newApproverTestCommand(stdin string) (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "test"}
	errOut := &bytes.Buffer{}
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetErr(errOut)

	return cmd, errOut
}

var testHostChangeRequest = localruntime.HostChangeRequest{
	Kind: localruntime.HostChangeInstallContainerRuntime,
	Commands: []localruntime.HostCommand{
		{Name: "winget", Args: []string{"install", "--exact", "--id", "RedHat.Podman"}},
	},
}

func TestHostChangeApprover_AutoApproveSkipsPrompt(t *testing.T) {
	t.Parallel()

	cmd, errOut := newApproverTestCommand("")
	approver := hostChangeApprover(cmd, approval.ModeApprove)

	approved, err := approver(context.Background(), testHostChangeRequest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approved {
		t.Error("expected --auto-approve to approve the change")
	}
	if errOut.Len() != 0 {
		t.Errorf("expected no prompt output with --auto-approve, got %q", errOut.String())
	}
}

// The behaviour this replaces silently defaulted to "yes" whenever stdin was
// not a terminal, so a scripted run installed Podman with no approval. A
// non-interactive run must now refuse and say how to proceed.
func TestHostChangeApprover_NonInteractiveRefusesWithoutAutoApprove(t *testing.T) {
	t.Parallel()

	cmd, _ := newApproverTestCommand("")
	approver := hostChangeApprover(cmd, approval.ModeNonInteractive)

	approved, err := approver(context.Background(), testHostChangeRequest)
	if err == nil {
		t.Fatal("expected a non-interactive run without --auto-approve to fail")
	}
	if approved {
		t.Error("expected the change to be denied")
	}
	if !strings.Contains(err.Error(), "--auto-approve") {
		t.Errorf("expected the error to name --auto-approve, got %v", err)
	}
}

func TestHostChangeApprover_InteractiveShowsCommandsAndAcceptsYes(t *testing.T) {
	t.Parallel()

	cmd, errOut := newApproverTestCommand("y\n")
	approver := hostChangeApprover(cmd, approval.ModePrompt)

	approved, err := approver(context.Background(), testHostChangeRequest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approved {
		t.Error("expected 'y' to approve the change")
	}
	// The user must see the exact command before deciding.
	if !strings.Contains(errOut.String(), "winget install --exact --id RedHat.Podman") {
		t.Errorf("expected the command to be displayed, got %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "Podman is required") {
		t.Errorf("expected an explanation of the change, got %q", errOut.String())
	}
}

func TestHostChangeApprover_InteractiveDefaultsToNo(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"\n", "n\n", "no\n", ""} {
		cmd, _ := newApproverTestCommand(input)
		approver := hostChangeApprover(cmd, approval.ModePrompt)

		approved, err := approver(context.Background(), testHostChangeRequest)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", input, err)
		}
		if approved {
			t.Errorf("input %q: expected the change to be declined by default", input)
		}
	}
}

func TestHostRuntimePreparationOptions_AlwaysSuppliesApproverAndProgress(t *testing.T) {
	t.Parallel()

	cmd, errOut := newApproverTestCommand("")
	options := hostRuntimePreparationOptions(cmd, approval.ModeApprove)

	if options.ApproveHostChange == nil {
		t.Error("expected an approver to always be supplied")
	}
	// Progress must not be gated on --verbose: it reports multi-minute steps.
	if options.Progress == nil {
		t.Fatal("expected a progress writer")
	}
	if _, err := options.Progress.Write([]byte("preparing")); err != nil {
		t.Fatalf("progress write failed: %v", err)
	}
	if errOut.String() != "preparing" {
		t.Errorf("expected progress on stderr, got %q", errOut.String())
	}
}
