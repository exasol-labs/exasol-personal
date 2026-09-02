// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts/runtimeartifactstest"
)

// testManagerContext returns a context carrying a Manager backed by the real
// embedded resource catalog, for tests that exercise code reading the shared
// Manager from context.
func testManagerContext(t *testing.T) context.Context {
	t.Helper()

	return runtimeartifactstest.NewContext(t)
}

func TestResolveBackendKind_UsesExplicitBackend(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeTofu}

	kind, err := resolveBackendKind(manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if kind != backendTypeTofu {
		t.Fatalf("expected kind %q, got %q", backendTypeTofu, kind)
	}
}

func TestResolveBackendKind_FallsBackToTofuForLegacyManifest(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{
		Tofu: &presets.InfrastructureTofu{},
	}

	kind, err := resolveBackendKind(manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if kind != backendTypeTofu {
		t.Fatalf("expected kind %q, got %q", backendTypeTofu, kind)
	}
}

func TestResolveBackendKind_RejectsUnknownBackend(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: "unknown"}

	_, err := resolveBackendKind(manifest)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrUnknownDeploymentType) {
		t.Fatalf("expected ErrUnknownDeploymentType, got %v", err)
	}
}

func TestNewDeploymentBackend_ReturnsTofuBackendForTofuManifest(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{
		Backend: backendTypeTofu,
		Tofu:    &presets.InfrastructureTofu{},
	}

	backend, err := newDeploymentBackend(testManagerContext(t), deployment, manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := backend.(*tofuBackend); !ok {
		t.Fatalf("expected *tofuBackend, got %T", backend)
	}
}

func TestNewDeploymentBackend_AcceptsTofuManifestWithoutTofuSectionAsNoop(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{Backend: backendTypeTofu}

	backend, err := newDeploymentBackend(testManagerContext(t), deployment, manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestTofuBackendConfigurationDefaults_AreEmpty(t *testing.T) {
	t.Parallel()

	backend := newTofuBackend(
		config.NewDeploymentDir(t.TempDir()),
		&presets.InfrastructureManifest{Backend: backendTypeTofu},
		nil,
	)
	defaults, err := backend.ConfigurationDefaults(
		context.Background(),
		map[string]string{"cluster_size": "2"},
	)
	if err != nil {
		t.Fatalf("expected no defaults error, got %v", err)
	}
	if len(defaults) != 0 {
		t.Fatalf("expected no launcher-owned tofu defaults, got %#v", defaults)
	}
}

func TestNewDeploymentBackend_ReturnsLocalBackendForLocalManifest(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	backend, err := newDeploymentBackend(testManagerContext(t), deployment, manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := backend.(*localBackend); !ok {
		t.Fatalf("expected *localBackend, got %T", backend)
	}
}

func TestNewLocalRuntimeForPlatform_SelectsRuntime(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	tests := []struct {
		name   string
		goos   string
		goarch string
		assert func(*testing.T, localruntime.Runtime)
	}{
		{
			name: "mac vm",
			goos: localMacOS, goarch: localMacArch,
			assert: func(t *testing.T, selected localruntime.Runtime) {
				t.Helper()
				if _, ok := selected.(*localruntime.MacVMRuntime); !ok {
					t.Fatalf("expected MacVMRuntime, got %T", selected)
				}
			},
		},
		{
			name: "linux amd64",
			goos: localLinuxOS, goarch: localLinuxAMD64,
			assert: func(t *testing.T, selected localruntime.Runtime) {
				t.Helper()
				assertHostRuntimePlatform(t, selected, localruntime.HostPlatformLinux)
			},
		},
		{
			name: "linux arm64",
			goos: localLinuxOS, goarch: localLinuxARM64,
			assert: func(t *testing.T, selected localruntime.Runtime) {
				t.Helper()
				assertHostRuntimePlatform(t, selected, localruntime.HostPlatformLinux)
			},
		},
		{
			name: "windows amd64",
			goos: localWindowsOS, goarch: localWindowsAMD64,
			assert: func(t *testing.T, selected localruntime.Runtime) {
				t.Helper()
				assertHostRuntimePlatform(t, selected, localruntime.HostPlatformWindows)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selected, err := newLocalRuntimeForPlatform(
				deployment, nil, test.goos, test.goarch,
			)
			if err != nil {
				t.Fatalf("expected runtime selection to succeed, got %v", err)
			}
			test.assert(t, selected)
		})
	}
}

func TestNewLocalRuntimeForPlatform_RejectsUnsupportedPlatform(t *testing.T) {
	t.Parallel()

	_, err := newLocalRuntimeForPlatform(
		config.NewDeploymentDir(t.TempDir()), nil, "freebsd", "amd64",
	)
	if !errors.Is(err, errUnsupportedLocalPlatform) {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}
}

// assertHostRuntimePlatform checks the selector produced a direct-host
// runtime for the expected platform. Linux and Windows share one runtime
// type, so the platform is what distinguishes them.
func assertHostRuntimePlatform(
	t *testing.T,
	selected localruntime.Runtime,
	expected localruntime.HostPlatform,
) {
	t.Helper()
	hostRuntime, ok := selected.(*localruntime.HostRuntime)
	if !ok {
		t.Fatalf("expected HostRuntime, got %T", selected)
	}
	if hostRuntime.Platform() != expected {
		t.Fatalf("expected platform %q, got %q", expected, hostRuntime.Platform())
	}
}
