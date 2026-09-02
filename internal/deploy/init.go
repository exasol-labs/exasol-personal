// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/util"
)

const (
	baseURL        = "https://www.exasol.com/terms-and-conditions/"
	eulaURI        = "#h-exasol-personal-end-user-license-agreement"
	eulaURL        = baseURL + eulaURI
	EulaNoticeText = `For your reference:
By using the Exasol Personal launcher, you accept its End User License Agreement (EULA):
` + eulaURL + `

A copy of the EULA is also included as 'eula.txt' in this directory.

`
)

var (
	ErrUnknownVariable             = errors.New("unknown variable")
	ErrDeploymentDirectoryNotEmpty = errors.New("deployment directory is not empty")
	ErrDeploymentPresetMismatch    = errors.New(
		"deployment directory is initialized with different presets",
	)
)

// InitOptions bundles the preset selection, variable overrides, and version-check
// metadata needed to initialize a deployment directory.
type InitOptions struct {
	InfrastructurePreset PresetRef
	InstallationPreset   PresetRef
	InfraVars            map[string]string
	InstallVars          map[string]string
	VersionCheckEnabled  bool
	CurrentVersion       string
}

// InitDeployment initializes a new deployment directory by extracting presets and
// creating the variables file based on the infrastructure manifest.
func InitDeployment(
	ctx context.Context,
	deployment config.DeploymentDir,
	options InitOptions,
) error {
	// Do an initial update version check if permitted
	if options.VersionCheckEnabled {
		_, _, _ = CheckLatestVersionUpdate(ctx, options.CurrentVersion, deployment)
	}

	if err := validateInitRequest(ctx, deployment, options); err != nil {
		return err
	}

	if err := ensureDeploymentDirReady(deployment); err != nil {
		return err
	}

	// Lock the deployment directory with exclusive access
	return withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			return initializeDeploymentLocked(ctx, deployment, options)
		})
}

// validateInitRequest proactively validates the preset selection and target
// environment to produce friendly errors before any state is mutated.
func validateInitRequest(
	ctx context.Context,
	deployment config.DeploymentDir,
	options InitOptions,
) error {
	slog.Info("validating presets")
	if err := ValidatePresetSelection(
		ctx,
		options.InfrastructurePreset,
		options.InstallationPreset,
	); err != nil {
		return err
	}
	infrastructureManifest, err := readInfrastructureManifestFromPreset(
		ctx,
		options.InfrastructurePreset,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to load infrastructure preset %q: %w",
			presetLabel(options.InfrastructurePreset),
			err,
		)
	}
	backend, err := newDeploymentBackend(ctx, deployment, infrastructureManifest)
	if err != nil {
		return err
	}
	if err := backend.ValidateEnvironment(); err != nil {
		return err
	}

	return validateLocalInitMemory(ctx, infrastructureManifest, options.InfraVars)
}

// ensureDeploymentDirReady makes sure the deployment directory has no prior
// exasol-personal state and exists as an empty directory.
//
// Init only creates fresh deployment state. Existing deployment orchestration
// belongs to the command layer.
func ensureDeploymentDirReady(deployment config.DeploymentDir) error {
	initialized, err := config.HasExasolPersonalStateFile(deployment)
	if err != nil {
		slog.Error("failed to check deployment directory initialization")
		return err
	}
	if initialized {
		return ErrDeploymentDirectoryNotEmpty
	}

	if err := util.EnsureDir(deployment.Root()); err != nil {
		return err
	}

	return ensureDirectoryIsEmpty(deployment)
}

func initializeDeploymentLocked(
	ctx context.Context,
	deployment config.DeploymentDir,
	options InitOptions,
) error {
	deploymentId, err := GenerateDeploymentId()
	if err != nil {
		return fmt.Errorf("failed to generate deployment id: %w", err)
	}
	clusterIdentity := ComputeClusterIdentity(
		deploymentId,
		options.InfrastructurePreset,
		options.InstallationPreset,
	)

	// Copy the presets into the deployment directory
	if err := extractPresets(
		ctx,
		options.InfrastructurePreset,
		options.InstallationPreset,
		deployment,
	); err != nil {
		return err
	}

	slog.Debug("Initializing deployment state")
	exasolState := newInitializedState(
		options.VersionCheckEnabled,
		options.CurrentVersion,
		deploymentId,
		clusterIdentity,
		time.Now().UTC(),
		options.InfrastructurePreset,
		options.InstallationPreset,
	)
	infraManifest, _, err := readExtractedManifests(deployment)
	if err != nil {
		return err
	}
	backend, err := newDeploymentBackend(ctx, deployment, infraManifest)
	if err != nil {
		return err
	}
	infraDefaults, err := backend.ConfigurationDefaults(ctx, options.InfraVars)
	if err != nil {
		return err
	}
	effectiveInfraVars := mergeConfigurationDefaults(infraDefaults, options.InfraVars)
	if err := backend.SetupWorkspace(ctx); err != nil {
		return err
	}
	if err := writeDeploymentConfiguration(
		ctx,
		deployment,
		exasolState,
		newDeploymentConfigurationFromRaw(effectiveInfraVars, options.InstallVars),
	); err != nil {
		return err
	}

	if err := exasolState.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{},
		deployment,
	); err != nil {
		return err
	}
	if err := config.WriteDeploymentVersionMarker(deployment, options.CurrentVersion); err != nil {
		return err
	}

	slog.Info(
		"successfully initialized deployment",
		"infrastructure",
		presetLabel(options.InfrastructurePreset),
		"installation",
		presetLabel(options.InstallationPreset),
	)

	return nil
}

func mergeConfigurationDefaults(
	defaults map[string]string,
	overrides map[string]string,
) map[string]string {
	merged := make(map[string]string, len(defaults)+len(overrides))
	maps.Copy(merged, defaults)
	maps.Copy(merged, overrides)

	return merged
}

// extractPresets writes infrastructure, installation,
// and shared assets into the deployment directory.
func extractPresets(
	ctx context.Context,
	infrastructurePreset PresetRef,
	installationPreset PresetRef,
	deployment config.DeploymentDir,
) error {
	slog.Info("extracting preset files",
		"infrastructure", infrastructurePreset,
		"installation", installationPreset)

	infrastructureDir := deployment.InfrastructureDir()
	installationDir := deployment.InstallationDir()

	// Write shared assets
	slog.Debug("writing shared files to deployment directory", "dest", ".")
	err := presets.WriteSharedDir(deployment.Root())
	if err != nil {
		slog.Error(
			"Failed to write shared assets",
			"err", err,
			"dir", util.AbsPathNoFail(deployment.Root()),
		)

		return err
	}

	// Write infrastructure preset
	slog.Debug("writing infrastructure preset to deployment directory", "path", infrastructureDir)
	err = presets.WriteDir(ctx, presets.Infrastructure, infrastructurePreset, infrastructureDir)
	if err != nil {
		slog.Error(
			"Failed to write infrastructure preset",
			"err", err,
			"infrastructure", presetLabel(infrastructurePreset),
			"dir", util.AbsPathNoFail(infrastructureDir),
		)

		return err
	}

	// Write installation preset into installation directory
	slog.Debug("writing installation preset to deployment directory", "path", installationDir)
	err = presets.WriteDir(ctx, presets.Installation, installationPreset, installationDir)
	if err != nil {
		slog.Error(
			"Failed to write installation preset",
			"err", err,
			"installation", presetLabel(installationPreset),
			"dir", util.AbsPathNoFail(installationDir),
		)

		return err
	}

	return nil
}

func ensureDirectoryIsEmpty(deployment config.DeploymentDir) error {
	// When init is called, the deployment dir must be empty.
	slog.Debug("testing if deployment directory is empty")
	entries, err := os.ReadDir(deployment.Root())
	if err != nil {
		return err
	}

	if len(entries) != 0 {
		badFile := filepath.Join(deployment.Root(), entries[0].Name())
		slog.Error(ErrDeploymentDirectoryNotEmpty.Error(), "file", util.AbsPathNoFail(badFile))

		return fmt.Errorf("%w: file: \"%s\"", ErrDeploymentDirectoryNotEmpty, badFile)
	}

	return nil
}

//nolint:revive // versionCheckEnabled chooses persisted state shape for user configuration.
func newInitializedState(
	versionCheckEnabled bool,
	deploymentVersion string,
	deploymentId string,
	clusterIdentity string,
	createdAt time.Time,
	infrastructurePreset PresetRef,
	installationPreset PresetRef,
) *config.ExasolPersonalState {
	lastVersionCheck := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
	if versionCheckEnabled {
		lastVersionCheck = time.Now()
	}

	return &config.ExasolPersonalState{
		DeploymentId:                 deploymentId,
		ClusterIdentity:              clusterIdentity,
		CreatedAt:                    createdAt.UTC(),
		DeploymentVersion:            deploymentVersion,
		VersionCheckEnabled:          versionCheckEnabled,
		LastVersionCheck:             lastVersionCheck,
		InfrastructurePresetIdentity: presetIdentityOf(infrastructurePreset),
		InstallationPresetIdentity:   presetIdentityOf(installationPreset),
	}
}

func validateInfrastructurePreset(ctx context.Context, infrastructurePreset PresetRef) error {
	return validatePreset(
		infrastructurePreset, "infrastructure.yaml", "infrastructure preset",
		presets.ListEmbeddedPresets(ctx, presets.Infrastructure),
	)
}

func validateInstallationPreset(ctx context.Context, installationPreset PresetRef) error {
	return validatePreset(
		installationPreset, "installation.yaml", "installation preset",
		presets.ListEmbeddedPresets(ctx, presets.Installation),
	)
}

// validatePreset validates a preset reference: a path reference must point
// at a directory containing requiredManifestFile; a named reference must
// appear in known.
func validatePreset(preset PresetRef, requiredManifestFile, label string, known []string) error {
	if preset.IsPath() {
		return validatePresetDir(preset.Path, requiredManifestFile)
	}
	if slices.Contains(known, preset.Name) {
		return nil
	}

	return fmt.Errorf("unknown %s %q; available: %s", label, preset.Name, strings.Join(known, ", "))
}

func validatePresetDir(dir, requiredFile string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("preset path %q does not exist", dir)
		}

		return fmt.Errorf("failed to access preset path %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("preset path %q is not a directory", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, requiredFile)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("preset path %q is missing required file %q", dir, requiredFile)
		}

		return fmt.Errorf(
			"failed to validate preset path %q (required file %q): %w",
			dir,
			requiredFile,
			err,
		)
	}

	return nil
}

func presetLabel(p PresetRef) string {
	if p.IsPath() {
		return p.Path
	}

	return p.Name
}
