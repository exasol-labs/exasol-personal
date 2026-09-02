// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

// DeploymentConfigValue describes a single configurable variable as seen by
// the launcher. The Name is the canonical variable name (underscore form, as
// it appears in backend manifests). Use DisplayName() to obtain the
// hyphenated form used in user-facing output and CLI flag names.
type DeploymentConfigValue struct {
	Name       string
	Type       ConfigVariableType
	Value      any
	Default    any
	RawValue   string
	RawDefault string
}

// DisplayName returns the user-facing form of the variable name, where any
// underscores in the canonical name are rendered as hyphens.
func (v DeploymentConfigValue) DisplayName() string {
	return strings.ReplaceAll(v.Name, "_", "-")
}

// PresetIdentityInfo describes the preset identity that owns a configuration
// section. Selector is the stable persisted identity used for comparisons
// (for example "name:aws" or "path:/abs/preset"); DisplayName and
// Description come from the extracted manifest and are informational only.
type PresetIdentityInfo struct {
	Selector    string `json:"selector,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	Path        string `json:"path,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
}

type DeploymentConfigurationSection struct {
	Identity PresetIdentityInfo
	Options  []DeploymentConfigValue
}

type DeploymentConfiguration struct {
	Infrastructure DeploymentConfigurationSection
	Installation   DeploymentConfigurationSection
	ApplyCommand   string
}

const (
	ConfigScopeInfrastructure = "infrastructure"
	ConfigScopeInstallation   = "installation"
)

func GetDeploymentConfiguration(
	ctx context.Context,
	deployment config.DeploymentDir,
	optionNames []string,
) (DeploymentConfiguration, error) {
	var configuration DeploymentConfiguration
	err := withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		var err error
		configuration, err = readDeploymentConfiguration(ctx, deployment)
		if err != nil {
			return err
		}
		configuration, err = filterDeploymentConfiguration(configuration, optionNames)

		return err
	})
	if err != nil {
		return DeploymentConfiguration{}, err
	}

	return configuration, nil
}

func SetDeploymentConfiguration(
	ctx context.Context,
	infraVars map[string]string,
	installVars map[string]string,
	deployment config.DeploymentDir,
) (DeploymentConfiguration, error) {
	if len(infraVars) == 0 && len(installVars) == 0 {
		return DeploymentConfiguration{}, errors.New("no configuration options were provided")
	}

	applyCommand := ""
	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			exasolState, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				return err
			}
			backendKind, err := deploymentBackendKind(deployment)
			if err != nil {
				return err
			}
			if err := WorkflowStatePermitsConfigure(exasolState, backendKind); err != nil {
				return err
			}
			applyCommand, err = configurationApplyCommand(exasolState)
			if err != nil {
				return err
			}

			return writeDeploymentConfigurationPatch(
				ctx,
				deployment,
				exasolState,
				infraVars,
				installVars,
			)
		})
	if err != nil {
		return DeploymentConfiguration{}, err
	}

	configuration, err := GetDeploymentConfiguration(ctx, deployment, nil)
	configuration.ApplyCommand = applyCommand

	return configuration, err
}

//nolint:revive // resetAll mirrors the command-level --all flag.
func ResetDeploymentConfiguration(
	ctx context.Context,
	deployment config.DeploymentDir,
	optionNames []string,
	resetAll bool,
) (DeploymentConfiguration, error) {
	if err := validateResetSelection(optionNames, resetAll); err != nil {
		return DeploymentConfiguration{}, err
	}

	applyCommand := ""
	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			var err error
			applyCommand, err = resetDeploymentConfigurationLocked(
				ctx, deployment, optionNames, resetAll,
			)

			return err
		})
	if err != nil {
		return DeploymentConfiguration{}, err
	}

	configuration, err := GetDeploymentConfiguration(ctx, deployment, nil)
	configuration.ApplyCommand = applyCommand

	return configuration, err
}

// validateResetSelection rejects the two invalid combinations of --all and
// explicit option names, before any lock is taken.
//
//nolint:revive // resetAll mirrors the command-level --all flag.
func validateResetSelection(optionNames []string, resetAll bool) error {
	if !resetAll && len(optionNames) == 0 {
		return errors.New("provide option names to reset, or pass --all")
	}
	if resetAll && len(optionNames) > 0 {
		return errors.New("pass either --all or option names, not both")
	}

	return nil
}

func resetDeploymentConfigurationLocked(
	ctx context.Context,
	deployment config.DeploymentDir,
	optionNames []string,
	resetAll bool,
) (string, error) {
	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return "", err
	}
	backendKind, err := deploymentBackendKind(deployment)
	if err != nil {
		return "", err
	}
	if err := WorkflowStatePermitsConfigure(exasolState, backendKind); err != nil {
		return "", err
	}
	applyCommand, err := configurationApplyCommand(exasolState)
	if err != nil {
		return "", err
	}

	configuration, err := readDeploymentConfiguration(ctx, deployment)
	if err != nil {
		return "", err
	}
	backendDefaults, err := configurationDefaultsForReset(
		ctx, deployment, configuration, optionNames, resetAll,
	)
	if err != nil {
		return "", err
	}
	configuration, err = applyConfigurationReset(configuration, optionNames, resetAll)
	if err != nil {
		return "", err
	}
	applyInfrastructureResetDefaults(
		configuration.Infrastructure.Options,
		backendDefaults,
		optionNames,
		resetAll,
	)

	if err := writeDeploymentConfiguration(
		ctx,
		deployment,
		exasolState,
		configuration,
	); err != nil {
		return "", err
	}

	return applyCommand, nil
}

func deploymentBackendKind(deployment config.DeploymentDir) (string, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return "", err
	}

	return resolveBackendKind(manifest)
}

func configurationApplyCommand(exasolState *config.ExasolPersonalState) (string, error) {
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		return "", err
	}
	if _, ok := workflowState.(*config.WorkflowStateStopped); ok {
		return "start", nil
	}

	return "deploy", nil
}

func configurationDefaultsForReset(
	ctx context.Context,
	deployment config.DeploymentDir,
	configuration DeploymentConfiguration,
	optionNames []string,
	resetAll bool,
) (map[string]string, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, err
	}
	backend, err := newDeploymentBackend(ctx, deployment, manifest)
	if err != nil {
		return nil, err
	}

	supplied := make(map[string]string)
	for _, value := range configuration.Infrastructure.Options {
		if configurationOptionSelected(value.Name, optionNames, resetAll) {
			continue
		}
		supplied[value.Name] = value.RawValue
	}

	return backend.ConfigurationDefaults(ctx, supplied)
}

func applyInfrastructureResetDefaults(
	values []DeploymentConfigValue,
	defaults map[string]string,
	optionNames []string,
	resetAll bool,
) {
	for idx := range values {
		if !configurationOptionSelected(values[idx].Name, optionNames, resetAll) {
			continue
		}
		if rawDefault, exists := defaults[values[idx].Name]; exists {
			values[idx].RawValue = rawDefault
		}
	}
}

func configurationOptionSelected(name string, optionNames []string, resetAll bool) bool {
	if resetAll {
		return true
	}
	normalized := normalizeConfigOptionName(name)
	for _, selected := range optionNames {
		if normalizeConfigOptionName(selected) == normalized {
			return true
		}
	}

	return false
}

// applyConfigurationReset resets either the explicitly selected options or,
// when resetAll is set, every option in the configuration.
//
//nolint:revive // resetAll mirrors the command-level --all flag.
func applyConfigurationReset(
	configuration DeploymentConfiguration,
	optionNames []string,
	resetAll bool,
) (DeploymentConfiguration, error) {
	if resetAll {
		resetAllConfigurationValues(configuration.Infrastructure.Options)
		resetAllConfigurationValues(configuration.Installation.Options)

		return configuration, nil
	}

	return resetSelectedDeploymentConfiguration(configuration, optionNames)
}

func readDeploymentConfiguration(
	ctx context.Context,
	deployment config.DeploymentDir,
) (DeploymentConfiguration, error) {
	infraManifest, installManifest, err := readExtractedManifests(deployment)
	if err != nil {
		return DeploymentConfiguration{}, err
	}
	backend, err := newDeploymentBackend(ctx, deployment, infraManifest)
	if err != nil {
		return DeploymentConfiguration{}, err
	}
	infraValues, err := backend.ReadConfiguration()
	if err != nil {
		return DeploymentConfiguration{}, err
	}
	installValues, err := readInstallationConfigurationValues(deployment, installManifest)
	if err != nil {
		return DeploymentConfiguration{}, err
	}
	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return DeploymentConfiguration{}, err
	}
	presetIdentities, backfilled, err := resolvePresetIdentity(ctx, exasolState, deployment)
	if err != nil {
		return DeploymentConfiguration{}, err
	}
	if backfilled {
		if err := config.WriteExasolPersonalState(exasolState, deployment); err != nil {
			return DeploymentConfiguration{}, err
		}
	}
	sortConfigurationValues(infraValues)
	sortConfigurationValues(installValues)

	return DeploymentConfiguration{
		Infrastructure: DeploymentConfigurationSection{
			Identity: presetIdentityInfo(
				presetIdentities.infrastructure,
				infrastructureManifestName(infraManifest),
				infrastructureManifestDescription(infraManifest),
			),
			Options: infraValues,
		},
		Installation: DeploymentConfigurationSection{
			Identity: presetIdentityInfo(
				presetIdentities.installation,
				installationManifestName(installManifest),
				installationManifestDescription(installManifest),
			),
			Options: installValues,
		},
	}, nil
}

func infrastructureManifestName(manifest *presets.InfrastructureManifest) string {
	if manifest == nil {
		return ""
	}

	return strings.TrimSpace(manifest.Name)
}

func infrastructureManifestDescription(manifest *presets.InfrastructureManifest) string {
	if manifest == nil {
		return ""
	}

	return strings.TrimSpace(manifest.Description)
}

func installationManifestName(manifest *presets.InstallManifest) string {
	if manifest == nil {
		return ""
	}

	return strings.TrimSpace(manifest.Name)
}

func installationManifestDescription(manifest *presets.InstallManifest) string {
	if manifest == nil {
		return ""
	}

	return strings.TrimSpace(manifest.Description)
}

func presetIdentityInfo(
	selector, displayName, description string,
) PresetIdentityInfo {
	info := PresetIdentityInfo{
		Selector:    strings.TrimSpace(selector),
		DisplayName: strings.TrimSpace(displayName),
		Description: strings.TrimSpace(description),
	}
	kind, value, ok := strings.Cut(info.Selector, ":")
	if !ok {
		return info
	}
	info.Kind = strings.TrimSpace(kind)
	switch info.Kind {
	case "name":
		info.Name = strings.TrimSpace(value)
	case "path":
		info.Path = strings.TrimSpace(value)
	default:
		// Keep Selector/Kind for forward-compatible identity kinds.
	}

	return info
}

func filterDeploymentConfiguration(
	configuration DeploymentConfiguration,
	optionNames []string,
) (DeploymentConfiguration, error) {
	if len(optionNames) == 0 {
		return configuration, nil
	}

	selected := map[string]struct{}{}
	for _, optionName := range optionNames {
		selected[normalizeConfigOptionName(optionName)] = struct{}{}
	}

	filtered := DeploymentConfiguration{
		Infrastructure: DeploymentConfigurationSection{
			Identity: configuration.Infrastructure.Identity,
			Options:  filterConfigurationValues(configuration.Infrastructure.Options, selected),
		},
		Installation: DeploymentConfigurationSection{
			Identity: configuration.Installation.Identity,
			Options:  filterConfigurationValues(configuration.Installation.Options, selected),
		},
	}
	for _, optionName := range optionNames {
		if !configurationContainsOption(filtered, optionName) {
			return DeploymentConfiguration{}, fmt.Errorf(
				"unknown configuration option %q", optionName,
			)
		}
	}

	return filtered, nil
}

func filterConfigurationValues(
	values []DeploymentConfigValue,
	selected map[string]struct{},
) []DeploymentConfigValue {
	filtered := make([]DeploymentConfigValue, 0, len(values))
	for _, value := range values {
		if _, ok := selected[normalizeConfigOptionName(value.Name)]; ok {
			filtered = append(filtered, value)
		}
	}

	return filtered
}

func configurationContainsOption(configuration DeploymentConfiguration, optionName string) bool {
	normalized := normalizeConfigOptionName(optionName)
	return configurationValuesContainOption(configuration.Infrastructure.Options, normalized) ||
		configurationValuesContainOption(configuration.Installation.Options, normalized)
}

func configurationValuesContainOption(values []DeploymentConfigValue, normalized string) bool {
	for _, value := range values {
		if normalizeConfigOptionName(value.Name) == normalized {
			return true
		}
	}

	return false
}

func resetSelectedDeploymentConfiguration(
	configuration DeploymentConfiguration,
	optionNames []string,
) (DeploymentConfiguration, error) {
	selected := map[string]struct{}{}
	for _, optionName := range optionNames {
		selected[normalizeConfigOptionName(optionName)] = struct{}{}
	}
	found := map[string]struct{}{}
	resetSelectedConfigurationValues(configuration.Infrastructure.Options, selected, found)
	resetSelectedConfigurationValues(configuration.Installation.Options, selected, found)

	missing := make([]string, 0, len(selected))
	for optionName := range selected {
		if _, ok := found[optionName]; !ok {
			missing = append(missing, optionName)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)

		return DeploymentConfiguration{}, fmt.Errorf("unknown configuration option %q", missing[0])
	}

	return configuration, nil
}

func resetSelectedConfigurationValues(
	values []DeploymentConfigValue,
	selected map[string]struct{},
	found map[string]struct{},
) {
	for idx := range values {
		normalized := normalizeConfigOptionName(values[idx].Name)
		if _, ok := selected[normalized]; !ok {
			continue
		}
		values[idx].RawValue = values[idx].RawDefault
		values[idx].Value = values[idx].Default
		found[normalized] = struct{}{}
	}
}

func resetAllConfigurationValues(values []DeploymentConfigValue) {
	for idx := range values {
		values[idx].RawValue = values[idx].RawDefault
		values[idx].Value = values[idx].Default
	}
}

func newDeploymentConfigurationFromRaw(
	infraVars, installVars map[string]string,
) DeploymentConfiguration {
	return DeploymentConfiguration{
		Infrastructure: DeploymentConfigurationSection{Options: rawConfigValues(infraVars)},
		Installation:   DeploymentConfigurationSection{Options: rawConfigValues(installVars)},
	}
}

func rawConfigValues(values map[string]string) []DeploymentConfigValue {
	result := make([]DeploymentConfigValue, 0, len(values))
	for name, rawValue := range values {
		result = append(result, DeploymentConfigValue{
			Name:     name,
			RawValue: rawValue,
		})
	}

	return result
}

func configValuesRawMap(values []DeploymentConfigValue) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		result[name] = value.RawValue
	}

	return result
}

func writeDeploymentConfigurationPatch(
	ctx context.Context,
	deployment config.DeploymentDir,
	exasolState *config.ExasolPersonalState,
	infraVars map[string]string,
	installVars map[string]string,
) error {
	configuration, err := readDeploymentConfiguration(ctx, deployment)
	if err != nil {
		return err
	}
	configuration.Infrastructure.Options = patchConfigurationValues(
		configuration.Infrastructure.Options,
		infraVars,
	)
	configuration.Installation.Options = patchConfigurationValues(
		configuration.Installation.Options,
		installVars,
	)

	return writeDeploymentConfiguration(
		ctx,
		deployment,
		exasolState,
		configuration,
	)
}

func patchConfigurationValues(
	values []DeploymentConfigValue,
	overrides map[string]string,
) []DeploymentConfigValue {
	for name, rawValue := range overrides {
		matched := false
		for idx := range values {
			if values[idx].Name != name {
				continue
			}
			values[idx].RawValue = rawValue
			matched = true

			break
		}
		if !matched {
			values = append(values, DeploymentConfigValue{
				Name:     name,
				RawValue: rawValue,
			})
		}
	}

	return values
}

func readInstallationConfigurationValues(
	deployment config.DeploymentDir,
	manifest *presets.InstallManifest,
) ([]DeploymentConfigValue, error) {
	if !manifestHasInstallationVariables(manifest) {
		return []DeploymentConfigValue{}, nil
	}

	currentValues, err := readCurrentInstallationVariables(deployment, manifest)
	if err != nil {
		return nil, err
	}

	values := make([]DeploymentConfigValue, 0, len(manifest.Variables.Vars))
	for name, def := range manifest.Variables.Vars {
		value, included, err := installationConfigValue(name, def, currentValues)
		if err != nil {
			return nil, err
		}
		if included {
			values = append(values, value)
		}
	}

	return values, nil
}

func manifestHasInstallationVariables(manifest *presets.InstallManifest) bool {
	return manifest != nil && manifest.Variables != nil && len(manifest.Variables.Vars) > 0
}

// installationConfigValue builds the DeploymentConfigValue for a single
// installation variable definition. included is false for names that should
// be skipped entirely (blank, nil definition, or reserved).
func installationConfigValue(
	name string,
	def *presets.VariableDef,
	currentValues map[string]any,
) (DeploymentConfigValue, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" || def == nil || isReservedInstallationVariableName(name) {
		return DeploymentConfigValue{}, false, nil
	}

	defaultValue, err := def.DefaultScalar()
	if err != nil {
		return DeploymentConfigValue{}, false, fmt.Errorf(
			"invalid default for installation variable %q: %w", name, err,
		)
	}
	effectiveType, err := def.EffectiveType()
	if err != nil {
		return DeploymentConfigValue{}, false, fmt.Errorf(
			"invalid definition of installation variable %q: %w", name, err,
		)
	}

	currentValue := defaultValue
	if current, ok := currentValues[name]; ok {
		currentValue = current
	}
	rawValue, err := scalarToRawString(currentValue)
	if err != nil {
		return DeploymentConfigValue{}, false, fmt.Errorf(
			"invalid current value for installation variable %q: %w", name, err,
		)
	}
	rawDefault, err := scalarToRawString(defaultValue)
	if err != nil {
		return DeploymentConfigValue{}, false, fmt.Errorf(
			"invalid default value for installation variable %q: %w", name, err,
		)
	}

	return DeploymentConfigValue{
		Name:       name,
		Type:       installationVariableType(effectiveType),
		Value:      currentValue,
		Default:    defaultValue,
		RawValue:   rawValue,
		RawDefault: rawDefault,
	}, true, nil
}

func readCurrentInstallationVariables(
	deployment config.DeploymentDir,
	manifest *presets.InstallManifest,
) (map[string]any, error) {
	result := map[string]any{}
	if manifest == nil || manifest.Variables == nil {
		return result, nil
	}
	outputRel := strings.TrimSpace(manifest.Variables.OutputFile)
	if outputRel == "" {
		return result, nil
	}
	outPath := filepath.Join(deployment.InstallationDir(), filepath.Clean(outputRel))
	data, err := os.ReadFile(outPath)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to read installation variables: %w", err)
	}

	return result, nil
}

func scalarToRawString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool:
		return strconv.FormatBool(typed), nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case json.Number:
		return typed.String(), nil
	default:
		return "", fmt.Errorf("unsupported scalar type %T", value)
	}
}

// normalizeConfigOptionName converts a user-supplied option name (either
// hyphen or underscore form) into the canonical underscore form used
// internally as DeploymentConfigValue.Name.
func normalizeConfigOptionName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "-", "_")
}

func installationVariableType(raw string) ConfigVariableType {
	switch strings.TrimSpace(raw) {
	case string(ConfigVariableTypeBool):
		return ConfigVariableTypeBool
	case string(ConfigVariableTypeNumber):
		return ConfigVariableTypeNumber
	default:
		return ConfigVariableTypeString
	}
}

func sortConfigurationValues(values []DeploymentConfigValue) {
	slices.SortFunc(values, func(left, right DeploymentConfigValue) int {
		return strings.Compare(left.Name, right.Name)
	})
}
