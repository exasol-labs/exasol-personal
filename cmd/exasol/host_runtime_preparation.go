// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/spf13/cobra"
)

// hostRuntimePreparationOptions builds the policy the local runtime applies
// when it needs to change the host. Deciding here rather than inside the
// runtime keeps command presentation (prompt wording, --auto-approve, TTY
// detection) in the command layer.
func hostRuntimePreparationOptions(
	cmd *cobra.Command,
	mode approval.Mode,
) localruntime.PrepareOptions {
	return localruntime.PrepareOptions{
		ApproveHostChange: hostChangeApprover(cmd, mode),
		// Preparation progress goes to stderr unconditionally: it reports
		// multi-minute steps the user needs to see, and is not the
		// --verbose-gated subprocess output.
		Progress: cmd.ErrOrStderr(),
	}
}

// hostChangeApprover refuses rather than assumes when it cannot ask. A
// non-interactive run without --auto-approve fails with instructions instead
// of silently mutating the host.
func hostChangeApprover(
	cmd *cobra.Command,
	mode approval.Mode,
) localruntime.HostChangeApprover {
	return func(_ context.Context, request localruntime.HostChangeRequest) (bool, error) {
		switch mode {
		case approval.ModeApprove:
			return true, nil
		case approval.ModeNonInteractive:
			return false, errors.New(
				"local runtime host preparation requires approval; " +
					"re-run with --auto-approve or run the displayed setup manually",
			)
		case approval.ModePrompt:
		default:
			return false, fmt.Errorf("unrecognised approval mode %q", mode)
		}

		writer := cmd.ErrOrStderr()
		_, _ = fmt.Fprintln(writer, hostChangeExplanation(request.Kind))
		_, _ = fmt.Fprintln(writer, "The launcher will run:")
		for _, command := range request.Commands {
			_, _ = fmt.Fprintf(writer, "  %s\n", command)
		}
		_, _ = fmt.Fprint(writer, "Continue? [y/N]: ")

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("failed to read host preparation approval: %w", err)
		}

		return confirmYes(line), nil
	}
}

func hostChangeExplanation(kind localruntime.HostChangeKind) string {
	switch kind {
	case localruntime.HostChangeInstallContainerRuntime:
		return "Podman is required for local deployments but is not available. " +
			"Installing it may request administrator (UAC) approval and take a few minutes."
	default:
		return "The local runtime requires a host environment change."
	}
}
