// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// root_preregister_test.go, so they must run sequentially.
//
//nolint:paralleltest // These tests exercise the package-global rootCmd/configSetCmd, like
package main

import (
	"path/filepath"
	"strings"
	"testing"
)

const localInfrastructureManifestForConfigSet = `name: Exasol Local on macOS
description: Exasol Local deployment using the macOS Apple Silicon VM runner.
backend: local

local:
  cpuCount: 2
  dataSizeGB: 100
`

func writeInitializedLocalDeployment(t *testing.T) string {
	t.Helper()

	return writeInitializedDeployment(t, localInfrastructureManifestForConfigSet)
}

// A non-help `config set` against a directory that is not an initialized deployment must
// fail with a clear error that names the directory, instead of silently registering no
// flags (which would later surface as a misleading "unknown flag" parse error). SPOT-31462.
func TestConfigSetClearErrorWhenDeploymentNotInitialized(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-a-deployment")

	err := prepareConfigSetInfrastructureVariableFlags(
		testManagerContext(t),
		[]string{"config", "set", "--ports", "db:8000", "--deployment-dir", dir},
	)
	if err == nil {
		t.Fatal("expected a clear error, got nil")
	}
	for _, want := range []string{
		dir, // names the offending directory
		"not an Exasol Personal deployment directory", // reuses the standard phrasing
		"exasol init",
		"--deployment-dir",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
	// It must not leak internal implementation detail such as manifest file names.
	if strings.Contains(err.Error(), "infrastructure.yaml") {
		t.Errorf("error should not expose internal manifest details, got: %v", err)
	}
}

// `config set --help` must remain non-fatal even when options cannot be loaded, so help
// can render.
func TestConfigSetHelpRendersWithoutResolvableDeployment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-a-deployment")

	if err := prepareConfigSetInfrastructureVariableFlags(
		testManagerContext(t),
		[]string{"config", "set", "--help", "--deployment-dir", dir},
	); err != nil {
		t.Fatalf("expected help to render (nil error), got: %v", err)
	}
}

// A destroyed (i.e. initialized) local deployment must still resolve its infrastructure
// options for `config set`, so pre-registration succeeds and registers the option flags.
func TestConfigSetRegistersOptionsForInitializedLocalDeployment(t *testing.T) {
	originalFlagMap := infraFlagToVarName
	infraFlagToVarName = map[string]string{}
	t.Cleanup(func() { infraFlagToVarName = originalFlagMap })

	dir := writeInitializedLocalDeployment(t)

	if err := prepareConfigSetInfrastructureVariableFlags(
		testManagerContext(t),
		[]string{"config", "set", "--ports", "db:8000", "--deployment-dir", dir},
	); err != nil {
		t.Fatalf("unexpected error resolving options for initialized deployment: %v", err)
	}
	if configSetCmd.Flags().Lookup("ports") == nil {
		t.Error("expected --ports to be registered on `config set`")
	}
}

func TestRawArgsRequestHelp(t *testing.T) {
	cases := map[string]struct {
		args []string
		want bool
	}{
		"long help":       {[]string{"config", "set", "--help"}, true},
		"short help":      {[]string{"config", "set", "-h"}, true},
		"help subcommand": {[]string{"help", "config", "set"}, true},
		"no help":         {[]string{"config", "set", "--ports", "db:8000"}, false},
		"empty":           {[]string{}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := rawArgsRequestHelp(tc.args); got != tc.want {
				t.Errorf("rawArgsRequestHelp(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
