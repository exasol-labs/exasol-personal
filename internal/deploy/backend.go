// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	backendTypeTofu  = "tofu"
	backendTypeLocal = "local"
)

// DeployOptions carries backend-agnostic options for a single deploy invocation.
// Individual backends interpret the options they understand and ignore the rest.
type DeployOptions struct {
	// UpdateDependencyLockfile signals that the backend may update any
	// dependency lockfile during initialization (e.g. OpenTofu's
	// .terraform.lock.hcl). When false, the backend must treat any such
	// lockfile as read-only.
	UpdateDependencyLockfile bool

	// RuntimePreparation carries the approval and progress policy applied
	// before the deployment records an operation in progress.
	RuntimePreparation localruntime.PrepareOptions
}

// StartOptions carries options for a single start invocation.
type StartOptions struct {
	WaitTimeoutSeconds int
	RuntimePreparation localruntime.PrepareOptions
}

// deploymentBackend exposes the lifecycle and configuration operations the
// launcher needs from an infrastructure backend, bound to a specific
// deployment directory and its infrastructure manifest.
//
// All methods operate against the deployment and manifest that were supplied
// when the backend was constructed (see newDeploymentBackend).
// nolint: interfacebloat
type deploymentBackend interface {
	// Prepare satisfies host prerequisites. It runs before the deployment
	// records an operation in progress, so a declined or failed
	// prerequisite leaves the deployment retryable.
	Prepare(
		ctx context.Context,
		out, outErr io.Writer,
		options localruntime.PrepareOptions,
	) error
	ValidateEnvironment() error
	SetupWorkspace(ctx context.Context) error
	ConfigurationDefaults(
		ctx context.Context,
		supplied map[string]string,
	) (map[string]string, error)
	Configure(
		ctx context.Context,
		overrides map[string]string,
		metadata DeploymentMetadata,
		layout DeploymentLayout,
	) error
	ReadConfiguration() ([]DeploymentConfigValue, error)
	ReadDeploymentConfigVariables() (map[string]ConfigVariableDefinition, error)
	OpenHostShell(ctx context.Context, selectedNode string) error
	OpenCOSShell(ctx context.Context) error
	Deploy(ctx context.Context, out, outErr io.Writer, options DeployOptions) error
	Start(ctx context.Context, out, outErr io.Writer, waitTimeoutSeconds int) error
	Stop(ctx context.Context, out, outErr io.Writer) error
	Destroy(ctx context.Context, out, outErr io.Writer) error
}

func resolveBackendKind(manifest *presets.InfrastructureManifest) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("%w: missing infrastructure manifest", ErrUnknownDeploymentType)
	}

	backend := strings.TrimSpace(manifest.Backend)
	if backend == "" && manifest.Tofu != nil {
		backend = backendTypeTofu
	}

	switch backend {
	case backendTypeTofu, backendTypeLocal:
		return backend, nil
	case "":
		return "", fmt.Errorf(
			"%w: infrastructure manifest does not declare a supported backend",
			ErrUnknownDeploymentType,
		)
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownDeploymentType, backend)
	}
}

func newDeploymentBackendForDeployment(
	ctx context.Context,
	deployment config.DeploymentDir,
) (deploymentBackend, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, err
	}

	return newDeploymentBackend(ctx, deployment, manifest)
}

func newDeploymentBackend(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
) (deploymentBackend, error) {
	kind, err := resolveBackendKind(manifest)
	if err != nil {
		return nil, err
	}

	manager := runtimeartifacts.FromContext(ctx)

	switch kind {
	case backendTypeTofu:
		return newTofuBackend(deployment, manifest, manager), nil
	case backendTypeLocal:
		localRuntime, err := newLocalRuntime(deployment, manager)
		if err != nil {
			return nil, err
		}

		return newLocalBackend(deployment, manifest, localRuntime), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownDeploymentType, kind)
	}
}

// readInfrastructurePresetConfigVariables exposes a preset's configurable
// infrastructure variables without requiring a deployment directory.
//
// It is used by the CLI to render preset-specific flags on `init` and
// `install`, before any deployment exists.
func readInfrastructurePresetConfigVariables(
	ctx context.Context,
	preset PresetRef,
	manifest *presets.InfrastructureManifest,
) (map[string]ConfigVariableDefinition, error) {
	kind, err := resolveBackendKind(manifest)
	if err != nil {
		return nil, err
	}

	switch kind {
	case backendTypeTofu:
		if manifest.Tofu == nil {
			return map[string]ConfigVariableDefinition{}, nil
		}

		return readTofuPresetConfigVariables(ctx, preset, *manifest.Tofu)
	case backendTypeLocal:
		return localConfigVariableDefinitions(ctx, manifest), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownDeploymentType, kind)
	}
}

func newLocalRuntime(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
) (localruntime.Runtime, error) {
	return newLocalRuntimeForPlatform(deployment, manager, runtime.GOOS, runtime.GOARCH)
}

func newLocalRuntimeForPlatform(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
	goos, goarch string,
) (localruntime.Runtime, error) {
	switch {
	case goos == localMacOS && goarch == localMacArch:
		return localruntime.NewMacVMRuntime(deployment, manager), nil
	case goos == localLinuxOS && (goarch == localLinuxAMD64 || goarch == localLinuxARM64):
		return localruntime.NewHostLinuxRuntime(deployment, manager), nil
	case goos == localWindowsOS && goarch == localWindowsAMD64:
		return localruntime.NewHostWindowsRuntime(deployment, manager), nil
	default:
		return nil, fmt.Errorf(
			"%w (current platform: %s/%s)", errUnsupportedLocalPlatform, goos, goarch,
		)
	}
}
