// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package util

import (
	"os"
	"path/filepath"
	"testing"
)

// The null device is a character device but not a terminal, and a run
// redirected from it is unattended.
func TestIsTerminalRejectsNonTerminals(t *testing.T) {
	t.Parallel()

	nullDevice, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("could not open the null device: %v", err)
	}
	t.Cleanup(func() { _ = nullDevice.Close() })

	regularPath := filepath.Join(t.TempDir(), "input.sql")
	if err := os.WriteFile(regularPath, []byte("y\n"), 0o600); err != nil {
		t.Fatalf("could not write the temporary file: %v", err)
	}
	regularFile, err := os.Open(regularPath)
	if err != nil {
		t.Fatalf("could not open the temporary file: %v", err)
	}
	t.Cleanup(func() { _ = regularFile.Close() })

	for name, file := range map[string]*os.File{
		"null device":  nullDevice,
		"regular file": regularFile,
		"nil file":     nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if isTerminal(file) {
				t.Errorf("expected %s not to be reported as a terminal", name)
			}
		})
	}
}
