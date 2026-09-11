// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package config provides deployment configuration types and persistence.
package config

import (
	"errors"
	"log/slog"
	"os"
	"time"
)

// ExasolPersonalStateFileName is the persistent workflow/metadata state file stored
// in a deployment directory.
const ExasolPersonalStateFileName = ".exasolLauncherState.json"

type (
	WorkflowStateInitialized struct{}
	WorkflowStateRunning     struct{}
	WorkflowStateStopped     struct{}
)

type WorkflowStateOperationInProgress struct {
	Operation string `json:"operation"`
}

const (
	DeployOperation  = "deploy"
	DestroyOperation = "destroy"
	StartOperation   = "start"
	StopOperation    = "stop"
)

type WorkflowStateInterrupted struct {
	Error                      string `json:"error"`
	InterruptedDuringOperation string `json:"interruptedDuringOperation"`
}

type WorkflowStateDeploymentFailed struct {
	Error string `json:"error"`
}

type WorkflowState struct {
	// Union. Exactly one field should be set.
	Initialized         *WorkflowStateInitialized         `json:"initialized,omitempty"`
	OperationInProgress *WorkflowStateOperationInProgress `json:"operationInProgress,omitempty"`
	Interrupted         *WorkflowStateInterrupted         `json:"interrupted,omitempty"`
	DeploymentFailed    *WorkflowStateDeploymentFailed    `json:"deploymentFailed,omitempty"`
	Running             *WorkflowStateRunning             `json:"running,omitempty"`
	Stopped             *WorkflowStateStopped             `json:"stopped,omitempty"`
}

type ExasolPersonalState struct {
	CurrentWorkflowState WorkflowState `json:"currentWorkflowState"`
	// DeploymentId is a launcher-governed stable identifier for this deployment.
	//
	// It can be injected into infrastructure presets (e.g. for tagging) and is also
	// used as a component of cluster identity for version checking.
	DeploymentId string `json:"deploymentId,omitempty"`
	// ClusterIdentity is a launcher-governed stable identity string for this deployment.
	// Presets and scripts should treat it as opaque.
	ClusterIdentity string `json:"clusterIdentity,omitempty"`
	// DeploymentVersion is the launcher version that created this deployment directory.
	//
	// Note: the long-term, stable deployment-version marker used for compatibility
	// checks is the plain-text file ".exasolLauncher.version".
	// This JSON field mirrors that marker for convenience, but compatibility checks
	// should not depend on it.
	DeploymentVersion   string    `json:"deploymentVersion"`
	LastVersionCheck    time.Time `json:"lastVersionCheck"`
	VersionCheckEnabled bool      `json:"versionCheckEnabled"`
	// InfrastructurePresetIdentity is the stable selector used to initialize this deployment.
	InfrastructurePresetIdentity string `json:"infrastructurePresetIdentity,omitempty"`
	// InstallationPresetIdentity is the stable selector used to initialize this deployment.
	InstallationPresetIdentity string `json:"installationPresetIdentity,omitempty"`
	// CreatedAt is the launcher-owned timestamp marking when this deployment
	// directory was initialized. It is persisted here so that backends can be
	// re-configured without needing to consult their own state storage to learn
	// the original deployment creation time (which is a launcher concept, not a
	// backend one). Backends may still receive CreatedAt via DeploymentMetadata
	// and persist it for their own purposes (e.g. resource tagging).
	CreatedAt time.Time `json:"createdAt,omitzero"`
	// InstalledSLCs is the set of official script language containers installed into
	// this (local) deployment. Image mounts are container-run arguments and do not
	// persist across container recreation, so this set is the source of truth that the
	// launcher re-applies on every start.
	//nolint:tagliatelle // JSON key mirrors the "SLC" domain abbreviation.
	InstalledSLCs []InstalledSLC `json:"installedSlcs,omitempty"`
	//nolint:tagliatelle // JSON key mirrors the "SLC" domain abbreviation.
	InstalledCustomSLCs []InstalledCustomSLC `json:"installedCustomSlcs,omitempty"`
}

// InstalledSLC records one installed script language container so the launcher can
// re-apply its image mount on every start and enforce alias uniqueness across the set.
type InstalledSLC struct {
	Language string   `json:"language"`
	Flavor   string   `json:"flavor"`
	Version  string   `json:"version"`
	Image    string   `json:"image"`
	Target   string   `json:"target"`
	Aliases  []string `json:"aliases"`
}

type InstalledCustomSLC struct {
	Alias          string   `json:"alias"`
	PackageAliases []string `json:"packageAliases,omitempty"`
	Language       string   `json:"language"`
	Image          string   `json:"image"`
	Target         string   `json:"target"`
	Package        string   `json:"package"`
	Sha256         string   `json:"sha256"`
	Source         string   `json:"source"`
	Activated      bool     `json:"activated"`
	DisplacedURI   string   `json:"displacedUri,omitempty"`
}

// HasExasolPersonalStateFile reports whether the deployment state file exists.
func HasExasolPersonalStateFile(deployment DeploymentDir) (bool, error) {
	path := deployment.ExasolPersonalStatePath()

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	if info.IsDir() {
		// The path exists but is a directory, not a file.
		return false, nil
	}

	return true, nil
}

// WriteExasolPersonalState writes a new Exasol Personal state to permanent storage.
func WriteExasolPersonalState(state *ExasolPersonalState, deployment DeploymentDir) error {
	return writeConfig(state, deployment.ExasolPersonalStatePath(), "exasol personal state")
}

var ErrNoExasolPersonalStateSet = errors.New("no exasol-personal state is set")

// ReadExasolPersonalState returns the deployment state of the deployment directory.
func ReadExasolPersonalState(deployment DeploymentDir) (*ExasolPersonalState, error) {
	slog.Debug("reading exasol personal state")
	state, err := readConfig[ExasolPersonalState](
		deployment.ExasolPersonalStatePath(),
		"exasol personal state")
	if err != nil {
		return nil, err
	}

	return state, nil
}

// SetWorkflowState writes a new workflow state to permanent storage. The passed struct must be one
// of the WorkflowState* structs.
func (exasolState *ExasolPersonalState) SetWorkflowState(anyState any) error {
	switch state := anyState.(type) {
	case *WorkflowStateInitialized:
		exasolState.CurrentWorkflowState = WorkflowState{Initialized: state}
	case *WorkflowStateOperationInProgress:
		exasolState.CurrentWorkflowState = WorkflowState{OperationInProgress: state}
	case *WorkflowStateInterrupted:
		exasolState.CurrentWorkflowState = WorkflowState{Interrupted: state}
	case *WorkflowStateDeploymentFailed:
		exasolState.CurrentWorkflowState = WorkflowState{DeploymentFailed: state}
	case *WorkflowStateRunning:
		exasolState.CurrentWorkflowState = WorkflowState{Running: state}
	case *WorkflowStateStopped:
		exasolState.CurrentWorkflowState = WorkflowState{Stopped: state}
	default:
		panic("invalid workflow state")
	}

	return nil
}

func (exasolState *ExasolPersonalState) SetWorkflowStateAndWrite(
	anyState any,
	deployment DeploymentDir,
) error {
	err := exasolState.SetWorkflowState(anyState)
	if err != nil {
		return err
	}

	err = WriteExasolPersonalState(exasolState, deployment)
	if err != nil {
		return err
	}

	return nil
}

var ErrNoWorkflowStateSet = errors.New("no workflow state is set in the workflow state file")

// GetWorkflowState returns the deployment state of the deployment directory.
func (exasolState *ExasolPersonalState) GetWorkflowState() (any, error) {
	if exasolState.CurrentWorkflowState.Initialized != nil {
		return exasolState.CurrentWorkflowState.Initialized, nil
	}
	if exasolState.CurrentWorkflowState.OperationInProgress != nil {
		return exasolState.CurrentWorkflowState.OperationInProgress, nil
	}
	if exasolState.CurrentWorkflowState.Running != nil {
		return exasolState.CurrentWorkflowState.Running, nil
	}
	if exasolState.CurrentWorkflowState.Stopped != nil {
		return exasolState.CurrentWorkflowState.Stopped, nil
	}
	if exasolState.CurrentWorkflowState.Interrupted != nil {
		return exasolState.CurrentWorkflowState.Interrupted, nil
	}
	if exasolState.CurrentWorkflowState.DeploymentFailed != nil {
		return exasolState.CurrentWorkflowState.DeploymentFailed, nil
	}

	return "", ErrNoWorkflowStateSet
}
