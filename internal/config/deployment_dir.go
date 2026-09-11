// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"github.com/exasol/exasol-personal/internal/presets"
)

const (
	deploymentsDirName       = "deployments"
	defaultDeploymentDirName = "default"
)

// DeploymentDir represents the root of a deployment directory.
//
// It is intentionally a layout-only value object: it knows the canonical paths
// of launcher-owned files and directories, but it does not perform filesystem
// I/O itself. Reading, writing, and existence checks remain in the dedicated
// config helpers for each artifact.
type DeploymentDir struct {
	root string
}

// NewDeploymentDir normalizes a deployment directory path into a stable,
// absolute root path when possible.
func NewDeploymentDir(path string) DeploymentDir {
	root := filepath.Clean(path)
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}

	return DeploymentDir{root: root}
}

func DefaultDeploymentDirPath() (string, error) {
	rootDir, err := DeploymentsRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, defaultDeploymentDirName), nil
}

func DeploymentsRootPath() (string, error) {
	return launcherpaths.DeploymentsRootPath()
}

// LegacyDeploymentsRootPath exists only so deployments there can be
// migrated to DeploymentsRootPath.
func LegacyDeploymentsRootPath() (string, error) {
	rootDir, err := LauncherRootDirPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, deploymentsDirName), nil
}

// NamedDeploymentDirPath returns the launcher-managed deployment directory
// path for name, in the same parent directory as the default deployment
// directory. Callers are responsible for validating name is safe to use as a
// literal directory name (see DeploymentNameVar in cmd/exasol).
func NamedDeploymentDirPath(name string) (string, error) {
	rootDir, err := DeploymentsRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, name), nil
}

func LauncherRootDirPath() (string, error) {
	return launcherpaths.RootDirPath()
}

// LauncherDirPath returns the launcher-owned directory below baseDir.
func LauncherDirPath(baseDir string) string {
	return launcherpaths.DirPath(baseDir)
}

func (d DeploymentDir) Root() string {
	return d.root
}

func (d DeploymentDir) InfrastructureDir() string {
	return filepath.Join(d.root, InfrastructureFilesDirectory)
}

func (d DeploymentDir) InstallationDir() string {
	return filepath.Join(d.root, InstallationFilesDirectory)
}

func (d DeploymentDir) ConnectionInstructionsPath() string {
	return filepath.Join(d.root, ConnectionInstruction)
}

// NodeDetailsPath returns the canonical path of deployment.json.
func (d DeploymentDir) NodeDetailsPath() string {
	return d.Resolve(nodeDetailsFileName)
}

// ExasolPersonalStatePath returns the canonical path of the launcher state file.
func (d DeploymentDir) ExasolPersonalStatePath() string {
	return d.Resolve(ExasolPersonalStateFileName)
}

// DeploymentVersionMarkerPath returns the canonical path of the plain-text
// deployment version marker.
func (d DeploymentDir) DeploymentVersionMarkerPath() string {
	return d.Resolve(DeploymentVersionMarkerFileName)
}

// SecretsPath returns the canonical path of secrets.json.
func (d DeploymentDir) SecretsPath() string {
	return d.Resolve(secretsFileName)
}

// InfrastructureManifestPath returns the canonical path of infrastructure.yaml.
func (d DeploymentDir) InfrastructureManifestPath() string {
	return filepath.Join(d.InfrastructureDir(), presets.InfrastructureManifestFilename)
}

// InstallManifestPath returns the canonical path of installation.yaml.
func (d DeploymentDir) InstallManifestPath() string {
	return filepath.Join(d.InstallationDir(), presets.InstallationManifestFilename)
}

// Resolve interprets pathValue as deployment-owned unless it is already
// absolute, in which case it is preserved unchanged for legacy compatibility.
func (d DeploymentDir) Resolve(pathValue string) string {
	if filepath.IsAbs(pathValue) {
		return pathValue
	}

	return filepath.Join(d.root, pathValue)
}

// RelativeInfrastructureArtifactDir returns the launcher-owned path from the
// extracted infrastructure preset directory back to the deployment root.
func (DeploymentDir) RelativeInfrastructureArtifactDir() string {
	return RelativeInfrastructureArtifactDir()
}

// RelativeInstallationPresetDir returns the launcher-owned path from the
// extracted infrastructure preset directory to the extracted installation
// preset directory.
func (DeploymentDir) RelativeInstallationPresetDir() string {
	return RelativeInstallationPresetDir()
}
