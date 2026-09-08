// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
)

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_EmptyWhenRootMissing(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got: %#v", entries)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_SortsAlphabeticallyAndIgnoresNonDirectories(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	mkdirTest(t, filepath.Join(deploymentsRoot, "staging"))
	mkdirTest(t, filepath.Join(deploymentsRoot, "prod-aws"))
	writeTestMarker(t, filepath.Join(deploymentsRoot, "not-a-directory"))

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got: %#v", entries)
	}
	if entries[0].Name != "prod-aws" || entries[1].Name != "staging" {
		t.Fatalf("expected alphabetical order, got: %q, %q", entries[0].Name, entries[1].Name)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_ReportsNotInitializedForUnrecognizedDirectory(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	mkdirTest(t, filepath.Join(deploymentsRoot, "empty"))

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != deploy.StatusNotInitialized {
		t.Fatalf("expected a single not_initialized entry, got: %#v", entries)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_ReportsNotInitializedForLegacyMarkerOnlyDirectory(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	legacyDir := filepath.Join(deploymentsRoot, "legacy")
	mkdirTest(t, legacyDir)
	writeTestMarker(t, filepath.Join(legacyDir, legacyWorkflowStateMarker))

	// A directory recognized only via the legacy .workflowState.json marker
	// (no modern state file) has no lifecycle state for deploy.GetStatus to
	// read, so it is reported as not_initialized.
	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != deploy.StatusNotInitialized {
		t.Fatalf("expected a single not_initialized entry, got: %#v", entries)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_ReportsUnparseableStateFileAsNotInitialized(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	corruptDir := filepath.Join(deploymentsRoot, "corrupt")
	mkdirTest(t, corruptDir)
	writeTestMarker(t, filepath.Join(corruptDir, config.ExasolPersonalStateFileName))

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != deploy.StatusNotInitialized {
		t.Fatalf("expected a single not_initialized entry, got: %#v", entries)
	}
	if entries[0].Infrastructure != "" || entries[0].Installation != "" {
		t.Fatalf("expected no preset identity, got: %#v", entries[0])
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_ReportsCanonicalStatusAndPresetIdentity(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	runningDir := filepath.Join(deploymentsRoot, "running")
	mkdirTest(t, runningDir)
	writeRunningStateWithPresetIdentity(t, runningDir, "name:aws", "name:standard")
	stubDeploymentStatus(t, func(
		ctx context.Context,
		_ config.DeploymentDir,
	) (*deploy.StatusOutput, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Duration(defaultStatusTimeoutSeconds)*time.Second {
			t.Fatal("expected bounded status context")
		}

		return &deploy.StatusOutput{Status: deploy.StatusDatabaseReady}, nil
	})

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected a single entry, got: %#v", entries)
	}
	if entries[0].Status != deploy.StatusDatabaseReady {
		t.Fatalf("expected status %q, got %#v", deploy.StatusDatabaseReady, entries[0])
	}
	if entries[0].Infrastructure != "aws" || entries[0].Installation != "standard" {
		t.Fatalf("expected preset identity to be displayed, got: %#v", entries[0])
	}
}

//nolint:paralleltest // t.Chdir, t.Setenv, and deploymentStatusFn change process state.
func TestListDeploymentDirectories_ResolvesStatusesConcurrently(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	mkdirTest(t, filepath.Join(deploymentsRoot, "alpha"))
	mkdirTest(t, filepath.Join(deploymentsRoot, "beta"))

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	stubDeploymentStatus(t, func(
		context.Context,
		config.DeploymentDir,
	) (*deploy.StatusOutput, error) {
		started <- struct{}{}
		<-release

		return &deploy.StatusOutput{Status: deploy.StatusInitialized}, nil
	})

	result := make(chan error, 1)
	go func() {
		_, err := listDeploymentDirectories(context.Background())
		result <- err
	}()

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("expected both status checks to start concurrently")
		}
	}
	close(release)

	if err := <-result; err != nil {
		t.Fatalf("expected listing to succeed, got: %v", err)
	}
}

//nolint:paralleltest // t.Chdir, t.Setenv, and deploymentStatusFn change process state.
func TestListDeploymentDirectories_StopsWaitingAtParentDeadline(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	mkdirTest(t, filepath.Join(deploymentsRoot, "alpha"))
	mkdirTest(t, filepath.Join(deploymentsRoot, "beta"))
	stubDeploymentStatus(t, func(
		ctx context.Context,
		_ config.DeploymentDir,
	) (*deploy.StatusOutput, error) {
		<-ctx.Done()

		return nil, ctx.Err()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()

	entries, err := listDeploymentDirectories(ctx)
	if err != nil {
		t.Fatalf("expected listing to tolerate timed-out entries, got: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("expected listing to honor the shared deadline, took %s", elapsed)
	}
	if len(entries) != 2 {
		t.Fatalf("expected both entries after timeout, got: %#v", entries)
	}
	for _, entry := range entries {
		if entry.Status != deploy.StatusNotInitialized {
			t.Fatalf("expected timed-out entry fallback, got: %#v", entry)
		}
	}
}

func TestDeploymentsListCommand_HasNoDeploymentDirOrNameFlag(t *testing.T) {
	t.Parallel()

	if deploymentsListCmd.Flags().Lookup(deploymentDirFlagName) != nil {
		t.Fatal("expected deployments list to not register --deployment-dir")
	}
	if deploymentsListCmd.Flags().Lookup(deploymentNameFlagName) != nil {
		t.Fatal("expected deployments list to not register --deployment")
	}
}

func mkdirTest(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
}

func stubDeploymentStatus(
	t *testing.T,
	stub func(context.Context, config.DeploymentDir) (*deploy.StatusOutput, error),
) {
	t.Helper()
	original := deploymentStatusFn
	deploymentStatusFn = stub
	t.Cleanup(func() { deploymentStatusFn = original })
}

func writeRunningStateWithPresetIdentity(
	t *testing.T,
	deploymentPath string,
	infrastructureIdentity string,
	installationIdentity string,
) {
	t.Helper()

	deployment := config.NewDeploymentDir(deploymentPath)
	state := &config.ExasolPersonalState{
		InfrastructurePresetIdentity: infrastructureIdentity,
		InstallationPresetIdentity:   installationIdentity,
	}
	err := state.SetWorkflowStateAndWrite(&config.WorkflowStateRunning{}, deployment)
	if err != nil {
		t.Fatalf("failed to write running state: %v", err)
	}
}
