// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const testTofuPresetName = "aws"

const tofuInfrastructureManifestForDeployment = `name: Exasol in Test Cloud
description: Exasol on a tofu-managed test cloud.
backend: tofu

tofu:
  variablesFile: variables.tf
`

const localInfrastructureManifestForDeployment = `name: Exasol Local for backend options
description: Exasol Local deployment used to check backend option ownership.
backend: local

local:
  cpuCount: 2
`

// Stands in for `install` or `deploy`, so registration leaves the real commands
// alone. The tests still reach the package-global rootCmd, whose lookup mutates
// shared Cobra state, so they run sequentially.
func newBackendOptionTestCommand() *cobra.Command {
	return &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error {
		return nil
	}}
}

//nolint:paralleltest
func TestInstallBackendOptions_TofuPresetOffersLockfileOption(t *testing.T) {
	cmd := newBackendOptionTestCommand()

	err := prepareInstallBackendOptionFlags(
		testManagerContext(t),
		cmd,
		[]string{"install", testTofuPresetName, "--tofu-update-lockfile"},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	flag := cmd.Flags().Lookup("tofu-update-lockfile")
	if flag == nil {
		t.Fatal("expected --tofu-update-lockfile to be registered for a tofu preset")
	}
	if flag.Value.Type() != "bool" {
		t.Fatalf("expected a boolean flag, got %q", flag.Value.Type())
	}
	if flag.Usage == "" {
		t.Error("expected the option's description to be rendered as flag usage")
	}
}

// Help must not list an option the preset's backend cannot act on.
//
//nolint:paralleltest
func TestInstallBackendOptions_LocalPresetOffersNoLockfileOption(t *testing.T) {
	cmd := newBackendOptionTestCommand()

	err := prepareInstallBackendOptionFlags(
		testManagerContext(t),
		cmd,
		[]string{"install", testLocalPresetName},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cmd.Flags().Lookup("tofu-update-lockfile") != nil {
		t.Error("expected --tofu-update-lockfile to be absent for a local preset")
	}
}

//nolint:paralleltest
func TestInstallBackendOptions_LocalPresetRejectsLockfileOption(t *testing.T) {
	err := prepareInstallBackendOptionFlags(
		testManagerContext(t),
		newBackendOptionTestCommand(),
		[]string{"install", testLocalPresetName, "--tofu-update-lockfile"},
	)
	if err == nil {
		t.Fatal("expected a clear error, got nil")
	}
	if !errors.Is(err, deploy.ErrUnsupportedDeployOption) {
		t.Fatalf("expected ErrUnsupportedDeployOption, got %v", err)
	}
	for _, want := range []string{"--tofu-update-lockfile", testLocalPresetName} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// Help must render even for an unsupported option, as it does for preset variables.
//
//nolint:paralleltest
func TestInstallBackendOptions_HelpRendersDespiteUnsupportedOption(t *testing.T) {
	cmd := newBackendOptionTestCommand()

	err := prepareInstallBackendOptionFlags(
		testManagerContext(t),
		cmd,
		[]string{"install", testLocalPresetName, "--tofu-update-lockfile", "--help"},
	)
	if err != nil {
		t.Fatalf("expected help to render (nil error), got %v", err)
	}
	if cmd.Flags().Lookup("tofu-update-lockfile") != nil {
		t.Error("expected --tofu-update-lockfile to stay absent for a local preset")
	}
}

// Written before the preset, the pre-parser consumes the preset as the option's
// value, leaving no backend to resolve.
//
//nolint:paralleltest
func TestInstallBackendOptions_OptionBeforePresetOffersNothing(t *testing.T) {
	cmd := newBackendOptionTestCommand()

	err := prepareInstallBackendOptionFlags(
		testManagerContext(t),
		cmd,
		[]string{"install", "--tofu-update-lockfile", testTofuPresetName},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cmd.Flags().Lookup("tofu-update-lockfile") != nil {
		t.Error("expected no option to be registered before the preset is known")
	}
}

// Options render under their own heading, not among the generic flags.
//
//nolint:paralleltest
func TestInstallBackendOptions_RenderUnderTheirOwnHelpHeading(t *testing.T) {
	cmd := newBackendOptionTestCommand()

	if err := prepareInstallBackendOptionFlags(
		testManagerContext(t),
		cmd,
		[]string{"install", testTofuPresetName},
	); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !hasBackendOptionFlags(cmd) {
		t.Fatal("expected the backend option section to be rendered")
	}
	if title := backendOptionFlagsTitle(cmd); !strings.Contains(title, testTofuPresetName) {
		t.Errorf("expected the title to name the preset, got %q", title)
	}
	if usages := backendOptionFlagUsages(cmd); !strings.Contains(usages, "--tofu-update-lockfile") {
		t.Errorf("expected the option in its own section, got %q", usages)
	}
	for _, flag := range otherLocalFlags(cmd) {
		if flag.Name == "tofu-update-lockfile" {
			t.Error("expected the option to be excluded from the generic flag section")
		}
	}
}

// A backend declaring no options renders no heading at all.
//
//nolint:paralleltest
func TestInstallBackendOptions_LocalPresetRendersNoHelpHeading(t *testing.T) {
	cmd := newBackendOptionTestCommand()

	if err := prepareInstallBackendOptionFlags(
		testManagerContext(t),
		cmd,
		[]string{"install", testLocalPresetName},
	); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hasBackendOptionFlags(cmd) {
		t.Error("expected no backend option section for a preset that declares none")
	}
}

// The command itself reports an unresolvable preset, so pre-registration stays silent.
//
//nolint:paralleltest
func TestInstallBackendOptions_UnknownPresetStaysSilent(t *testing.T) {
	cmd := newBackendOptionTestCommand()

	if err := prepareInstallBackendOptionFlags(
		testManagerContext(t),
		cmd,
		[]string{"install", "no-such-preset"},
	); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cmd.Flags().Lookup("tofu-update-lockfile") != nil {
		t.Error("expected no options to be registered for an unknown preset")
	}
}

//nolint:paralleltest
func TestDeployBackendOptions_TofuDeploymentOffersLockfileOption(t *testing.T) {
	cmd := newBackendOptionTestCommand()
	dir := writeInitializedDeployment(t, tofuInfrastructureManifestForDeployment)

	err := prepareDeployBackendOptionFlags(
		cmd,
		[]string{"deploy", "--deployment-dir", dir, "--tofu-update-lockfile"},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cmd.Flags().Lookup("tofu-update-lockfile") == nil {
		t.Error("expected --tofu-update-lockfile to be registered for a tofu deployment")
	}
}

//nolint:paralleltest
func TestDeployBackendOptions_LocalDeploymentRejectsLockfileOption(t *testing.T) {
	dir := writeInitializedDeployment(t, localInfrastructureManifestForDeployment)

	err := prepareDeployBackendOptionFlags(
		newBackendOptionTestCommand(),
		[]string{"deploy", "--deployment-dir", dir, "--tofu-update-lockfile"},
	)
	if err == nil {
		t.Fatal("expected a clear error, got nil")
	}
	if !errors.Is(err, deploy.ErrUnsupportedDeployOption) {
		t.Fatalf("expected ErrUnsupportedDeployOption, got %v", err)
	}
	if !strings.Contains(err.Error(), "Exasol Local for backend options") {
		t.Errorf("error should name the deployment's preset, got: %v", err)
	}
}

//nolint:paralleltest
func TestDeployBackendOptions_LocalDeploymentOffersNoLockfileOption(t *testing.T) {
	cmd := newBackendOptionTestCommand()
	dir := writeInitializedDeployment(t, localInfrastructureManifestForDeployment)

	if err := prepareDeployBackendOptionFlags(
		cmd,
		[]string{"deploy", "--deployment-dir", dir},
	); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cmd.Flags().Lookup("tofu-update-lockfile") != nil {
		t.Error("expected --tofu-update-lockfile to be absent for a local deployment")
	}
}

// Parsing must still succeed without a backend, so the initialized-deployment gate
// reports the real problem instead of an unknown flag.
//
//nolint:paralleltest
func TestDeployBackendOptions_UninitializedDeploymentKeepsOptionsParseable(t *testing.T) {
	cmd := newBackendOptionTestCommand()
	dir := filepath.Join(t.TempDir(), "not-a-deployment")

	if err := prepareDeployBackendOptionFlags(
		cmd,
		[]string{"deploy", "--deployment-dir", dir, "--tofu-update-lockfile"},
	); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	flag := cmd.Flags().Lookup("tofu-update-lockfile")
	if flag == nil {
		t.Fatal("expected every known option to stay parseable")
	}
	if !flag.Hidden {
		t.Error("expected the option to stay out of help while no backend is known")
	}
	if hasBackendOptionFlags(cmd) {
		t.Error("expected no backend option section while no backend is known")
	}
}

//nolint:paralleltest
func TestCollectBackendOptions_ReturnsSuppliedValues(t *testing.T) {
	cmd := newBackendOptionTestCommand()

	if err := registerBackendOptionFlags(cmd, deploy.AllDeployOptions()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := cmd.Flags().Parse([]string{"--tofu-update-lockfile"}); err != nil {
		t.Fatalf("expected the option to parse, got %v", err)
	}

	options := collectBackendOptions(cmd)
	if options["tofu-update-lockfile"] != "true" {
		t.Fatalf("expected the supplied value, got %v", options)
	}
}

// An unsupplied option stays absent, so the backend keeps its own default.
//
//nolint:paralleltest
func TestCollectBackendOptions_OmitsUnsuppliedOptions(t *testing.T) {
	cmd := newBackendOptionTestCommand()

	if err := registerBackendOptionFlags(cmd, deploy.AllDeployOptions()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := cmd.Flags().Parse(nil); err != nil {
		t.Fatalf("expected parsing to succeed, got %v", err)
	}

	if options := collectBackendOptions(cmd); len(options) != 0 {
		t.Fatalf("expected no options, got %v", options)
	}
}

//nolint:paralleltest
func TestRegisterBackendOptionFlag_ReportsNameConflict(t *testing.T) {
	cmd := newBackendOptionTestCommand()
	cmd.Flags().Bool("tofu-update-lockfile", false, "")

	err := registerBackendOptionFlags(cmd, deploy.AllDeployOptions())
	if !errors.Is(err, errBackendOptionFlagConflict) {
		t.Fatalf("expected a flag name conflict, got %v", err)
	}
}

//nolint:paralleltest
func TestRawArgsContainFlag(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected bool
	}{
		{name: "absent", args: []string{"install", "local"}},
		{name: "bare", args: []string{"--tofu-update-lockfile"}, expected: true},
		{name: "with value", args: []string{"--tofu-update-lockfile=true"}, expected: true},
		{name: "other flag", args: []string{"--tofu-update-lockfile-x"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := rawArgsContainFlag(testCase.args, "tofu-update-lockfile")
			if actual != testCase.expected {
				t.Fatalf("expected %v, got %v", testCase.expected, actual)
			}
		})
	}
}
