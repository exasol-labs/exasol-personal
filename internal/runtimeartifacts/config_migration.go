// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// MigrateLegacyConfig is a no-op if legacyConfigPath does not exist.
func MigrateLegacyConfig(legacyConfigPath, newConfigPath string) error {
	data, err := os.ReadFile(legacyConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read legacy cache config %s: %w", legacyConfigPath, err)
	}

	cfg := DefaultCacheConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse legacy cache config %s: %w", legacyConfigPath, err)
	}
	if err := validateCacheConfig(cfg); err != nil {
		return fmt.Errorf("validate legacy cache config %s: %w", legacyConfigPath, err)
	}

	if err := writeCacheConfig(newConfigPath, cfg); err != nil {
		return fmt.Errorf("migrate legacy cache config to %s: %w", newConfigPath, err)
	}
	if err := os.Remove(legacyConfigPath); err != nil {
		return fmt.Errorf("remove legacy cache config %s: %w", legacyConfigPath, err)
	}

	return nil
}
