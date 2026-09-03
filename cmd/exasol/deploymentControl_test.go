// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

func TestRenderLifecycleCompletionJSON(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name            string
		output          lifecycleCompletionOutput
		deploymentState string
		databaseReady   bool
	}{
		{
			name: "start completion",
			output: lifecycleCompletionOutput{
				DeploymentState: deploy.StatusRunning,
				DatabaseReady:   true,
			},
			deploymentState: deploy.StatusRunning,
			databaseReady:   true,
		},
		{
			name: "stop completion",
			output: lifecycleCompletionOutput{
				DeploymentState: deploy.StatusStopped,
				DatabaseReady:   false,
			},
			deploymentState: deploy.StatusStopped,
			databaseReady:   false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			var buf bytes.Buffer

			// When
			err := renderLifecycleCompletionJSON(&buf, test.output)
			// Then
			if err != nil {
				t.Fatalf("expected lifecycle completion JSON to render: %v", err)
			}
			var decoded lifecycleCompletionOutput
			if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
				t.Fatalf("expected valid JSON, got %q: %v", buf.String(), err)
			}
			if decoded.DeploymentState != test.deploymentState {
				t.Fatalf(
					"expected deployment state %q, got %q",
					test.deploymentState,
					decoded.DeploymentState,
				)
			}
			if decoded.DatabaseReady != test.databaseReady {
				t.Fatalf(
					"expected databaseReady %t, got %t",
					test.databaseReady,
					decoded.DatabaseReady,
				)
			}
		})
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestAddLocalPortRecoveryCallToActionQueuesStructuredRecovery(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	addLocalPortRecoveryCallToAction(&deploy.LocalPortRecoveryError{
		Service: "db",
		Port:    28563,
		Cause:   errors.New("runtime command failed"),
	})
	var stderr bytes.Buffer
	writeTerminalCallsToAction(&stderr, true, false)

	for _, expected := range []string{
		`local service "db"`,
		"exasol config set --ports db:<available-port>",
		"exasol config set --ports auto",
	} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("expected recovery guidance %q, got %q", expected, stderr.String())
		}
	}
}

func TestLifecycleCommandsRegisterJSONFlag(t *testing.T) {
	t.Parallel()

	for name, cmd := range map[string]*cobra.Command{
		"start": startCmd,
		"stop":  stopCmd,
	} {
		if cmd.Flags().Lookup("json") == nil {
			t.Fatalf("expected %s to register json output flag", name)
		}
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestAddLifecycleCompletionTerminalOutputQueuesJSON(t *testing.T) {
	// Given
	resetTerminalMessages()
	defer resetTerminalMessages()

	// When
	err := addLifecycleCompletionTerminalOutput(lifecycleCompletionOutput{
		DeploymentState: deploy.StatusRunning,
		DatabaseReady:   true,
	})
	// Then
	if err != nil {
		t.Fatalf("expected lifecycle completion output to queue: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	var decoded lifecycleCompletionOutput
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("expected queued stdout to contain valid JSON, got %q: %v", stdout.String(), err)
	}
	if decoded.DeploymentState != deploy.StatusRunning || !decoded.DatabaseReady {
		t.Fatalf("unexpected lifecycle output: %+v", decoded)
	}
}
