// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

func TestReadRunnerStateUsesLabeledForwardsWithoutTransportMetadata(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "vm-state.json")
	state := []byte(`{
  "vm_name": "exasol-local-vm",
  "shared_dir": "./vm-shared",
  "forwards": {"db": {"guest_port": 8563, "host_port": 28563}}
}`)
	if err := os.WriteFile(statePath, state, 0o600); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	parsed, err := readRunnerState(statePath)
	if err != nil {
		t.Fatalf("failed to parse state: %v", err)
	}
	endpoint, err := runtimeEndpointFromRunnerState(parsed)
	if err != nil || endpoint.DBPort != 28563 {
		t.Fatalf("unexpected endpoint %#v, err=%v", endpoint, err)
	}
	if !endpoint.ShellSupported {
		t.Fatalf("expected runner endpoint to advertise shell support, got %#v", endpoint)
	}
}

func TestReadRunnerStateRejectsMissingOrWrongDatabaseForward(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"missing":      `{"forwards":{}}`,
		"wrong guest":  `{"forwards":{"db":{"guest_port":9999,"host_port":28563}}}`,
		"invalid host": `{"forwards":{"db":{"guest_port":8563,"host_port":0}}}`,
	}
	for name, state := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			statePath := filepath.Join(t.TempDir(), "vm-state.json")
			if err := os.WriteFile(statePath, []byte(state), 0o600); err != nil {
				t.Fatalf("failed to write state: %v", err)
			}
			if _, err := readRunnerState(statePath); err == nil {
				t.Fatal("expected invalid state to fail")
			}
		})
	}
}

func TestResolveMacHostDBPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ports string
		want  int
	}{
		{ports: "db:28563", want: 28563},
		{ports: "ssh:20022, db:28563", want: 28563},
	}
	for _, test := range tests {
		actual, err := resolveMacHostDBPort(test.ports)
		if err != nil || actual != test.want {
			t.Fatalf(
				"resolveMacHostDBPort(%q) = %d, %v; want %d",
				test.ports,
				actual,
				err,
				test.want,
			)
		}
	}

	for _, ports := range []string{
		"", "auto", "db", "db:0", "db:-1", "db:65536", "db:1,db:2", "ssh:20022",
	} {
		if _, err := resolveMacHostDBPort(ports); err == nil {
			t.Fatalf("expected invalid mapping %q to fail", ports)
		}
	}
}

//nolint:paralleltest // test runner scripts fork executable fixtures.
func TestMacVMRuntimePrepareCreatesAndReusesManagedHostDataLink(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given
	eventsPath := filepath.Join(t.TempDir(), "events")
	runtime := NewMacVMRuntime(
		config.NewDeploymentDir(t.TempDir()),
		newTestManagerForRunner(t, []byte(fakeV2RunnerScript(eventsPath))),
	)

	// When
	if err := runtime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("failed to prepare runtime: %v", err)
	}
	if err := runtime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("failed to prepare runtime again: %v", err)
	}

	// Then
	target, err := os.Readlink(runtime.paths.HostNanoDataDir)
	if err != nil {
		t.Fatalf("failed to read host data link: %v", err)
	}
	wantTarget := filepath.Join(sharedDirName, nanoDataDirName)
	if target != wantTarget {
		t.Fatalf("host data link target = %q, want %q", target, wantTarget)
	}
}

//nolint:paralleltest // test runner scripts fork executable fixtures.
func TestMacVMRuntimePrepareReplacesGuestWhenRunnerChanged(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given a deployment whose guest came from a different runner
	eventsPath := filepath.Join(t.TempDir(), "events")
	runtime := newPreparedGuestRuntime(t, eventsPath, "2.0.0-dev", "1.1.0")

	// When
	if err := runtime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("failed to prepare runtime: %v", err)
	}

	// Then the guest is replaced and the resolved runner recorded
	assertRunnerEvents(t, eventsPath, "runner-init")
	assertMarkerVersion(t, runtime, "2.0.0-dev")
}

//nolint:paralleltest // test runner scripts fork executable fixtures.
func TestMacVMRuntimePrepareKeepsGuestOfRecordedRunner(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given a deployment whose guest came from the resolved runner
	eventsPath := filepath.Join(t.TempDir(), "events")
	runtime := newPreparedGuestRuntime(t, eventsPath, "2.0.0-dev", "2.0.0-dev")

	// When
	if err := runtime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("failed to prepare runtime: %v", err)
	}

	// Then the guest is left alone
	assertRunnerEvents(t, eventsPath)
}

//nolint:paralleltest // test runner scripts fork executable fixtures.
func TestMacVMRuntimePrepareStopsRunningVMBeforeReplacingGuest(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given a running VM whose guest came from a different runner
	eventsPath := filepath.Join(t.TempDir(), "events")
	runtime := newPreparedGuestRuntime(t, eventsPath, "2.0.0-dev", "1.1.0")
	if err := os.WriteFile(
		filepath.Join(runtime.paths.WorkDir, "running"), nil, markerFileMode,
	); err != nil {
		t.Fatalf("failed to mark the VM as running: %v", err)
	}

	// When
	if err := runtime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("failed to prepare runtime: %v", err)
	}

	// Then the VM stops before its guest disk is rewritten
	assertRunnerEvents(t, eventsPath, "runner-stop", "runner-init")
}

//nolint:paralleltest // test runner scripts fork executable fixtures.
func TestMacVMRuntimePrepareKeepsRecordedVersionWhenReplacementFails(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given a runner that cannot rebuild the guest
	eventsPath := filepath.Join(t.TempDir(), "events")
	script := strings.Replace(
		fakeV2RunnerScriptWithVersion(eventsPath, "2.0.0-dev"),
		"    mkdir -p vm vm-shared\n",
		"    exit 3\n",
		1,
	)
	runtime := NewMacVMRuntime(
		config.NewDeploymentDir(t.TempDir()), newTestManagerForRunner(t, []byte(script)),
	)
	seedPreparedGuest(t, runtime, "1.1.0")

	// When
	err := runtime.Prepare(context.Background(), nil, nil, PrepareOptions{})

	// Then the recorded version still reports the guest that is actually there
	if err == nil {
		t.Fatal("expected guest replacement to fail")
	}
	assertMarkerVersion(t, runtime, "1.1.0")
}

func newPreparedGuestRuntime(
	t *testing.T,
	eventsPath, runnerVersion, recordedVersion string,
) *MacVMRuntime {
	t.Helper()

	runtime := NewMacVMRuntime(
		config.NewDeploymentDir(t.TempDir()),
		newTestManagerForRunner(
			t, []byte(fakeV2RunnerScriptWithVersion(eventsPath, runnerVersion)),
		),
	)
	seedPreparedGuest(t, runtime, recordedVersion)

	return runtime
}

func seedPreparedGuest(t *testing.T, runtime *MacVMRuntime, recordedVersion string) {
	t.Helper()

	if err := os.MkdirAll(runtime.paths.VMDir, dirMode); err != nil {
		t.Fatalf("failed to seed VM directory: %v", err)
	}
	seedVersionMarker(t, runtime, recordedVersion)
}

func assertRunnerEvents(t *testing.T, eventsPath string, expected ...string) {
	t.Helper()

	content, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) && len(expected) == 0 {
			return
		}
		t.Fatalf("failed to read runner events: %v", err)
	}
	actual := strings.Fields(string(content))
	if !slices.Equal(actual, expected) {
		t.Fatalf("runner events = %v, want %v", actual, expected)
	}
}

func TestMacVMRuntimePrepareRejectsConflictingHostDataPath(t *testing.T) {
	t.Parallel()

	tests := map[string]func(t *testing.T, path string){
		"directory": func(t *testing.T, path string) {
			t.Helper()
			if err := os.Mkdir(path, dirMode); err != nil {
				t.Fatalf("failed to create conflicting directory: %v", err)
			}
		},
		"file": func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte("data"), markerFileMode); err != nil {
				t.Fatalf("failed to create conflicting file: %v", err)
			}
		},
		"different symlink": func(t *testing.T, path string) {
			t.Helper()
			if err := os.Symlink("somewhere-else", path); err != nil {
				t.Fatalf("failed to create conflicting symlink: %v", err)
			}
		},
	}
	for name, createConflict := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runtime := NewMacVMRuntime(config.NewDeploymentDir(t.TempDir()), nil)
			if err := os.MkdirAll(runtime.paths.WorkDir, dirMode); err != nil {
				t.Fatalf("failed to create runtime directory: %v", err)
			}
			createConflict(t, runtime.paths.HostNanoDataDir)

			err := runtime.Prepare(context.Background(), nil, nil, PrepareOptions{})

			if err == nil || !strings.Contains(err.Error(), runtime.paths.HostNanoDataDir) {
				t.Fatalf("expected path-rich conflict error, got %v", err)
			}
		})
	}
}

func TestMaterializeFileAtomicallyStagesAndReusesUnchangedArtifact(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.tar")
	targetPath := filepath.Join(root, "share", "nano.tar")
	if err := os.WriteFile(sourcePath, []byte("image"), 0o600); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	modTime := time.Unix(1_700_000_000, 123)
	if err := os.Chtimes(sourcePath, modTime, modTime); err != nil {
		t.Fatalf("failed to set source time: %v", err)
	}
	if err := materializeFileAtomically(sourcePath, targetPath); err != nil {
		t.Fatalf("failed to stage artifact: %v", err)
	}
	firstInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat target: %v", err)
	}
	if firstInfo.Mode().Perm() != 0o640 || !firstInfo.ModTime().Equal(modTime) {
		t.Fatalf(
			"unexpected staged metadata: mode=%o time=%s",
			firstInfo.Mode().Perm(),
			firstInfo.ModTime(),
		)
	}
	if err := materializeFileAtomically(sourcePath, targetPath); err != nil {
		t.Fatalf("failed to reuse staged artifact: %v", err)
	}
	secondInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat reused target: %v", err)
	}
	if !os.SameFile(firstInfo, secondInfo) {
		t.Fatal("expected unchanged staged artifact not to be replaced")
	}
}

func TestMaterializeFileAtomicallyRepairsWrongModeWithoutContentChange(t *testing.T) {
	t.Parallel()

	// Given
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.tar")
	targetPath := filepath.Join(root, "share", "nano.tar")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}
	content := []byte("image")
	if err := os.WriteFile(sourcePath, content, 0o600); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	if err := os.WriteFile(targetPath, content, 0o600); err != nil {
		t.Fatalf("failed to write target: %v", err)
	}
	modTime := time.Unix(1_700_000_000, 123)
	if err := os.Chtimes(sourcePath, modTime, modTime); err != nil {
		t.Fatalf("failed to set source time: %v", err)
	}
	if err := os.Chtimes(targetPath, modTime, modTime); err != nil {
		t.Fatalf("failed to set target time: %v", err)
	}

	// When
	if err := materializeFileAtomically(sourcePath, targetPath); err != nil {
		t.Fatalf("failed to repair staged artifact: %v", err)
	}

	// Then
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat repaired target: %v", err)
	}
	if targetInfo.Mode().Perm() != artifactFileMode {
		t.Fatalf(
			"expected repaired mode %o, got %o",
			artifactFileMode,
			targetInfo.Mode().Perm(),
		)
	}
	actualContent, err := os.ReadFile(targetPath)
	if err != nil || string(actualContent) != string(content) {
		t.Fatalf("repaired content = %q, err=%v; want %q", actualContent, err, content)
	}
}

func TestMaterializeFileAtomicallyRejectsDirectorySource(t *testing.T) {
	t.Parallel()

	err := materializeFileAtomically(t.TempDir(), filepath.Join(t.TempDir(), "target"))
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected directory source error, got %v", err)
	}
}

func TestMacVMRuntimeWorkaroundNanoStartupDurabilityFinalizesMigration(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := NewMacVMRuntime(
		deployment, newTestManagerForRunner(t, []byte("#!/bin/sh\n")),
	)
	migration := &recordingMacDataMigrator{}
	localRuntime.migrationFactory = func(localinstall.ExecutionEnvironment) macDataMigrator {
		return migration
	}
	if err := os.MkdirAll(localRuntime.paths.WorkDir, dirMode); err != nil {
		t.Fatalf("failed to create runtime work dir: %v", err)
	}

	if err := localRuntime.WorkaroundNanoStartupDurability(
		context.Background(), nil, nil,
	); err != nil {
		t.Fatalf("expected migration finalization to succeed, got %v", err)
	}
	if migration.finalizeCalls != 1 {
		t.Fatalf("migration finalize calls = %d, want 1", migration.finalizeCalls)
	}
}

func TestValidatePort(t *testing.T) {
	t.Parallel()

	if err := validatePort("database", 8563); err != nil {
		t.Fatalf("expected valid port: %v", err)
	}
	if err := validatePort("database", 0); err == nil {
		t.Fatalf("expected invalid port error, got %v", err)
	}
}
