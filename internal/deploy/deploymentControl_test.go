// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localports"
)

func TestRestoreStateAfterUnavailableLocalPortPreservesCauseAndAddsRecovery(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		want any
	}{
		{name: "initialized", want: &config.WorkflowStateInitialized{}},
		{name: "stopped", want: &config.WorkflowStateStopped{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			deployment := config.NewDeploymentDir(t.TempDir())
			state := &config.ExasolPersonalState{}
			if err := state.SetWorkflowStateAndWrite(
				&config.WorkflowStateOperationInProgress{Operation: config.StartOperation},
				deployment,
			); err != nil {
				t.Fatalf("write initial state failed: %v", err)
			}
			commandErr := errors.New("runtime command failed")
			portErr := &localports.UnavailableError{
				Service: "db", Port: 28563, Cause: commandErr,
			}

			err := restoreStateAfterUnavailableLocalPort(
				state, deployment, test.want, portErr,
			)

			if !errors.Is(err, commandErr) {
				t.Fatalf("expected original command error in chain, got %v", err)
			}
			for _, guidance := range []string{
				"service \"db\"", "28563",
				"exasol config set --ports db:<available-port>",
				"exasol config set --ports auto",
			} {
				if !strings.Contains(err.Error(), guidance) {
					t.Fatalf("expected guidance %q, got %v", guidance, err)
				}
			}
			persisted, readErr := config.ReadExasolPersonalState(deployment)
			if readErr != nil {
				t.Fatalf("read restored state failed: %v", readErr)
			}
			actual, readErr := persisted.GetWorkflowState()
			if readErr != nil {
				t.Fatalf("decode restored state failed: %v", readErr)
			}
			if fmt.Sprintf("%T", actual) != fmt.Sprintf("%T", test.want) {
				t.Fatalf("restored state is %T, want %T", actual, test.want)
			}
		})
	}
}

func TestWorkflowStatePermitsStart_RequiresStartForStoppedDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that is stopped.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateStopped{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(),
		exasolState,
		config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller should start the backend.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !decision.shouldRun {
		t.Fatal("expected stopped deployment to require backend start")
	}
}

//nolint:paralleltest // Mutates package-level getStatusForStart.
func TestWorkflowStatePermitsStart_SkipsStartForReadyDeployment(t *testing.T) {
	// Given: a running deployment whose database is already ready.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateRunning{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	originalGetStatusForStart := getStatusForStart
	getStatusForStart = func(
		_ context.Context,
		_ config.DeploymentDir,
		checkConnection bool,
	) (*StatusOutput, error) {
		if !checkConnection {
			t.Fatal("expected start check to verify database readiness")
		}

		return &StatusOutput{Status: StatusDatabaseReady}, nil
	}
	t.Cleanup(func() {
		getStatusForStart = originalGetStatusForStart
	})

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(),
		exasolState,
		config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller can return successfully without starting the backend again.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected ready deployment to skip backend start")
	}
	if !decision.showConnectionInstructions {
		t.Fatal("expected ready deployment to show connection instructions")
	}
	if !strings.Contains(decision.guidance, "already ready") {
		t.Fatalf("expected already-ready guidance, got %q", decision.guidance)
	}
}

//nolint:paralleltest // Mutates package-level getStatusForStart.
func TestWorkflowStatePermitsStart_GuidesForRunningDeploymentThatIsNotReady(t *testing.T) {
	// Given: a running deployment whose database is not ready.
	deployment := config.NewDeploymentDir(t.TempDir())
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowStateAndWrite(
		&config.WorkflowStateRunning{},
		deployment,
	); err != nil {
		t.Fatalf("write workflow state failed: %v", err)
	}

	originalGetStatusForStart := getStatusForStart
	getStatusForStart = func(
		_ context.Context,
		_ config.DeploymentDir,
		_ bool,
	) (*StatusOutput, error) {
		return &StatusOutput{Status: StatusDatabaseConnectionFailed}, nil
	}
	t.Cleanup(func() {
		getStatusForStart = originalGetStatusForStart
	})

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(),
		exasolState,
		deployment,
	)
	// Then: the command gives next-step guidance without retrying the backend.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected non-ready running deployment to skip backend start")
	}
	for _, expected := range []string{"not ready", "exasol status", "exasol stop", "exasol start"} {
		if !strings.Contains(decision.guidance, expected) {
			t.Fatalf("expected guidance to contain %q, got %q", expected, decision.guidance)
		}
	}
}

func TestWorkflowStatePermitsStart_GuidesForInitializedDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that has not been deployed yet.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateInitialized{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(),
		exasolState,
		config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller gets guidance instead of an unexpected-status error.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected initialized deployment to skip backend start")
	}
	for _, expected := range []string{"initialized", "exasol deploy"} {
		if !strings.Contains(decision.guidance, expected) {
			t.Fatalf("expected guidance to contain %q, got %q", expected, decision.guidance)
		}
	}
}

func TestWorkflowStatePermitsStop_RequiresStopForRunningDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that is running.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateRunning{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller should stop the backend.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !decision.shouldRun {
		t.Fatal("expected running deployment to require backend stop")
	}
}

func TestWorkflowStatePermitsStop_GuidesForAlreadyStoppedDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that is already stopped.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateStopped{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller gets an idempotent no-op decision.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected stopped deployment to skip backend stop")
	}
	if !strings.Contains(decision.guidance, "already stopped") {
		t.Fatalf("expected already-stopped guidance, got %q", decision.guidance)
	}
}

func TestWorkflowStatePermitsStop_GuidesForInitializedDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that has not been deployed yet.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateInitialized{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller gets guidance instead of an unexpected-status error.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected initialized deployment to skip backend stop")
	}
	for _, expected := range []string{"nothing to stop", "exasol deploy"} {
		if !strings.Contains(decision.guidance, expected) {
			t.Fatalf("expected guidance to contain %q, got %q", expected, decision.guidance)
		}
	}
}
