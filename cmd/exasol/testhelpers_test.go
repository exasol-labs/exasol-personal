// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts/runtimeartifactstest"
)

// testManagerContext returns a context carrying a Manager backed by the real
// embedded resource catalog, for tests that exercise commands reading the
// shared Manager from context.
func testManagerContext(t *testing.T) context.Context {
	t.Helper()

	return runtimeartifactstest.NewContext(t)
}

// Initialized is the state a deployment returns to after `exasol destroy`.
func writeInitializedDeployment(t *testing.T, infrastructureManifest string) string {
	t.Helper()
	dir := t.TempDir()
	dep := config.NewDeploymentDir(dir)

	if err := os.MkdirAll(filepath.Dir(dep.InfrastructureManifestPath()), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		dep.InfrastructureManifestPath(),
		[]byte(infrastructureManifest),
		0o600,
	); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	state := &config.ExasolPersonalState{DeploymentId: "local", ClusterIdentity: "local"}
	if err := state.SetWorkflowState(&config.WorkflowStateInitialized{}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	if err := config.WriteExasolPersonalState(state, dep); err != nil {
		t.Fatalf("write state: %v", err)
	}

	return dir
}
