// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"github.com/stretchr/testify/require"
)

func TestDeploymentDir_LayoutPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deployment := NewDeploymentDir(root)

	require.Equal(t, root, deployment.Root())
	require.Equal(
		t,
		filepath.Join(root, InfrastructureFilesDirectory),
		deployment.InfrastructureDir(),
	)
	require.Equal(
		t,
		filepath.Join(root, InstallationFilesDirectory),
		deployment.InstallationDir(),
	)
	require.Equal(
		t,
		filepath.Join(root, ConnectionInstruction),
		deployment.ConnectionInstructionsPath(),
	)
}

func TestDeploymentsRootPath_UsesSharedLauncherDeploymentsRoot(t *testing.T) {
	t.Parallel()

	// Given
	want, err := launcherpaths.DeploymentsRootPath()
	require.NoError(t, err)

	// When
	got, err := DeploymentsRootPath()

	// Then
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLegacyDeploymentsRootPath_UsesLauncherRootDirectory(t *testing.T) {
	t.Parallel()

	// Given
	rootDir, err := LauncherRootDirPath()
	require.NoError(t, err)

	// When
	got, err := LegacyDeploymentsRootPath()

	// Then
	require.NoError(t, err)
	require.Equal(t, filepath.Join(rootDir, deploymentsDirName), got)
}

func TestNamedDeploymentDirPath_SameParentAsDefault(t *testing.T) {
	t.Parallel()

	defaultPath, err := DefaultDeploymentDirPath()
	require.NoError(t, err)

	namedPath, err := NamedDeploymentDirPath("staging")
	require.NoError(t, err)

	require.Equal(t, filepath.Dir(defaultPath), filepath.Dir(namedPath))
	require.Equal(t, "staging", filepath.Base(namedPath))
}

func TestNamedDeploymentDirPath_DefaultNameMatchesDefaultPath(t *testing.T) {
	t.Parallel()

	defaultPath, err := DefaultDeploymentDirPath()
	require.NoError(t, err)

	namedPath, err := NamedDeploymentDirPath(defaultDeploymentDirName)
	require.NoError(t, err)

	require.Equal(t, defaultPath, namedPath)
}

func TestDeploymentDir_Resolve(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deployment := NewDeploymentDir(root)
	relPath := "ssh/private-key.pem"
	absPath := filepath.Join(t.TempDir(), "external-key.pem")

	require.Equal(t, filepath.Join(root, relPath), deployment.Resolve(relPath))
	require.Equal(t, absPath, deployment.Resolve(absPath))
}
