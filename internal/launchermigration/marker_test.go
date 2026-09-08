// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package launchermigration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarker_DoneIsFalseWhenFileIsMissing(t *testing.T) {
	t.Parallel()

	// Given
	marker := NewMarker(filepath.Join(t.TempDir(), "marker"))

	// When
	done, err := marker.Done()
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if done {
		t.Fatal("expected marker to not be done")
	}
}

func TestMarker_WriteThenDoneIsTrue(t *testing.T) {
	t.Parallel()

	// Given
	marker := NewMarker(filepath.Join(t.TempDir(), "nested", "marker"))

	// When
	if err := marker.Write(); err != nil {
		t.Fatalf("expected no error writing marker, got %v", err)
	}
	done, err := marker.Done()
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !done {
		t.Fatal("expected marker to be done after Write")
	}
}

func TestMarker_DoneIsFalseWhenFileContentDoesNotMatch(t *testing.T) {
	t.Parallel()

	// Given
	path := filepath.Join(t.TempDir(), "marker")
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatalf("failed to seed marker file: %v", err)
	}
	marker := NewMarker(path)

	// When
	done, err := marker.Done()
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if done {
		t.Fatal("expected marker with unexpected content to not be done")
	}
}
