// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package util

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyDir_PreservesRestrictivePermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}

	// Given a source tree whose file and nested directory are private
	root := t.TempDir()
	src := filepath.Join(root, "src")
	nested := filepath.Join(src, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("failed to create source tree: %v", err)
	}
	secret := filepath.Join(nested, "secret")
	if err := os.WriteFile(secret, []byte("token"), 0o600); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	// When the tree is copied
	dst := filepath.Join(root, "dst")
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then the copy keeps the source's modes rather than being widened
	fileInfo, err := os.Stat(filepath.Join(dst, "nested", "secret"))
	if err != nil {
		t.Fatalf("expected copied file to exist: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected copied file mode 0600, got %04o", got)
	}
	dirInfo, err := os.Stat(filepath.Join(dst, "nested"))
	if err != nil {
		t.Fatalf("expected copied directory to exist: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected copied directory mode 0700, got %04o", got)
	}
}

func TestCopyDir_PreservesExecutableBit(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}

	// Given a source tree holding an executable file
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o750); err != nil {
		t.Fatalf("failed to create source tree: %v", err)
	}
	//nolint:gosec // the executable bit is the property under test.
	if err := os.WriteFile(filepath.Join(src, "tool"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	// When the tree is copied
	dst := filepath.Join(root, "dst")
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then the copied file stays executable
	info, err := os.Stat(filepath.Join(dst, "tool"))
	if err != nil {
		t.Fatalf("expected copied file to exist: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("expected copied file mode 0755, got %04o", got)
	}
}
