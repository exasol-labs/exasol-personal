// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

func TestInitDeployment_CreatesTfVarsWhenTofuConfigured(t *testing.T) {
	t.Parallel()

	// Given a deployment directory
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)

	// When the deployment is intialized
	err := InitDeployment(
		testManagerContext(t),
		deployment,
		InitOptions{
			InfrastructurePreset: PresetRef{Name: presets.DefaultInfrastructure},
			InstallationPreset:   PresetRef{Name: presets.DefaultInstallation},
			InfraVars:            map[string]string{"cluster_size": "2"},
			InstallVars:          map[string]string{},
			CurrentVersion:       "0.0.0",
		},
	)
	if err != nil {
		t.Fatalf("InitDeployment failed: %v", err)
	}

	assertInitializedDeploymentState(t, deployment, "0.0.0")
	assertTfVarsFile(t, deployment)
	assertInstallationVariablesFile(t, deployment)
}

// assertInitializedDeploymentState asserts that the persisted workflow state,
// preset identities, and version marker reflect a freshly initialized deployment.
func assertInitializedDeploymentState(
	t *testing.T,
	deployment config.DeploymentDir,
	wantVersion string,
) {
	t.Helper()

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("expected workflow state to be readable, got error: %v", err)
	}
	if state.DeploymentVersion != wantVersion {
		t.Fatalf(
			"expected deployment version to be persisted as %q, got %q",
			wantVersion,
			state.DeploymentVersion,
		)
	}
	if strings.TrimSpace(state.DeploymentId) == "" {
		t.Fatal("expected deploymentId to be persisted, got empty")
	}
	if strings.TrimSpace(state.ClusterIdentity) == "" {
		t.Fatal("expected clusterIdentity to be persisted, got empty")
	}
	if state.InfrastructurePresetIdentity != "name:"+presets.DefaultInfrastructure {
		t.Fatalf(
			"expected infrastructure preset identity to be persisted, got %q",
			state.InfrastructurePresetIdentity,
		)
	}
	if state.InstallationPresetIdentity != "name:"+presets.DefaultInstallation {
		t.Fatalf(
			"expected installation preset identity to be persisted, got %q",
			state.InstallationPresetIdentity,
		)
	}
	if ver, ok, err := config.ReadDeploymentVersionMarker(deployment); err != nil {
		t.Fatalf("expected deployment version marker to be readable, got error: %v", err)
	} else if !ok {
		t.Fatalf("expected deployment version marker %q to exist",
			config.DeploymentVersionMarkerFileName)
	} else if ver != wantVersion {
		t.Fatalf("expected deployment version marker to be %q, got %q", wantVersion, ver)
	}
	workflowState, err := state.GetWorkflowState()
	if err != nil {
		t.Fatalf("expected workflow state to be set, got error: %v", err)
	}
	if _, ok := workflowState.(*config.WorkflowStateInitialized); !ok {
		t.Fatalf("expected Initialized workflow state, got %T", workflowState)
	}
}

// assertTfVarsFile asserts that the tfvars file exists at the manifest-defined
// default path and contains the expected cluster and preset directory values.
func assertTfVarsFile(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	tfvarsPath := filepath.Join(deployment.InfrastructureDir(), tofu.DefaultVarsOutput)
	data, err := os.ReadFile(tfvarsPath)
	if err != nil {
		t.Fatalf("expected %s to exist, got read error: %v", tfvarsPath, err)
	}
	content := string(data)

	if !strings.Contains(content, "cluster_size") {
		t.Fatalf("expected tfvars to contain cluster_size, got: %s", content)
	}

	if !strings.Contains(content, "installation_preset_dir") {
		t.Fatalf("expected tfvars to contain installation_preset_dir, got: %s", content)
	}

	parsedVars, err := parseTFVarsFile(data, tfvarsPath)
	if err != nil {
		t.Fatalf("expected tfvars to be parseable, got error: %v", err)
	}
	artifactDir, ok := parsedVars["infrastructure_artifact_dir"]
	if !ok {
		t.Fatalf("expected tfvars to contain infrastructure_artifact_dir, got: %s", content)
	}
	if artifactDir != deployment.RelativeInfrastructureArtifactDir() {
		t.Fatalf(
			"expected infrastructure_artifact_dir to be %q, got %q",
			deployment.RelativeInfrastructureArtifactDir(),
			artifactDir,
		)
	}
	if filepath.IsAbs(artifactDir) {
		t.Fatalf("expected infrastructure_artifact_dir to stay relative, got %q", artifactDir)
	}
	installDir, ok := parsedVars["installation_preset_dir"]
	if !ok {
		t.Fatalf("expected tfvars to contain installation_preset_dir value, got: %s", content)
	}
	if installDir != deployment.RelativeInstallationPresetDir() {
		t.Fatalf(
			"expected installation_preset_dir to be %q, got %q",
			deployment.RelativeInstallationPresetDir(),
			installDir,
		)
	}
	if filepath.IsAbs(installDir) {
		t.Fatalf("expected installation_preset_dir to stay relative, got %q", installDir)
	}
}

// assertInstallationVariablesFile asserts that the installation variables file
// exists at the manifest-defined path and carries the launcher-governed keys.
func assertInstallationVariablesFile(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	const installationVarsRelPath = "files/etc/exasol_launcher/installation.json"

	installVarsPath := filepath.Join(
		deployment.InstallationDir(),
		installationVarsRelPath,
	)
	if _, err := os.Stat(installVarsPath); err != nil {
		t.Fatalf("expected installation variables file %s to exist, got: %v", installVarsPath, err)
	}
	installData, err := os.ReadFile(installVarsPath)
	if err != nil {
		t.Fatalf("expected %s to exist, got read error: %v", installVarsPath, err)
	}
	installContent := string(installData)
	if !strings.Contains(installContent, "\"deployment_id\"") {
		t.Fatalf("expected installation vars to contain deployment_id, got: %s", installContent)
	}
	if !strings.Contains(installContent, "\"cluster_identity\"") {
		t.Fatalf("expected installation vars to contain cluster_identity, got: %s", installContent)
	}
	if !strings.Contains(installContent, "\"version_check_url\"") {
		t.Fatalf("expected installation vars to contain version_check_url, got: %s", installContent)
	}
}

func TestInitDeployment_CreatesDeploymentDir(t *testing.T) {
	t.Parallel()

	// Given
	root := t.TempDir()
	deploymentDir := filepath.Join(root, "deployment")
	deployment := config.NewDeploymentDir(deploymentDir)

	// When
	err := InitDeployment(
		testManagerContext(t),
		deployment,
		InitOptions{
			InfrastructurePreset: PresetRef{Name: presets.DefaultInfrastructure},
			InstallationPreset:   PresetRef{Name: presets.DefaultInstallation},
			InfraVars:            map[string]string{},
			InstallVars:          map[string]string{},
			CurrentVersion:       "",
		},
	)
	// Then
	if err != nil {
		t.Fatalf("InitDeployment failed: %v", err)
	}

	info, _ := os.Stat(deploymentDir)
	if !info.IsDir() {
		t.Fatal("Deployment directory does not exist")
	}
}

func TestInitDeployment_PersistsConcreteLocalPort(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	err := InitDeployment(
		testManagerContext(t),
		deployment,
		InitOptions{
			InfrastructurePreset: PresetRef{Name: "local"},
			InstallationPreset:   PresetRef{Name: "local"},
			InfraVars:            map[string]string{},
			InstallVars:          map[string]string{},
			CurrentVersion:       "0.0.0",
		},
	)
	if err != nil {
		t.Fatalf("InitDeployment failed: %v", err)
	}

	manifest, err := presets.ReadInfrastructureManifestFromDir(deployment.InfrastructureDir())
	if err != nil {
		t.Fatalf("failed to read local manifest: %v", err)
	}
	if manifest.Local == nil {
		t.Fatal("expected local manifest configuration")
	}
	mappings, _, err := parseLocalPortMappings(manifest.Local.Ports)
	if err != nil || mappings[localDatabaseService] <= 0 {
		t.Fatalf("expected concrete database port, got %#v", manifest.Local)
	}
}

func parseTFVarsFile(tfvars []byte, filename string) (map[string]string, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(tfvars, filename)
	if diags.HasErrors() {
		return nil, diags
	}

	attrs, diags := file.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, diags
	}

	result := make(map[string]string, len(attrs))
	for name, attr := range attrs {
		value, diags := attr.Expr.Value(nil)
		if diags.HasErrors() {
			return nil, diags
		}
		if value.Type() == cty.String {
			result[name] = value.AsString()
		}
	}

	return result, nil
}

func TestInitDeployment_ErrWhenDirNotEmpty(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	nonEmptyPath := filepath.Join(deploymentDir, "not-empty")
	if err := os.WriteFile(nonEmptyPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write preexisting file failed: %v", err)
	}

	// When
	err := InitDeployment(
		testManagerContext(t),
		deployment,
		InitOptions{
			InfrastructurePreset: PresetRef{Name: presets.DefaultInfrastructure},
			InstallationPreset:   PresetRef{Name: presets.DefaultInstallation},
			InfraVars:            map[string]string{},
			InstallVars:          map[string]string{},
			CurrentVersion:       "",
		},
	)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), ErrDeploymentDirectoryNotEmpty.Error()) {
		t.Fatalf("expected ErrDeploymentDirectoryNotEmpty, got: %v", err)
	}
}
