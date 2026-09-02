// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

func TestLinuxHostPodmanStartConfig_RequiresConcreteDatabasePort(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := NewHostLinuxRuntime(deployment, nil)

	// When
	_, err := localRuntime.podmanStartConfig(RuntimeConfig{})
	// Then
	if err == nil || !strings.Contains(err.Error(), "positive concrete port") {
		t.Fatalf("expected concrete-port requirement, got %v", err)
	}
}

func TestLinuxHostPodmanStartConfig_UsesOnlyCommonRuntimeConfig(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	vmConfig := VMConfig{
		RuntimeConfig: RuntimeConfig{
			Ports: "ssh:20022, db:28563, ui:28443",
			VersionCheck: localinstall.VersionCheckConfig{
				Enabled:  true,
				URL:      "https://example.test",
				Identity: "cluster-id",
			},
			SLCs: []localinstall.SLCConfig{},
		},
		CPUCount:   32,
		MemoryMB:   131072,
		DataSizeGB: 4096,
	}

	// When
	startConfig, err := localRuntime.podmanStartConfig(vmConfig.RuntimeConfig)
	// Then
	if err != nil {
		t.Fatalf("expected host port override, got %v", err)
	}
	if startConfig.ContainerDBPort != 28563 {
		t.Fatalf("expected published DB port override, got %#v", startConfig)
	}
	if startConfig.ContainerDBBindHost != hostLoopbackHost {
		t.Fatalf("expected loopback DB bind, got %#v", startConfig)
	}
	if startConfig.DataDir != filepath.Join(localRuntime.paths.WorkDir, nanoDataDirName) {
		t.Fatalf("unexpected Nano data directory %q", startConfig.DataDir)
	}
	if len(startConfig.InitParams) != 1 ||
		startConfig.InitParams[0] != "maxConnectionsLicenseLimit=20" {
		t.Fatalf("unexpected Nano init params: %#v", startConfig.InitParams)
	}
	if !reflect.DeepEqual(startConfig.VersionCheck, vmConfig.VersionCheck) {
		t.Fatalf("expected portable version-check settings, got %#v", startConfig.VersionCheck)
	}
	if startConfig.SLCs == nil || len(startConfig.SLCs) != 0 {
		t.Fatalf("expected authoritative empty SLC set, got %#v", startConfig.SLCs)
	}
}

func TestLinuxHostReadEndpoint_ReturnsPublishedDatabasePort(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	localRuntime.endpoint = &RuntimeEndpoint{DBPort: 28563}

	// When
	endpoint, err := localRuntime.ReadEndpoints()
	// Then
	if err != nil {
		t.Fatalf("expected endpoint, got %v", err)
	}
	if endpoint.DBPort != 28563 {
		t.Fatalf("expected published DB port 28563, got %#v", endpoint)
	}
	if endpoint.ShellSupported {
		t.Fatalf("expected Linux host shell to be unsupported, got %#v", endpoint)
	}
}

func TestLinuxHostReadEndpoint_RejectsReadBeforeStart(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)

	// When
	_, err := localRuntime.ReadEndpoints()
	// Then
	if err == nil || !strings.Contains(err.Error(), "endpoint is unavailable") {
		t.Fatalf("expected endpoint availability error, got %v", err)
	}
}

func TestLinuxHostShellErrorsPreserveUnsupportedIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		openShell func(*HostRuntime) error
		sentinel  error
	}{
		{
			name: "host shell",
			openShell: func(runtime *HostRuntime) error {
				return runtime.OpenHostShell(context.Background(), nil, nil, nil)
			},
			sentinel: ErrHostShellUnsupported,
		},
		{
			name: "container shell",
			openShell: func(runtime *HostRuntime) error {
				return runtime.OpenContainerShell(context.Background(), nil, nil, nil)
			},
			sentinel: ErrContainerShellUnsupported,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			runtime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)

			// When
			err := test.openShell(runtime)

			// Then
			if !errors.Is(err, test.sentinel) {
				t.Fatalf("expected unsupported shell identity, got %v", err)
			}
			if !strings.Contains(err.Error(), "linux host runtime") {
				t.Fatalf("expected Linux host-runtime context, got %v", err)
			}
		})
	}
}

func TestLinuxHostWorkaroundNanoStartupDurabilityDelegatesToExecutionEnvironment(
	t *testing.T,
) {
	t.Parallel()
	if os.PathSeparator == '\\' {
		t.Skip("fake sync executable is a POSIX shell script")
	}

	root := t.TempDir()
	logPath := filepath.Join(root, "sync-args")
	scriptPath := filepath.Join(root, "fake-sync.sh")
	writeLinuxRuntimeTestFile(t, scriptPath, `#!/bin/sh
printf '%s' "$*" > "$1"
printf 'sync-out'
printf 'sync-err' >&2
`)
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	localRuntime.runtimeExec = []string{"/bin/sh", scriptPath, logPath}
	var stdout, stderr bytes.Buffer

	err := localRuntime.WorkaroundNanoStartupDurability(
		context.Background(), &stdout, &stderr,
	)
	if err != nil {
		t.Fatalf("expected host sync to succeed, got %v", err)
	}
	args, readErr := os.ReadFile(logPath)
	if readErr != nil || string(args) != logPath+" sync" {
		t.Fatalf("expected host sync command, got %q, %v", string(args), readErr)
	}
	if stdout.String() != "sync-out" || stderr.String() != "sync-err" {
		t.Fatalf("unexpected sync output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestLinuxHostStatus_MapsExactPodmanContainerStatus(t *testing.T) {
	t.Parallel()
	if os.PathSeparator == '\\' {
		t.Skip("fake Podman executable is a POSIX shell script")
	}

	tests := []struct {
		name            string
		containerExists bool
		running         bool
	}{
		{name: "missing"},
		{name: "stopped", containerExists: true},
		{name: "running", containerExists: true, running: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			deployment := newLinuxRuntimeStatusDeployment(t)
			scenarioDir := t.TempDir()
			if test.containerExists {
				writeLinuxRuntimeTestFile(t, filepath.Join(scenarioDir, "existing"), "present")
			}
			if test.running {
				writeLinuxRuntimeTestFile(t, filepath.Join(scenarioDir, "running"), "present")
			}
			scriptPath := filepath.Join(t.TempDir(), "fake-podman.sh")
			writeLinuxRuntimeTestFile(t, scriptPath, fakeLinuxRuntimePodmanScript)
			localRuntime := NewHostLinuxRuntime(deployment, nil)
			localRuntime.runtimeExec = []string{"/bin/sh", scriptPath, scenarioDir}

			// When
			status, err := localRuntime.Status(context.Background())
			// Then
			if err != nil {
				t.Fatalf("expected Linux status to succeed: %v", err)
			}
			if status.Running != test.running {
				t.Fatalf("expected running=%t, got %#v", test.running, status)
			}
		})
	}
}

func TestLinuxHostReadEndpoint_RecoversPublishedPortFromDeploymentInfo(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeLinuxDeploymentInfo(t, deployment, 28563)
	localRuntime := NewHostLinuxRuntime(deployment, nil)

	// When
	endpoint, err := localRuntime.ReadEndpoints()
	// Then
	if err != nil {
		t.Fatalf("expected deployment endpoint recovery: %v", err)
	}
	if endpoint.DBPort != 28563 {
		t.Fatalf("expected recovered DB port 28563, got %#v", endpoint)
	}
}

func TestLinuxHostHealthCheck_ProbesRecoveredPublishedPort(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeLinuxDeploymentInfo(t, deployment, 28563)
	localRuntime := NewHostLinuxRuntime(deployment, nil)
	var dialedNetwork, dialedAddress string
	connection, peer := net.Pipe()
	t.Cleanup(func() {
		_ = connection.Close()
		_ = peer.Close()
	})
	localRuntime.dialContext = func(_ context.Context, network, address string) (net.Conn, error) {
		dialedNetwork, dialedAddress = network, address
		return connection, nil
	}

	// When
	health, err := localRuntime.HealthCheck(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected Linux health check: %v", err)
	}
	if health.Ports["db"].State != PortStateReachable {
		t.Fatalf("expected reachable published DB port, got %#v", health)
	}
	if dialedNetwork != "tcp" || dialedAddress != "127.0.0.1:28563" {
		t.Fatalf("expected published loopback probe, got %s %s", dialedNetwork, dialedAddress)
	}
}

// The Winsock cases are asserted with the numeric codes Windows actually
// reports rather than syscall's POSIX-named errnos, whose Windows values are
// placeholders no dial ever returns.
func TestClassifyHostPortHealth_ClassifiesDialOutcomes(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		dialError error
		expected  PortState
	}{
		"connection refused": {
			dialError: syscall.ECONNREFUSED,
			expected:  PortStateRefused,
		},
		"connection reset": {
			dialError: syscall.ECONNRESET,
			expected:  PortStateRefused,
		},
		"winsock connection refused": {
			dialError: syscall.Errno(10061),
			expected:  PortStateRefused,
		},
		"winsock connection reset": {
			dialError: syscall.Errno(10054),
			expected:  PortStateRefused,
		},
		"winsock connection aborted": {
			dialError: syscall.Errno(10053),
			expected:  PortStateRefused,
		},
		"winsock not connected": {
			dialError: syscall.Errno(10057),
			expected:  PortStateRefused,
		},
		"deadline exceeded": {
			dialError: context.DeadlineExceeded,
			expected:  PortStateTimeout,
		},
		"unrecognised failure": {
			dialError: errors.New("no route to host"),
			expected:  PortStateBlocked,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given
			dialError := &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: os.NewSyscallError("connect", testCase.dialError),
			}

			// When
			state := classifyHostPortHealth(dialError)

			// Then
			if state != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, state)
			}
		})
	}
}

func TestClassifyHostPortHealth_ReachableWithoutError(t *testing.T) {
	t.Parallel()

	// When
	state := classifyHostPortHealth(nil)

	// Then
	if state != PortStateReachable {
		t.Fatalf("expected a successful dial to be reachable, got %q", state)
	}
}

func newLinuxRuntimeStatusDeployment(t *testing.T) config.DeploymentDir {
	t.Helper()
	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{DeploymentId: "linux-runtime-status"}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{}, deployment,
	); err != nil {
		t.Fatalf("failed to write deployment state: %v", err)
	}

	return deployment
}

func writeLinuxDeploymentInfo(t *testing.T, deployment config.DeploymentDir, dbPort int) {
	t.Helper()
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		DeploymentId: "linux-runtime-status",
		Connection:   &config.DeploymentConnection{Host: "127.0.0.1", DBPort: dbPort},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}
}

func writeLinuxRuntimeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}

const fakeLinuxRuntimePodmanScript = `#!/bin/sh
set -eu
scenario_dir=$1
shift
if [ "$1" != "podman" ] || [ "$2" != "container" ]; then
  exit 90
fi
case "$3" in
  exists)
    if [ ! -f "$scenario_dir/existing" ] && [ ! -f "$scenario_dir/running" ]; then
      exit 1
    fi
    ;;
  inspect)
    if [ -f "$scenario_dir/running" ]; then
      printf 'true\n'
    else
      printf 'false\n'
    fi
    ;;
  *) exit 91 ;;
esac
`

func TestResolveLinuxHostDBPort_RejectsInvalidMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ports string
	}{
		{name: "missing separator", ports: "db"},
		{name: "missing service", ports: ":8563"},
		{name: "missing port", ports: "db:"},
		{name: "non numeric", ports: "db:abc"},
		{name: "automatic", ports: "auto"},
		{name: "empty", ports: ""},
		{name: "zero", ports: "db:0"},
		{name: "too large", ports: "db:65536"},
		{name: "duplicate DB", ports: "db:8563,db:28563"},
		{name: "malformed ignored service", ports: "ssh:not-a-port,db:8563"},
		{name: "missing database service", ports: "ssh:20022"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given / When
			_, err := resolveHostPodmanDBPort(test.ports)

			// Then
			if err == nil {
				t.Fatalf("expected invalid mapping error for %q", test.ports)
			}
			if !strings.Contains(err.Error(), "local") {
				t.Fatalf("expected contextual local port error, got %v", err)
			}
		})
	}
}

// Linux needs no readying step: Podman on PATH does not stop being reachable,
// so EnsureQueryable must not invoke it at all.
//
//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestLinuxEnsureQueryable_InvokesNothing(t *testing.T) {
	dir := newIsolatedShimDir(t)
	logPath := dropShim(t, dir, "podman", `exit 1`)

	hostRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)

	if err := hostRuntime.EnsureQueryable(context.Background(), nil, nil); err != nil {
		t.Fatalf("EnsureQueryable() unexpected error: %v", err)
	}
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Linux must not invoke podman to become queryable (err=%v)", err)
	}
}
