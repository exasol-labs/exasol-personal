// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

// Registering a descriptor must be enough to make a new backend usable.
func TestBackendDescriptors_AreComplete(t *testing.T) {
	t.Parallel()

	for kind, descriptor := range backendDescriptors {
		if descriptor.kind != kind {
			t.Errorf("descriptor registered as %q reports kind %q", kind, descriptor.kind)
		}
		if descriptor.newBackend == nil {
			t.Errorf("backend %q cannot be constructed", kind)
		}
		if descriptor.presetConfigVariables == nil {
			t.Errorf("backend %q declares no preset configuration variables", kind)
		}
		for _, option := range descriptor.deployOptions {
			if option.Name == "" || option.Description == "" {
				t.Errorf("backend %q declares an unusable option %+v", kind, option)
			}
		}
	}
}

// Otherwise the CLI cannot tell which backend interprets a supplied value.
func TestBackendDescriptors_DeclareDistinctDeployOptions(t *testing.T) {
	t.Parallel()

	owners := map[string]string{}
	for kind, descriptor := range backendDescriptors {
		for _, option := range descriptor.deployOptions {
			if owner, ok := owners[option.Name]; ok {
				t.Errorf(
					"option %q is declared by both %q and %q", option.Name, owner, kind,
				)
			}
			owners[option.Name] = kind
		}
	}
}

func TestBackendDescriptorForKind_RejectsUnregisteredKind(t *testing.T) {
	t.Parallel()

	_, err := backendDescriptorForKind("unregistered")
	if !errors.Is(err, ErrUnknownDeploymentType) {
		t.Fatalf("expected ErrUnknownDeploymentType, got %v", err)
	}
}

func TestBackendDescriptorForManifest_ResolvesLegacyTofuManifest(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Tofu: &presets.InfrastructureTofu{}}

	descriptor, err := backendDescriptorForManifest(manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if descriptor.kind != backendTypeTofu {
		t.Fatalf("expected the tofu backend, got %q", descriptor.kind)
	}
}
