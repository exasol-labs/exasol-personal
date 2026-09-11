// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package launcherpaths resolves the launcher's own directory location. It
// has no dependency on internal/config or internal/presets, so any package
// that needs this alone (internal/runtimeartifacts, for one) can depend on
// it directly without pulling in either — and, in particular, without
// risking an import cycle should internal/presets ever depend on
// internal/runtimeartifacts.
package launcherpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	rootDirName     = ".exasol"
	personalDirName = "personal"

	namespaceDirName       = "exasol"
	appDirName             = "launcher"
	deploymentsDirName     = "deployments"
	historyDirName         = "history"
	resourceConfigFileName = "resources.yaml"
	resourceCacheDirName   = "resources"
	migrationMarkerName    = ".migrated"
)

// RootDirPath returns the launcher-owned directory under the current user's
// home directory.
func RootDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for launcher root directory: %w", err)
	}

	return DirPath(home), nil
}

// DirPath returns the launcher-owned directory below baseDir.
func DirPath(baseDir string) string {
	return filepath.Join(baseDir, rootDirName, personalDirName)
}

func DeploymentsRootPath() (string, error) {
	root, err := durableRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, deploymentsDirName), nil
}

func HistoryRootPath() (string, error) {
	root, err := durableRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, historyDirName), nil
}

func ResourceConfigFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory for resource cache config file: %w", err)
	}

	return filepath.Join(launcherNamespaceRoot(configDir), resourceConfigFileName), nil
}

func CacheRootPath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve cache directory for launcher cache root: %w", err)
	}

	return filepath.Join(launcherNamespaceRoot(cacheDir), resourceCacheDirName), nil
}

func MigrationMarkerPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory for launcher migration marker: %w", err)
	}

	return filepath.Join(launcherNamespaceRoot(configDir), migrationMarkerName), nil
}

// On Linux this stays a dotfile location separate from config; elsewhere it
// shares the config base, since os.UserConfigDir() already resolves to the
// platform's per-app directory.
func durableRootPath() (string, error) {
	return durableRootPathForOS(runtime.GOOS)
}

func durableRootPathForOS(goos string) (string, error) {
	if goos == "linux" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory for launcher durable root: %w", err)
		}

		return legacyStyleRoot(home), nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory for launcher durable root: %w", err)
	}

	return launcherNamespaceRoot(configDir), nil
}

func legacyStyleRoot(home string) string {
	return filepath.Join(home, rootDirName, appDirName)
}

func launcherNamespaceRoot(base string) string {
	return filepath.Join(base, namespaceDirName, appDirName)
}
