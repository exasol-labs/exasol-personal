// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// tofuReservedVariableNames lists the infrastructure variable names that the
// launcher injects automatically when configuring the tofu backend. They are
// not user-configurable and must never appear in the configuration surface
// exposed by ReadConfiguration / ReadDeploymentConfigVariables.
//
// This list is a tofu-backend implementation detail: a future backend may
// reserve a completely different set of names (or none at all).
var tofuReservedVariableNames = map[string]struct{}{
	"deployment_id":               {},
	"cluster_identity":            {},
	"deployment_created_at":       {},
	"infrastructure_artifact_dir": {},
	"installation_preset_dir":     {},
}

// tofuBackend is a deploymentBackend implementation bound to a specific
// deployment directory and the tofu portion of its infrastructure manifest.
//
// It caches the parsed defaults of the preset's variables file so that a
// single high-level operation (e.g. `config set`) does not parse the same file
// repeatedly across calls to ReadConfiguration, ReadDeploymentConfigVariables
// and Configure. A backend instance is intended for use within a single
// operation and is not safe for concurrent use.
type tofuBackend struct {
	deployment config.DeploymentDir
	cfg        *tofu.Config

	defaultsCached bool
	defaults       map[string]*tofu.Variable
	defaultsErr    error
}

func newTofuBackend(
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	manager *runtimeartifacts.Manager,
) *tofuBackend {
	b := &tofuBackend{deployment: deployment}
	if manifest != nil && manifest.Tofu != nil {
		b.cfg = tofu.NewTofuConfigFromDeployment(deployment.Root(), *manifest.Tofu, manager)
	}

	return b
}

// Cloud deployments have no host prerequisites to satisfy.
func (*tofuBackend) Prepare(
	context.Context,
	io.Writer,
	io.Writer,
	localruntime.PrepareOptions,
) error {
	return nil
}

func (*tofuBackend) ValidateEnvironment() error {
	return nil
}

func (b *tofuBackend) SetupWorkspace(_ context.Context) error {
	if !b.hasTofu() {
		slog.Info("tofu: no configuration defined; skipping workspace setup")
		return nil
	}

	return tofu.SetupWorkspace(b.cfg)
}

func (b *tofuBackend) Configure(
	_ context.Context,
	overrides map[string]string,
	metadata DeploymentMetadata,
	layout DeploymentLayout,
) error {
	if !b.hasTofu() {
		slog.Info("tofu: no configuration defined; skipping configuration")
		return nil
	}

	infraVars := make(map[string]string, len(overrides)+len(tofuReservedVariableNames))
	for name, value := range overrides {
		if _, reserved := tofuReservedVariableNames[name]; reserved {
			// Reserved names are launcher-governed; silently drop any caller
			// attempt to override them.
			continue
		}
		infraVars[name] = value
	}
	infraVars["deployment_id"] = string(metadata.ID)
	infraVars["cluster_identity"] = string(metadata.ClusterIdentity)
	infraVars["deployment_created_at"] = metadata.CreatedAt.UTC().Format(time.RFC3339)
	infraVars["infrastructure_artifact_dir"] = string(layout.InfrastructureArtifactDir)
	infraVars["installation_preset_dir"] = string(layout.InstallationPresetDir)

	return tofu.Configure(b.cfg, infraVars)
}

func (b *tofuBackend) ReadConfiguration() ([]DeploymentConfigValue, error) {
	if !b.hasTofu() {
		return []DeploymentConfigValue{}, nil
	}
	defaults, err := b.loadDefaults()
	if err != nil {
		return nil, err
	}

	currentValues, err := b.loadCurrentTofuValues()
	if err != nil {
		return nil, err
	}

	values := make([]DeploymentConfigValue, 0, len(defaults))
	for name, variable := range defaults {
		value, ok, err := tofuConfigValueForVariable(name, variable, currentValues)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		values = append(values, value)
	}

	return values, nil
}

// tofuConfigValueForVariable resolves a single infrastructure variable's
// current and default config values. The second return value reports whether
// the variable is user-facing at all (false for nil entries and reserved
// names, which callers should skip).
func tofuConfigValueForVariable(
	name string,
	variable *tofu.Variable,
	currentValues map[string]cty.Value,
) (DeploymentConfigValue, bool, error) {
	if variable == nil {
		return DeploymentConfigValue{}, false, nil
	}
	if _, reserved := tofuReservedVariableNames[name]; reserved {
		return DeploymentConfigValue{}, false, nil
	}

	value := variable.Value
	if current, ok := currentValues[name]; ok {
		value = current
	}

	scalar, err := ctyScalarToGoValue(value)
	if err != nil {
		return DeploymentConfigValue{}, false, fmt.Errorf(
			"invalid current value for infrastructure variable %q: %w",
			name,
			err,
		)
	}
	defaultScalar, err := ctyScalarToGoValue(variable.Value)
	if err != nil {
		return DeploymentConfigValue{}, false, fmt.Errorf(
			"invalid default value for infrastructure variable %q: %w",
			name,
			err,
		)
	}
	rawValue, err := ctyScalarToRawString(value)
	if err != nil {
		return DeploymentConfigValue{}, false, fmt.Errorf(
			"invalid current value for infrastructure variable %q: %w",
			name,
			err,
		)
	}
	rawDefault, err := ctyScalarToRawString(variable.Value)
	if err != nil {
		return DeploymentConfigValue{}, false, fmt.Errorf(
			"invalid default value for infrastructure variable %q: %w",
			name,
			err,
		)
	}

	return DeploymentConfigValue{
		Name:       name,
		Type:       tofuVariableType(variable.Type),
		Value:      scalar,
		Default:    defaultScalar,
		RawValue:   rawValue,
		RawDefault: rawDefault,
	}, true, nil
}

func (b *tofuBackend) ReadDeploymentConfigVariables() (
	map[string]ConfigVariableDefinition,
	error,
) {
	defaults, err := b.loadDefaults()
	if err != nil {
		return nil, err
	}

	return tofuVariableDefinitions(defaults), nil
}

// readTofuPresetConfigVariables loads the configurable infrastructure
// variables declared by a preset without consulting any deployment directory.
func readTofuPresetConfigVariables(
	ctx context.Context,
	preset PresetRef,
	tofuManifest presets.InfrastructureTofu,
) (map[string]ConfigVariableDefinition, error) {
	var (
		variableData []byte
		filename     string
		err          error
	)
	if preset.IsPath() {
		tofuCfg := tofu.NewTofuConfigFromPreset(preset.Path, tofuManifest)
		filename = tofuCfg.VariablesFile()
		variableData, err = os.ReadFile(filename)
	} else {
		filename = tofuManifest.VariablesFile
		variableData, err = presets.ReadInfrastructureFile(ctx, preset.Name, filename)
	}
	if err != nil {
		return nil, err
	}

	tofuVars, err := tofu.ParseVarFile(variableData, filename)
	if err != nil {
		return nil, err
	}

	return tofuVariableDefinitions(tofuVars), nil
}

func tofuVariableDefinitions(
	tofuVars map[string]*tofu.Variable,
) map[string]ConfigVariableDefinition {
	variables := make(map[string]ConfigVariableDefinition, len(tofuVars))
	for name, variable := range tofuVars {
		if variable == nil {
			continue
		}
		if _, reserved := tofuReservedVariableNames[name]; reserved {
			continue
		}
		variables[name] = ConfigVariableDefinition{
			Name:           name,
			Description:    variable.Description,
			Type:           tofuVariableType(variable.Type),
			DefaultDisplay: ctyDefaultDisplay(variable.Value),
			Required:       variable.Required,
		}
	}

	return variables
}

func tofuVariableType(raw string) ConfigVariableType {
	switch strings.TrimSpace(raw) {
	case string(ConfigVariableTypeBool):
		return ConfigVariableTypeBool
	case string(ConfigVariableTypeNumber):
		return ConfigVariableTypeNumber
	default:
		return ConfigVariableTypeString
	}
}

func ctyDefaultDisplay(value cty.Value) string {
	if value.IsNull() || !value.IsWhollyKnown() {
		return ""
	}
	repr, err := ctyjson.Marshal(value, value.Type())
	if err != nil {
		return "<error>"
	}

	return string(repr)
}

func (b *tofuBackend) OpenHostShell(ctx context.Context, selectedNode string) error {
	sshRemote, err := sshRemoteForNodeUnsafe(b.deployment, selectedNode)
	if err != nil {
		return err
	}

	return sshRemote.Shell(ctx, os.Stdout, os.Stderr)
}

func (b *tofuBackend) OpenCOSShell(ctx context.Context) error {
	sshRemote, err := sshRemoteForNodeUnsafe(b.deployment, "n11")
	if err != nil {
		return err
	}

	cosCommand := "/usr/bin/env bash /opt/exasol_launcher/scripts/connectCos.sh"

	return sshRemote.RunInteractiveCommand(ctx, cosCommand, os.Stdout, os.Stderr)
}

func ctyScalarToRawString(value cty.Value) (string, error) {
	if !value.IsWhollyKnown() || value.IsNull() {
		return "", errors.New("value is unknown or null")
	}
	switch {
	case value.Type() == cty.String:
		return value.AsString(), nil
	case value.Type() == cty.Bool:
		return strconv.FormatBool(value.True()), nil
	case value.Type() == cty.Number:
		float := value.AsBigFloat()

		return float.Text('f', -1), nil
	default:
		return "", fmt.Errorf("unsupported scalar type %s", value.Type().FriendlyName())
	}
}

func ctyScalarToGoValue(value cty.Value) (any, error) {
	raw, err := ctyScalarToRawString(value)
	if err != nil {
		return nil, err
	}
	switch {
	case value.Type() == cty.String:
		return raw, nil
	case value.Type() == cty.Bool:
		return value.True(), nil
	case value.Type() == cty.Number:
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, err
		}

		return parsed, nil
	default:
		return nil, fmt.Errorf("unsupported scalar type %s", value.Type().FriendlyName())
	}
}

func (b *tofuBackend) Deploy(
	ctx context.Context,
	out, outErr io.Writer,
	options DeployOptions,
) error {
	if !b.hasTofu() {
		slog.Info("tofu: no configuration defined; skipping")
		return nil
	}

	slog.Info("beginning deployment with tofu")

	logBuffer := task_runner.NewLogBuffer()
	stdOutWriter := util.CombineWriters(logBuffer, out)
	stdErrWriter := util.CombineWriters(logBuffer, outErr)

	lockfileMode := tofu.LockfileReadonly
	if options.UpdateDependencyLockfile {
		lockfileMode = tofu.LockfileUpdate
	}

	if err := tofu.Initialize(
		ctx, b.cfg, stdOutWriter, stdErrWriter, lockfileMode,
	); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to init", "error", err)

		return err
	}

	if err := tofu.Plan(ctx, b.cfg, stdOutWriter, stdErrWriter); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to plan")

		return err
	}

	if err := tofu.ApplyPlan(ctx, b.cfg, stdOutWriter, stdErrWriter); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to apply")

		return err
	}

	return nil
}

func (b *tofuBackend) Start(
	ctx context.Context,
	out, outErr io.Writer,
	waitTimeoutSeconds int,
) error {
	logBuffer := task_runner.NewLogBuffer()
	output := util.CombineWriters(logBuffer, out)
	errOutput := util.CombineWriters(logBuffer, outErr)

	if err := b.applyAction(ctx, "power_state=running", output, errOutput); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("failed to start deployment")

		return err
	}

	instPollCond := func(ctx context.Context) (bool, error) {
		n11Details, err := Getn11Details(b.deployment)
		if err != nil {
			return false, err
		}
		if n11Details.Host != "" {
			return true, nil
		}

		if err := b.applyAction(ctx, "", output, errOutput); err != nil {
			logBuffer.ReplayLogMessages(ctx)
			slog.Error("ApplyAction failed while refreshing", "error", err)

			return false, err
		}

		return false, nil
	}

	waitCtx, cancel := context.WithTimeout(
		ctx,
		time.Duration(InstanceRefreshTimeoutSeconds)*time.Second,
	)
	defer cancel()

	if err := PollWithBackoff(waitCtx, instPollCond, WaitParams{
		InitialBackoff: StartedInitialBackoff,
		MaxBackoff:     StartedMaxBackoff,
		LogPrefix:      "waiting to update EC2 Resources",
	}); err != nil {
		slog.Error("Updated EC2 resources not available in time")
		return err
	}

	if waitTimeoutSeconds <= 0 {
		waitTimeoutSeconds = StartedDefaultTimeoutSeconds
	}

	waitCtx, cancel = context.WithTimeout(ctx, time.Duration(waitTimeoutSeconds)*time.Second)
	defer cancel()

	if err := WaitForDatabaseStarted(waitCtx, b.deployment); err != nil {
		slog.Error("database did not become operational with timeout", "error", err.Error())
		return err
	}

	return nil
}

func (b *tofuBackend) Stop(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	logBuffer := task_runner.NewLogBuffer()

	if err := b.applyAction(
		ctx,
		"power_state=stopped",
		util.CombineWriters(logBuffer, out),
		util.CombineWriters(logBuffer, outErr),
	); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("failed to stop the deployment")

		return err
	}

	return nil
}

func (b *tofuBackend) Destroy(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	if !b.hasTofu() {
		slog.Info("no tofu configuration defined; skipping destroy")
		return nil
	}

	logBuffer := task_runner.NewLogBuffer()
	output := util.CombineWriters(logBuffer, out)
	errOutput := util.CombineWriters(logBuffer, outErr)
	if err := tofu.Initialize(
		ctx, b.cfg, output, errOutput, tofu.LockfileReadonly,
	); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to init before destroy", "error", err)

		return err
	}

	if err := tofu.Destroy(ctx, b.cfg, output, errOutput); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("failed to destroy cloud resources")

		return err
	}

	return nil
}

func (b *tofuBackend) hasTofu() bool {
	return b.cfg != nil
}

// loadCurrentTofuValues reads the deployment's applied tfvars file, returning
// an empty set (rather than an error) when the deployment has not been
// configured yet.
func (b *tofuBackend) loadCurrentTofuValues() (map[string]cty.Value, error) {
	tfvarsData, err := os.ReadFile(b.cfg.VarsOutputFile())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]cty.Value{}, nil
		}

		return nil, err
	}

	return tofu.ParseVarsValuesFile(tfvarsData, b.cfg.VarsOutputFile())
}

func (b *tofuBackend) loadDefaults() (map[string]*tofu.Variable, error) {
	if !b.hasTofu() {
		return map[string]*tofu.Variable{}, nil
	}
	if b.defaultsCached {
		return b.defaults, b.defaultsErr
	}

	variableData, err := os.ReadFile(b.cfg.VariablesFile())
	if err != nil {
		b.defaultsCached = true
		b.defaultsErr = err

		return nil, err
	}
	defaults, err := tofu.ParseVarFile(variableData, b.cfg.VariablesFile())
	b.defaultsCached = true
	b.defaults = defaults
	b.defaultsErr = err

	return defaults, err
}

func (b *tofuBackend) applyAction(
	ctx context.Context,
	startStopArg string,
	out, outErr io.Writer,
) error {
	if !b.hasTofu() {
		slog.Info("no tofu configuration defined; skipping apply action")
		return nil
	}
	if err := tofu.ApplyAction(ctx, b.cfg, startStopArg, out, outErr); err != nil {
		slog.Error("Tofu Apply Failed:", "error", err.Error())
		return err
	}

	return nil
}
