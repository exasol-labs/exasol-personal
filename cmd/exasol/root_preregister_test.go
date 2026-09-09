// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//nolint:paralleltest // These tests intentionally exercise Cobra's global rootCmd.
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

const testLocalPresetName = "local"

// WARNING: Keep the tests in this file sequential.
//
// The preregistration helpers call rootCmd.Find before Cobra's real Parse step.
// Cobra's Find is not read-only: it merges persistent flags and mutates internal
// command flag structures. Running these tests with t.Parallel() races on the
// package-global rootCmd under `go test -race` (and therefore `task tests-unit`).
//
// If these tests ever need parallelism, first stop sharing rootCmd by building a
// fresh command tree per test or by extracting preregistration command discovery
// behind an interface that can be tested without Cobra's global command state.

func TestScanInfrastructurePresetFromArgs_Defaults(t *testing.T) {
	preset, _ := scanInfrastructurePresetSelection(testManagerContext(t), []string{"init"})
	if preset != nil {
		t.Fatalf("expected no preset selection, got: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_PositionalName(t *testing.T) {
	preset, err := scanInfrastructurePresetSelection(
		testManagerContext(t), []string{"init", presets.DefaultInfrastructure},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Name != presets.DefaultInfrastructure || preset.Path != "" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_PositionalPath(t *testing.T) {
	presetDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(presetDir, presets.InfrastructureManifestFilename),
		[]byte("kind: infrastructure"),
		0o600,
	); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	preset, err := scanInfrastructurePresetSelection(
		testManagerContext(t),
		[]string{"init", presetDir},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Path == "" || preset.Name != "" {
		t.Fatalf("expected resolved path, got: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_SkipsUnrelatedFlags(t *testing.T) {
	preset, err := scanInfrastructurePresetSelection(
		testManagerContext(t), []string{
			"--log-level",
			"debug",
			"init",
			"--deployment-dir",
			"./deploy",
			presets.DefaultInfrastructure,
		},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Name != presets.DefaultInfrastructure || preset.Path != "" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_InstallCommand(t *testing.T) {
	preset, err := scanInfrastructurePresetSelection(
		testManagerContext(
			t,
		),
		[]string{"install", presets.DefaultInfrastructure, presets.DefaultInstallation},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Name != presets.DefaultInfrastructure || preset.Path != "" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestScanPresetFromArgs_BooleanFlagsBeforeInfrastructurePreset(t *testing.T) {
	testCases := []struct {
		name string
		flag string
	}{
		{name: "verbose", flag: "--verbose"},
		{name: "verbose shorthand", flag: "-v"},
		{name: "launcher version check", flag: "--no-launcher-version-check"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Given
			args := []string{"install", testCase.flag, testLocalPresetName, "--help"}

			// When
			ctx := testManagerContext(t)
			infraPreset, infraErr := scanInfrastructurePresetSelection(ctx, args)
			installPreset, installErr := scanInstallationPresetSelection(ctx, args)

			// Then
			if infraErr != nil || infraPreset == nil {
				t.Fatalf("unexpected infrastructure preset error: %v", infraErr)
			}
			if infraPreset.Name != testLocalPresetName || infraPreset.Path != "" {
				t.Fatalf("unexpected infrastructure preset: %#v", infraPreset)
			}
			if installErr != nil || installPreset == nil {
				t.Fatalf("unexpected installation preset error: %v", installErr)
			}
			if installPreset.Name != testLocalPresetName || installPreset.Path != "" {
				t.Fatalf("unexpected installation preset: %#v", installPreset)
			}
		})
	}
}

func TestScanPresetFromArgs_BooleanFlagWithoutInfrastructurePreset(t *testing.T) {
	// Given
	args := []string{"install", "--verbose", "--help"}

	// When
	ctx := testManagerContext(t)
	infraPreset, infraErr := scanInfrastructurePresetSelection(ctx, args)
	installPreset, installErr := scanInstallationPresetSelection(ctx, args)

	// Then
	if infraErr == nil || infraPreset != nil {
		t.Fatalf("expected missing infrastructure preset, got %#v and %v", infraPreset, infraErr)
	}
	if installErr == nil || installPreset != nil {
		t.Fatalf("expected missing installation preset, got %#v and %v", installPreset, installErr)
	}
}

func TestScanPresetFromArgs_BooleanFlagBeforeExplicitInstallationPreset(t *testing.T) {
	// Given
	args := []string{
		"install", "--verbose", testLocalPresetName, testLocalPresetName, "--help",
	}

	// When
	ctx := testManagerContext(t)
	infraPreset, infraErr := scanInfrastructurePresetSelection(ctx, args)
	installPreset, installErr := scanInstallationPresetSelection(ctx, args)

	// Then
	if infraErr != nil || infraPreset == nil {
		t.Fatalf("unexpected infrastructure preset error: %v", infraErr)
	}
	if infraPreset.Name != testLocalPresetName || infraPreset.Path != "" {
		t.Fatalf("unexpected infrastructure preset: %#v", infraPreset)
	}
	if installErr != nil || installPreset == nil {
		t.Fatalf("unexpected installation preset error: %v", installErr)
	}
	if installPreset.Name != testLocalPresetName || installPreset.Path != "" {
		t.Fatalf("unexpected installation preset: %#v", installPreset)
	}
}

func TestScanInstallationPresetFromArgs_DefaultFromInfrastructure(t *testing.T) {
	preset, err := scanInstallationPresetSelection(
		testManagerContext(t), []string{"install", presets.DefaultInfrastructure},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Name != presets.DefaultInstallation || preset.Path != "" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestScanInstallationPresetFromArgs_ExplicitInstallation(t *testing.T) {
	presetDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(presetDir, presets.InstallationManifestFilename),
		[]byte("kind: installation"),
		0o600,
	); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	preset, err := scanInstallationPresetSelection(
		testManagerContext(t), []string{"install", presets.DefaultInfrastructure, presetDir},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Path == "" || preset.Name != "" {
		t.Fatalf("expected resolved path, got: %#v", preset)
	}
}

func TestPreregisteredCommandDetectsConfigSet(t *testing.T) {
	if !preregisteredCommandIs([]string{"config", "set", "--cluster-size", "3"}, configSetCmd) {
		t.Fatal("expected config set command")
	}
}

func TestDeploymentDirFromRawArgs_SeparateFlagValue(t *testing.T) {
	deploymentDir := filepath.Join(t.TempDir(), "deployment")
	deployment, err := deploymentDirFromRawArgs([]string{
		"config",
		"set",
		"--deployment-dir",
		deploymentDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Root() != deploymentDir {
		t.Fatalf("expected deployment dir %q, got %q", deploymentDir, deployment.Root())
	}
}

func TestDeploymentDirFromRawArgs_EqualsFlagValue(t *testing.T) {
	deploymentDir := filepath.Join(t.TempDir(), "deployment")
	deployment, err := deploymentDirFromRawArgs([]string{
		"config",
		"set",
		"--deployment-dir=" + deploymentDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Root() != deploymentDir {
		t.Fatalf("expected deployment dir %q, got %q", deploymentDir, deployment.Root())
	}
}

func TestDeploymentDirFromRawArgs_DeploymentFlag(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	deployment, err := deploymentDirFromRawArgs([]string{
		"config",
		"set",
		"--deployment",
		"staging",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(config.LauncherDirPath(home), "deployments", "staging")
	if deployment.Root() != expected {
		t.Fatalf("expected deployment dir %q, got %q", expected, deployment.Root())
	}
}

func TestDeploymentDirFromRawArgs_DeploymentShorthandFlag(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	deployment, err := deploymentDirFromRawArgs([]string{
		"config",
		"set",
		"-d",
		"staging",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(config.LauncherDirPath(home), "deployments", "staging")
	if deployment.Root() != expected {
		t.Fatalf("expected deployment dir %q, got %q", expected, deployment.Root())
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestResolveDeploymentDirAndDeploymentDirFromRawArgs_AgreeOnSameInputs(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	setTestHome(t, home)
	t.Chdir(cwd)

	testCases := []struct {
		name           string
		deploymentDir  string
		deploymentName string
	}{
		{name: "explicit deployment-dir", deploymentDir: filepath.Join(t.TempDir(), "explicit")},
		{name: "explicit name", deploymentName: "staging"},
		{name: "neither flag falls back to default", deploymentDir: "", deploymentName: ""},
	}

	for _, testCase := range testCases {
		args := []string{"config", "set"}
		if testCase.deploymentDir != "" {
			args = append(args, "--deployment-dir", testCase.deploymentDir)
		}
		if testCase.deploymentName != "" {
			args = append(args, "--deployment", testCase.deploymentName)
		}

		cmd, state := commandWithDeploymentSelection(
			t, testCase.deploymentDir, testCase.deploymentName,
		)

		fromCommand, _, err := resolveDeploymentDir(cmd, state)
		if err != nil {
			t.Fatalf("[%s] resolveDeploymentDir failed: %v", testCase.name, err)
		}

		fromRawArgs, err := deploymentDirFromRawArgs(args)
		if err != nil {
			t.Fatalf("[%s] deploymentDirFromRawArgs failed: %v", testCase.name, err)
		}

		if fromCommand.Root() != fromRawArgs.Root() {
			t.Fatalf(
				"[%s] resolveDeploymentDir and deploymentDirFromRawArgs disagree: %q vs %q",
				testCase.name, fromCommand.Root(), fromRawArgs.Root(),
			)
		}
	}
}
