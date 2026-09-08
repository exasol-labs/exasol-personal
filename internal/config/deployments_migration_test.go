// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrateLegacyDeployments_IsNoOpWhenLegacyRootDoesNotExist(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	legacyRoot := filepath.Join(dir, "legacy")
	newRoot := filepath.Join(dir, "new")

	// When
	migrated, err := MigrateLegacyDeployments(legacyRoot, newRoot)

	// Then
	require.NoError(t, err)
	require.False(t, migrated)
	_, err = os.Stat(newRoot)
	require.True(t, os.IsNotExist(err))
}

func TestMigrateLegacyDeployments_MigratesEachLegacyDeployment(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	legacyRoot := filepath.Join(dir, "legacy")
	newRoot := filepath.Join(dir, "new")
	writeTestFile(t, filepath.Join(legacyRoot, "default", "deployment.json"), "{}")
	writeTestFile(t, filepath.Join(legacyRoot, "staging", "deployment.json"), "{}")

	// When
	migrated, migrationErr := MigrateLegacyDeployments(legacyRoot, newRoot)

	// Then
	require.NoError(t, migrationErr)
	require.True(t, migrated)
	requireFileContent(t, filepath.Join(newRoot, "default", "deployment.json"), "{}")
	requireFileContent(t, filepath.Join(newRoot, "staging", "deployment.json"), "{}")
	_, err := os.Stat(filepath.Join(legacyRoot, "default"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(legacyRoot, "staging"))
	require.True(t, os.IsNotExist(err))
}

func TestMigrateLegacyDeployments_CollisionIsReportedButOthersStillMigrate(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	legacyRoot := filepath.Join(dir, "legacy")
	newRoot := filepath.Join(dir, "new")
	writeTestFile(t, filepath.Join(legacyRoot, "default", "deployment.json"), "legacy default")
	writeTestFile(t, filepath.Join(legacyRoot, "staging", "deployment.json"), "legacy staging")
	writeTestFile(t, filepath.Join(newRoot, "default", "deployment.json"), "new default")

	// When
	migrated, err := MigrateLegacyDeployments(legacyRoot, newRoot)

	// Then
	require.True(t, migrated)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeploymentNameCollision)
	require.Contains(t, err.Error(), legacyRoot)
	require.Contains(t, err.Error(), newRoot)
	requireFileContent(t, filepath.Join(legacyRoot, "default", "deployment.json"), "legacy default")
	requireFileContent(t, filepath.Join(newRoot, "default", "deployment.json"), "new default")
	requireFileContent(t, filepath.Join(newRoot, "staging", "deployment.json"), "legacy staging")
	_, statErr := os.Stat(filepath.Join(legacyRoot, "staging"))
	require.True(t, os.IsNotExist(statErr))
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func requireFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}
