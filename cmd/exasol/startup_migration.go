// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/exasol/exasol-personal/internal/launchermigration"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// runStartupMigration runs once, gated by a completion marker; a failure
// leaves the marker unset so the next invocation retries.
func runStartupMigration(out io.Writer) error {
	markerPath, err := launcherpaths.MigrationMarkerPath()
	if err != nil {
		return err
	}
	marker := launchermigration.NewMarker(markerPath)

	done, err := marker.Done()
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	messages, err := migrateLegacyLocations()

	for _, message := range messages {
		if _, reportErr := fmt.Fprintln(out, message); reportErr != nil {
			return errors.Join(err, fmt.Errorf("report migration result: %w", reportErr))
		}
	}
	if err != nil {
		return err
	}

	return marker.Write()
}

func migrateLegacyLocations() ([]string, error) {
	var messages []string

	message, err := migrateLegacyCacheConfig()
	if err != nil {
		return messages, err
	}
	if message != "" {
		messages = append(messages, message)
	}

	message, err = deleteLegacyCache()
	if err != nil {
		return messages, err
	}
	if message != "" {
		messages = append(messages, message)
	}

	message, err = migrateLegacyHistory()
	if err != nil {
		return messages, err
	}
	if message != "" {
		messages = append(messages, message)
	}

	message, err = migrateLegacyDeployments()
	if message != "" {
		messages = append(messages, message)
	}
	if err != nil {
		return messages, err
	}

	return messages, nil
}

func migrateLegacyCacheConfig() (string, error) {
	legacyPath, err := runtimeartifacts.LegacyConfigPath()
	if err != nil {
		return "", err
	}
	newPath, err := runtimeartifacts.DefaultConfigPath()
	if err != nil {
		return "", err
	}

	hadLegacy, err := pathExists(legacyPath)
	if err != nil {
		return "", err
	}

	if err := runtimeartifacts.MigrateLegacyConfig(legacyPath, newPath); err != nil {
		return "", fmt.Errorf("migrate cache configuration: %w", err)
	}
	if !hadLegacy {
		return "", nil
	}

	return "Migrated cache configuration to " + newPath, nil
}

func deleteLegacyCache() (string, error) {
	legacyRoot, err := runtimeartifacts.LegacyCacheRoot()
	if err != nil {
		return "", err
	}

	hadLegacy, err := pathExists(legacyRoot)
	if err != nil {
		return "", err
	}

	if err := runtimeartifacts.DeleteLegacyCache(legacyRoot); err != nil {
		return "", fmt.Errorf("delete legacy resource cache: %w", err)
	}
	if !hadLegacy {
		return "", nil
	}

	return "Deleted legacy resource cache at " + legacyRoot, nil
}

func migrateLegacyHistory() (string, error) {
	legacyPath, err := connect.LegacyHistoryFilePath()
	if err != nil {
		return "", err
	}
	newPath, err := connect.HistoryFilePath()
	if err != nil {
		return "", err
	}

	hadLegacy, err := pathExists(legacyPath)
	if err != nil {
		return "", err
	}
	newAlreadyExists, err := pathExists(newPath)
	if err != nil {
		return "", err
	}

	if err := connect.MigrateLegacyHistoryFile(legacyPath, newPath); err != nil {
		return "", fmt.Errorf("migrate SQL history: %w", err)
	}
	if !hadLegacy || newAlreadyExists {
		return "", nil
	}

	return "Migrated SQL history to " + newPath, nil
}

func migrateLegacyDeployments() (string, error) {
	legacyRoot, err := config.LegacyDeploymentsRootPath()
	if err != nil {
		return "", err
	}
	newRoot, err := config.DeploymentsRootPath()
	if err != nil {
		return "", err
	}

	hadLegacy, err := pathExists(legacyRoot)
	if err != nil {
		return "", err
	}

	migrated, err := config.MigrateLegacyDeployments(legacyRoot, newRoot)
	if err != nil && !migrated {
		return "", fmt.Errorf("migrate managed deployments: %w", err)
	}
	if !hadLegacy || !migrated {
		return "", nil
	}

	message := "Migrated managed deployments to " + newRoot
	if err != nil {
		return message, fmt.Errorf("migrate managed deployments: %w", err)
	}

	return message, nil
}
