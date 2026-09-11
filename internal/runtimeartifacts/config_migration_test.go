// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyConfig_MovesRetentionIntoResourceConfigAndRemovesLegacyFile(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "runtime-artifacts.yaml")
	newPath := filepath.Join(dir, "resources.yaml")
	if err := os.WriteFile(legacyPath, []byte("retention_days: 45\n"), filePerm); err != nil {
		t.Fatalf("failed to seed legacy config: %v", err)
	}

	// When
	if err := MigrateLegacyConfig(legacyPath, newPath); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	cfg, present, err := LoadCacheConfig(newPath)
	if err != nil {
		t.Fatalf("expected no error loading migrated config, got %v", err)
	}
	if !present {
		t.Fatal("expected migrated config file to be present after migration")
	}
	if cfg.RetentionDays != 45 {
		t.Fatalf("expected retention 45, got %d", cfg.RetentionDays)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy config file to be removed, stat returned err=%v", err)
	}
}

func TestMigrateLegacyConfig_IsNoOpWhenLegacyFileDoesNotExist(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "runtime-artifacts.yaml")
	newPath := filepath.Join(dir, "resources.yaml")

	// When
	if err := MigrateLegacyConfig(legacyPath, newPath); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("expected no config file to be created, stat returned err=%v", err)
	}
}

func TestMigrateLegacyConfig_RejectsInvalidLegacyRetention(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "runtime-artifacts.yaml")
	newPath := filepath.Join(dir, "resources.yaml")
	if err := os.WriteFile(legacyPath, []byte("retention_days: 0\n"), filePerm); err != nil {
		t.Fatalf("failed to seed legacy config: %v", err)
	}

	// When
	err := MigrateLegacyConfig(legacyPath, newPath)

	// Then
	if err == nil {
		t.Fatal("expected an error for invalid legacy retention")
	}
	if _, statErr := os.Stat(legacyPath); statErr != nil {
		t.Fatalf("expected legacy config file to remain after a failed migration: %v", statErr)
	}
}
