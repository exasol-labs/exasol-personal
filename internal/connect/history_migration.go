// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/launchermigration"
)

// LegacyHistoryFilePath exists only so an existing file there can be
// migrated into the new history location.
func LegacyHistoryFilePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(cacheDir, historyFileName), nil
}

// MigrateLegacyHistoryFile is a no-op if legacyPath is missing or newPath
// already exists.
func MigrateLegacyHistoryFile(legacyPath, newPath string) error {
	if _, err := os.Lstat(legacyPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("inspect legacy history file %s: %w", legacyPath, err)
	}

	if _, err := os.Lstat(newPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect history file %s: %w", newPath, err)
	}

	if err := launchermigration.Migrate(legacyPath, newPath); err != nil {
		return fmt.Errorf("migrate legacy history file %s to %s: %w", legacyPath, newPath, err)
	}

	return nil
}
