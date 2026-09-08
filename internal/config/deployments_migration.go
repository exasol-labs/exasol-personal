// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/launchermigration"
)

// ErrDeploymentNameCollision is returned by MigrateLegacyDeployments when a
// deployment name exists at both the legacy and the new managed-deployments
// root. The colliding name is left untouched at both locations.
var ErrDeploymentNameCollision = errors.New(
	"deployment name exists at both the legacy and new managed-deployments root",
)

// MigrateLegacyDeployments joins one error per name collision but still
// migrates every other, non-colliding deployment in the same run. It reports
// whether at least one deployment was migrated, including when it returns an
// error for another deployment.
func MigrateLegacyDeployments(legacyRoot, newRoot string) (bool, error) {
	entries, err := os.ReadDir(legacyRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("list legacy deployments root %s: %w", legacyRoot, err)
	}

	migrated := false
	var collisions []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if err := migrateOneLegacyDeployment(legacyRoot, newRoot, entry.Name()); err != nil {
			if errors.Is(err, ErrDeploymentNameCollision) {
				collisions = append(collisions, err)

				continue
			}

			return migrated, err
		}
		migrated = true
	}

	if len(collisions) > 0 {
		return migrated, errors.Join(collisions...)
	}

	return migrated, nil
}

// migrateOneLegacyDeployment wraps ErrDeploymentNameCollision when name
// already exists under newRoot.
func migrateOneLegacyDeployment(legacyRoot, newRoot, name string) error {
	legacyPath := filepath.Join(legacyRoot, name)
	newPath := filepath.Join(newRoot, name)

	err := launchermigration.Migrate(legacyPath, newPath)
	if errors.Is(err, launchermigration.ErrDestinationExists) {
		return fmt.Errorf(
			"%w: %q (legacy: %s, new: %s)", ErrDeploymentNameCollision, name, legacyPath, newPath,
		)
	} else if err != nil {
		return fmt.Errorf("migrate deployment %q: %w", name, err)
	}

	return nil
}
