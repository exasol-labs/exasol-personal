// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/stretchr/testify/require"
)

func setStartupMigrationTestHome(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", home)
		t.Setenv("APPDATA", home)
	}
}

func seedLegacyDeployment(t *testing.T) {
	t.Helper()

	legacyRoot, err := config.LegacyDeploymentsRootPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(legacyRoot, "default"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(legacyRoot, "default", "deployment.json"), []byte("{}"), 0o600,
	))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_AppliesChangesAutomaticallyWithoutPrompting(t *testing.T) {
	// Given
	home := t.TempDir()
	setStartupMigrationTestHome(t, home)
	seedLegacyDeployment(t)

	// When: nothing supplies interactive input.
	var out strings.Builder
	err := runStartupMigration(&out)

	// Then
	require.NoError(t, err)
	newDeploymentsRoot, rootErr := config.DeploymentsRootPath()
	require.NoError(t, rootErr)
	_, statErr := os.Stat(filepath.Join(newDeploymentsRoot, "default", "deployment.json"))
	require.NoError(t, statErr)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_ReportsWhatItChanged(t *testing.T) {
	// Given
	home := t.TempDir()
	setStartupMigrationTestHome(t, home)
	seedLegacyDeployment(t)

	// When
	var out strings.Builder
	require.NoError(t, runStartupMigration(&out))

	// Then
	require.Contains(t, out.String(), "Migrated managed deployments")
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_StaysQuietWhenNothingToMigrate(t *testing.T) {
	// Given
	home := t.TempDir()
	setStartupMigrationTestHome(t, home)

	// When
	var out strings.Builder
	require.NoError(t, runStartupMigration(&out))

	// Then
	require.Empty(t, out.String())
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_SucceedsWhenNothingToMigrate(t *testing.T) {
	// Given
	home := t.TempDir()
	setStartupMigrationTestHome(t, home)

	// When
	err := runStartupMigration(&strings.Builder{})

	// Then
	require.NoError(t, err)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_FailsAndDoesNotWriteMarkerOnCollision(t *testing.T) {
	// Given: a deployment named "default" exists at both the legacy and the
	// new location.
	home := t.TempDir()
	setStartupMigrationTestHome(t, home)
	seedLegacyDeployment(t)
	newDeploymentsRoot, err := config.DeploymentsRootPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(newDeploymentsRoot, "default"), 0o700))

	// When
	migrationErr := runStartupMigration(&strings.Builder{})

	// Then
	require.Error(t, migrationErr)
	markerPath, markerPathErr := launcherpaths.MigrationMarkerPath()
	require.NoError(t, markerPathErr)
	_, statErr := os.Stat(markerPath)
	require.True(t, os.IsNotExist(statErr))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_ReportsCompletedChangesBeforeLaterCollision(t *testing.T) {
	// Given
	home := t.TempDir()
	setStartupMigrationTestHome(t, home)
	seedLegacyDeployment(t)

	legacyDeploymentsRoot, err := config.LegacyDeploymentsRootPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(legacyDeploymentsRoot, "staging"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(legacyDeploymentsRoot, "staging", "deployment.json"), []byte("{}"), 0o600,
	))
	newDeploymentsRoot, err := config.DeploymentsRootPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(newDeploymentsRoot, "default"), 0o700))

	legacyHistoryPath, err := connect.LegacyHistoryFilePath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyHistoryPath), 0o700))
	require.NoError(t, os.WriteFile(legacyHistoryPath, []byte("SELECT 1;\n"), 0o600))

	legacyConfigPath, err := runtimeartifacts.LegacyConfigPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyConfigPath), 0o700))
	require.NoError(t, os.WriteFile(legacyConfigPath, []byte("retention_days: 45\n"), 0o600))

	legacyCacheRoot, err := runtimeartifacts.LegacyCacheRoot()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(legacyCacheRoot, "artifacts"), 0o700))

	// When
	var out strings.Builder
	migrationErr := runStartupMigration(&out)

	// Then
	require.Error(t, migrationErr)
	require.ErrorIs(t, migrationErr, config.ErrDeploymentNameCollision)
	report := out.String()
	require.Contains(t, report, "Migrated cache configuration")
	require.Contains(t, report, "Deleted legacy resource cache")
	require.Contains(t, report, "Migrated SQL history")
	require.Contains(t, report, "Migrated managed deployments")
	_, err = os.Stat(filepath.Join(newDeploymentsRoot, "staging", "deployment.json"))
	require.NoError(t, err)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_SkipsAlreadyCompletedLocation(t *testing.T) {
	// Given: migration has already completed once.
	home := t.TempDir()
	setStartupMigrationTestHome(t, home)
	seedLegacyDeployment(t)
	require.NoError(t, runStartupMigration(&strings.Builder{}))

	// When
	var out strings.Builder
	err := runStartupMigration(&out)

	// Then
	require.NoError(t, err)
	require.Empty(t, out.String())
}

// Beyond the per-scenario tests above: a real upgrade hits all four
// locations at once, so this exercises them together rather than in
// isolation.
//
//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_MigratesEveryLocation(t *testing.T) {
	// Given
	home := t.TempDir()
	setStartupMigrationTestHome(t, home)
	seedLegacyDeployment(t)

	legacyHistoryPath, err := connect.LegacyHistoryFilePath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyHistoryPath), 0o700))
	require.NoError(t, os.WriteFile(legacyHistoryPath, []byte("SELECT 1;\n"), 0o600))

	legacyConfigPath, err := runtimeartifacts.LegacyConfigPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyConfigPath), 0o700))
	require.NoError(t, os.WriteFile(legacyConfigPath, []byte("retention_days: 45\n"), 0o600))

	legacyCacheRoot, err := runtimeartifacts.LegacyCacheRoot()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(legacyCacheRoot, "artifacts"), 0o700))

	// When
	var out strings.Builder
	require.NoError(t, runStartupMigration(&out))

	// Then
	newDeploymentsRoot, err := config.DeploymentsRootPath()
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(newDeploymentsRoot, "default", "deployment.json"))
	require.NoError(t, err)
	require.Equal(t, "{}", string(content))

	newHistoryPath, err := connect.HistoryFilePath()
	require.NoError(t, err)
	historyContent, err := os.ReadFile(newHistoryPath)
	require.NoError(t, err)
	require.Equal(t, "SELECT 1;\n", string(historyContent))

	newConfigPath, err := runtimeartifacts.DefaultConfigPath()
	require.NoError(t, err)
	cfg, present, err := runtimeartifacts.LoadCacheConfig(newConfigPath)
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, 45, cfg.RetentionDays)

	_, err = os.Stat(legacyCacheRoot)
	require.True(t, os.IsNotExist(err))

	report := out.String()
	require.Contains(t, report, "Migrated managed deployments")
	require.Contains(t, report, "Migrated SQL history")
	require.Contains(t, report, "Migrated cache configuration")
	require.Contains(t, report, "Deleted legacy resource cache")
}
