// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

func TestConfigCommandsDeclareDeploymentPreRunRequirements(t *testing.T) {
	t.Parallel()

	expectedVersion, err := normalizeVersionToMinor(minSupportedDeploymentVersionBaseline)
	if err != nil {
		expectedVersion = minSupportedDeploymentVersionBaseline
	}

	for _, cmd := range []*cobra.Command{configCmd, configGetCmd, configSetCmd, configResetCmd} {
		t.Run(cmd.Name(), func(t *testing.T) {
			t.Parallel()

			// Then: the executable config commands opt into root-level pre-run gates.
			if !deploymentCompatibilityIsRequired(cmd) {
				t.Fatal("expected deployment compatibility to be required")
			}
			if got := minSupportedDeploymentVersionFromAnnotations(cmd); got != expectedVersion {
				t.Fatalf(
					"expected minimum supported deployment version %q, got %q",
					expectedVersion,
					got,
				)
			}
			if !deploymentDirMustBeInitialized(cmd) {
				t.Fatal("expected initialized deployment directory to be required")
			}
			if !deploymentFileLoggingIsRequired(cmd) {
				t.Fatal("expected deployment file logging to be required")
			}
		})
	}
}

func TestFormatConfigurationValuesKeepsPresetTypesSeparate(t *testing.T) {
	t.Parallel()

	// Given
	configuration := deploy.DeploymentConfiguration{
		Infrastructure: deploy.DeploymentConfigurationSection{
			Identity: deploy.PresetIdentityInfo{
				Selector:    "name:test-infra",
				Kind:        "name",
				Name:        "test-infra",
				DisplayName: "Test Infrastructure",
			},
			Options: []deploy.DeploymentConfigValue{{Name: "cluster_size", Value: 3}},
		},
		Installation: deploy.DeploymentConfigurationSection{
			Identity: deploy.PresetIdentityInfo{
				Selector:    "name:test-install",
				Kind:        "name",
				Name:        "test-install",
				DisplayName: "Test Installation",
			},
			Options: []deploy.DeploymentConfigValue{{Name: "bucketfs_enabled", Value: true}},
		},
	}

	// When
	formatted := formatConfigurationValues(configuration)

	// Then
	for _, expected := range []string{
		"Active configuration:",
		"Infrastructure (Test Infrastructure):",
		"Identity: name:test-infra",
		"Options:",
		"cluster-size = 3",
		"Installation (Test Installation):",
		"Identity: name:test-install",
		"bucketfs-enabled = true",
	} {
		if !strings.Contains(formatted, expected) {
			t.Fatalf(
				"expected formatted configuration to contain %q, got:\n%s", expected, formatted,
			)
		}
	}
}

//nolint:paralleltest // mutates shared terminal message queues; must run serially
func TestAddConfigurationChangedOutputSplitsValuesAndGuidance(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	configuration := deploy.DeploymentConfiguration{
		Infrastructure: deploy.DeploymentConfigurationSection{
			Options: []deploy.DeploymentConfigValue{{Name: "cluster_size", Value: 3}},
		},
	}

	// Given a configuration change is routed to the terminal, and calls to action
	// are not visible (under --json).
	addConfigurationChangedOutput(configuration)
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{
		stdout: &stdout, stderr: &stderr, showCallsToAction: false,
	})

	// Then the effective configuration is primary output on stdout, and the apply
	// guidance is suppressed.
	if !strings.Contains(stdout.String(), "cluster-size = 3") {
		t.Fatalf("expected configuration values on stdout, got: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "exasol deploy") {
		t.Fatalf("expected apply guidance to be suppressed, got stderr: %q", stderr.String())
	}

	// And when calls to action are visible, the apply guidance appears on stderr
	// while stdout still carries only the configuration values.
	resetTerminalMessages()
	addConfigurationChangedOutput(configuration)
	stdout.Reset()
	stderr.Reset()
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	if strings.Contains(stdout.String(), "exasol deploy") {
		t.Fatalf("apply guidance must not appear on stdout, got: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "run `exasol deploy`") {
		t.Fatalf("expected apply guidance on stderr, got: %q", stderr.String())
	}
}

//nolint:paralleltest // mutates shared terminal message queues; must run serially
func TestAddConfigurationChangedOutputUsesStartGuidanceForStoppedLocal(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	addConfigurationChangedOutput(deploy.DeploymentConfiguration{ApplyCommand: "start"})
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{
		stdout: &stdout, stderr: &stderr, showCallsToAction: true,
	})

	if !strings.Contains(stderr.String(), "run `exasol start`") {
		t.Fatalf("expected start guidance, got %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "exasol deploy") {
		t.Fatalf("unexpected deploy guidance, got %q", stderr.String())
	}
}

func TestConfigurationJSONIncludesPresetNames(t *testing.T) {
	t.Parallel()

	// Given
	configuration := deploy.DeploymentConfiguration{
		Infrastructure: deploy.DeploymentConfigurationSection{
			Identity: deploy.PresetIdentityInfo{
				Selector:    "name:test-infra",
				Kind:        "name",
				Name:        "test-infra",
				DisplayName: "Test Infrastructure",
				Description: "test infrastructure",
			},
			Options: []deploy.DeploymentConfigValue{{Name: "cluster_size", Value: 3}},
		},
		Installation: deploy.DeploymentConfigurationSection{
			Identity: deploy.PresetIdentityInfo{
				Selector:    "path:/tmp/test-install",
				Kind:        "path",
				Path:        "/tmp/test-install",
				DisplayName: "Test Installation",
			},
			Options: []deploy.DeploymentConfigValue{{Name: "bucketfs_enabled", Value: true}},
		},
	}

	// When
	actual := configurationJSON(configuration)

	// Then
	expected := map[string]any{
		"infrastructure": map[string]any{
			"identity": map[string]string{
				"selector":    "name:test-infra",
				"kind":        "name",
				"name":        "test-infra",
				"displayName": "Test Infrastructure",
				"description": "test infrastructure",
			},
			"options": map[string]any{"cluster-size": 3},
		},
		"installation": map[string]any{
			"identity": map[string]string{
				"selector":    "path:/tmp/test-install",
				"kind":        "path",
				"path":        "/tmp/test-install",
				"displayName": "Test Installation",
			},
			"options": map[string]any{"bucketfs-enabled": true},
		},
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected %#v, got %#v", expected, actual)
	}
}
