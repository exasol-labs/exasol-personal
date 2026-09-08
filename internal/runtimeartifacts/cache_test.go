// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
)

type testClock struct {
	now time.Time
}

func (c *testClock) Now() time.Time {
	return c.now
}

func TestDefaultCacheRootUsesLauncherRuntimeArtifactsNamespace(t *testing.T) {
	t.Parallel()

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("failed to resolve user cache dir: %v", err)
	}

	root, err := DefaultCacheRoot()
	if err != nil {
		t.Fatalf("expected cache root, got error: %v", err)
	}

	expected := filepath.Join(launcherpaths.DirPath(cacheDir), runtimeArtifactsDirName)
	if root != expected {
		t.Fatalf("expected %q, got %q", expected, root)
	}
}

func TestDefaultConfigPathUsesResourceConfigFile(t *testing.T) {
	t.Parallel()

	// Given
	want, err := launcherpaths.ResourceConfigFilePath()
	if err != nil {
		t.Fatalf("failed to resolve resource config file path: %v", err)
	}

	// When
	got, err := DefaultConfigPath()
	// Then
	if err != nil {
		t.Fatalf("expected config path, got error: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

//nolint:paralleltest // home-directory environment is process-global.
func TestLegacyConfigPathUsesLauncherRootDirectory(t *testing.T) {
	// Given
	home := t.TempDir()
	setRuntimeArtifactsTestHome(t, home)

	// When
	path, err := LegacyConfigPath()
	// Then
	if err != nil {
		t.Fatalf("expected config path, got error: %v", err)
	}
	expected := filepath.Join(launcherpaths.DirPath(home), cacheConfigFileName)
	if path != expected {
		t.Fatalf("expected %q, got %q", expected, path)
	}
}

func TestEnsureCacheConfigCreatesDefaultAndRejectsInvalidRetention(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config", "resources.yaml")

	cfg, err := EnsureCacheConfig(configPath)
	if err != nil {
		t.Fatalf("expected default config creation, got error: %v", err)
	}
	if cfg.RetentionDays != defaultRetentionDays {
		t.Fatalf("expected default retention %d, got %d", defaultRetentionDays, cfg.RetentionDays)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
	if !strings.Contains(string(content), "retention_days: 30") {
		t.Fatalf("expected retention_days in config, got: %s", string(content))
	}

	invalidCfg := CacheConfig{RetentionDays: 0}
	writeErr := writeCacheConfig(configPath, invalidCfg)
	if writeErr != nil {
		t.Fatalf("failed to write invalid config: %v", writeErr)
	}
	_, _, err = LoadCacheConfig(configPath)
	if !errors.Is(err, ErrInvalidCacheConfig) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func TestCacheIndexReadWriteRoundTripAndRejectsEscapingPaths(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t, testNow())
	index := emptyCacheIndex()
	entry := seedCacheEntry(
		t,
		cache,
		&index,
		"artifact",
		"payload",
		checksumString("payload"),
		testNow(),
	)

	if err := cache.writeIndex(index); err != nil {
		t.Fatalf("expected index write, got error: %v", err)
	}
	if _, err := os.Stat(cache.IndexPath() + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temporary index file to be absent, got %v", err)
	}

	read, exists, err := cache.readIndex()
	if err != nil {
		t.Fatalf("expected index read, got error: %v", err)
	}
	if !exists {
		t.Fatal("expected index to exist")
	}
	if read.Entries["artifact"].ArtifactPath != entry.ArtifactPath {
		t.Fatalf(
			"expected artifact path %q, got %q",
			entry.ArtifactPath,
			read.Entries["artifact"].ArtifactPath,
		)
	}

	malicious := `{
		"version":1,
		"entries":{
			"bad":{
				"entryPath":"../bad",
				"artifactPath":"artifacts/bad/file",
				"resolvedPath":"artifacts/bad/file"
			}
		}
	}`
	if err := os.WriteFile(cache.IndexPath(), []byte(malicious), filePerm); err != nil {
		t.Fatalf("failed to write malicious index: %v", err)
	}
	_, _, err = cache.readIndex()
	if err == nil || !strings.Contains(err.Error(), "must stay within") {
		t.Fatalf("expected containment error, got %v", err)
	}
}

func TestArtifactIdentityChangesWhenArtifactMetadataChanges(t *testing.T) {
	t.Parallel()

	base, err := artifactIdentity(
		"tofu",
		"linux/amd64",
		false,
		ArtifactSpec{
			URL:    "https://example.com/tofu.tgz",
			Sha256: strings.Repeat("A", 64),
		},
		"tofu.tgz",
		"",
	)
	if err != nil {
		t.Fatalf("expected identity, got error: %v", err)
	}
	changed, err := artifactIdentity(
		"tofu",
		"linux/amd64",
		false,
		ArtifactSpec{
			URL:    "https://example.com/tofu-v2.tgz",
			Sha256: strings.Repeat("A", 64),
		},
		"tofu.tgz",
		"",
	)
	if err != nil {
		t.Fatalf("expected changed identity, got error: %v", err)
	}
	if base == changed {
		t.Fatal("expected URL change to produce a distinct artifact identity")
	}
}

func TestCacheCleanRemovesStaleEntriesAndKeepsRecentEntries(t *testing.T) {
	t.Parallel()

	now := testNow()
	cache := newTestCache(t, now)
	writeTestCacheConfig(t, cache, 10)
	index := emptyCacheIndex()
	stale := seedCacheEntry(
		t,
		cache,
		&index,
		"stale",
		"old",
		checksumString("old"),
		now.AddDate(0, 0, -30),
	)
	recent := seedCacheEntry(
		t,
		cache,
		&index,
		"recent",
		"new",
		checksumString("new"),
		now.AddDate(0, 0, -1),
	)
	writeTestIndex(t, cache, index)

	summary, err := cache.Clean(context.Background(), CleanOptions{})
	if err != nil {
		t.Fatalf("expected clean to succeed, got %v", err)
	}

	if summary.Mode != CleanupModeStale || summary.RemovedEntries != 1 {
		t.Fatalf("unexpected cleanup summary: %+v", summary)
	}
	if _, err := os.Stat(cache.absolutePath(stale.EntryPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected stale entry to be removed, got %v", err)
	}
	if _, err := os.Stat(cache.absolutePath(recent.EntryPath)); err != nil {
		t.Fatalf("expected recent entry to remain, got %v", err)
	}
	read, _, err := cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read index: %v", err)
	}
	if _, ok := read.Entries["stale"]; ok {
		t.Fatal("expected stale metadata to be removed")
	}
	if _, ok := read.Entries["recent"]; !ok {
		t.Fatal("expected recent metadata to remain")
	}
}

func TestCacheCleanDryRunDoesNotMutateFilesOrMetadata(t *testing.T) {
	t.Parallel()

	now := testNow()
	cache := newTestCache(t, now)
	writeTestCacheConfig(t, cache, 1)
	index := emptyCacheIndex()
	entry := seedCacheEntry(
		t,
		cache,
		&index,
		"stale",
		"old",
		checksumString("old"),
		now.AddDate(0, 0, -10),
	)
	writeTestIndex(t, cache, index)

	summary, err := cache.Clean(context.Background(), CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("expected dry-run clean to succeed, got %v", err)
	}

	if !summary.DryRun || summary.RemovedEntries != 1 {
		t.Fatalf("unexpected dry-run summary: %+v", summary)
	}
	if _, err := os.Stat(cache.absolutePath(entry.EntryPath)); err != nil {
		t.Fatalf("expected dry-run to keep files, got %v", err)
	}
	read, _, err := cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read index: %v", err)
	}
	if !read.LastCleanup.Equal(index.LastCleanup) {
		t.Fatalf("expected dry-run to preserve last cleanup, got %s", read.LastCleanup)
	}
	if _, ok := read.Entries["stale"]; !ok {
		t.Fatal("expected dry-run to keep metadata")
	}
}

func TestCacheCleanInvalidRemovesChecksumMismatches(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t, testNow())
	index := emptyCacheIndex()
	good := seedCacheEntry(t, cache, &index, "good", "good", checksumString("good"), testNow())
	bad := seedCacheEntry(t, cache, &index, "bad", "bad", checksumString("expected"), testNow())
	writeTestIndex(t, cache, index)

	summary, err := cache.Clean(context.Background(), CleanOptions{Mode: CleanupModeInvalid})
	if err != nil {
		t.Fatalf("expected invalid clean to succeed, got %v", err)
	}

	if summary.Mode != CleanupModeInvalid ||
		summary.RemovedEntries != 1 ||
		summary.InvalidEntries != 1 {
		t.Fatalf("unexpected invalid cleanup summary: %+v", summary)
	}
	if _, err := os.Stat(cache.absolutePath(bad.EntryPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected invalid entry to be removed, got %v", err)
	}
	if _, err := os.Stat(cache.absolutePath(good.EntryPath)); err != nil {
		t.Fatalf("expected valid entry to remain, got %v", err)
	}
}

func TestCacheCleanAllWipesCacheContentsAndResetsMetadata(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t, testNow())
	index := emptyCacheIndex()
	seedCacheEntry(
		t,
		cache,
		&index,
		"artifact",
		"payload",
		checksumString("payload"),
		testNow(),
	)
	writeTestIndex(t, cache, index)
	unexpected := filepath.Join(
		cache.artifactsRoot(),
		"unexpected",
		"linux_amd64",
		"orphan",
		"file",
	)
	if err := os.MkdirAll(filepath.Dir(unexpected), dirPerm); err != nil {
		t.Fatalf("failed to create unexpected entry: %v", err)
	}
	if err := os.WriteFile(unexpected, []byte("orphan"), filePerm); err != nil {
		t.Fatalf("failed to write unexpected entry: %v", err)
	}
	partialDownload := filepath.Join(cache.downloadsRoot(), ".tmp-partial", "download.bin")
	if err := os.MkdirAll(filepath.Dir(partialDownload), dirPerm); err != nil {
		t.Fatalf("failed to create partial download directory: %v", err)
	}
	if err := os.WriteFile(partialDownload, []byte("partial"), filePerm); err != nil {
		t.Fatalf("failed to write partial download: %v", err)
	}
	rootUnexpected := filepath.Join(cache.root, "orphan-root")
	if err := os.WriteFile(rootUnexpected, []byte("root-orphan"), filePerm); err != nil {
		t.Fatalf("failed to write root-level unexpected content: %v", err)
	}

	summary, err := cache.Clean(context.Background(), CleanOptions{Mode: CleanupModeAll})
	if err != nil {
		t.Fatalf("expected full clean to succeed, got %v", err)
	}

	if summary.Mode != CleanupModeAll || summary.RemovedEntries != 1 {
		t.Fatalf("unexpected full cleanup summary: %+v", summary)
	}
	if _, err := os.Stat(cache.artifactsRoot()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected artifact tree to be removed, got %v", err)
	}
	if _, err := os.Stat(cache.downloadsRoot()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected downloads tree to be removed, got %v", err)
	}
	if _, err := os.Stat(rootUnexpected); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected root-level unexpected content to be removed, got %v", err)
	}
	read, _, err := cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read index: %v", err)
	}
	if len(read.Entries) != 0 {
		t.Fatalf("expected metadata reset, got %+v", read.Entries)
	}
	if _, err := os.Stat(cache.configPath); err != nil {
		t.Fatalf("expected cache configuration to be preserved, got %v", err)
	}
}

func TestCacheDiagnoseReportsChecksumMismatchAndUnexpectedPaths(t *testing.T) {
	t.Parallel()

	now := testNow()
	cache := newTestCache(t, now)
	writeTestCacheConfig(t, cache, 5)
	index := emptyCacheIndex()
	seedCacheEntry(t, cache, &index, "valid", "valid", checksumString("valid"), now)
	seedCacheEntry(
		t,
		cache,
		&index,
		"invalid",
		"invalid",
		checksumString("expected"),
		now.AddDate(0, 0, -10),
	)
	writeTestIndex(t, cache, index)
	unexpected := filepath.Join(
		cache.artifactsRoot(),
		"unexpected",
		"linux_amd64",
		"orphan",
		"file",
	)
	if err := os.MkdirAll(filepath.Dir(unexpected), dirPerm); err != nil {
		t.Fatalf("failed to create unexpected entry: %v", err)
	}
	if err := os.WriteFile(unexpected, []byte("orphan"), filePerm); err != nil {
		t.Fatalf("failed to write unexpected entry: %v", err)
	}

	report := cache.Diagnose()

	if report.ConfigError != "" || report.IndexError != "" {
		t.Fatalf(
			"expected clean diagnostic read, got config=%q index=%q",
			report.ConfigError,
			report.IndexError,
		)
	}
	if report.ArtifactCount != 2 || report.InvalidArtifacts != 1 || report.StaleCandidates != 1 {
		t.Fatalf("unexpected diagnostic counts: %+v", report)
	}
	statuses := map[string]string{}
	for _, entry := range report.Entries {
		statuses[entry.ID] = entry.IntegrityStatus
	}
	if statuses["valid"] != integrityStatusOK {
		t.Fatalf("expected valid entry to be ok, got %q", statuses["valid"])
	}
	if statuses["invalid"] != integrityStatusMismatch {
		t.Fatalf("expected invalid entry mismatch, got %q", statuses["invalid"])
	}
	expectedUnexpected := filepath.ToSlash(filepath.Join(
		artifactsDirName,
		"unexpected",
		"linux_amd64",
		"orphan",
	))
	if len(report.UnexpectedPaths) != 1 || report.UnexpectedPaths[0] != expectedUnexpected {
		t.Fatalf("expected unexpected entry root, got %+v", report.UnexpectedPaths)
	}
}

func TestCacheDiagnoseContinuesWhenOneEntryHasInvalidPath(t *testing.T) {
	t.Parallel()

	now := testNow()
	cache := newTestCache(t, now)
	index := emptyCacheIndex()
	seedCacheEntry(t, cache, &index, "valid", "valid", checksumString("valid"), now)
	index.Entries["bad"] = cacheIndexEntry{
		ResourceID:   "bad-resource",
		Platform:     "linux/amd64",
		Sha256:       checksumString("bad"),
		EntryPath:    "../bad",
		ArtifactPath: "artifacts/bad/file",
		ResolvedPath: "artifacts/bad/file",
		LastUsedAt:   now,
	}
	writeTestIndex(t, cache, index)

	report := cache.Diagnose()

	if report.IndexError != "" {
		t.Fatalf("expected diagnostics to continue past bad entry, got %q", report.IndexError)
	}
	if report.ArtifactCount != 1 || report.InvalidArtifacts != 1 {
		t.Fatalf("unexpected diagnostic counts: %+v", report)
	}
	statuses := map[string]string{}
	for _, entry := range report.Entries {
		statuses[entry.ID] = entry.IntegrityStatus
	}
	if statuses["valid"] != integrityStatusOK {
		t.Fatalf("expected valid entry to be ok, got %q", statuses["valid"])
	}
	if statuses["bad"] != integrityStatusReadError {
		t.Fatalf("expected bad entry read error, got %q", statuses["bad"])
	}
}

func TestCacheLockStatusAndUnlockUseDirectoryMutex(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t, testNow())
	if err := os.MkdirAll(cache.Root(), dirPerm); err != nil {
		t.Fatalf("failed to create cache root: %v", err)
	}
	mutex, err := directorymutex.New(cache.Root())
	if err != nil {
		t.Fatalf("failed to create mutex: %v", err)
	}
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		t.Fatalf("failed to acquire mutex: %v", err)
	}

	status := cache.lockStatus()
	if !status.Locked || status.Mode != "exclusive" {
		t.Fatalf("expected exclusive lock status, got %+v", status)
	}

	if err := cache.Unlock(); err != nil {
		t.Fatalf("expected unlock to clear marker, got %v", err)
	}
	status = cache.lockStatus()
	if status.Locked || status.Error != "" {
		t.Fatalf("expected unlocked cache, got %+v", status)
	}
}

func TestCacheLockContentionReturnsUserFacingError(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t, testNow())
	if err := os.MkdirAll(cache.Root(), dirPerm); err != nil {
		t.Fatalf("failed to create cache root: %v", err)
	}
	mutex, err := directorymutex.New(cache.Root())
	if err != nil {
		t.Fatalf("failed to create mutex: %v", err)
	}
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		t.Fatalf("failed to acquire mutex: %v", err)
	}
	t.Cleanup(func() {
		_ = mutex.ReleaseExclusive(context.Background())
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = cache.Clean(ctx, CleanOptions{})

	if !errors.Is(err, ErrCacheLocked) {
		t.Fatalf("expected cache locked error, got %v", err)
	}
	if !strings.Contains(err.Error(), "exasol cache unlock") {
		t.Fatalf("expected user-facing unlock hint, got %v", err)
	}
}

func setRuntimeArtifactsTestHome(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", home)
	}
}

func newTestCache(t *testing.T, now time.Time) *Cache {
	t.Helper()

	clk := &testClock{now: now}
	root := filepath.Join(t.TempDir(), "cache")
	configPath := filepath.Join(t.TempDir(), "config", "resources.yaml")

	return newCacheWithClock(root, configPath, clk)
}

func writeTestCacheConfig(t *testing.T, cache *Cache, retentionDays int) {
	t.Helper()

	cfg := CacheConfig{RetentionDays: retentionDays}
	if err := writeCacheConfig(cache.configPath, cfg); err != nil {
		t.Fatalf("failed to write cache config: %v", err)
	}
}

func writeTestIndex(t *testing.T, cache *Cache, index cacheIndex) {
	t.Helper()

	if err := cache.writeIndex(index); err != nil {
		t.Fatalf("failed to write cache index: %v", err)
	}
}

func seedCacheEntry(
	t *testing.T,
	cache *Cache,
	index *cacheIndex,
	artifactID, content, expectedSha string,
	lastUsedAt time.Time,
) cacheIndexEntry {
	t.Helper()

	entryPath := filepath.Join(cache.artifactsRoot(), "resource", "linux_amd64", artifactID)
	artifactPath := filepath.Join(entryPath, "artifact.bin")
	if err := os.MkdirAll(filepath.Dir(artifactPath), dirPerm); err != nil {
		t.Fatalf("failed to create artifact directory: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte(content), filePerm); err != nil {
		t.Fatalf("failed to write artifact: %v", err)
	}
	entryRelPath, err := cache.relativePath(entryPath)
	if err != nil {
		t.Fatalf("failed to resolve entry rel path: %v", err)
	}
	artifactRelPath, err := cache.relativePath(artifactPath)
	if err != nil {
		t.Fatalf("failed to resolve artifact rel path: %v", err)
	}

	entry := cacheIndexEntry{
		ResourceID:   "resource",
		Platform:     "linux/amd64",
		URL:          "https://example.com/" + artifactID,
		Sha256:       expectedSha,
		EntryPath:    entryRelPath,
		ArtifactPath: artifactRelPath,
		ResolvedPath: artifactRelPath,
		CreatedAt:    lastUsedAt.Add(-time.Hour),
		LastUsedAt:   lastUsedAt,
		SizeBytes:    int64(len(content)),
	}
	index.Entries[artifactID] = entry

	return entry
}

func checksumString(content string) string {
	sum := sha256.Sum256([]byte(content))

	return hex.EncodeToString(sum[:])
}

func testNow() time.Time {
	return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
}
