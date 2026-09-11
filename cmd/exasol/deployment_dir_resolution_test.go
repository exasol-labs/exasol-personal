// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/spf13/cobra"
)

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_ExplicitFlagWins(t *testing.T) {
	// Given: a current deployment directory and an explicit deployment directory flag.
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeTestMarker(t, filepath.Join(cwd, config.ExasolPersonalStateFileName))

	explicit := filepath.Join(t.TempDir(), "explicit")
	cmd, state := commandWithDeploymentDirFlag(t, explicit)

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the explicit flag value is used.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceExplicit {
		t.Fatalf("expected explicit source, got %v", source)
	}
	if deployment.Root() != explicit {
		t.Fatalf("expected %q, got %q", explicit, deployment.Root())
	}
}

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_CurrentDeploymentDirWinsWhenFlagOmitted(t *testing.T) {
	// Given: the current working directory is recognized as a deployment directory.
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeTestMarker(t, filepath.Join(cwd, config.DeploymentVersionMarkerFileName))
	cmd, state := commandWithDeploymentDirFlag(t, "")

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the current working directory is used.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceCurrent {
		t.Fatalf("expected current directory source, got %v", source)
	}
	if deployment.Root() != cwd {
		t.Fatalf("expected %q, got %q", cwd, deployment.Root())
	}
}

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_DefaultWinsWhenFlagOmittedOutsideDeploymentDir(t *testing.T) {
	// Given: the current working directory is not recognized as a deployment directory.
	home := t.TempDir()
	cwd := t.TempDir()
	setTestHome(t, home)
	t.Chdir(cwd)
	cmd, state := commandWithDeploymentDirFlag(t, "")

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the default deployment directory is used.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceDefault {
		t.Fatalf("expected default source, got %v", source)
	}
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	expected := filepath.Join(deploymentsRoot, "default")
	if deployment.Root() != expected {
		t.Fatalf("expected %q, got %q", expected, deployment.Root())
	}
}

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_NamedFlagWins(t *testing.T) {
	// Given: the current working directory is not recognized as a deployment directory.
	home := t.TempDir()
	cwd := t.TempDir()
	setTestHome(t, home)
	t.Chdir(cwd)
	cmd, state := commandWithDeploymentSelection(t, "", "staging")

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the named deployment directory is used.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceNamed {
		t.Fatalf("expected named source, got %v", source)
	}
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	expected := filepath.Join(deploymentsRoot, "staging")
	if deployment.Root() != expected {
		t.Fatalf("expected %q, got %q", expected, deployment.Root())
	}
}

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_NamedFlagWinsOverCurrentDeploymentDir(t *testing.T) {
	// Given: the current working directory is itself a different recognized deployment directory.
	home := t.TempDir()
	cwd := t.TempDir()
	setTestHome(t, home)
	t.Chdir(cwd)
	writeTestMarker(t, filepath.Join(cwd, config.ExasolPersonalStateFileName))
	cmd, state := commandWithDeploymentSelection(t, "", "staging")

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the explicit --deployment flag wins over the current directory.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceNamed {
		t.Fatalf("expected named source, got %v", source)
	}
	deploymentsRoot, err := config.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	expected := filepath.Join(deploymentsRoot, "staging")
	if deployment.Root() != expected {
		t.Fatalf("expected %q, got %q", expected, deployment.Root())
	}
}

func TestResolveDeploymentDirFromValues_DeploymentDirWinsOverNameWhenBothChanged(t *testing.T) {
	t.Parallel()

	// Given: both --deployment-dir and --deployment are marked as explicitly set.
	explicit := filepath.Join(t.TempDir(), "explicit")
	values := deploymentDirFlagValues{
		deploymentDir:        explicit,
		deploymentDirChanged: true,
		name:                 "staging",
		nameChanged:          true,
	}

	// When: the shared precedence resolver runs.
	deployment, source, err := resolveDeploymentDirFromValues(values)
	// Then: --deployment-dir wins.
	//
	// Real command execution never reaches this combination: Cobra's
	// MarkFlagsMutuallyExclusive plus the early ValidateFlagGroups call in root's
	// PersistentPreRunE reject it first. This deterministic precedence only
	// matters for the harmless pre-Cobra raw-args pre-scan.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceExplicit {
		t.Fatalf("expected explicit source, got %v", source)
	}
	if deployment.Root() != explicit {
		t.Fatalf("expected %q, got %q", explicit, deployment.Root())
	}
}

func TestRegisterDeploymentDirFlag_DeploymentDirAndNameAreMutuallyExclusive(t *testing.T) {
	t.Parallel()

	// Given: a command with both flags set.
	cmd, _ := commandWithDeploymentSelection(t, "/tmp/explicit", "staging")

	// When: flag groups are validated.
	err := cmd.ValidateFlagGroups()
	// Then: it fails, naming both flags.
	if err == nil {
		t.Fatal("expected an error for setting both --deployment-dir and --deployment")
	}
	if !strings.Contains(err.Error(), deploymentDirFlagName) ||
		!strings.Contains(err.Error(), deploymentNameFlagName) {
		t.Fatalf("expected error to mention both flags, got: %v", err)
	}
}

func commandWithDeploymentSelection(
	t *testing.T,
	deploymentDirValue string,
	nameValue string,
) (*cobra.Command, *CommonFlags) {
	t.Helper()

	state := &CommonFlags{}
	cmd := &cobra.Command{Use: "test"}
	registerDeploymentDirFlag(cmd, state)
	if deploymentDirValue != "" {
		if err := cmd.Flags().Set(deploymentDirFlagName, deploymentDirValue); err != nil {
			t.Fatalf("failed to set deployment-dir flag: %v", err)
		}
	}
	if nameValue != "" {
		if err := cmd.Flags().Set(deploymentNameFlagName, nameValue); err != nil {
			t.Fatalf("failed to set name flag: %v", err)
		}
	}

	return cmd, state
}

func setTestHome(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
}

func TestResolveDeploymentDir_IgnoresCommandsWithoutDeploymentDirFlag(t *testing.T) {
	t.Parallel()

	// Given: a command that does not operate on deployment directories.
	cmd := &cobra.Command{Use: "version"}
	state := &CommonFlags{}

	// When: deployment directory resolution runs.
	_, source, err := resolveDeploymentDir(cmd, state)
	// Then: no deployment directory source is selected.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceNone {
		t.Fatalf("expected no source, got %v", source)
	}
}

func commandWithDeploymentDirFlag(t *testing.T, value string) (*cobra.Command, *CommonFlags) {
	t.Helper()

	state := &CommonFlags{}
	cmd := &cobra.Command{Use: "test"}
	registerDeploymentDirFlag(cmd, state)
	if value != "" {
		if err := cmd.Flags().Set(deploymentDirFlagName, value); err != nil {
			t.Fatalf("failed to set deployment-dir flag: %v", err)
		}
	}

	return cmd, state
}

func writeTestMarker(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}
}
