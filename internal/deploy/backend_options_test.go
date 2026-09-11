// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

const tofuInfrastructureManifest = `name: Test Cloud
description: Test cloud infrastructure.
backend: tofu

tofu:
  variablesFile: variables.tf
`

const localInfrastructureManifest = `name: Test Local
description: Test local infrastructure.
backend: local

local:
  cpuCount: 2
`

// The manifest is all that resolving a deployment's backend needs.
func writeDeploymentWithInfrastructureManifest(
	t *testing.T, manifest string,
) config.DeploymentDir {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	path := deployment.InfrastructureManifestPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	return deployment
}

func TestAllDeployOptions_DeclaresTofuLockfileOptionAsBoolean(t *testing.T) {
	t.Parallel()

	options := AllDeployOptions()

	var found *DeployOptionDefinition
	for index, option := range options {
		if option.Name == tofuUpdateLockfileOption {
			found = &options[index]
		}
	}
	if found == nil {
		t.Fatalf("expected %q among %v", tofuUpdateLockfileOption, options)
	}
	if found.Type != ConfigVariableTypeBool {
		t.Fatalf("expected a boolean option, got %q", found.Type)
	}
	if found.Description == "" {
		t.Fatal("expected a description the CLI can render as flag usage")
	}
}

func TestAllDeployOptions_IsOrderedByName(t *testing.T) {
	t.Parallel()

	options := AllDeployOptions()
	for index := 1; index < len(options); index++ {
		if options[index-1].Name > options[index].Name {
			t.Fatalf("expected options ordered by name, got %v", options)
		}
	}
}

func TestResolveDeployOptions_TofuPresetDeclaresLockfileOption(t *testing.T) {
	t.Parallel()

	presetDir := t.TempDir()
	writePresetInfrastructureManifest(t, presetDir, tofuInfrastructureManifest)

	resolution, err := ResolveDeployOptions(
		context.Background(), PresetRef{Path: presetDir},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resolution.Options) != 1 ||
		resolution.Options[0].Name != tofuUpdateLockfileOption {
		t.Fatalf("expected the tofu lockfile option, got %v", resolution.Options)
	}
	if resolution.PresetLabel != presetDir {
		t.Fatalf("expected label %q, got %q", presetDir, resolution.PresetLabel)
	}
}

func TestResolveDeployOptions_LocalPresetDeclaresNoOptions(t *testing.T) {
	t.Parallel()

	presetDir := t.TempDir()
	writePresetInfrastructureManifest(t, presetDir, localInfrastructureManifest)

	resolution, err := ResolveDeployOptions(
		context.Background(), PresetRef{Path: presetDir},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resolution.Options) != 0 {
		t.Fatalf("expected no options, got %v", resolution.Options)
	}
}

func writePresetInfrastructureManifest(t *testing.T, dir, manifest string) {
	t.Helper()

	path := filepath.Join(dir, presets.InfrastructureManifestFilename)
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write preset manifest: %v", err)
	}
}

func TestResolveDeployOptionsFromDeployment_UsesTheDeploymentsBackend(t *testing.T) {
	t.Parallel()

	deployment := writeDeploymentWithInfrastructureManifest(t, tofuInfrastructureManifest)

	resolution, err := ResolveDeployOptionsFromDeployment(deployment)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resolution.Options) != 1 ||
		resolution.Options[0].Name != tofuUpdateLockfileOption {
		t.Fatalf("expected the tofu lockfile option, got %v", resolution.Options)
	}
	if resolution.PresetLabel != "Test Cloud" {
		t.Fatalf("expected the manifest name as label, got %q", resolution.PresetLabel)
	}
}

func TestResolveDeployOptionsFromDeployment_LocalDeploymentDeclaresNoOptions(t *testing.T) {
	t.Parallel()

	deployment := writeDeploymentWithInfrastructureManifest(t, localInfrastructureManifest)

	resolution, err := ResolveDeployOptionsFromDeployment(deployment)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resolution.Options) != 0 {
		t.Fatalf("expected no options, got %v", resolution.Options)
	}
}

func TestResolveDeployOptionsFromDeployment_ReportsMissingManifest(t *testing.T) {
	t.Parallel()

	_, err := ResolveDeployOptionsFromDeployment(config.NewDeploymentDir(t.TempDir()))
	if err == nil {
		t.Fatal("expected an error for a directory without an infrastructure manifest")
	}
}

func TestValidateDeployOptionsForManifest_AcceptsDeclaredOption(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{
		Backend: backendTypeTofu,
		Tofu:    &presets.InfrastructureTofu{},
	}
	options := DeployOptions{
		BackendOptions: map[string]string{tofuUpdateLockfileOption: "true"},
	}

	if err := validateDeployOptionsForManifest(manifest, options); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateDeployOptionsForManifest_RejectsOptionOfAnotherBackend(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}
	options := DeployOptions{
		BackendOptions: map[string]string{tofuUpdateLockfileOption: "true"},
	}

	err := validateDeployOptionsForManifest(manifest, options)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, ErrUnsupportedDeployOption) {
		t.Fatalf("expected ErrUnsupportedDeployOption, got %v", err)
	}
	if !strings.Contains(err.Error(), tofuUpdateLockfileOption) ||
		!strings.Contains(err.Error(), backendTypeLocal) {
		t.Fatalf("expected the option and the backend to be named, got %q", err.Error())
	}
}

func TestValidateDeployOptionsForManifest_AcceptsNoOptions(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	if err := validateDeployOptionsForManifest(manifest, DeployOptions{}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateDeployOptionsForManifest_ReportsUnknownBackend(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: "unknown"}

	err := validateDeployOptionsForManifest(manifest, DeployOptions{})
	if !errors.Is(err, ErrUnknownDeploymentType) {
		t.Fatalf("expected ErrUnknownDeploymentType, got %v", err)
	}
}

func TestDeployOptionsBoolOption(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		raw       map[string]string
		expected  bool
		expectErr bool
	}{
		{name: "not supplied", raw: nil},
		{name: "empty value", raw: map[string]string{"option": ""}},
		{name: "true", raw: map[string]string{"option": "true"}, expected: true},
		{name: "false", raw: map[string]string{"option": "false"}},
		{name: "invalid", raw: map[string]string{"option": "maybe"}, expectErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			options := DeployOptions{BackendOptions: testCase.raw}

			value, err := options.boolOption("option")
			if testCase.expectErr {
				if !errors.Is(err, ErrInvalidDeployOption) {
					t.Fatalf("expected ErrInvalidDeployOption, got %v", err)
				}

				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if value != testCase.expected {
				t.Fatalf("expected %v, got %v", testCase.expected, value)
			}
		})
	}
}
