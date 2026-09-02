// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
)

const testLocalDBPortConfig = "db:28563"

type logCaptureHandler struct {
	records []slog.Record
}

func (*logCaptureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *logCaptureHandler) Handle(_ context.Context, record slog.Record) error {
	h.records = append(h.records, record.Clone())

	return nil
}

func (h *logCaptureHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *logCaptureHandler) WithGroup(string) slog.Handler {
	return h
}

func TestValidateLocalPlatform_AcceptsMacOSAppleSilicon(t *testing.T) {
	t.Parallel()

	// Given / When
	err := validateLocalPlatform(localMacOS, localMacArch)
	// Then
	if err != nil {
		t.Fatalf("expected platform to be accepted, got %v", err)
	}
}

func TestValidateLocalPlatform_AcceptsLinux(t *testing.T) {
	t.Parallel()

	for _, architecture := range []string{localLinuxAMD64, localLinuxARM64} {
		if err := validateLocalPlatform(localLinuxOS, architecture); err != nil {
			t.Fatalf("expected linux/%s to be accepted, got %v", architecture, err)
		}
	}
}

func TestValidateLocalPlatform_AcceptsWindowsAMD64(t *testing.T) {
	t.Parallel()

	if err := validateLocalPlatform(localWindowsOS, localWindowsAMD64); err != nil {
		t.Fatalf("expected windows/amd64 to be accepted, got %v", err)
	}
}

func TestValidateLocalPlatform_RejectsUnsupportedPlatform(t *testing.T) {
	t.Parallel()

	// Given / When
	err := validateLocalPlatform("freebsd", "amd64")

	// Then
	if !errors.Is(err, errUnsupportedLocalPlatform) {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}
}

func TestResolveLocalRuntimeConfig_UsesDefaults(t *testing.T) {
	t.Parallel()

	// Given / When
	runtimeConfig, err := resolveLocalRuntimeConfigForPlatform(
		&presets.InfrastructureManifest{}, 0, localMacOS, localMacArch,
	)
	expectedDefaults := defaultLocalRuntimeConfig(0)
	// Then
	if err != nil {
		t.Fatalf("expected default config, got %v", err)
	}
	if runtimeConfig.cpuCount != expectedDefaults.cpuCount ||
		runtimeConfig.memoryMB != expectedDefaults.memoryMB ||
		runtimeConfig.dataSizeGB != expectedDefaults.dataSizeGB {
		t.Fatalf("unexpected default local config: %#v", runtimeConfig)
	}
}

func TestDefaultLocalRuntimeConfig_UsesHalfHostMemoryWhenAvailable(t *testing.T) {
	t.Parallel()

	expectedMemoryMB := 12288
	runtimeConfig := defaultLocalRuntimeConfig(24576)
	if runtimeConfig.memoryMB != expectedMemoryMB {
		t.Fatalf(
			"expected default memory %d MB, got %d",
			expectedMemoryMB,
			runtimeConfig.memoryMB,
		)
	}
}

func TestResolveLocalRuntimeConfig_UsesManifestValues(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{
			CPUCount:   4,
			MemoryMB:   8192,
			DataSizeGB: 250,
		},
	}

	// When
	runtimeConfig, err := resolveLocalRuntimeConfigForPlatform(
		manifest, 0, localMacOS, localMacArch,
	)
	// Then
	if err != nil {
		t.Fatalf("expected local config, got %v", err)
	}
	if runtimeConfig.cpuCount != 4 || runtimeConfig.memoryMB != 8192 ||
		runtimeConfig.dataSizeGB != 250 {
		t.Fatalf("unexpected local config: %#v", runtimeConfig)
	}
}

func TestResolveLocalRuntimeConfig_ExplicitMemoryOverridesComputedDefault(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{
			MemoryMB: 8192,
		},
	}

	// When
	runtimeConfig, err := resolveLocalRuntimeConfigForPlatform(
		manifest, 24576, localMacOS, localMacArch,
	)
	// Then
	if err != nil {
		t.Fatalf("expected local config, got %v", err)
	}
	if runtimeConfig.memoryMB != 8192 {
		t.Fatalf("expected explicit memory override to win, got %d", runtimeConfig.memoryMB)
	}
}

func TestResolveLocalRuntimeConfig_RejectsInvalidValues(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{DataSizeGB: -1},
	}

	// When
	_, err := resolveLocalRuntimeConfigForPlatform(manifest, 0, localMacOS, localMacArch)

	// Then
	if err == nil {
		t.Fatal("expected invalid local config error, got nil")
	}
}

func TestResolveLocalRuntimeConfig_LinuxIgnoresVMSizing(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Local: &presets.InfrastructureLocal{
		CPUCount: -1, MemoryMB: -1, DataSizeGB: -1, Ports: testLocalDBPortConfig,
	}}

	runtimeConfig, err := resolveLocalRuntimeConfigForPlatform(
		manifest, 1024, localLinuxOS, localLinuxAMD64,
	)
	if err != nil {
		t.Fatalf("expected Linux to ignore VM sizing, got %v", err)
	}
	if runtimeConfig.cpuCount != 0 || runtimeConfig.memoryMB != 0 ||
		runtimeConfig.dataSizeGB != 0 || runtimeConfig.ports != testLocalDBPortConfig {
		t.Fatalf("unexpected Linux runtime config: %#v", runtimeConfig)
	}
}

func TestValidateLocalRuntimeConfig_RejectsHostMemoryBelowMinimum(t *testing.T) {
	t.Parallel()

	err := validateLocalRuntimeConfig(
		localRuntimeConfig{cpuCount: 2, memoryMB: 4096, dataSizeGB: 100},
		6144,
	)
	if err == nil {
		t.Fatal("expected host memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "requires at least 8192 MB host memory") {
		t.Fatalf("unexpected host memory error: %v", err)
	}
	if !strings.Contains(err.Error(), "detected 6144 MB") {
		t.Fatalf("unexpected host memory error: %v", err)
	}
}

func TestValidateLocalRuntimeConfig_PrefersHostMemoryError(t *testing.T) {
	t.Parallel()

	err := validateLocalRuntimeConfig(
		localRuntimeConfig{cpuCount: 2, memoryMB: 2048, dataSizeGB: 100},
		6144,
	)
	if err == nil {
		t.Fatal("expected host memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "requires at least 8192 MB host memory") {
		t.Fatalf("unexpected host memory error: %v", err)
	}
	if strings.Contains(err.Error(), "memory-mb must be at least 4096 MB") {
		t.Fatalf("expected host memory error to take precedence, got %v", err)
	}
}

func TestValidateLocalRuntimeConfig_RejectsMemoryBelowMinimum(t *testing.T) {
	t.Parallel()

	err := validateLocalRuntimeConfig(
		localRuntimeConfig{cpuCount: 2, memoryMB: 4095, dataSizeGB: 100},
		8192,
	)
	if err == nil {
		t.Fatal("expected minimum memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "local memory-mb must be at least 4096 MB") {
		t.Fatalf("unexpected minimum memory error: %v", err)
	}
}

func TestValidateLocalRuntimeConfig_AcceptsMinimumMemory(t *testing.T) {
	t.Parallel()

	err := validateLocalRuntimeConfig(
		localRuntimeConfig{cpuCount: 2, memoryMB: 4096, dataSizeGB: 100},
		8192,
	)
	if err != nil {
		t.Fatalf("expected minimum memory to be accepted, got %v", err)
	}
}

func TestValidateLocalInitMemory_RejectsOverrideBelowMinimum(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	err := validateLocalInitMemoryForPlatform(
		context.Background(),
		manifest,
		map[string]string{localMemoryMBConfigName: "4095"},
		localMacOS,
		localMacArch,
	)
	if err == nil {
		t.Fatal("expected minimum memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "local memory-mb must be at least 4096 MB") {
		t.Fatalf("unexpected minimum memory error: %v", err)
	}
}

func TestValidateLocalInitMemory_AcceptsValidOverride(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	err := validateLocalInitMemoryForPlatform(
		context.Background(),
		manifest,
		map[string]string{localMemoryMBConfigName: "4096"},
		localMacOS,
		localMacArch,
	)
	if err != nil {
		t.Fatalf("expected valid override to be accepted, got %v", err)
	}
}

func TestValidateLocalInitMemory_IgnoresNonLocalBackend(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeTofu}

	err := validateLocalInitMemoryForPlatform(
		context.Background(),
		manifest,
		map[string]string{localMemoryMBConfigName: "4095"},
		localMacOS,
		localMacArch,
	)
	if err != nil {
		t.Fatalf("expected non-local backend to be ignored, got %v", err)
	}
}

func TestLocalBackendSetupWorkspace_Noops(t *testing.T) {
	t.Parallel()

	// Given / When
	backend := newLocalBackend(
		config.NewDeploymentDir(t.TempDir()),
		&presets.InfrastructureManifest{},
		nil,
	)
	err := backend.SetupWorkspace(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected local workspace setup to no-op, got %v", err)
	}
}

func TestLocalBackendReadConfiguration_ExposesSizingValues(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{
			CPUCount:   4,
			MemoryMB:   8192,
			DataSizeGB: 250,
		},
	}

	// When
	backend := newLocalBackendForPlatform(
		config.NewDeploymentDir(t.TempDir()), manifest, nil, localMacOS, localMacArch,
	)
	values, err := backend.ReadConfiguration()
	// Then
	if err != nil {
		t.Fatalf("expected local configuration values, got %v", err)
	}
	defaults := defaultLocalRuntimeConfig(detectLocalHostMemoryMB(context.Background()))
	assertConfigValue(t, values, localCPUCountConfigName, 4, localDefaultCPUCount)
	assertConfigValue(t, values, localMemoryMBConfigName, 8192, defaults.memoryMB)
	assertConfigValue(t, values, localDataSizeGBConfigName, 250, localDefaultDataSizeGB)
}

func TestLocalBackendDescribesMacOSRuntimeDisk(t *testing.T) {
	t.Parallel()

	backend := newLocalBackendForPlatform(
		config.NewDeploymentDir(t.TempDir()),
		&presets.InfrastructureManifest{},
		nil,
		localMacOS,
		localMacArch,
	)
	definitions, err := backend.ReadDeploymentConfigVariables()
	if err != nil {
		t.Fatalf("expected local configuration definitions, got %v", err)
	}
	definition := definitions[localDataSizeGBConfigName]
	const wantDescription = "VM runtime disk size in GB for Podman images and runtime state."
	if definition.Description != wantDescription {
		t.Fatalf("runtime disk description = %q, want %q", definition.Description, wantDescription)
	}
}

func TestLocalBackendReadConfiguration_LinuxExposesOnlyPorts(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Local: &presets.InfrastructureLocal{
		CPUCount: -1, MemoryMB: -1, DataSizeGB: -1, Ports: testLocalDBPortConfig,
	}}
	backend := newLocalBackendForPlatform(
		config.NewDeploymentDir(t.TempDir()), manifest, nil, localLinuxOS, localLinuxAMD64,
	)

	values, err := backend.ReadConfiguration()
	if err != nil {
		t.Fatalf("expected Linux configuration, got %v", err)
	}
	if len(values) != 1 || values[0].Name != localPortsConfigName ||
		values[0].Value != testLocalDBPortConfig {
		t.Fatalf("expected only the port configuration, got %#v", values)
	}
	definitions, err := backend.ReadDeploymentConfigVariables()
	if err != nil {
		t.Fatalf("expected Linux configuration definitions, got %v", err)
	}
	if len(definitions) != 1 || definitions[localPortsConfigName].Name != localPortsConfigName {
		t.Fatalf("expected only the port definition, got %#v", definitions)
	}
}

func TestLocalBackendConfigurationDefaults_ReturnsOnlyUnsetValues(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{
		Backend: backendTypeLocal,
		Local: &presets.InfrastructureLocal{
			CPUCount: 4, MemoryMB: 8192, DataSizeGB: 250,
		},
	}
	backend := newLocalBackendForPlatform(
		config.NewDeploymentDir(t.TempDir()), manifest, nil, localMacOS, localMacArch,
	)
	backend.ports = testLocalPortAllocator(
		t,
		localMinimumAutomaticPort,
		localMaximumPort,
		func(_ string, port int) bool { return port == localDatabasePort },
	)

	defaults, err := backend.ConfigurationDefaults(
		context.Background(),
		map[string]string{localCPUCountConfigName: "8"},
	)
	if err != nil {
		t.Fatalf("expected local defaults, got %v", err)
	}
	if _, exists := defaults[localCPUCountConfigName]; exists {
		t.Fatalf("expected supplied CPU count to be omitted, got %#v", defaults)
	}
	if defaults[localMemoryMBConfigName] != "8192" ||
		defaults[localDataSizeGBConfigName] != "250" ||
		defaults[localPortsConfigName] != "db:8563" {
		t.Fatalf("unexpected local defaults: %#v", defaults)
	}
}

func TestLocalBackendConfigure_WritesSizingValuesToManifest(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o750); err != nil {
		t.Fatalf("failed to create infrastructure dir: %v", err)
	}
	manifest := &presets.InfrastructureManifest{
		Name:        "Local",
		Description: "Local preset",
		Backend:     backendTypeLocal,
		Local:       &presets.InfrastructureLocal{},
	}

	// When
	backend := newLocalBackendForPlatform(
		deployment, manifest, nil, localMacOS, localMacArch,
	)
	err := backend.Configure(
		context.Background(),
		map[string]string{
			localCPUCountConfigName:   "4",
			localMemoryMBConfigName:   "8192",
			localDataSizeGBConfigName: "250",
		},
		DeploymentMetadata{},
		DeploymentLayout{},
	)
	// Then
	if err != nil {
		t.Fatalf("expected local configuration to be written, got %v", err)
	}
	written, err := presets.ReadInfrastructureManifestFromDir(deployment.InfrastructureDir())
	if err != nil {
		t.Fatalf("expected local infrastructure manifest to be readable, got %v", err)
	}
	if written.Local == nil {
		t.Fatal("expected local manifest configuration, got nil")
	}
	if written.Local.CPUCount != 4 ||
		written.Local.MemoryMB != 8192 ||
		written.Local.DataSizeGB != 250 {
		t.Fatalf("unexpected local manifest configuration: %#v", written.Local)
	}
}

//nolint:paralleltest // This test temporarily replaces the process-wide default logger.
func TestLocalBackendConfigure_WarnsForLowMemory(t *testing.T) {
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o750); err != nil {
		t.Fatalf("failed to create infrastructure dir: %v", err)
	}
	manifest := &presets.InfrastructureManifest{
		Name:        "Local",
		Description: "Local preset",
		Backend:     backendTypeLocal,
		Local:       &presets.InfrastructureLocal{},
	}
	logCapture := &logCaptureHandler{}
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(logCapture))
	defer slog.SetDefault(originalLogger)

	// When
	backend := newLocalBackendForPlatform(
		deployment, manifest, nil, localMacOS, localMacArch,
	)
	err := backend.Configure(
		context.Background(),
		map[string]string{localMemoryMBConfigName: "8192"},
		DeploymentMetadata{},
		DeploymentLayout{},
	)
	// Then
	if err != nil {
		t.Fatalf("expected local configuration to be written, got %v", err)
	}
	foundMacNotice := false
	for _, record := range logCapture.records {
		if record.Level == slog.LevelWarn && record.Message == localInfraMemoryNoticeText {
			foundMacNotice = true
		}
	}
	if !foundMacNotice {
		t.Fatalf("expected warning log %q, got %#v", localInfraMemoryNoticeText, logCapture.records)
	}

	logCapture.records = nil
	linuxDeployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(linuxDeployment.InfrastructureDir(), 0o750); err != nil {
		t.Fatalf("failed to create Linux infrastructure dir: %v", err)
	}
	linuxBackend := newLocalBackendForPlatform(
		linuxDeployment,
		&presets.InfrastructureManifest{Backend: backendTypeLocal},
		nil,
		localLinuxOS,
		localLinuxAMD64,
	)
	if err := linuxBackend.Configure(
		context.Background(), nil, DeploymentMetadata{}, DeploymentLayout{},
	); err != nil {
		t.Fatalf("expected Linux configuration to succeed, got %v", err)
	}
	for _, record := range logCapture.records {
		if record.Level == slog.LevelWarn && record.Message == localInfraMemoryNoticeText {
			t.Fatalf("Linux configuration emitted Mac VM notice: %#v", logCapture.records)
		}
	}
}

func TestLocalBackendConfigure_RejectsInvalidSizingValues(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{
		Name:        "Local",
		Description: "Local preset",
		Backend:     backendTypeLocal,
		Local:       &presets.InfrastructureLocal{},
	}

	// When
	backend := newLocalBackendForPlatform(
		deployment, manifest, nil, localMacOS, localMacArch,
	)
	err := backend.Configure(
		context.Background(),
		map[string]string{localCPUCountConfigName: "0"},
		DeploymentMetadata{},
		DeploymentLayout{},
	)

	// Then
	if err == nil {
		t.Fatal("expected invalid local configuration error, got nil")
	}
}

func TestLocalBackendConfigure_RejectsMemoryBelowMinimum(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{
		Name:        "Local",
		Description: "Local preset",
		Backend:     backendTypeLocal,
		Local:       &presets.InfrastructureLocal{},
	}

	backend := newLocalBackendForPlatform(
		deployment, manifest, nil, localMacOS, localMacArch,
	)
	err := backend.Configure(
		context.Background(),
		map[string]string{localMemoryMBConfigName: "4095"},
		DeploymentMetadata{},
		DeploymentLayout{},
	)
	if err == nil {
		t.Fatal("expected minimum memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "local memory-mb must be at least 4096 MB") {
		t.Fatalf("unexpected minimum memory error: %v", err)
	}
}

func TestLocalBackendConfigure_LinuxIgnoresVMSizingOverrides(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o750); err != nil {
		t.Fatalf("failed to create infrastructure dir: %v", err)
	}
	manifest := &presets.InfrastructureManifest{
		Name: "Local", Backend: backendTypeLocal, Local: &presets.InfrastructureLocal{},
	}
	backend := newLocalBackendForPlatform(
		deployment, manifest, nil, localLinuxOS, localLinuxAMD64,
	)

	err := backend.Configure(
		context.Background(),
		map[string]string{
			localCPUCountConfigName: "invalid", localMemoryMBConfigName: "-1",
			localDataSizeGBConfigName: "0", localPortsConfigName: testLocalDBPortConfig,
		},
		DeploymentMetadata{}, DeploymentLayout{},
	)
	if err != nil {
		t.Fatalf("expected Linux to ignore VM sizing overrides, got %v", err)
	}
	if manifest.Local.CPUCount != 0 || manifest.Local.MemoryMB != 0 ||
		manifest.Local.DataSizeGB != 0 || manifest.Local.Ports != testLocalDBPortConfig {
		t.Fatalf("unexpected Linux manifest configuration: %#v", manifest.Local)
	}
}

func TestLocalBackend_LinuxShellsAreUnsupported(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	backend := newLocalBackendForPlatform(
		deployment,
		&presets.InfrastructureManifest{},
		localruntime.NewHostLinuxRuntime(deployment, nil),
		localLinuxOS, localLinuxAMD64,
	)
	if err := backend.OpenHostShell(
		context.Background(),
		"",
	); !errors.Is(
		err,
		localruntime.ErrHostShellUnsupported,
	) ||
		!strings.Contains(err.Error(), "linux host runtime") {
		t.Fatalf("expected explicit Linux host shell error, got %v", err)
	}
	if err := backend.OpenCOSShell(
		context.Background(),
	); !errors.Is(
		err,
		localruntime.ErrContainerShellUnsupported,
	) ||
		!strings.Contains(err.Error(), "linux host runtime") {
		t.Fatalf("expected explicit Linux container shell error, got %v", err)
	}
}

func TestLocalBackendShellErrorsBypassReachabilityClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		openShell      func(*localBackend) error
		configureError func(*endpointRuntimeStub, error)
	}{
		{
			name: "host shell",
			openShell: func(backend *localBackend) error {
				return backend.OpenHostShell(context.Background(), "")
			},
			configureError: func(runtime *endpointRuntimeStub, err error) {
				runtime.hostShellErr = err
			},
		},
		{
			name: "container shell",
			openShell: func(backend *localBackend) error {
				return backend.OpenCOSShell(context.Background())
			},
			configureError: func(runtime *endpointRuntimeStub, err error) {
				runtime.containerShellErr = err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			deployment := newLocalTestDeployment(t)
			shellErr := errors.New("runtime shell failed")
			runtime := &endpointRuntimeStub{
				deployment: deployment,
				healthResult: &localruntime.HealthCheckResult{
					Ports: map[string]localruntime.PortHealth{
						"db": {State: localruntime.PortStateBlocked},
					},
				},
			}
			test.configureError(runtime, shellErr)
			backend := newLocalBackendForPlatform(
				deployment,
				&presets.InfrastructureManifest{Backend: backendTypeLocal},
				runtime,
				localMacOS,
				localMacArch,
			)

			// When
			err := test.openShell(backend)

			// Then
			if !errors.Is(err, shellErr) {
				t.Fatalf("expected runtime shell error, got %v", err)
			}
			if errors.Is(err, ErrLocalReachability) {
				t.Fatalf("runtime shell error was replaced by reachability error: %v", err)
			}
			if runtime.healthCalls != 0 {
				t.Fatalf(
					"expected shell failure not to query endpoint health, got %d calls",
					runtime.healthCalls,
				)
			}
		})
	}
}

func assertConfigValue(
	t *testing.T,
	values []DeploymentConfigValue,
	name string,
	expectedValue int,
	expectedDefault int,
) {
	t.Helper()

	for _, value := range values {
		if value.Name != name {
			continue
		}
		if value.Type != ConfigVariableTypeNumber {
			t.Fatalf("expected %s type %q, got %q", name, ConfigVariableTypeNumber, value.Type)
		}
		if value.Value != expectedValue {
			t.Fatalf("expected %s value %d, got %v", name, expectedValue, value.Value)
		}
		if value.Default != expectedDefault {
			t.Fatalf("expected %s default %d, got %v", name, expectedDefault, value.Default)
		}

		return
	}

	t.Fatalf("expected configuration value %q in %#v", name, values)
}
