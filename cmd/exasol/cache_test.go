// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/spf13/cobra"
)

func TestCacheCommandsDoNotUseDeploymentDirectoryConcerns(t *testing.T) {
	t.Parallel()

	commands := map[string]*cobra.Command{
		"cache":        cacheCmd,
		"cache list":   cacheListCmd,
		"cache clean":  cacheCleanCmd,
		"cache unlock": cacheUnlockCmd,
		"diag cache":   diagCacheCmd,
	}
	for name, cmd := range commands {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if deploymentFileLoggingIsRequired(cmd) {
				t.Fatalf("expected %s to not require deployment file logging", name)
			}
			if deploymentCompatibilityIsRequired(cmd) {
				t.Fatalf("expected %s to not require deployment compatibility", name)
			}
			if deploymentDirMustBeInitialized(cmd) {
				t.Fatalf("expected %s to not require initialized deployment dir", name)
			}
		})
	}
}

//nolint:paralleltest // modifies process environment and global flag state.
func TestCacheListCommandInitializesConfig(t *testing.T) {
	home := t.TempDir()
	cacheHome := t.TempDir()
	setCacheCommandTestEnv(t, home, cacheHome)
	oldOutputJSON := commonFlags.OutputJson
	commonFlags.OutputJson = false
	t.Cleanup(func() {
		commonFlags.OutputJson = oldOutputJSON
	})

	cmd := *cacheListCmd
	var buf bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&buf)
	resetTerminalMessages()

	if err := cmd.RunE(&cmd, nil); err != nil {
		t.Fatalf("expected cache list to succeed, got %v", err)
	}
	writeTerminalMessages(terminalConfig{stdout: &buf, stderr: &stderr, showCallsToAction: true})
	if stderr.String() != "" {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}

	expectedConfig, err := runtimeartifacts.DefaultConfigPath()
	if err != nil {
		t.Fatalf("failed to resolve expected config path: %v", err)
	}
	if _, err := os.Stat(expectedConfig); err != nil {
		t.Fatalf("expected cache config to be created, got %v", err)
	}
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("failed to resolve user cache dir: %v", err)
	}
	expectedCacheRoot := filepath.Join(
		config.LauncherDirPath(userCacheDir),
		"runtime-artifacts",
	)
	if !strings.Contains(buf.String(), "Runtime artifact cache: "+expectedCacheRoot) {
		t.Fatalf("expected cache root in output, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "No cached runtime artifacts.") {
		t.Fatalf("expected empty-cache message, got %q", buf.String())
	}
}

//nolint:paralleltest // mutates shared terminal message queues and global env
func TestCacheUnlockCommandReportsConfirmationOnStderr(t *testing.T) {
	home := t.TempDir()
	cacheHome := t.TempDir()
	setCacheCommandTestEnv(t, home, cacheHome)

	cmd := *cacheUnlockCmd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	resetTerminalMessages()

	if err := cmd.RunE(&cmd, nil); err != nil {
		t.Fatalf("expected cache unlock to succeed, got %v", err)
	}
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	// The unlock confirmation is an operational notice: it belongs on stderr and
	// must not pollute stdout, which stays reserved for primary/JSON output.
	if strings.Contains(stdout.String(), "Runtime artifact cache lock cleared.") {
		t.Fatalf("confirmation must not appear on stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Runtime artifact cache lock cleared.") {
		t.Fatalf("expected confirmation on stderr, got %q", stderr.String())
	}
}

func TestCacheListCommandRegistersJSONFlag(t *testing.T) {
	t.Parallel()

	if cacheListCmd.Flags().Lookup("json") == nil {
		t.Fatal("expected cache list to register json output flag")
	}
}

func TestCacheCleanCommandRegistersCleanupFlags(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"invalid", "all", "partial-downloads", "dry-run"} {
		if cacheCleanCmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected cache clean to register --%s", name)
		}
	}
	for _, name := range []string{"invalid", "all", "partial-downloads"} {
		flag := cacheCleanCmd.Flags().Lookup(name)
		annotations := flag.Annotations["cobra_annotation_mutually_exclusive"]
		if len(annotations) != 1 || annotations[0] != "invalid all partial-downloads" {
			t.Fatalf(
				"expected --%s mutually exclusive with cleanup selectors, got %+v",
				name,
				annotations,
			)
		}
	}
}

//nolint:paralleltest // modifies global clean option state.
func TestCacheCleanRejectsMultipleCleanupSelectors(t *testing.T) {
	oldOpts := cacheCleanOpts
	cacheCleanOpts.Invalid = true
	cacheCleanOpts.All = false
	cacheCleanOpts.PartialDownloads = true
	cacheCleanOpts.DryRun = true
	t.Cleanup(func() {
		cacheCleanOpts = oldOpts
	})

	cmd := &cobra.Command{Use: "clean"}
	err := cacheCleanCmd.RunE(cmd, nil)

	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually exclusive flag error, got %v", err)
	}
}

func TestCacheUnlockHelpWarnsAboutActiveLauncherProcesses(t *testing.T) {
	t.Parallel()

	if !strings.Contains(cacheUnlockCmd.Long, "no launcher process") {
		t.Fatalf("expected cache unlock warning, got %q", cacheUnlockCmd.Long)
	}
}

func TestDiagCacheHelpDescribesReadOnlyBehavior(t *testing.T) {
	t.Parallel()

	if !strings.Contains(diagCacheCmd.Long, "without removing artifacts") ||
		!strings.Contains(diagCacheCmd.Long, "rewriting metadata") ||
		!strings.Contains(diagCacheCmd.Long, "clearing cache locks") {
		t.Fatalf("expected read-only diagnostic help, got %q", diagCacheCmd.Long)
	}
}

func TestRenderCacheListJSON(t *testing.T) {
	t.Parallel()

	entries := []runtimeartifacts.CacheEntryInfo{cacheEntryInfoFixture()}
	var buf bytes.Buffer

	if err := renderCacheListJSON(&buf, entries); err != nil {
		t.Fatalf("expected json render to succeed, got %v", err)
	}

	var decoded []runtimeartifacts.CacheEntryInfo
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("expected valid json, got %v: %s", err, buf.String())
	}
	if len(decoded) != 1 ||
		decoded[0].ResourceID != "tofu" ||
		decoded[0].ArtifactPath != "artifacts/tofu/download.tgz" ||
		decoded[0].ResolvedPath != "artifacts/tofu" {
		t.Fatalf("unexpected decoded entries: %+v", decoded)
	}
}

func TestRenderCacheListTextIncludesLastUsedTimestamp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	if err := renderCacheListText(&buf, "/cache", []runtimeartifacts.CacheEntryInfo{
		cacheEntryInfoFixture(),
	}); err != nil {
		t.Fatalf("expected text render to succeed, got %v", err)
	}

	output := buf.String()
	for _, expected := range []string{
		"Runtime artifact cache: /cache",
		"tofu linux/amd64",
		"last_used=2026-06-08T12:00:00Z",
		"size=1.5 KB",
		"path=artifacts/tofu",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in output, got %q", expected, output)
		}
	}
}

func TestRenderCacheCleanTextUsesDryRunAndInvalidWording(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := renderCacheCleanText(&buf, runtimeartifacts.CleanSummary{
		Mode:           "invalid",
		DryRun:         true,
		RemovedEntries: 2,
		RemovedBytes:   2 * 1024 * 1024,
		InvalidEntries: 2,
	})
	if err != nil {
		t.Fatalf("expected clean summary render to succeed, got %v", err)
	}

	output := buf.String()
	for _, expected := range []string{
		"Would remove 2 runtime artifact(s), 2 MB (mode: invalid).",
		"Invalid artifacts: 2",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in output, got %q", expected, output)
		}
	}
}

func TestRenderCacheDiagnosticsTextIncludesCacheState(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	report := runtimeartifacts.DiagnosticReport{
		CacheRoot:        "/cache",
		ConfigPath:       "/config/runtime-artifacts.yaml",
		ConfigExists:     true,
		RetentionDays:    30,
		IndexPath:        "/cache/index.json",
		IndexExists:      true,
		Lock:             runtimeartifacts.CacheLockStatus{Locked: true, Mode: "exclusive"},
		ArtifactCount:    1,
		TotalBytes:       3 * 1024 * 1024 * 1024,
		StaleCandidates:  1,
		InvalidArtifacts: 1,
		MissingFiles:     []string{"/cache/missing"},
		UnexpectedPaths:  []string{"/cache/unexpected"},
		Entries: []runtimeartifacts.DiagnosticEntry{
			{
				CacheEntryInfo:  cacheEntryInfoFixture(),
				Stale:           true,
				IntegrityStatus: "mismatch",
			},
		},
	}

	if err := renderCacheDiagnosticsText(&buf, report); err != nil {
		t.Fatalf("expected diagnostics render to succeed, got %v", err)
	}

	output := buf.String()
	for _, expected := range []string{
		"Cache root: /cache",
		"Config file: /config/runtime-artifacts.yaml",
		"Index file: index.json",
		"Config status: ok (retention_days=30)",
		"Index status: ok",
		"Lock status: locked (exclusive)",
		"Total size: 3 GB",
		"Invalid artifacts: 1",
		"tofu linux/amd64 integrity=mismatch stale=true path=artifacts/tofu",
		"Missing: missing",
		"Unexpected: unexpected",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in output, got %q", expected, output)
		}
	}
}

func TestFormatCachePathReturnsRelativePathsInsideCacheRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cacheRoot string
		pathValue string
		expected  string
	}{
		{
			name:      "inside root",
			cacheRoot: "/cache",
			pathValue: "/cache/artifacts/tofu/download.tgz",
			expected:  "artifacts/tofu/download.tgz",
		},
		{
			name:      "outside root",
			cacheRoot: "/cache",
			pathValue: "/config/runtime-artifacts.yaml",
			expected:  "/config/runtime-artifacts.yaml",
		},
		{
			name:      "already relative",
			cacheRoot: "/cache",
			pathValue: "artifacts/tofu",
			expected:  "artifacts/tofu",
		},
	}
	for _, test := range tests {
		if actual := formatCachePath(test.cacheRoot, test.pathValue); actual != test.expected {
			t.Fatalf("%s: expected %q, got %q", test.name, test.expected, actual)
		}
	}
}

func TestFormatByteSizeUsesHumanFriendlyUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		size     int64
		expected string
	}{
		{size: 42, expected: "42 B"},
		{size: 1024, expected: "1 KB"},
		{size: 1536, expected: "1.5 KB"},
		{size: 2 * 1024 * 1024, expected: "2 MB"},
		{size: 5*1024*1024 + 512*1024, expected: "5.5 MB"},
		{size: 3 * 1024 * 1024 * 1024, expected: "3 GB"},
	}
	for _, test := range tests {
		if actual := formatByteSize(test.size); actual != test.expected {
			t.Fatalf("expected %d bytes to render as %q, got %q", test.size, test.expected, actual)
		}
	}
}

func cacheEntryInfoFixture() runtimeartifacts.CacheEntryInfo {
	return runtimeartifacts.CacheEntryInfo{
		ID:           "identity",
		ResourceID:   "tofu",
		Platform:     "linux/amd64",
		URL:          "https://example.com/tofu",
		Sha256:       strings.Repeat("a", 64),
		ArtifactPath: "artifacts/tofu/download.tgz",
		ResolvedPath: "artifacts/tofu",
		CreatedAt:    time.Date(2026, 6, 8, 11, 0, 0, 0, time.UTC),
		LastUsedAt:   time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		SizeBytes:    1536,
	}
}

func setCacheCommandTestEnv(t *testing.T, home, cacheHome string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", cacheHome)
	case "darwin":
		t.Setenv("HOME", home)
	default:
		t.Setenv("XDG_CACHE_HOME", cacheHome)
	}
}
