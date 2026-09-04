// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
)

var installCmdShortDesc = `Initialize, apply configuration, and deploy Exasol in one step`

// installCmdBaseDesc is the part of the long description that preset-specific help keeps.
var installCmdBaseDesc = installCmdShortDesc + `

	Initializes the deployment directory or applies supplied configuration, prepares infrastructure,
	and installs Exasol.
	Rerunning install with the same presets is safe for retrying failed deployments.
	To switch presets, run exasol destroy --remove first, or exasol remove if the
	deployment resources are already gone.` +
	deploymentDirectoryResolutionHelp

var installCmdLongDesc = presetCommandLongDesc(installCmdBaseDesc, presetArgumentsHelp)

var installCmd = &cobra.Command{
	Use:   "install <infra preset name-or-path> [install preset name-or-path]",
	Short: installCmdShortDesc,
	Long:  installCmdLongDesc,
	Example: "  exasol install " + presets.DefaultInfrastructure + "\n" +
		"  exasol install " + presets.DefaultInfrastructure + " " + presets.DefaultInstallation,
	Args:    cobra.RangeArgs(minPresetArgs, maxPresetArgs),
	GroupID: rootCmdGroupEssential,
}

// nolint: gochecknoinits
func init() {
	// Install creates (init) and then operates on (deploy) the deployment directory.
	requireDeploymentFileLogging(installCmd)

	appendInstallCmdPresetHelp()

	// Run initialization
	installCmd.PreRunE = runInstallPreRun

	// Perform deployment after initialization completes.
	installCmd.PersistentPostRunE = runInstallPersistentPostRun

	// Make "install" runnable without subcommands; no-op RunE prevents usage output.
	// init runs in PreRunE, deploy runs in PersistentPostRunE
	installCmd.RunE = func(_ *cobra.Command, _ []string) error { return nil }

	requireDefaultDeploymentCompatibility(installCmd)
	registerInitFlags(installCmd, commonFlags)
	registerDeploymentDirFlag(installCmd, commonFlags)
	registerVerboseFlag(installCmd, commonFlags)
	registerDeployFlags(installCmd, commonFlags)
	rootCmd.AddCommand(installCmd)
}

// appendInstallCmdPresetHelp defers appending the list of embedded presets and
// their compatibility matrix to the command's long description (see
// deferPresetHelpText).
func appendInstallCmdPresetHelp() {
	deferPresetHelpText(installCmd, installCmdBaseDesc)
}

func runInstallPreRun(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	deployment := commonFlags.Deployment()
	wasInitialized, err := config.HasExasolPersonalStateFile(deployment)
	if err != nil {
		return err
	}
	infraVars := collectInfrastructureVariableOverrides(cmd)
	installVars := collectInstallationVariableOverrides(cmd)
	infraPreset, err := resolvePresetRef(
		cmd.Context(),
		args[0],
		presets.PresetTypeInfrastructure,
	)
	if err != nil {
		return err
	}
	installPreset, err := resolveInstallationPresetRef(cmd.Context(), args, 1, infraPreset)
	if err != nil {
		return err
	}

	if err := initializeInstall(
		cmd,
		deployment,
		wasInitialized,
		infraPreset,
		installPreset,
		infraVars,
		installVars,
	); err != nil {
		return err
	}

	if err := setupDeploymentLogSession(cmd, deployment); err != nil {
		return err
	}
	addTerminalNotice(deploy.EulaNoticeText)

	return nil
}

// initializeInstall either reconciles an already-initialized deployment with
// the requested presets, or creates a new deployment from scratch.
//
//nolint:revive // wasInitialized reflects on-disk deployment state, not internal control coupling.
func initializeInstall(
	cmd *cobra.Command,
	deployment config.DeploymentDir,
	wasInitialized bool,
	infraPreset deploy.PresetRef,
	installPreset deploy.PresetRef,
	infraVars map[string]string,
	installVars map[string]string,
) error {
	if wasInitialized {
		if err := prepareInitializedInstall(
			cmd,
			deployment,
			infraPreset,
			installPreset,
			infraVars,
			installVars,
		); err != nil {
			return fmt.Errorf("initialization failed: %w", err)
		}

		return nil
	}

	if err := deploy.InitDeployment(
		cmd.Context(),
		deployment,
		deploy.InitOptions{
			InfrastructurePreset: infraPreset,
			InstallationPreset:   installPreset,
			InfraVars:            infraVars,
			InstallVars:          installVars,
			VersionCheckEnabled:  !commonFlags.NoLauncherVersionCheck,
			CurrentVersion:       CurrentLauncherVersion,
		},
	); err != nil {
		return fmt.Errorf("initialization failed: %w", err)
	}

	return nil
}

func runInstallPersistentPostRun(cmd *cobra.Command, _ []string) error {
	deployment := commonFlags.Deployment()
	if err := deploy.Deploy(
		cmd.Context(),
		deployment,
		commonFlags.DeployVerbose,
		deploy.DeployOptions{
			UpdateDependencyLockfile: commonFlags.DeployTofuUpdateLockfile,
			RuntimePreparation: hostRuntimePreparationOptions(
				cmd, commonFlags.LocalRuntimeAutoApprove,
			),
		},
	); err != nil {
		return installDeploymentFailure(err)
	}

	err := addConnectionInstructionsTerminalOutput(deployment)
	if err != nil {
		return fmt.Errorf("failed to print deployment info: %w", err)
	}

	return nil
}

func installDeploymentFailure(cause error) error {
	err := fmt.Errorf("deployment failed: %w", cause)
	addLocalPortRecoveryCallToAction(err)

	return err
}

func prepareInitializedInstall(
	cmd *cobra.Command,
	deployment config.DeploymentDir,
	infraPreset deploy.PresetRef,
	installPreset deploy.PresetRef,
	infraVars map[string]string,
	installVars map[string]string,
) error {
	if err := ensureRequestedPresetsMatchInitializedDeployment(
		cmd.Context(),
		deployment,
		infraPreset,
		installPreset,
	); err != nil {
		return err
	}
	_, _, err := applyConfigurationPatchIfProvided(cmd, infraVars, installVars)

	return err
}
