// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package launcherpaths

import (
	"path/filepath"
	"testing"
)

func TestDirPath_JoinsRootAndPersonalUnderBaseDir(t *testing.T) {
	t.Parallel()

	got := DirPath("/home/user")
	want := filepath.Join("/home/user", ".exasol", "personal")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRootDirPath_ResolvesUnderTheCurrentUsersHomeDirectory(t *testing.T) {
	t.Parallel()

	got, err := RootDirPath()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filepath.Base(got) != "personal" || filepath.Base(filepath.Dir(got)) != ".exasol" {
		t.Fatalf("expected a path ending in .exasol/personal, got %q", got)
	}
}

func TestLegacyStyleRoot_JoinsHomeDotExasolLauncher(t *testing.T) {
	t.Parallel()

	// When
	got := legacyStyleRoot("/home/user")
	// Then
	want := filepath.Join("/home/user", ".exasol", "launcher")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLauncherNamespaceRoot_JoinsBaseExasolLauncher(t *testing.T) {
	t.Parallel()

	// When
	got := launcherNamespaceRoot("/home/user/Library/Application Support")
	// Then
	want := filepath.Join("/home/user/Library/Application Support", "exasol", "launcher")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDurableRootPathForOS_LinuxUsesDotExasolLauncherUnderHome(t *testing.T) {
	t.Parallel()

	// When
	got, err := durableRootPathForOS("linux")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filepath.Base(got) != "launcher" || filepath.Base(filepath.Dir(got)) != ".exasol" {
		t.Fatalf("expected a path ending in .exasol/launcher, got %q", got)
	}
}

func TestDurableRootPathForOS_NonLinuxUsesConfigDirNamespace(t *testing.T) {
	t.Parallel()

	for _, goos := range []string{"darwin", "windows"} {
		// When
		got, err := durableRootPathForOS(goos)
		// Then
		if err != nil {
			t.Fatalf("%s: expected no error, got %v", goos, err)
		}
		if filepath.Base(got) != "launcher" || filepath.Base(filepath.Dir(got)) != "exasol" {
			t.Fatalf("%s: expected a path ending in exasol/launcher, got %q", goos, got)
		}
	}
}

func TestDeploymentsRootPath_ResolvesUnderTheDurableRoot(t *testing.T) {
	t.Parallel()

	// When
	got, err := DeploymentsRootPath()
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filepath.Base(got) != "deployments" || filepath.Base(filepath.Dir(got)) != "launcher" {
		t.Fatalf("expected a path ending in launcher/deployments, got %q", got)
	}
}

func TestHistoryRootPath_ResolvesUnderTheDurableRoot(t *testing.T) {
	t.Parallel()

	// When
	got, err := HistoryRootPath()
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filepath.Base(got) != "history" || filepath.Base(filepath.Dir(got)) != "launcher" {
		t.Fatalf("expected a path ending in launcher/history, got %q", got)
	}
}

func TestResourceConfigFilePath_ResolvesUnderTheNamespaceRoot(t *testing.T) {
	t.Parallel()

	// When
	got, err := ResourceConfigFilePath()
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filepath.Base(got) != "resources.yaml" || filepath.Base(filepath.Dir(got)) != "launcher" {
		t.Fatalf("expected a path ending in launcher/resources.yaml, got %q", got)
	}
}

func TestCacheRootPath_ResolvesUnderTheNamespaceRoot(t *testing.T) {
	t.Parallel()

	// When
	got, err := CacheRootPath()
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filepath.Base(got) != "resources" || filepath.Base(filepath.Dir(got)) != "launcher" {
		t.Fatalf("expected a path ending in launcher/resources, got %q", got)
	}
}
