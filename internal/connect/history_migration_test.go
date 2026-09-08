// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrateLegacyHistoryFile_MovesFileAndPreservesContent(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "exasol_history")
	newPath := filepath.Join(dir, "history", "exasol_history")
	require.NoError(t, os.WriteFile(legacyPath, []byte("SELECT 1;\n"), 0o600))

	// When
	require.NoError(t, MigrateLegacyHistoryFile(legacyPath, newPath))

	// Then
	content, err := os.ReadFile(newPath)
	require.NoError(t, err)
	require.Equal(t, "SELECT 1;\n", string(content))

	_, err = os.Stat(legacyPath)
	require.True(t, os.IsNotExist(err))
}

func TestMigrateLegacyHistoryFile_IsNoOpWhenLegacyFileDoesNotExist(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "exasol_history")
	newPath := filepath.Join(dir, "history", "exasol_history")

	// When
	require.NoError(t, MigrateLegacyHistoryFile(legacyPath, newPath))

	// Then
	_, err := os.Stat(newPath)
	require.True(t, os.IsNotExist(err))
}

func TestMigrateLegacyHistoryFile_IsNoOpWhenNewFileAlreadyExists(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "exasol_history")
	newPath := filepath.Join(dir, "history", "exasol_history")
	require.NoError(t, os.WriteFile(legacyPath, []byte("legacy content"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Dir(newPath), 0o700))
	require.NoError(t, os.WriteFile(newPath, []byte("new content"), 0o600))

	// When
	require.NoError(t, MigrateLegacyHistoryFile(legacyPath, newPath))

	// Then
	content, err := os.ReadFile(newPath)
	require.NoError(t, err)
	require.Equal(t, "new content", string(content))

	legacyContent, err := os.ReadFile(legacyPath)
	require.NoError(t, err)
	require.Equal(t, "legacy content", string(legacyContent))
}
