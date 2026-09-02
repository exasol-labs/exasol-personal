// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localports"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	testDeploymentID       = "podman-install-test"
	testContainerName      = "exasol-db-" + testDeploymentID
	testLoadedImage        = "docker.io/exasol/nano:test"
	testExecutableFileMode = 0o700
)

func TestPodmanInstallStart_StartsFreshPersistentDatabase(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	var out, outErr bytes.Buffer

	// When
	err := install.Start(context.Background(), &out, &outErr, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected fresh start to succeed, got %v", err)
	}
	if info, statErr := os.Stat(startConfig.DataDir); statErr != nil || !info.IsDir() {
		t.Fatalf("expected persistent data directory, got info=%v error=%v", info, statErr)
	}
	assertCommandLog(t, fixture.logPath, []string{
		"<podman><container><exists><" + testContainerName + ">",
		"<podman><load><-i><" + fixture.imagePath + ">",
		"<sync>",
		"<podman><run><-d><--replace><--name><" + testContainerName + ">" +
			"<--shm-size=512mb><--pids-limit=-1><--security-opt><unmask=ALL>" +
			"<--restart><always><-p><127.0.0.1:28563:8563>" +
			"<-v><" + startConfig.DataDir + ":/exa:Z>" +
			"<" + testLoadedImage + "><init><params=maxConnectionsLicenseLimit=20>" +
			"<VERSION_CHECK_ENABLED=0>",
	})
	if !strings.Contains(out.String(), loadedImagePrefix+" "+testLoadedImage) {
		t.Fatalf("expected load output to be forwarded, got %q", out.String())
	}
}

func TestPodmanInstallStart_PublishesDatabaseForVMForwarding(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	startConfig.ContainerDBBindHost = ""

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected VM-facing start to succeed, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	runCommand := commands[len(commands)-1]
	if !strings.Contains(runCommand, "<-p><28563:8563>") {
		t.Fatalf("expected VM-accessible DB port mapping, got %q", runCommand)
	}
}

func TestPodmanInstallStart_UsesLoadedNanoImageID(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	const loadedImageID = "sha256:0123456789abcdef"
	install, startConfig, fixture := newPodmanInstallFixture(t)
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "load-output"),
		loadedImagePrefix+" "+loadedImageID+"\n",
	)

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected start with loaded image ID to succeed, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	runCommand := commands[len(commands)-1]
	if !strings.Contains(runCommand, "<"+loadedImageID+"><init>") {
		t.Fatalf("expected run command to use loaded image ID, got %q", runCommand)
	}
}

func TestPodmanInstallStart_LoadsNanoFromRuntimePath(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	const runtimeImagePath = "/mnt/host/runtime-artifacts/nano.tar"
	install, startConfig, fixture := newPodmanInstallFixture(t)
	install.resolveImage = func(context.Context) (RuntimePath, error) {
		return RuntimePath{
			HostPath:    fixture.imagePath,
			RuntimePath: runtimeImagePath,
		}, nil
	}

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected mapped Nano image start to succeed, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	if got, want := commands[1], "<podman><load><-i><"+runtimeImagePath+">"; got != want {
		t.Fatalf("load command = %q, want %q", got, want)
	}
}

func TestPodmanInstallStart_ReusesExistingDatabaseConfiguration(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	writeTestFile(t, filepath.Join(startConfig.DataDir, "exasol.conf"), "existing")
	for _, relativePath := range nanoTLSFiles {
		writeTestFile(t, filepath.Join(startConfig.DataDir, relativePath), "existing TLS")
	}

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected existing database start to succeed, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	if len(commands) != 4 {
		t.Fatalf("expected four execution-environment commands, got %#v", commands)
	}
	runCommand := commands[len(commands)-1]
	if !strings.HasSuffix(
		runCommand,
		"<"+testLoadedImage+"><init><VERSION_CHECK_ENABLED=0>",
	) {
		t.Fatalf("expected existing database to run init without params, got %q", runCommand)
	}
	if strings.Contains(runCommand, "<params=") {
		t.Fatalf("expected first-start params to be omitted, got %q", runCommand)
	}
	for _, relativePath := range nanoTLSFiles {
		content, readErr := os.ReadFile(
			filepath.Join(startConfig.DataDir, relativePath),
		) //nolint:gosec // test-owned path
		if readErr != nil || string(content) != "existing TLS" {
			t.Fatalf(
				"expected existing TLS file %s to be preserved, got %q, %v",
				relativePath,
				content,
				readErr,
			)
		}
	}
}

func TestPodmanInstallStart_ConfiguresEnabledNanoVersionChecks(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	tests := []struct {
		name                  string
		interval              int
		expectedInterval      int
		expectedRetryInterval int
	}{
		{
			name:                  "default interval",
			interval:              0,
			expectedInterval:      86400,
			expectedRetryInterval: 86400,
		},
		{name: "minimum interval", interval: 1, expectedInterval: 60, expectedRetryInterval: 60},
		{
			name:                  "maximum interval",
			interval:              700000,
			expectedInterval:      604800,
			expectedRetryInterval: 86400,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			install, startConfig, fixture := newPodmanInstallFixture(t)
			startConfig.VersionCheck = VersionCheckConfig{
				Enabled:         true,
				URL:             "https://version-check.example.test",
				Identity:        "exasol-personal;deployment;local;local",
				OperatingSystem: "Linux",
				IntervalSeconds: test.interval,
			}

			// When
			err := install.Start(context.Background(), nil, nil, startConfig)
			// Then
			if err != nil {
				t.Fatalf("expected enabled version-check start to succeed: %v", err)
			}
			commands := readCommandLog(t, fixture.logPath)
			runCommand := commands[len(commands)-1]
			expectedArgs := []string{
				"<-e><VERSION_CHECK_IDENTITY=exasol-personal;deployment;local;local>",
				"<VERSION_CHECK_ENABLED=1>",
				"<VERSION_CHECK_ENDPOINT=https://version-check.example.test>",
				fmt.Sprintf("<VERSION_CHECK_INTERVAL_SEC=%d>", test.expectedInterval),
				fmt.Sprintf("<VERSION_CHECK_RETRY_INTERVAL_SEC=%d>", test.expectedRetryInterval),
				"<VERSION_CHECK_OPERATING_SYSTEM=Linux>",
			}
			for _, expected := range expectedArgs {
				if !strings.Contains(runCommand, expected) {
					t.Fatalf("expected run command to contain %q, got %q", expected, runCommand)
				}
			}
		})
	}
}

func TestPodmanInstallStart_ReturnsWhenContainerAlreadyRunning(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "running"), testContainerName+"\n")
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "mounts-output"),
		fmt.Sprintf(
			`[{"Type":"bind","Source":%q,"Destination":"/exa"}]`, startConfig.DataDir,
		),
	)
	install.resolveImage = nil

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected already-running start to succeed, got %v", err)
	}
	assertCommandLog(t, fixture.logPath, []string{
		"<podman><container><exists><" + testContainerName + ">",
		"<podman><container><inspect><--format><{{.State.Running}}><" + testContainerName + ">",
		"<podman><container><inspect><--format><{{json .Mounts}}><" + testContainerName + ">",
	})
	if _, statErr := os.Stat(startConfig.DataDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected no data-directory work for running container, got %v", statErr)
	}
}

//nolint:revive
func TestPodmanInstallStatus_InspectsExactDeploymentContainer(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	tests := []struct {
		name             string
		scenarioFile     string
		expectedRunning  bool
		expectedCommands []string
	}{
		{
			name:            "missing",
			expectedRunning: false,
			expectedCommands: []string{
				"<podman><container><exists><" + testContainerName + ">",
			},
		},
		{
			name:            "stopped",
			scenarioFile:    "existing",
			expectedRunning: false,
			expectedCommands: []string{
				"<podman><container><exists><" + testContainerName + ">",
				"<podman><container><inspect><--format><{{.State.Running}}><" + testContainerName + ">",
			},
		},
		{
			name:            "running",
			scenarioFile:    "running",
			expectedRunning: true,
			expectedCommands: []string{
				"<podman><container><exists><" + testContainerName + ">",
				"<podman><container><inspect><--format><{{.State.Running}}><" + testContainerName + ">",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			install, _, fixture := newPodmanInstallFixture(t)
			if test.scenarioFile != "" {
				writeTestFile(t, filepath.Join(fixture.scenarioDir, test.scenarioFile), "present")
			}

			// When
			status, err := install.Status(context.Background(), nil, nil)
			// Then
			if err != nil {
				t.Fatalf("expected status to succeed, got %v", err)
			}
			if status.Running != test.expectedRunning {
				t.Fatalf("expected running=%t, got %#v", test.expectedRunning, status)
			}
			assertCommandLog(t, fixture.logPath, test.expectedCommands)
		})
	}
}

func TestPodmanInstallStart_StopsAfterCommandFailure(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	tests := []struct {
		name             string
		failedCommand    string
		loadOutput       string
		expectedError    string
		expectedCommands int
	}{
		{
			name:             "load",
			failedCommand:    "load",
			expectedError:    "failed to load Nano image",
			expectedCommands: 6,
		},
		{
			name:             "run",
			failedCommand:    "run",
			expectedError:    "failed to start Nano container",
			expectedCommands: 9,
		},
		{
			name:             "unparseable load output",
			loadOutput:       "Loaded something else\n",
			expectedError:    "could not determine the image reference",
			expectedCommands: 6,
		},
		{
			name:             "multiple loaded images",
			loadOutput:       "Loaded image: first:test\nLoaded image: second:test\n",
			expectedError:    "multiple Nano images",
			expectedCommands: 6,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			install, startConfig, fixture := newPodmanInstallFixture(t)
			install.portAvailable = func(string, int) bool { return true }
			if test.failedCommand != "" {
				writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail"), test.failedCommand)
			}
			if test.loadOutput != "" {
				writeTestFile(t, filepath.Join(fixture.scenarioDir, "load-output"), test.loadOutput)
			}

			// When
			err := install.Start(context.Background(), nil, nil, startConfig)

			// Then
			if err == nil || !strings.Contains(err.Error(), test.expectedError) {
				t.Fatalf("expected error containing %q, got %v", test.expectedError, err)
			}
			commands := readCommandLog(t, fixture.logPath)
			if len(commands) != test.expectedCommands {
				t.Fatalf("expected %d commands, got %#v", test.expectedCommands, commands)
			}
		})
	}
}

func TestPodmanInstallStartClassifiesConfirmedBindFailures(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	for _, test := range []struct {
		name           string
		bindDiagnostic bool
		portAvailable  bool
	}{
		{name: "diagnostic", bindDiagnostic: true, portAvailable: true},
		{name: "post failure availability", portAvailable: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			install, startConfig, fixture := newPodmanInstallFixture(t)
			writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail"), "run")
			if test.bindDiagnostic {
				writeTestFile(t, filepath.Join(fixture.scenarioDir, "bind-diagnostic"), "enabled")
			}
			install.portAvailable = func(string, int) bool { return test.portAvailable }

			err := install.Start(context.Background(), nil, nil, startConfig)

			var unavailable *localports.UnavailableError
			if !errors.As(err, &unavailable) || unavailable.Service != "db" ||
				unavailable.Port != startConfig.ContainerDBPort {
				t.Fatalf("expected unavailable database port, got %v", err)
			}
			if _, ok := errors.AsType[*exec.ExitError](err); !ok {
				t.Fatalf("expected original command error in chain, got %v", err)
			}
		})
	}
}

func TestPodmanInstallStartDoesNotClassifyPartialStartAsBindFailure(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	install, startConfig, fixture := newPodmanInstallFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail"), "run")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "bind-diagnostic"), "enabled")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "partial-start"), "enabled")
	install.portAvailable = func(string, int) bool { return false }

	err := install.Start(context.Background(), nil, nil, startConfig)

	if _, unavailable := localports.AsUnavailable(err); unavailable {
		t.Fatalf("partial start must retain generic command failure, got %v", err)
	}
}

func TestPodmanInstallStart_StopsBeforeNanoWhenStorageSyncFails(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	install, startConfig, fixture := newPodmanInstallFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail-sync"), "enabled")

	err := install.Start(context.Background(), nil, nil, startConfig)

	if err == nil || !strings.Contains(
		err.Error(),
		"failed to synchronize local runtime storage before starting Nano",
	) {
		t.Fatalf("expected storage synchronization failure, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	syncIndex := -1
	for index, command := range commands {
		if command == "<sync>" {
			syncIndex = index
		}
		if strings.Contains(command, "<podman><run>") {
			t.Fatalf("Nano must not start after storage synchronization failure: %q", command)
		}
	}
	if syncIndex < 0 || syncIndex < 2 {
		t.Fatalf("expected sync after image load, got %#v", commands)
	}
}

func TestPodmanInstallStart_AttemptsAllDiagnosticsWithoutReplacingFailure(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail"), "run")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail-diagnostics"), "enabled")
	var outErr bytes.Buffer

	// When
	err := install.Start(context.Background(), nil, &outErr, startConfig)

	// Then
	if err == nil || !strings.Contains(err.Error(), "failed to start Nano container") {
		t.Fatalf("expected original startup failure, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	expectedTail := []string{
		"<podman><info>",
		"<podman><ps><-a>",
		"<podman><container><inspect><" + testContainerName + ">",
		"<podman><logs><" + testContainerName + ">",
	}
	if len(commands) < len(expectedTail) {
		t.Fatalf("expected diagnostic commands, got %#v", commands)
	}
	for index, expected := range expectedTail {
		actual := commands[len(commands)-len(expectedTail)+index]
		if actual != expected {
			t.Fatalf("expected diagnostic command %q, got %q", expected, actual)
		}
	}
	if failures := strings.Count(outErr.String(), "diagnostic command failed"); failures != 4 {
		t.Fatalf(
			"expected four best-effort diagnostic failures, got %d in %q",
			failures,
			outErr.String(),
		)
	}
}

func TestPodmanInstallStart_RejectsInvalidConfigurationBeforePodman(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	tests := []struct {
		name   string
		mutate func(*PodmanInstall, *StartConfig)
	}{
		{
			name:   "published port",
			mutate: func(_ *PodmanInstall, config *StartConfig) { config.ContainerDBPort = 65536 },
		},
		{
			name: "data directory",
			mutate: func(_ *PodmanInstall, config *StartConfig) {
				config.DataDir = " "
			},
		},
		{
			name: "enabled version-check URL",
			mutate: func(_ *PodmanInstall, config *StartConfig) {
				config.VersionCheck = validTestVersionCheckConfig()
				config.VersionCheck.URL = ""
			},
		},
		{
			name: "enabled version-check identity",
			mutate: func(_ *PodmanInstall, config *StartConfig) {
				config.VersionCheck = validTestVersionCheckConfig()
				config.VersionCheck.Identity = ""
			},
		},
		{
			name: "enabled version-check operating system",
			mutate: func(_ *PodmanInstall, config *StartConfig) {
				config.VersionCheck = validTestVersionCheckConfig()
				config.VersionCheck.OperatingSystem = ""
			},
		},
		{
			name: "SLC image",
			mutate: func(_ *PodmanInstall, config *StartConfig) {
				config.SLCs = []SLCConfig{{Target: "/exa/slc/python"}}
			},
		},
		{
			name: "SLC target",
			mutate: func(_ *PodmanInstall, config *StartConfig) {
				config.SLCs = []SLCConfig{{Image: "example.test/slc:latest"}}
			},
		},
		{
			name: "SLC status path",
			mutate: func(install *PodmanInstall, config *StartConfig) {
				config.SLCs = []SLCConfig{}
				install.slcStatusPath.RuntimePath = " "
			},
		},
		{
			name: "SLC staging directory",
			mutate: func(install *PodmanInstall, config *StartConfig) {
				config.SLCs = []SLCConfig{}
				install.slcStagingDir.RuntimePath = " "
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			install, startConfig, fixture := newPodmanInstallFixture(t)
			test.mutate(install, &startConfig)

			// When
			err := install.Start(context.Background(), nil, nil, startConfig)

			// Then
			if err == nil {
				t.Fatal("expected invalid configuration error, got nil")
			}
			if commands := readCommandLog(t, fixture.logPath); len(commands) != 0 {
				t.Fatalf("expected no Podman commands, got %#v", commands)
			}
			if _, statErr := os.Stat(startConfig.DataDir); !os.IsNotExist(statErr) {
				t.Fatalf("expected no data-directory work, got %v", statErr)
			}
		})
	}
}

func TestPodmanInstallStart_RejectsInvalidSLCBeforeAlreadyRunningReturn(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	startConfig.SLCs = []SLCConfig{{Image: " ", Target: "/exa/slc/python"}}
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "running"), testContainerName+"\n")

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)

	// Then
	if err == nil || !strings.Contains(err.Error(), "nano SLC 0 image is required") {
		t.Fatalf("expected invalid SLC configuration error, got %v", err)
	}
	if commands := readCommandLog(t, fixture.logPath); len(commands) != 0 {
		t.Fatalf("expected no Podman commands, got %#v", commands)
	}
}

func validTestVersionCheckConfig() VersionCheckConfig {
	return VersionCheckConfig{
		Enabled:         true,
		URL:             "https://version-check.example.test",
		Identity:        "test-identity",
		OperatingSystem: "Linux",
	}
}

func TestPodmanInstallStop_RemovesContainerIdempotently(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, _, fixture := newPodmanInstallFixture(t)

	// When
	err := install.Stop(context.Background(), nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected stop to succeed, got %v", err)
	}
	assertCommandLog(t, fixture.logPath, []string{
		"<podman><rm><--force><--ignore><" + testContainerName + ">",
	})
}

type podmanInstallFixture struct {
	logPath       string
	scenarioDir   string
	imagePath     string
	slcStagingDir string
	slcStatusPath string
}

func newPodmanInstallFixture(t *testing.T) (*PodmanInstall, StartConfig, podmanInstallFixture) {
	t.Helper()

	root := t.TempDir()
	deployment := config.NewDeploymentDir(filepath.Join(root, "deployment"))
	if err := os.MkdirAll(deployment.Root(), 0o750); err != nil {
		t.Fatalf("failed to create deployment directory: %v", err)
	}
	state := &config.ExasolPersonalState{DeploymentId: testDeploymentID}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{},
		deployment,
	); err != nil {
		t.Fatalf("failed to write deployment state: %v", err)
	}
	scenarioDir := filepath.Join(root, "scenario")
	if err := os.MkdirAll(scenarioDir, 0o750); err != nil {
		t.Fatalf("failed to create fake Podman scenario: %v", err)
	}
	scriptPath := filepath.Join(root, "fake-podman.sh")
	if err := os.WriteFile(
		scriptPath,
		[]byte(fakePodmanScript),
		testExecutableFileMode,
	); err != nil {
		t.Fatalf("failed to write fake Podman executable: %v", err)
	}
	logPath := filepath.Join(root, "podman.log")
	imagePath := filepath.Join(root, "nano-image.tar")
	writeTestFile(t, imagePath, "image")
	manager := runtimeartifacts.NewResourceManagerForPlatform(
		runtimeartifacts.ResourceSpec{
			exasolNanoImageResourceID: {
				Artifact: map[string]runtimeartifacts.ArtifactSpec{
					"any": {URL: imagePath},
				},
			},
		},
		filepath.Join(root, "cache"),
		runtime.GOOS,
		runtime.GOARCH,
	)

	slcStagingDir := filepath.Join(root, "slc-packages")
	slcStatusPath := filepath.Join(root, "slc-status.json")
	install := NewPodmanInstall(
		deployment,
		manager,
		[]string{"/bin/sh", scriptPath, logPath, scenarioDir},
		slcStagingDir,
		slcStatusPath,
	)
	startConfig := StartConfig{
		ContainerDBPort:     28563,
		ContainerDBBindHost: "127.0.0.1",
		DataDir:             filepath.Join(root, "runtime", "exa"),
		InitParams:          []string{"maxConnectionsLicenseLimit=20"},
	}
	fixture := podmanInstallFixture{
		logPath:       logPath,
		scenarioDir:   scenarioDir,
		imagePath:     imagePath,
		slcStagingDir: slcStagingDir,
		slcStatusPath: slcStatusPath,
	}

	return install, startConfig, fixture
}

func readCommandLog(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("failed to read fake Podman command log: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

func assertCommandLog(t *testing.T, path string, expected []string) {
	t.Helper()

	actual := readCommandLog(t, path)
	if len(actual) != len(expected) {
		t.Fatalf("expected command log %#v, got %#v", expected, actual)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("expected command %d to be %q, got %q", index, expected[index], actual[index])
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("failed to create test file directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}

func skipPodmanInstallTestOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake Podman executable is a POSIX shell script")
	}
}

//nolint:dupword // Repeated shell terminators are required by this fixture.
const fakePodmanScript = `#!/bin/sh
set -eu

log_path=$1
scenario_dir=$2
shift 2

for argument in "$@"; do
  printf '<%s>' "$argument" >> "$log_path"
done
printf '\n' >> "$log_path"

if [ "$1" = "sync" ]; then
  if [ -f "$scenario_dir/fail-sync" ]; then
    printf 'fake sync failure\n' >&2
    exit 42
  fi
  exit 0
fi
if [ "$1" != "podman" ]; then
  exit 90
fi
command=$2
if [ -f "$scenario_dir/fail" ] && [ "$(cat "$scenario_dir/fail")" = "$command" ]; then
	if [ "$command" = "run" ] && [ -f "$scenario_dir/partial-start" ]; then
		touch "$scenario_dir/running"
	fi
	if [ "$command" = "run" ] && [ -f "$scenario_dir/bind-diagnostic" ]; then
		printf 'bind: address already in use\n' >&2
	else
		printf 'fake %s failure\n' "$command" >&2
	fi
	exit 42
fi
if [ "$command" = "import" ] && [ -f "$scenario_dir/fail-import-image" ]; then
  for last_argument in "$@"; do :; done
  if [ "$(cat "$scenario_dir/fail-import-image")" = "$last_argument" ]; then
    printf 'fake import failure\n' >&2
    exit 44
  fi
fi
if [ "$command" = "rmi" ] &&
  [ -f "$scenario_dir/fail-rmi-image" ] &&
  [ "$(cat "$scenario_dir/fail-rmi-image")" = "$3" ]; then
  printf 'fake rmi failure\n' >&2
  exit 45
fi
if [ -f "$scenario_dir/fail-diagnostics" ]; then
  case "$command" in
    info|ps|logs)
      printf 'fake diagnostic failure\n' >&2
      exit 43
      ;;
    container)
      if [ "$3" = "inspect" ] && [ "$4" != "--format" ]; then
        printf 'fake diagnostic failure\n' >&2
        exit 43
      fi
      ;;
  esac
fi

case "$command" in
  image)
    if [ "$3" != "exists" ]; then
      exit 93
    fi
    if [ ! -f "$scenario_dir/images" ] || ! grep -Fxq "$4" "$scenario_dir/images"; then
      exit 1
    fi
    ;;
  images)
    if [ "$3" = "--filter" ]; then
      if [ -f "$scenario_dir/labeled-images-output" ]; then
        cat "$scenario_dir/labeled-images-output"
      fi
    elif [ -f "$scenario_dir/images-output" ]; then
      cat "$scenario_dir/images-output"
    fi
    ;;
  container)
    operation=$3
    case "$operation" in
      exists)
		if [ -f "$scenario_dir/legacy-name" ]; then
		  if [ "$(cat "$scenario_dir/legacy-name")" = "$4" ]; then
		    exit 0
		  fi
		  exit 1
		fi
        if [ ! -f "$scenario_dir/running" ] && [ ! -f "$scenario_dir/existing" ]; then
          exit 1
        fi
        ;;
      inspect)
        if [ "$4" = "--format" ] && [ "$5" = "{{json .Mounts}}" ]; then
          if [ -f "$scenario_dir/mounts-output" ]; then
            cat "$scenario_dir/mounts-output"
          else
            printf '[{"Destination":"/exa"}]\n'
          fi
        elif [ -f "$scenario_dir/running" ]; then
          printf 'true\n'
        else
          printf 'false\n'
        fi
        ;;
      *)
        printf 'unexpected fake Podman container operation: %s\n' "$operation" >&2
        exit 92
        ;;
    esac
    ;;
  load)
    if [ -f "$scenario_dir/load-output" ]; then
      cat "$scenario_dir/load-output"
    else
      printf 'Loaded image: docker.io/exasol/nano:test\n'
    fi
    ;;
  rm)
    rm -f "$scenario_dir/running" "$scenario_dir/existing"
    ;;
  stop)
    rm -f "$scenario_dir/running"
    : > "$scenario_dir/existing"
    ;;
  cp)
    if [ -f "$scenario_dir/populate-destination-during-cp" ]; then
      destination_path=$(cat "$scenario_dir/populate-destination-during-cp")
      mkdir -p "$destination_path"
      printf 'raced\n' > "$destination_path/raced-data"
    fi
    if [ -d "$scenario_dir/cp-source" ]; then
      cp -R "$scenario_dir/cp-source/." "$4"
    fi
    ;;
  rename)
	rm -f "$scenario_dir/legacy-name"
	: > "$scenario_dir/existing"
	;;
	info|ps|logs|pull|import|run|rmi)
    ;;
  *)
    printf 'unexpected fake Podman command: %s\n' "$command" >&2
    exit 91
    ;;
esac
`
