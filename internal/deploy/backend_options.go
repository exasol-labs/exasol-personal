// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

var (
	ErrUnsupportedDeployOption = errors.New("unsupported deployment option")
	ErrInvalidDeployOption     = errors.New("invalid deployment option")
)

// DeployOptionDefinition declares one option a backend accepts for a deploy
// operation. Name is the option as written on the command line.
type DeployOptionDefinition struct {
	Name        string
	Description string
	Type        ConfigVariableType
}

// DeployOptionResolution pairs a backend's options with the label identifying the
// preset they came from: an embedded preset name, a preset path, or a manifest name.
type DeployOptionResolution struct {
	Options     []DeployOptionDefinition
	PresetLabel string
}

// ResolveDeployOptions reads the preset's manifest only, so `install` can offer a
// backend's options before a deployment directory exists.
func ResolveDeployOptions(
	ctx context.Context,
	preset PresetRef,
) (DeployOptionResolution, error) {
	manifest, label, err := readInfrastructureManifestForPreset(ctx, preset)
	if err != nil {
		return DeployOptionResolution{PresetLabel: label}, err
	}

	options, err := deployOptionsForManifest(manifest)
	if err != nil {
		return DeployOptionResolution{PresetLabel: label}, err
	}

	return DeployOptionResolution{Options: options, PresetLabel: label}, nil
}

func ResolveDeployOptionsFromDeployment(
	deployment config.DeploymentDir,
) (DeployOptionResolution, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return DeployOptionResolution{}, err
	}

	options, err := deployOptionsForManifest(manifest)
	if err != nil {
		return DeployOptionResolution{PresetLabel: manifest.Name}, err
	}

	return DeployOptionResolution{Options: options, PresetLabel: manifest.Name}, nil
}

// AllDeployOptions lets the CLI tell an option no backend knows from one the
// selected backend does not support.
func AllDeployOptions() []DeployOptionDefinition {
	options := make([]DeployOptionDefinition, 0)
	for _, descriptor := range backendDescriptors {
		options = append(options, descriptor.deployOptions...)
	}

	slices.SortFunc(options, func(left, right DeployOptionDefinition) int {
		return strings.Compare(left.Name, right.Name)
	})

	return options
}

func deployOptionsForManifest(
	manifest *presets.InfrastructureManifest,
) ([]DeployOptionDefinition, error) {
	descriptor, err := backendDescriptorForManifest(manifest)
	if err != nil {
		return nil, err
	}

	return descriptor.deployOptions, nil
}

// validateDeployOptionsForManifest rejects options the manifest's backend never
// declared, so a backend cannot silently drop one that reaches it.
func validateDeployOptionsForManifest(
	manifest *presets.InfrastructureManifest,
	options DeployOptions,
) error {
	descriptor, err := backendDescriptorForManifest(manifest)
	if err != nil {
		return err
	}

	declared := make(map[string]struct{}, len(descriptor.deployOptions))
	for _, option := range descriptor.deployOptions {
		declared[option.Name] = struct{}{}
	}

	supplied := make([]string, 0, len(options.BackendOptions))
	for name := range options.BackendOptions {
		supplied = append(supplied, name)
	}
	slices.Sort(supplied)

	for _, name := range supplied {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf(
				"%w: the %s backend does not support --%s",
				ErrUnsupportedDeployOption, descriptor.kind, name,
			)
		}
	}

	return nil
}

// boolOption reads false for an option the user did not supply.
func (options DeployOptions) boolOption(name string) (bool, error) {
	raw := strings.TrimSpace(options.BackendOptions[name])
	if raw == "" {
		return false, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf(
			"%w: --%s expects a boolean, got %q", ErrInvalidDeployOption, name, raw,
		)
	}

	return value, nil
}
