// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
)

//nolint:paralleltest // mutates shared terminal message queues
func TestInstallDeploymentFailureShowsLocalPortRecovery(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	cause := &deploy.LocalPortRecoveryError{
		Service: "db",
		Port:    28563,
		Cause:   errors.New("runtime command failed"),
	}
	err := installDeploymentFailure(cause)
	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped deployment failure to preserve its cause, got %v", err)
	}

	var stderr bytes.Buffer
	writeTerminalCallsToAction(&stderr, true, false)
	for _, expected := range []string{
		"exasol config set --ports db:<available-port>",
		"exasol config set --ports auto",
	} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("expected install recovery guidance %q, got %q", expected, stderr.String())
		}
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestInstallDeploymentFailureDoesNotShowPortRecoveryForUnrelatedError(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	err := installDeploymentFailure(errors.New("unrelated deployment failure"))
	if err == nil {
		t.Fatal("expected deployment failure")
	}

	var stderr bytes.Buffer
	writeTerminalCallsToAction(&stderr, true, false)
	if stderr.Len() != 0 {
		t.Fatalf("expected no port recovery guidance, got %q", stderr.String())
	}
}
