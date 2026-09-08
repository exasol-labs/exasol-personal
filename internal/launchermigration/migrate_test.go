// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package launchermigration

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMigrate_FileWithinSameFilesystemMoves(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	source := filepath.Join(dir, "source", "history")
	destination := filepath.Join(dir, "destination", "history")
	writeFile(t, source, "some history")

	// When
	if err := Migrate(source, destination); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	assertFileContent(t, destination, "some history")
	assertGone(t, source)
}

func TestMigrate_DirectoryWithinSameFilesystemMoves(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	source := filepath.Join(dir, "source", "deployment")
	destination := filepath.Join(dir, "destination", "deployment")
	writeFile(t, filepath.Join(source, "deployment.json"), "{}")

	// When
	if err := Migrate(source, destination); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	assertFileContent(t, filepath.Join(destination, "deployment.json"), "{}")
	assertGone(t, source)
}

func TestMigrate_DestinationAlreadyExistsIsRefused(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	writeFile(t, source, "content")
	writeFile(t, destination, "existing content")

	// When
	err := Migrate(source, destination)

	// Then
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("expected ErrDestinationExists, got %v", err)
	}
	assertFileContent(t, source, "content")
	assertFileContent(t, destination, "existing content")
}

func TestMigrateViaStaging_FileRoundTrips(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	writeFile(t, source, "history contents")

	// When
	if err := migrateViaStaging(source, destination); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	assertFileContent(t, destination, "history contents")
	assertGone(t, source)
	assertGone(t, destination+stagingSuffix)
	assertGone(t, destination+stagingSuffix+stagingSuffix+".complete")
}

func TestMigrateViaStaging_DirectoryRoundTrips(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	writeFile(t, filepath.Join(source, "nested", "file.txt"), "nested content")

	// When
	if err := migrateViaStaging(source, destination); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	assertFileContent(t, filepath.Join(destination, "nested", "file.txt"), "nested content")
	assertGone(t, source)
}

func TestMigrateViaStaging_PreservesDirectoryAndFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	t.Parallel()

	// Given
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	privateDir := filepath.Join(source, "private")
	secretFile := filepath.Join(privateDir, "secrets.json")
	writeFile(t, secretFile, "secret")
	//nolint:gosec // verifies exact source mode preservation
	if err := os.Chmod(source, 0o750); err != nil {
		t.Fatalf("failed to set source permissions: %v", err)
	}
	//nolint:gosec // verifies exact source mode preservation
	if err := os.Chmod(privateDir, 0o710); err != nil {
		t.Fatalf("failed to set private directory permissions: %v", err)
	}
	if err := os.Chmod(secretFile, 0o600); err != nil {
		t.Fatalf("failed to set secret permissions: %v", err)
	}

	// When
	if err := migrateViaStaging(source, destination); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	assertPermissions(t, destination, 0o750)
	assertPermissions(t, filepath.Join(destination, "private"), 0o710)
	assertPermissions(t, filepath.Join(destination, "private", "secrets.json"), 0o600)
}

func TestMigrateViaStaging_SourceIsPreservedWhenStagingCopyFails(t *testing.T) {
	t.Parallel()

	// Given: the staging copy cannot be written, so the copy never completes.
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destParent := filepath.Join(dir, "readonly")
	destination := filepath.Join(destParent, "destination")
	writeFile(t, source, "must survive")
	if err := os.MkdirAll(destParent, 0o700); err != nil {
		t.Fatalf("failed to create destination parent: %v", err)
	}
	//nolint:gosec // remove write bit to force the staging copy to fail
	if err := os.Chmod(destParent, 0o500); err != nil {
		t.Fatalf("failed to make destination parent read-only: %v", err)
	}
	t.Cleanup(func() {
		//nolint:errcheck,gosec,revive // best-effort cleanup
		os.Chmod(destParent, 0o700)
	})

	// When
	err := migrateViaStaging(source, destination)

	// Then
	if err == nil {
		t.Fatal("expected an error when the staging copy cannot be written")
	}
	assertFileContent(t, source, "must survive")
	if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected destination to not exist, stat returned err=%v", statErr)
	}
}

func TestMigrateViaStaging_ResetsIncompleteStagingAndRetries(t *testing.T) {
	t.Parallel()

	// Given: a staging directory left over from a previous attempt, with no
	// completion marker, so it must not be trusted.
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	writeFile(t, source, "correct content")
	staging := destination + stagingSuffix
	writeFile(t, filepath.Join(staging, "leftover"), "stale")

	// When
	if err := migrateViaStaging(source, destination); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	assertFileContent(t, destination, "correct content")
	assertGone(t, source)
}

func TestMigrateViaStaging_ResumesFromCompletedStagingWithoutRecopying(t *testing.T) {
	t.Parallel()

	// Given: a process died after finishing the staged copy and writing the
	// marker, but before publishing and removing the source. The source no
	// longer needs to exist for a resume to succeed, since the copy step is
	// skipped once the marker is already done.
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	staging := destination + stagingSuffix
	writeFile(t, staging, "already staged content")
	marker := migrationMarker(destination)
	if err := marker.Write(); err != nil {
		t.Fatalf("failed to seed marker: %v", err)
	}

	// When
	if err := migrateViaStaging(source, destination); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	assertFileContent(t, destination, "already staged content")
	assertGone(t, staging)
	assertGone(t, marker.Path())
}

func TestMigrate_ResumesAfterCompletedStagingWasPublished(t *testing.T) {
	t.Parallel()

	// Given: a process died after publishing the staged copy but before removing
	// the legacy source.
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	writeFile(t, source, "legacy content")
	writeFile(t, destination, "published content")
	marker := migrationMarker(destination)
	if err := marker.Write(); err != nil {
		t.Fatalf("failed to seed marker: %v", err)
	}

	// When
	if err := Migrate(source, destination); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Then
	assertFileContent(t, destination, "published content")
	assertGone(t, source)
	assertGone(t, marker.Path())
}

func TestMigrate_DoesNotTrustCompletedMarkerWhileStagingStillExists(t *testing.T) {
	t.Parallel()

	// Given: another destination appeared after staging completed, so the
	// staged copy was never published.
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	staging := destination + stagingSuffix
	writeFile(t, source, "legacy content")
	writeFile(t, destination, "collision content")
	writeFile(t, staging, "staged content")
	marker := migrationMarker(destination)
	if err := marker.Write(); err != nil {
		t.Fatalf("failed to seed marker: %v", err)
	}

	// When
	err := Migrate(source, destination)

	// Then
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("expected ErrDestinationExists, got %v", err)
	}
	assertFileContent(t, source, "legacy content")
	assertFileContent(t, destination, "collision content")
	assertFileContent(t, staging, "staged content")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("failed to create parent directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("expected %s to contain %q, got %q", path, want, string(got))
	}
}

func assertGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to not exist, stat returned err=%v", path, err)
	}
}

func assertPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("expected %s permissions to be %04o, got %04o", path, want, got)
	}
}
