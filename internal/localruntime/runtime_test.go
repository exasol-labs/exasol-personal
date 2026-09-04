// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localports"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	windowsGOOS        = "windows"
	runnerZipEntryName = "launcher"
)

func newTestManagerForRunner(t *testing.T, scriptContent []byte) *runtimeartifacts.Manager {
	t.Helper()

	zipPath := filepath.Join(t.TempDir(), "runner.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create runner zip: %v", err)
	}
	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: runnerZipEntryName, Method: zip.Deflate}
	header.SetMode(0o755)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("failed to create runner entry: %v", err)
	}
	if _, err := entry.Write(scriptContent); err != nil {
		t.Fatalf("failed to write runner entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close runner zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed to close runner fixture: %v", err)
	}

	spec := runtimeartifacts.ResourceSpec{
		exasolLocalRunnerResourceID: {
			Extract: true,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"any": {URL: zipPath, ResourcePath: runnerZipEntryName},
			},
		},
	}

	return runtimeartifacts.NewResourceManagerForPlatform(
		spec, t.TempDir(), runtime.GOOS, runtime.GOARCH,
	)
}

func writeExecutableTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, executableFileMode); err != nil {
		t.Fatalf("failed to write executable fixture: %v", err)
	}
}

//nolint:paralleltest // test runner scripts share process environment and fork executable files.
func TestMacVMRuntimeLifecycleUsesV2RunnerThenSharedInstall(t *testing.T) {
	requirePOSIXRunnerTest(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	eventsPath := filepath.Join(t.TempDir(), "events")
	runnerScript := fakeV2RunnerScript(eventsPath)
	localRuntime := NewMacVMRuntime(deployment, newTestManagerForRunner(t, []byte(runnerScript)))
	install := &recordingLocalInstall{eventsPath: eventsPath, running: true}
	localRuntime.installFactory = func(string) (localinstall.LocalInstall, error) {
		return install, nil
	}
	localRuntime.migrationFactory = func(localinstall.ExecutionEnvironment) macDataMigrator {
		return &recordingMacDataMigrator{eventsPath: eventsPath}
	}

	if err := localRuntime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "db:28563"},
		CPUCount:      4,
		MemoryMB:      8192,
		DataSizeGB:    100,
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if install.startConfig.ContainerDBPort != nanoDBPort ||
		install.startConfig.DataDir != vmSharedNanoDataDir {
		t.Fatalf("unexpected guest Podman config: %#v", install.startConfig)
	}
	if install.startConfig.ContainerDBBindHost != "" {
		t.Fatalf("expected VM-accessible DB bind, got %#v", install.startConfig)
	}
	if len(install.startConfig.LegacyContainerNames) != 1 ||
		install.startConfig.LegacyContainerNames[0] != legacyNanoContainer {
		t.Fatalf("expected legacy container migration config, got %#v", install.startConfig)
	}
	endpoint, err := localRuntime.ReadEndpoints()
	if err != nil || endpoint.DBPort != 28563 {
		t.Fatalf("unexpected endpoint %#v, err=%v", endpoint, err)
	}
	status, err := localRuntime.Status(context.Background())
	if err != nil || !status.Running {
		t.Fatalf("expected combined running status, got %#v err=%v", status, err)
	}
	if err := localRuntime.Stop(context.Background(), nil, nil); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "db:28563"},
		CPUCount:      4,
		MemoryMB:      8192,
		DataSizeGB:    100,
	}); err != nil {
		t.Fatalf("restart failed: %v", err)
	}
	restartedEndpoint, err := localRuntime.ReadEndpoints()
	if err != nil || restartedEndpoint.DBPort != 28563 {
		t.Fatalf("unexpected restarted endpoint %#v, err=%v", restartedEndpoint, err)
	}

	events, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("failed to read lifecycle events: %v", err)
	}
	want := []string{
		"runner-init", "runner-start", "migration-prepare", "install-start",
		"install-stop", "runner-stop",
	}
	for _, event := range want {
		if !strings.Contains(string(events), event+"\n") {
			t.Fatalf("missing %q in lifecycle events:\n%s", event, events)
		}
	}
	if strings.Index(string(events), "install-stop") >
		strings.Index(string(events), "runner-stop") {
		t.Fatalf("expected container cleanup before VM stop:\n%s", events)
	}

	args, err := os.ReadFile(filepath.Join(localRuntime.paths.WorkDir, "start-args"))
	if err != nil {
		t.Fatalf("failed to read start args: %v", err)
	}
	if string(args) != "--forward db:8563:28563 4 8192 100" {
		t.Fatalf("unexpected v2 runner start args %q", args)
	}
}

//nolint:paralleltest // test runner scripts fork executable files.
func TestMacVMRuntimeStartStopsVMWhenInstallFails(t *testing.T) {
	requirePOSIXRunnerTest(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	eventsPath := filepath.Join(t.TempDir(), "events")
	localRuntime := NewMacVMRuntime(
		deployment,
		newTestManagerForRunner(t, []byte(fakeV2RunnerScript(eventsPath))),
	)
	localRuntime.installFactory = func(string) (localinstall.LocalInstall, error) {
		return &recordingLocalInstall{
			eventsPath: eventsPath,
			startErr:   errors.New("podman failed"),
		}, nil
	}
	localRuntime.migrationFactory = func(localinstall.ExecutionEnvironment) macDataMigrator {
		return &recordingMacDataMigrator{eventsPath: eventsPath}
	}
	if err := localRuntime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "db:28563"},
		CPUCount:      2,
		MemoryMB:      4096,
		DataSizeGB:    100,
	})
	if err == nil || !strings.Contains(err.Error(), "podman failed") {
		t.Fatalf("expected install failure, got %v", err)
	}
	events, readErr := os.ReadFile(eventsPath)
	if readErr != nil {
		t.Fatalf("failed to read lifecycle events: %v", readErr)
	}
	if !strings.Contains(string(events), "install-start\nrunner-stop\n") {
		t.Fatalf("expected failed installation to stop VM, got:\n%s", events)
	}
}

//nolint:paralleltest // test runner scripts fork executable files.
func TestMacVMRuntimeStartStopsVMWhenMigrationFails(t *testing.T) {
	requirePOSIXRunnerTest(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	eventsPath := filepath.Join(t.TempDir(), "events")
	localRuntime := NewMacVMRuntime(
		deployment,
		newTestManagerForRunner(t, []byte(fakeV2RunnerScript(eventsPath))),
	)
	install := &recordingLocalInstall{eventsPath: eventsPath}
	localRuntime.installFactory = func(string) (localinstall.LocalInstall, error) {
		return install, nil
	}
	localRuntime.migrationFactory = func(localinstall.ExecutionEnvironment) macDataMigrator {
		return &recordingMacDataMigrator{
			eventsPath: eventsPath,
			prepareErr: errors.New("migration failed"),
		}
	}
	if err := localRuntime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "db:28563"},
		CPUCount:      2,
		MemoryMB:      4096,
		DataSizeGB:    100,
	})
	if err == nil || !strings.Contains(err.Error(), "migration failed") {
		t.Fatalf("expected migration failure, got %v", err)
	}
	events, readErr := os.ReadFile(eventsPath)
	if readErr != nil {
		t.Fatalf("failed to read lifecycle events: %v", readErr)
	}
	if !strings.Contains(string(events), "migration-prepare\nrunner-stop\n") {
		t.Fatalf("expected failed migration to stop VM, got:\n%s", events)
	}
	if strings.Contains(string(events), "install-start") {
		t.Fatalf("Nano started after failed migration:\n%s", events)
	}
}

//nolint:paralleltest // test runner scripts fork executable files.
func TestMacVMRuntimeStartRejectsMismatchedReportedPort(t *testing.T) {
	requirePOSIXRunnerTest(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	eventsPath := filepath.Join(t.TempDir(), "events")
	localRuntime := NewMacVMRuntime(
		deployment,
		newTestManagerForRunner(t, []byte(fakeV2RunnerScriptWithPort(eventsPath, 38563))),
	)
	install := &recordingLocalInstall{eventsPath: eventsPath}
	localRuntime.installFactory = func(string) (localinstall.LocalInstall, error) {
		return install, nil
	}
	if err := localRuntime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "db:28563"},
		CPUCount:      2,
		MemoryMB:      4096,
		DataSizeGB:    100,
	})

	if err == nil || !strings.Contains(err.Error(), "expected configured port 28563") {
		t.Fatalf("expected mismatched endpoint error, got %v", err)
	}
	events, readErr := os.ReadFile(eventsPath)
	if readErr != nil {
		t.Fatalf("failed to read lifecycle events: %v", readErr)
	}
	if strings.Contains(string(events), "install-start") ||
		!strings.Contains(string(events), "runner-start\nrunner-stop\n") {
		t.Fatalf("expected mismatched VM to stop before installation, got:\n%s", events)
	}
}

//nolint:paralleltest // test runner scripts fork executable files.
func TestMacVMRuntimeStartClassifiesBindDiagnostic(t *testing.T) {
	requirePOSIXRunnerTest(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	eventsPath := filepath.Join(t.TempDir(), "events")
	localRuntime := NewMacVMRuntime(
		deployment,
		newTestManagerForRunner(
			t, []byte(failingV2RunnerScript(eventsPath, "address already in use", false, false)),
		),
	)
	if err := localRuntime.Prepare(
		context.Background(), nil, nil, PrepareOptions{},
	); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "db:28563"},
		CPUCount:      2,
		MemoryMB:      4096,
		DataSizeGB:    100,
	})

	var unavailable *localports.UnavailableError
	if !errors.As(err, &unavailable) || unavailable.Port != 28563 {
		t.Fatalf("expected unavailable database port, got %v", err)
	}
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Fatalf("expected runner command error in chain, got %v", err)
	}
	events, readErr := os.ReadFile(eventsPath)
	if readErr != nil || !strings.Contains(string(events), "runner-stop\n") {
		t.Fatalf("expected failed VM cleanup, got %q, %v", events, readErr)
	}
}

//nolint:paralleltest // test runner scripts fork executable files.
func TestMacVMRuntimeStartKeepsUnrelatedAndPartialFailuresGeneric(t *testing.T) {
	requirePOSIXRunnerTest(t)

	for _, test := range []struct {
		name       string
		diagnostic string
		partial    bool
	}{
		{name: "unrelated", diagnostic: "runner image failure"},
		{name: "partial endpoint", diagnostic: "address already in use", partial: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			deployment := config.NewDeploymentDir(t.TempDir())
			eventsPath := filepath.Join(t.TempDir(), "events")
			localRuntime := NewMacVMRuntime(
				deployment,
				newTestManagerForRunner(
					t,
					[]byte(failingV2RunnerScript(
						eventsPath, test.diagnostic, test.partial, false,
					)),
				),
			)
			if err := localRuntime.Prepare(
				context.Background(), nil, nil, PrepareOptions{},
			); err != nil {
				t.Fatalf("prepare failed: %v", err)
			}

			err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
				RuntimeConfig: RuntimeConfig{Ports: "db:28563"},
				CPUCount:      2,
				MemoryMB:      4096,
				DataSizeGB:    100,
			})

			var unavailable *localports.UnavailableError
			if err == nil || errors.As(err, &unavailable) {
				t.Fatalf("expected generic runner failure, got %v", err)
			}
			events, readErr := os.ReadFile(eventsPath)
			if readErr != nil {
				t.Fatalf("failed to read events: %v", readErr)
			}
			if strings.Contains(string(events), "runner-stop\n") {
				t.Fatalf("generic or partial failure must not trigger bind cleanup: %s", events)
			}
		})
	}
}

//nolint:paralleltest // test runner scripts fork executable files.
func TestMacVMRuntimeStartCleanupFailureIsNotRecoverable(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	eventsPath := filepath.Join(t.TempDir(), "events")
	localRuntime := NewMacVMRuntime(
		deployment,
		newTestManagerForRunner(
			t, []byte(failingV2RunnerScript(eventsPath, "address already in use", false, true)),
		),
	)
	if err := localRuntime.Prepare(
		context.Background(), nil, nil, PrepareOptions{},
	); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	// When
	err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "db:28563"},
		CPUCount:      2,
		MemoryMB:      4096,
		DataSizeGB:    100,
	})

	// Then
	if err == nil || !strings.Contains(err.Error(), "failed to stop VM after startup failure") {
		t.Fatalf("expected startup and cleanup failure, got %v", err)
	}
	if _, unavailable := localports.AsUnavailable(err); unavailable {
		t.Fatalf("cleanup failure must not be recoverable as a port conflict: %v", err)
	}
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Fatalf("expected runner command error in chain, got %v", err)
	}
}

//nolint:paralleltest // test runner scripts fork executable fixtures.
func TestMacVMRuntimeOpenHostShellDelegatesToRunner(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	observationsDir := t.TempDir()
	argsPath := filepath.Join(observationsDir, "host-args")
	workingDirPath := filepath.Join(observationsDir, "host-working-dir")
	stdinPath := filepath.Join(observationsDir, "host-stdin")
	runnerScript := fmt.Sprintf(`#!/bin/sh
set -eu
: > %q
for arg do printf '%%s\n' "$arg" >> %q; done
printf '%%s' "$PWD" > %q
cat > %q
printf 'host-stdout'
printf 'host-stderr' >&2
`, argsPath, argsPath, workingDirPath, stdinPath)
	localRuntime := NewMacVMRuntime(
		deployment, newTestManagerForRunner(t, []byte(runnerScript)),
	)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, dirMode); err != nil {
		t.Fatalf("failed to create runtime work dir: %v", err)
	}
	var stdout, stderr bytes.Buffer

	// When
	if err := localRuntime.OpenHostShell(
		context.Background(), strings.NewReader("host-input\n"), &stdout, &stderr,
	); err != nil {
		t.Fatalf("host shell failed: %v", err)
	}

	// Then
	args, err := os.ReadFile(argsPath)
	if err != nil || string(args) != "run\n" {
		t.Fatalf("host shell args = %q, err=%v; want %q", args, err, "run\n")
	}
	workingDir, err := os.ReadFile(workingDirPath)
	if err != nil || string(workingDir) != localRuntime.paths.WorkDir {
		t.Fatalf(
			"host shell working directory = %q, err=%v; want %q",
			workingDir,
			err,
			localRuntime.paths.WorkDir,
		)
	}
	stdin, err := os.ReadFile(stdinPath)
	if err != nil || string(stdin) != "host-input\n" {
		t.Fatalf("host shell stdin = %q, err=%v; want %q", stdin, err, "host-input\n")
	}
	if stdout.String() != "host-stdout" || stderr.String() != "host-stderr" {
		t.Fatalf(
			"unexpected host shell streams stdout=%q stderr=%q",
			stdout.String(),
			stderr.String(),
		)
	}
}

//nolint:paralleltest // test runner scripts fork executable fixtures.
func TestMacVMRuntimeOpenContainerShellDelegatesToRunner(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{DeploymentId: "shell-test"}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateRunning{}, deployment,
	); err != nil {
		t.Fatalf("failed to write deployment state: %v", err)
	}
	observationsDir := t.TempDir()
	argsPath := filepath.Join(observationsDir, "container-args")
	workingDirPath := filepath.Join(observationsDir, "container-working-dir")
	stdinPath := filepath.Join(observationsDir, "container-stdin")
	runnerScript := fmt.Sprintf(`#!/bin/sh
set -eu
: > %q
for arg do printf '%%s\n' "$arg" >> %q; done
printf '%%s' "$PWD" > %q
cat > %q
printf 'container-stdout'
printf 'container-stderr' >&2
`, argsPath, argsPath, workingDirPath, stdinPath)
	localRuntime := NewMacVMRuntime(
		deployment, newTestManagerForRunner(t, []byte(runnerScript)),
	)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, dirMode); err != nil {
		t.Fatalf("failed to create runtime work dir: %v", err)
	}
	var stdout, stderr bytes.Buffer

	// When
	if err := localRuntime.OpenContainerShell(
		context.Background(), strings.NewReader("container-input\n"), &stdout, &stderr,
	); err != nil {
		t.Fatalf("container shell failed: %v", err)
	}

	// Then
	args, err := os.ReadFile(argsPath)
	wantArgs := strings.Join([]string{
		"run",
		"--tty",
		"--",
		"sh",
		"-c",
		containerShellScript,
		"sh",
		"exasol-db-shell-test",
		"",
	}, "\n")
	if err != nil || string(args) != wantArgs {
		t.Fatalf("container shell args = %q, err=%v; want %q", args, err, wantArgs)
	}
	workingDir, err := os.ReadFile(workingDirPath)
	if err != nil || string(workingDir) != localRuntime.paths.WorkDir {
		t.Fatalf(
			"container shell working directory = %q, err=%v; want %q",
			workingDir,
			err,
			localRuntime.paths.WorkDir,
		)
	}
	stdin, err := os.ReadFile(stdinPath)
	if err != nil || string(stdin) != "container-input\n" {
		t.Fatalf(
			"container shell stdin = %q, err=%v; want %q",
			stdin,
			err,
			"container-input\n",
		)
	}
	if stdout.String() != "container-stdout" || stderr.String() != "container-stderr" {
		t.Fatalf(
			"unexpected container shell streams stdout=%q stderr=%q",
			stdout.String(),
			stderr.String(),
		)
	}
}

//nolint:paralleltest // test runner scripts fork executable fixtures.
func TestMacVMRuntimeOpenHostShellPreservesRunnerFailure(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	runnerScript := []byte("#!/bin/sh\nexit 23\n")
	localRuntime := NewMacVMRuntime(deployment, newTestManagerForRunner(t, runnerScript))
	if err := os.MkdirAll(localRuntime.paths.WorkDir, dirMode); err != nil {
		t.Fatalf("failed to create runtime work dir: %v", err)
	}

	// When
	err := localRuntime.OpenHostShell(
		context.Background(), strings.NewReader(""), io.Discard, io.Discard,
	)

	// Then
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected underlying command failure, got %v", err)
	}
	if exitErr.ExitCode() != 23 {
		t.Fatalf("expected runner exit code 23, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(err.Error(), `local runner command "run" failed`) {
		t.Fatalf("expected contextual runner error, got %v", err)
	}
}

func fakeV2RunnerScript(eventsPath string) string {
	return fakeV2RunnerScriptWithVersionAndPort(eventsPath, "2.0.0-dev", 28563)
}

func fakeV2RunnerScriptWithPort(eventsPath string, hostDBPort int) string {
	return fakeV2RunnerScriptWithVersionAndPort(eventsPath, "2.0.0-dev", hostDBPort)
}

func fakeV2RunnerScriptWithVersion(eventsPath, version string) string {
	return fakeV2RunnerScriptWithVersionAndPort(eventsPath, version, 28563)
}

func fakeV2RunnerScriptWithVersionAndPort(eventsPath, version string, hostDBPort int) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
events=%q
case "$1" in
  version)
    printf 'v%s\n'
    ;;
  init)
    mkdir -p vm vm-shared
    printf 'runner-init\n' >> "$events"
    ;;
  start)
    shift
    printf '%%s' "$*" > start-args
    touch running
    printf 'runner-start\n' >> "$events"
    cat > vm-state.json <<'EOF'
{
  "vm_name":"test-vm",
  "shared_dir":"./vm-shared",
  "forwards":{"db":{"guest_port":8563,"host_port":%d}}
}
EOF
    ;;
  status)
    if [ -f running ]; then printf '{"running":true}\n'; else printf '{"running":false}\n'; fi
    ;;
  stop)
    rm -f running vm-state.json
    printf 'runner-stop\n' >> "$events"
    ;;
  health-check)
    printf '{"ports":{"db":{"state":"reachable"}}}\n'
    ;;
  *)
    printf 'unexpected command: %%s\n' "$1" >&2
    exit 2
    ;;
esac
`, eventsPath, version, hostDBPort)
}

func failingV2RunnerScript(
	eventsPath, diagnostic string,
	partial, stopFails bool,
) string {
	partialState := ""
	if partial {
		partialState = `cat > vm-state.json <<'EOF'
{"forwards":{"db":{"guest_port":8563,"host_port":28563}}}
EOF`
	}
	stopAction := `rm -f vm-state.json; printf 'runner-stop\n' >> "$events"`
	if stopFails {
		stopAction = `printf 'runner-stop\n' >> "$events"
    printf 'cleanup failed\n' >&2
    exit 43`
	}

	return fmt.Sprintf(`#!/bin/sh
set -eu
events=%q
case "$1" in
  version) printf 'v2.0.0-dev\n' ;;
  init) mkdir -p vm vm-shared; printf 'runner-init\n' >> "$events" ;;
  start)
    printf 'runner-start\n' >> "$events"
    %s
    printf '%%s\n' %q >&2
    exit 42
    ;;
  stop)
    %s
    ;;
  *) exit 2 ;;
esac
`, eventsPath, partialState, diagnostic, stopAction)
}

type recordingLocalInstall struct {
	eventsPath  string
	startConfig localinstall.StartConfig
	startErr    error
	running     bool
}

type recordingMacDataMigrator struct {
	eventsPath    string
	prepareErr    error
	finalizeErr   error
	finalizeCalls int
}

func (migration *recordingMacDataMigrator) prepare(
	context.Context,
	io.Writer,
	io.Writer,
	func() error,
) error {
	recordTestEvent(migration.eventsPath, "migration-prepare")

	return migration.prepareErr
}

func (migration *recordingMacDataMigrator) finalize(
	context.Context,
	io.Writer,
	io.Writer,
) error {
	migration.finalizeCalls++
	recordTestEvent(migration.eventsPath, "migration-finalize")

	return migration.finalizeErr
}

func (install *recordingLocalInstall) Start(
	_ context.Context,
	_, _ io.Writer,
	startConfig localinstall.StartConfig,
) error {
	install.startConfig = startConfig
	install.record("install-start")
	if install.startErr == nil {
		install.running = true
	}

	return install.startErr
}

func (install *recordingLocalInstall) Stop(context.Context, io.Writer, io.Writer) error {
	install.record("install-stop")
	install.running = false

	return nil
}

func (install *recordingLocalInstall) Status(
	context.Context,
	io.Writer,
	io.Writer,
) (*localinstall.InstallStatus, error) {
	return &localinstall.InstallStatus{Running: install.running}, nil
}

func (install *recordingLocalInstall) Destroy(ctx context.Context, out, outErr io.Writer) error {
	return install.Stop(ctx, out, outErr)
}

func (install *recordingLocalInstall) record(event string) {
	recordTestEvent(install.eventsPath, event)
}

func recordTestEvent(eventsPath, event string) {
	file, err := os.OpenFile(
		eventsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600,
	)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = fmt.Fprintln(file, event)
}
