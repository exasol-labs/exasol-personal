// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/tofu"
)

func TestEnsureDeploymentPresetIdentityMatches_RejectsDifferentPreset(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		testManagerContext(t),
		deployment,
		InitOptions{
			InfrastructurePreset: PresetRef{Name: presets.DefaultInfrastructure},
			InstallationPreset:   PresetRef{Name: presets.DefaultInstallation},
			InfraVars:            map[string]string{},
			InstallVars:          map[string]string{},
			CurrentVersion:       "0.0.0",
		},
	); err != nil {
		t.Fatalf("initial init failed: %v", err)
	}

	// When
	err := EnsureDeploymentPresetIdentityMatches(
		testManagerContext(t),
		deployment,
		PresetRef{Name: "stackit"},
		PresetRef{Name: presets.DefaultInstallation},
	)

	// Then
	if !errors.Is(err, ErrDeploymentPresetMismatch) {
		t.Fatalf("expected preset mismatch, got %v", err)
	}
}

func TestSetDeploymentConfiguration_UpdatesVariablesAndPreservesStateFiles(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testManagerContext(t)
	mgr := runtimeartifacts.FromContext(ctx)

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		ctx,
		deployment,
		InitOptions{
			InfrastructurePreset: PresetRef{Name: presets.DefaultInfrastructure},
			InstallationPreset:   PresetRef{Name: presets.DefaultInstallation},
			InfraVars:            map[string]string{"cluster_size": "2"},
			InstallVars:          map[string]string{},
			CurrentVersion:       "0.0.0",
		},
	); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	statePath := filepath.Join(deployment.InfrastructureDir(), tofu.DefaultStateFile)
	if err := os.WriteFile(statePath, []byte("state"), 0o600); err != nil {
		t.Fatalf("write state file failed: %v", err)
	}
	tofuBinaryPath, err := mgr.Request(ctx, "tofu")
	if err != nil {
		t.Fatalf("resolve tofu binary path failed: %v", err)
	}
	if err := os.Remove(tofuBinaryPath); err != nil {
		t.Fatalf("remove tofu binary before config set failed: %v", err)
	}

	// When
	if _, err := SetDeploymentConfiguration(
		ctx,
		map[string]string{"cluster_size": "3"},
		map[string]string{},
		deployment,
	); err != nil {
		t.Fatalf("config set failed: %v", err)
	}

	// Then
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected state file to be preserved, got %v", err)
	}
	if _, err := os.Stat(tofuBinaryPath); !os.IsNotExist(err) {
		t.Fatalf("expected config set not to recreate tofu binary, got %v", err)
	}
	tfvarsPath := filepath.Join(deployment.InfrastructureDir(), tofu.DefaultVarsOutput)
	content, err := os.ReadFile(tfvarsPath)
	if err != nil {
		t.Fatalf("read tfvars failed: %v", err)
	}
	if !strings.Contains(string(content), `cluster_size`) ||
		!strings.Contains(string(content), `= 3`) {
		t.Fatalf("expected updated cluster size, got: %s", string(content))
	}
}

func TestSetDeploymentConfiguration_PreservesDeploymentCreatedAt(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		testManagerContext(t),
		deployment,
		InitOptions{
			InfrastructurePreset: PresetRef{Name: presets.DefaultInfrastructure},
			InstallationPreset:   PresetRef{Name: presets.DefaultInstallation},
			InfraVars:            map[string]string{"cluster_size": "2"},
			InstallVars:          map[string]string{},
			CurrentVersion:       "0.0.0",
		},
	); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	tfvarsPath := filepath.Join(deployment.InfrastructureDir(), tofu.DefaultVarsOutput)
	createdAt := "2001-02-03T04:05:06Z"
	parsedCreatedAt, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		t.Fatalf("parse createdAt failed: %v", err)
	}
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("read state failed: %v", err)
	}
	state.CreatedAt = parsedCreatedAt
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("write state failed: %v", err)
	}

	// When
	if _, err := SetDeploymentConfiguration(
		testManagerContext(t),
		map[string]string{"cluster_size": "3"},
		map[string]string{},
		deployment,
	); err != nil {
		t.Fatalf("config set failed: %v", err)
	}

	// Then
	content, err := os.ReadFile(tfvarsPath)
	if err != nil {
		t.Fatalf("read tfvars failed: %v", err)
	}
	if !deploymentCreatedAtPattern(createdAt).Match(content) {
		t.Fatalf("expected deployment_created_at to be preserved, got:\n%s", string(content))
	}
}

func TestWorkflowStatePermitsConfigure_RejectsCloudNonInitializedStates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		state any
	}{
		{name: "running", state: &config.WorkflowStateRunning{}},
		{name: "stopped", state: &config.WorkflowStateStopped{}},
		{name: "deployment_failed", state: &config.WorkflowStateDeploymentFailed{Error: "boom"}},
		{
			name: "interrupted_during_deploy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DeployOperation,
			},
		},
		{
			name: "interrupted_during_destroy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DestroyOperation,
			},
		},
		{
			name: "operation_in_progress",
			state: &config.WorkflowStateOperationInProgress{
				Operation: config.DeployOperation,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			exasolState := &config.ExasolPersonalState{}
			if err := exasolState.SetWorkflowState(test.state); err != nil {
				t.Fatalf("set workflow state failed: %v", err)
			}

			err := WorkflowStatePermitsConfigure(exasolState, backendTypeTofu)

			if !errors.Is(err, ErrConfigureNotAllowed) {
				t.Fatalf("expected ErrConfigureNotAllowed, got %v", err)
			}
			if !strings.Contains(err.Error(), "run `exasol destroy`") {
				t.Fatalf("expected destroy guidance, got %v", err)
			}
			if !strings.Contains(err.Error(), "exasol remove") {
				t.Fatalf("expected remove guidance, got %v", err)
			}
		})
	}
}

func TestWorkflowStatePermitsConfigure_AllowsStoppedLocalDeployment(t *testing.T) {
	t.Parallel()

	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateStopped{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	if err := WorkflowStatePermitsConfigure(exasolState, backendTypeLocal); err != nil {
		t.Fatalf("expected stopped local configuration to be allowed, got %v", err)
	}
}

func TestWorkflowStatePermitsConfigure_GuidesActiveLocalDeploymentToStop(t *testing.T) {
	t.Parallel()

	for _, workflowState := range []any{
		&config.WorkflowStateRunning{},
		&config.WorkflowStateOperationInProgress{Operation: config.DeployOperation},
	} {
		exasolState := &config.ExasolPersonalState{}
		if err := exasolState.SetWorkflowState(workflowState); err != nil {
			t.Fatalf("set workflow state failed: %v", err)
		}

		err := WorkflowStatePermitsConfigure(exasolState, backendTypeLocal)
		if !errors.Is(err, ErrConfigureNotAllowed) ||
			!strings.Contains(err.Error(), "exasol stop") {
			t.Fatalf("expected local stop guidance, got %v", err)
		}
	}
}

func TestStoppedLocalDeploymentCanBeSetAndReset(t *testing.T) {
	t.Parallel()

	ctx := testManagerContext(t)
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		ctx,
		deployment,
		InitOptions{
			InfrastructurePreset: PresetRef{Name: "local"},
			InstallationPreset:   PresetRef{Name: "local"},
			InfraVars:            map[string]string{localPortsConfigName: "db:18563"},
			InstallVars:          map[string]string{},
			CurrentVersion:       "0.0.0",
		},
	); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("read state failed: %v", err)
	}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateStopped{}, deployment,
	); err != nil {
		t.Fatalf("set stopped state failed: %v", err)
	}

	updated, err := SetDeploymentConfiguration(
		ctx,
		map[string]string{localPortsConfigName: "db:28563"},
		map[string]string{},
		deployment,
	)
	if err != nil {
		t.Fatalf("config set failed: %v", err)
	}
	updatedPorts := deploymentConfigValueByName(
		t, updated.Infrastructure.Options, localPortsConfigName,
	)
	if updated.ApplyCommand != "start" ||
		updatedPorts.RawValue != "db:28563" {
		t.Fatalf("unexpected stopped-local update: %#v", updated)
	}

	reset, err := ResetDeploymentConfiguration(
		ctx,
		deployment,
		[]string{localPortsConfigName},
		false,
	)
	if err != nil {
		t.Fatalf("config reset failed: %v", err)
	}
	resetPorts := deploymentConfigValueByName(
		t, reset.Infrastructure.Options, localPortsConfigName,
	)
	if reset.ApplyCommand != "start" ||
		resetPorts.RawValue == "" || resetPorts.RawValue == "auto" {
		t.Fatalf("expected concrete reset port and start guidance, got %#v", reset)
	}
	portMappings, _, err := parseLocalPortMappings(resetPorts.RawValue)
	if err != nil || portMappings[localDatabaseService] <= 0 {
		t.Fatalf("expected a positive concrete reset port, got %q: %v", resetPorts.RawValue, err)
	}
	state, err = config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("read state after reset failed: %v", err)
	}
	if _, ok := mustWorkflowState(t, state).(*config.WorkflowStateStopped); !ok {
		t.Fatalf("expected stopped state to be preserved, got %T", mustWorkflowState(t, state))
	}
}

func deploymentConfigValueByName(
	t *testing.T,
	values []DeploymentConfigValue,
	name string,
) DeploymentConfigValue {
	t.Helper()
	for _, value := range values {
		if value.Name == name {
			return value
		}
	}
	t.Fatalf("configuration option %q not found in %#v", name, values)

	return DeploymentConfigValue{}
}

func mustWorkflowState(t *testing.T, state *config.ExasolPersonalState) any {
	t.Helper()
	workflowState, err := state.GetWorkflowState()
	if err != nil {
		t.Fatalf("read workflow state failed: %v", err)
	}

	return workflowState
}

// deploymentInState returns an initialized deployment whose persisted workflow state has
// been set to workflowState, plus the state handle for guards that take one.
func deploymentInState(
	t *testing.T,
	workflowState any,
) (config.DeploymentDir, *config.ExasolPersonalState) {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	writeMinimalInitializedDeployment(t, deployment)
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("read state failed: %v", err)
	}
	if err := state.SetWorkflowStateAndWrite(workflowState, deployment); err != nil {
		t.Fatalf("write workflow state failed: %v", err)
	}

	return deployment, state
}

// assertBlockedStateError asserts that a blocked lifecycle guard returned an actionable
// error: it unwraps to the sentinel, names the current state and deployment directory,
// includes the expected recovery guidance, and never labels the state "unexpected".
func assertBlockedStateError(
	t *testing.T,
	deployment config.DeploymentDir,
	err error,
	sentinel error,
	wantStatus string,
	wantGuidance ...string,
) {
	t.Helper()

	if err == nil {
		t.Fatal("expected command to be blocked, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel %v, got %v", sentinel, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, wantStatus) {
		t.Fatalf("expected message to name state %q, got %q", wantStatus, msg)
	}
	if !strings.Contains(msg, deployment.Root()) {
		t.Fatalf("expected message to name dir %q, got %q", deployment.Root(), msg)
	}
	if strings.Contains(msg, "unexpected") {
		t.Fatalf("recoverable state must not be labelled \"unexpected\", got %q", msg)
	}
	for _, guidance := range wantGuidance {
		if !strings.Contains(msg, guidance) {
			t.Fatalf("expected recovery guidance %q, got %q", guidance, msg)
		}
	}
}

func assertContainsAll(t *testing.T, message string, wantSubstrings ...string) {
	t.Helper()

	for _, substring := range wantSubstrings {
		if !strings.Contains(message, substring) {
			t.Fatalf("expected message to contain %q, got %q", substring, message)
		}
	}
}

func TestWorkflowStatePermitsDeploy_BlockedStatesSurfaceRecoveryGuidance(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		state        any
		sentinel     error
		wantStatus   string
		wantGuidance []string
	}{
		{
			name: "interrupted_during_destroy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DestroyOperation,
			},
			sentinel:     ErrUnexpectedDeploymentStatus,
			wantStatus:   StatusInterrupted,
			wantGuidance: []string{"Interrupted during", "destroy"},
		},
		{
			name:         "stopped",
			state:        &config.WorkflowStateStopped{},
			sentinel:     ErrUnexpectedDeploymentStatus,
			wantStatus:   StatusStopped,
			wantGuidance: []string{"start", "destroy"},
		},
		{
			name: "operation_in_progress_non_deploy",
			state: &config.WorkflowStateOperationInProgress{
				Operation: config.DestroyOperation,
			},
			sentinel:     ErrUnspportedOperation,
			wantStatus:   StatusOperationInProgress,
			wantGuidance: []string{"destroy"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			deployment, state := deploymentInState(t, test.state)
			err := WorkflowStatePermitsDeploy(state, deployment)
			assertBlockedStateError(
				t, deployment, err, test.sentinel, test.wantStatus, test.wantGuidance...,
			)
		})
	}
}

func TestWorkflowStatePermitsDeploy_PermittedStates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		state any
	}{
		{name: "initialized", state: &config.WorkflowStateInitialized{}},
		{name: "deployment_failed", state: &config.WorkflowStateDeploymentFailed{Error: "boom"}},
		{name: "running", state: &config.WorkflowStateRunning{}},
		{
			name:  "operation_in_progress_deploy",
			state: &config.WorkflowStateOperationInProgress{Operation: config.DeployOperation},
		},
		{
			name: "interrupted_during_deploy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DeployOperation,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			deployment, state := deploymentInState(t, test.state)
			if err := WorkflowStatePermitsDeploy(state, deployment); err != nil {
				t.Fatalf("expected deploy to be permitted, got %v", err)
			}
		})
	}
}

func TestWorkflowStatePermitsConnect_BlockedStatesSurfaceRecoveryGuidance(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		state        any
		wantStatus   string
		wantGuidance []string
	}{
		{
			name:         "initialized",
			state:        &config.WorkflowStateInitialized{},
			wantStatus:   StatusInitialized,
			wantGuidance: []string{"deploy"},
		},
		{
			name:         "stopped",
			state:        &config.WorkflowStateStopped{},
			wantStatus:   StatusStopped,
			wantGuidance: []string{"start"},
		},
		{
			name: "interrupted_during_destroy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DestroyOperation,
			},
			wantStatus:   StatusInterrupted,
			wantGuidance: []string{"destroy"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			deployment, _ := deploymentInState(t, test.state)
			err := WorkflowStatePermitsConnect(deployment)
			assertBlockedStateError(
				t, deployment, err, ErrUnexpectedDeploymentStatus, test.wantStatus,
				test.wantGuidance...,
			)
		})
	}
}

func TestWorkflowStatePermitsConnect_PermitsRunning(t *testing.T) {
	t.Parallel()

	deployment, _ := deploymentInState(t, &config.WorkflowStateRunning{})
	if err := WorkflowStatePermitsConnect(deployment); err != nil {
		t.Fatalf("expected connect to be permitted while running, got %v", err)
	}
}

func TestWorkflowStatePermitsStart_SkippedStatesSurfaceRecoveryGuidance(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		state        any
		wantGuidance []string
	}{
		{
			name:         "initialized",
			state:        &config.WorkflowStateInitialized{},
			wantGuidance: []string{"deploy"},
		},
		{
			name:         "deployment_failed",
			state:        &config.WorkflowStateDeploymentFailed{Error: "boom"},
			wantGuidance: []string{"deploy"},
		},
		{
			name: "interrupted_during_deploy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DeployOperation,
			},
			wantGuidance: []string{"deploy"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			deployment, state := deploymentInState(t, test.state)
			decision, err := workflowStatePermitsStart(testManagerContext(t), state, deployment)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if decision.shouldRun {
				t.Fatal("expected start to be skipped")
			}
			assertContainsAll(
				t, decision.guidance, test.wantGuidance...,
			)
		})
	}
}

func TestWorkflowStatePermitsStop_SkippedStatesSurfaceRecoveryGuidance(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		state        any
		wantGuidance []string
	}{
		{
			name:         "initialized",
			state:        &config.WorkflowStateInitialized{},
			wantGuidance: []string{"deploy"},
		},
		{
			name:         "stopped",
			state:        &config.WorkflowStateStopped{},
			wantGuidance: []string{"already stopped"},
		},
		{
			name: "interrupted_during_deploy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DeployOperation,
			},
			wantGuidance: []string{"deploy"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, state := deploymentInState(t, test.state)
			decision, err := workflowStatePermitsStop(state)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if decision.shouldRun {
				t.Fatal("expected stop to be skipped")
			}
			assertContainsAll(
				t, decision.guidance, test.wantGuidance...,
			)
		})
	}
}

func TestDestroyThenRemoveLocalDeploymentDirectoryRemovesLocalFiles(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeMinimalInitializedDeployment(t, deployment)

	// When
	if err := Destroy(testManagerContext(t), deployment, false); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}
	if err := RemoveLocalDeploymentDirectory(testManagerContext(t), deployment); err != nil {
		t.Fatalf("remove local deployment directory failed: %v", err)
	}

	// Then
	if _, err := os.Stat(deployment.Root()); !os.IsNotExist(err) {
		t.Fatalf("expected deployment directory to be removed, got %v", err)
	}
}

func TestDestroyPreservesLocalFilesWhenDestroyFails(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: unknown
`)
	state := newInitializedState(
		false,
		"0.0.0",
		"test-deployment",
		"test-cluster",
		time.Now().UTC(),
		PresetRef{Name: "test-infra"},
		PresetRef{Name: "test-install"},
	)
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{},
		deployment,
	); err != nil {
		t.Fatalf("write state failed: %v", err)
	}
	localPath := deployment.Resolve("local.txt")
	if err := os.WriteFile(localPath, []byte("local"), 0o600); err != nil {
		t.Fatalf("write local file failed: %v", err)
	}

	// When
	destroyErr := Destroy(testManagerContext(t), deployment, false)

	// Then
	if destroyErr == nil {
		t.Fatal("expected destroy to fail, got nil")
	}
	if _, statErr := os.Stat(localPath); statErr != nil {
		t.Fatalf("expected local file to be preserved, got %v", statErr)
	}
	updatedState, readErr := config.ReadExasolPersonalState(deployment)
	if readErr != nil {
		t.Fatalf("read state failed: %v", readErr)
	}
	workflowState, stateErr := updatedState.GetWorkflowState()
	if stateErr != nil {
		t.Fatalf("read workflow state failed: %v", stateErr)
	}
	interrupted, ok := workflowState.(*config.WorkflowStateInterrupted)
	if !ok {
		t.Fatalf("expected interrupted workflow state, got %T", workflowState)
	}
	if interrupted.InterruptedDuringOperation != config.DestroyOperation {
		t.Fatalf("expected destroy interruption, got %q", interrupted.InterruptedDuringOperation)
	}
}

func TestRemoveLocalDeploymentDirectory_RemovesDeploymentDirectory(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeMinimalInitializedDeployment(t, deployment)
	localPath := deployment.Resolve("local.txt")
	if err := os.WriteFile(localPath, []byte("local"), 0o600); err != nil {
		t.Fatalf("write local file failed: %v", err)
	}

	// When
	if err := RemoveLocalDeploymentDirectory(testManagerContext(t), deployment); err != nil {
		t.Fatalf("remove local deployment directory failed: %v", err)
	}

	// Then
	if _, err := os.Stat(deployment.Root()); !os.IsNotExist(err) {
		t.Fatalf("expected deployment directory to be removed, got %v", err)
	}
}

func TestRemoveLocalDeploymentDirectory_RejectsNonDeploymentDirectory(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localPath := deployment.Resolve("local.txt")
	if err := os.WriteFile(localPath, []byte("local"), 0o600); err != nil {
		t.Fatalf("write local file failed: %v", err)
	}

	// When
	err := RemoveLocalDeploymentDirectory(testManagerContext(t), deployment)

	// Then
	if !errors.Is(err, ErrNotExasolPersonalDeploymentDirectory) {
		t.Fatalf("expected ErrNotExasolPersonalDeploymentDirectory, got %v", err)
	}
	if _, statErr := os.Stat(localPath); statErr != nil {
		t.Fatalf("expected local file to be preserved, got %v", statErr)
	}
}

//nolint:paralleltest // This test intentionally changes process cwd.
func TestRemoveLocalDeploymentDirectory_RejectsCurrentDirectoryInsideDeployment(
	t *testing.T,
) {
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeMinimalInitializedDeployment(t, deployment)
	cwd := deployment.Resolve("subdir")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("create cwd failed: %v", err)
	}
	t.Chdir(cwd)

	// When
	err := RemoveLocalDeploymentDirectory(testManagerContext(t), deployment)

	// Then
	if !errors.Is(err, ErrDeploymentDirectoryRemovalUnsafe) {
		t.Fatalf("expected ErrDeploymentDirectoryRemovalUnsafe, got %v", err)
	}
	if !strings.Contains(err.Error(), "change to another directory") {
		t.Fatalf("expected cwd guidance, got %v", err)
	}
	if _, statErr := os.Stat(deployment.Root()); statErr != nil {
		t.Fatalf("expected deployment directory to be preserved, got %v", statErr)
	}
}

func TestValidateDeploymentDirectoryRemovalContext_RejectsLauncherBinaryInsideDeployment(
	t *testing.T,
) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	cwd := t.TempDir()
	executable := deployment.Resolve("bin/exasol")

	// When
	err := validateDeploymentDirectoryRemovalContext(deployment, cwd, executable)

	// Then
	if !errors.Is(err, ErrDeploymentDirectoryRemovalUnsafe) {
		t.Fatalf("expected ErrDeploymentDirectoryRemovalUnsafe, got %v", err)
	}
	if !strings.Contains(err.Error(), "move the launcher binary") {
		t.Fatalf("expected launcher binary guidance, got %v", err)
	}
}

func TestValidateDeploymentDirectoryRemovalContext_AllowsOutsidePaths(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	cwd := t.TempDir()
	executable := filepath.Join(t.TempDir(), "exasol")

	// When
	err := validateDeploymentDirectoryRemovalContext(deployment, cwd, executable)
	// Then
	if err != nil {
		t.Fatalf("expected outside paths to be allowed, got %v", err)
	}
}

func TestRemoveLocalDeploymentDirectory_AllowsExtractedPresetManifestsWithoutState(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	if err := os.MkdirAll(deployment.InstallationDir(), 0o700); err != nil {
		t.Fatalf("create installation dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)
	writeTestFile(t, deployment.InstallManifestPath(), `
name: Test Installation
description: test installation
install: []
`)

	// When
	if err := RemoveLocalDeploymentDirectory(testManagerContext(t), deployment); err != nil {
		t.Fatalf("remove local deployment directory failed: %v", err)
	}

	// Then
	if _, err := os.Stat(deployment.Root()); !os.IsNotExist(err) {
		t.Fatalf("expected deployment directory to be removed, got %v", err)
	}
}

func writeMinimalInitializedDeployment(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)
	state := newInitializedState(
		false,
		"0.0.0",
		"test-deployment",
		"test-cluster",
		time.Now().UTC(),
		PresetRef{Name: "test-infra"},
		PresetRef{Name: "test-install"},
	)
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{},
		deployment,
	); err != nil {
		t.Fatalf("write state failed: %v", err)
	}
}

func deploymentCreatedAtPattern(createdAt string) *regexp.Regexp {
	return regexp.MustCompile(`deployment_created_at\s*=\s*"` + regexp.QuoteMeta(createdAt) + `"`)
}
