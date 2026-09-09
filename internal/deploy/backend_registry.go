// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// backendDescriptor declares one infrastructure backend. Everything in it is
// reachable from a manifest alone, so the CLI can offer a backend's options before
// a deployment directory exists to construct that backend from.
type backendDescriptor struct {
	kind string

	// A backend that declares no options accepts none.
	deployOptions []DeployOptionDefinition

	newBackend func(
		deployment config.DeploymentDir,
		manifest *presets.InfrastructureManifest,
		manager *runtimeartifacts.Manager,
	) (deploymentBackend, error)

	presetConfigVariables func(
		ctx context.Context,
		preset PresetRef,
		manifest *presets.InfrastructureManifest,
	) (map[string]ConfigVariableDefinition, error)
}

var backendDescriptors = map[string]backendDescriptor{
	backendTypeTofu:  tofuBackendDescriptor,
	backendTypeLocal: localBackendDescriptor,
}

func backendDescriptorForKind(kind string) (backendDescriptor, error) {
	descriptor, ok := backendDescriptors[kind]
	if !ok {
		return backendDescriptor{}, fmt.Errorf("%w: %q", ErrUnknownDeploymentType, kind)
	}

	return descriptor, nil
}

func backendDescriptorForManifest(
	manifest *presets.InfrastructureManifest,
) (backendDescriptor, error) {
	kind, err := resolveBackendKind(manifest)
	if err != nil {
		return backendDescriptor{}, err
	}

	return backendDescriptorForKind(kind)
}
