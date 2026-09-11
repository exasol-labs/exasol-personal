// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

// Recorded per command, so one command's backend options stay out of another's.
const (
	backendOptionFlagsAnnotationKey       = "exasol.backendOptionFlags"
	backendOptionPresetLabelAnnotationKey = "exasol.backendOptionPresetLabel"
)

var errBackendOptionFlagConflict = errors.New("backend option flag name conflict")

// Runs before Cobra parses, which is what keeps an option out of the help and the
// accepted flags of a deployment whose backend cannot act on it.
func prepareBackendOptionFlags(ctx context.Context, args []string) error {
	cmd, _ := preregisteredCommand(args)

	switch cmd {
	case installCmd:
		return prepareInstallBackendOptionFlags(ctx, installCmd, args)
	case deployCmd:
		return prepareDeployBackendOptionFlags(deployCmd, args)
	default:
		return nil
	}
}

func prepareInstallBackendOptionFlags(
	ctx context.Context,
	cmd *cobra.Command,
	args []string,
) error {
	resolution, ok := resolveInstallBackendOptions(ctx, args)
	if !ok {
		// The command itself reports an unusable preset, and can list the valid ones.
		return nil
	}

	return applyBackendOptionFlags(cmd, args, resolution)
}

func resolveInstallBackendOptions(
	ctx context.Context,
	args []string,
) (deploy.DeployOptionResolution, bool) {
	preset, err := scanInfrastructurePresetSelection(ctx, args)
	if err != nil || preset == nil {
		return deploy.DeployOptionResolution{}, false
	}

	resolution, err := deploy.ResolveDeployOptions(ctx, *preset)
	if err != nil {
		return deploy.DeployOptionResolution{}, false
	}

	return resolution, true
}

func prepareDeployBackendOptionFlags(cmd *cobra.Command, args []string) error {
	resolution, ok := resolveDeploymentBackendOptions(args)
	if ok {
		return applyBackendOptionFlags(cmd, args, resolution)
	}

	// No backend to consult. Registering every known option lets parsing succeed so
	// the initialized-deployment gate reports the real problem instead of an unknown
	// flag; hiding them keeps help offering only what a known backend supports.
	if err := registerBackendOptionFlags(cmd, deploy.AllDeployOptions()); err != nil {
		return err
	}
	hideBackendOptionFlags(cmd)

	return nil
}

func hideBackendOptionFlags(cmd *cobra.Command) {
	for _, name := range backendOptionFlagNames(cmd) {
		if flag := cmd.Flags().Lookup(name); flag != nil {
			flag.Hidden = true
		}
	}
}

func resolveDeploymentBackendOptions(
	args []string,
) (deploy.DeployOptionResolution, bool) {
	deployment, err := deploymentDirFromRawArgs(args)
	if err != nil {
		return deploy.DeployOptionResolution{}, false
	}

	resolution, err := deploy.ResolveDeployOptionsFromDeployment(deployment)
	if err != nil {
		return deploy.DeployOptionResolution{}, false
	}

	return resolution, true
}

func applyBackendOptionFlags(
	cmd *cobra.Command,
	args []string,
	resolution deploy.DeployOptionResolution,
) error {
	if err := rejectUnsupportedBackendOptions(args, resolution); err != nil {
		return err
	}
	if err := registerBackendOptionFlags(cmd, resolution.Options); err != nil {
		return err
	}
	if len(resolution.Options) > 0 {
		recordBackendOptionPresetLabel(cmd, resolution.PresetLabel)
	}

	return nil
}

// rejectUnsupportedBackendOptions names the preset, where Cobra would report an
// unknown flag and read like a typo.
func rejectUnsupportedBackendOptions(
	args []string,
	resolution deploy.DeployOptionResolution,
) error {
	if rawArgsRequestHelp(args) {
		return nil
	}

	supported := map[string]struct{}{}
	for _, option := range resolution.Options {
		supported[option.Name] = struct{}{}
	}

	for _, option := range deploy.AllDeployOptions() {
		if _, ok := supported[option.Name]; ok {
			continue
		}
		if !rawArgsContainFlag(args, option.Name) {
			continue
		}

		return fmt.Errorf(
			"%w: infrastructure preset %q does not support --%s",
			deploy.ErrUnsupportedDeployOption, resolution.PresetLabel, option.Name,
		)
	}

	return nil
}

func rawArgsContainFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--"+name || strings.HasPrefix(arg, "--"+name+"=") {
			return true
		}
	}

	return false
}

func registerBackendOptionFlags(
	cmd *cobra.Command,
	options []deploy.DeployOptionDefinition,
) error {
	for _, option := range options {
		if err := registerBackendOptionFlag(cmd, option); err != nil {
			return err
		}
	}

	return nil
}

func registerBackendOptionFlag(
	cmd *cobra.Command,
	option deploy.DeployOptionDefinition,
) error {
	if cmd.Flags().Lookup(option.Name) != nil || cmd.InheritedFlags().Lookup(option.Name) != nil {
		return fmt.Errorf(
			"%w: --%s is already defined", errBackendOptionFlagConflict, option.Name,
		)
	}

	switch option.Type {
	case deploy.ConfigVariableTypeBool:
		cmd.Flags().Bool(option.Name, false, option.Description)
	case deploy.ConfigVariableTypeNumber:
		cmd.Flags().Var(&numberFlag{}, option.Name, option.Description)
	case deploy.ConfigVariableTypeString:
		cmd.Flags().String(option.Name, "", option.Description)
	default:
		cmd.Flags().String(option.Name, "", option.Description)
	}
	recordBackendOptionFlag(cmd, option.Name)

	return nil
}

func recordBackendOptionPresetLabel(cmd *cobra.Command, label string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[backendOptionPresetLabelAnnotationKey] = label
}

func recordBackendOptionFlag(cmd *cobra.Command, name string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}

	registered := backendOptionFlagNames(cmd)
	cmd.Annotations[backendOptionFlagsAnnotationKey] = strings.Join(
		append(registered, name), ",",
	)
}

func backendOptionFlagNames(cmd *cobra.Command) []string {
	recorded := strings.TrimSpace(cmd.Annotations[backendOptionFlagsAnnotationKey])
	if recorded == "" {
		return nil
	}

	return strings.Split(recorded, ",")
}

func isBackendOptionFlagName(cmd *cobra.Command, flagName string) bool {
	return slices.Contains(backendOptionFlagNames(cmd), flagName)
}

func collectBackendOptions(cmd *cobra.Command) map[string]string {
	options := map[string]string{}
	for _, name := range backendOptionFlagNames(cmd) {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || !flag.Changed {
			continue
		}
		options[name] = flag.Value.String()
	}

	return options
}
