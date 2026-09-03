// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const startCmdShortDesc = `Start a deployment`

const startCmdLongDesc = startCmdShortDesc + `

Start a deployment after it has been stopped.
After the deployment starts, it will have a new public IP address and DNS name assigned.

This command now waits until the Exasol database becomes ready and can accept connections.
If the database does not become ready within the timeout, the command fails.`

var startOpts = struct {
	WaitTimeoutMin int
}{}

type lifecycleCompletionOutput struct {
	DeploymentState string `json:"deploymentState"`
	DatabaseReady   bool   `json:"databaseReady"`
}

func renderLifecycleCompletionJSON(writer io.Writer, output lifecycleCompletionOutput) error {
	return json.NewEncoder(writer).Encode(output)
}

const stopCmdShortDesc = `Stop a deployment`

const stopCmdLongDesc = stopCmdShortDesc + `

Stop a deployment without removing its configuration, data, or infrastructure.
`

var startCmd = &cobra.Command{
	Use:     "start",
	Short:   startCmdShortDesc,
	Long:    startCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupLifecycle,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		deployment := commonFlags.Deployment()
		waitTimeout := time.Duration(startOpts.WaitTimeoutMin) * time.Minute
		waitTimeoutSeconds := int(waitTimeout.Seconds())

		if err := deploy.Start(
			cmd.Context(),
			deployment,
			commonFlags.DeployVerbose,
			deploy.StartOptions{
				WaitTimeoutSeconds: waitTimeoutSeconds,
				RuntimePreparation: hostRuntimePreparationOptions(
					cmd, commonFlags.LocalRuntimeAutoApprove,
				),
			},
		); err != nil {
			if errors.Is(err, deploy.ErrLifecycleActionSkipped) {
				return nil
			}
			addLocalPortRecoveryCallToAction(err)

			return err
		}

		if commonFlags.OutputJson {
			return addLifecycleCompletionTerminalOutput(lifecycleCompletionOutput{
				DeploymentState: deploy.StatusRunning,
				DatabaseReady:   true,
			})
		}

		return addConnectionInstructionsTerminalOutput(deployment)
	},
}

var stopCmd = &cobra.Command{
	Use:     "stop",
	Short:   stopCmdShortDesc,
	Long:    stopCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupLifecycle,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		deployment := commonFlags.Deployment()

		if err := deploy.Stop(
			cmd.Context(),
			deployment,
			commonFlags.DeployVerbose,
		); err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return addLifecycleCompletionTerminalOutput(lifecycleCompletionOutput{
				DeploymentState: deploy.StatusStopped,
				DatabaseReady:   false,
			})
		}

		return nil
	},
}

func addLifecycleCompletionTerminalOutput(output lifecycleCompletionOutput) error {
	return addJSONTerminalOutput(output)
}

func registerStartFlags() {
	startCmd.Flags().IntVar(
		&startOpts.WaitTimeoutMin,
		"wait-timeout-minutes",
		deploy.StartedDefaultTimeoutInMinutes,
		"Maximum minutes to wait for the database to become ready",
	)
	registerRuntimePreparationFlag(startCmd, commonFlags)
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(startCmd)
	requireInitializedDeploymentDir(startCmd)
	requireDeploymentFileLogging(startCmd)
	registerStartFlags()
	registerVerboseFlag(startCmd, commonFlags)
	registerDeploymentDirFlag(startCmd, commonFlags)
	registerOutputFlags(startCmd, commonFlags)
	rootCmd.AddCommand(startCmd)

	requireDefaultDeploymentCompatibility(stopCmd)
	requireInitializedDeploymentDir(stopCmd)
	requireDeploymentFileLogging(stopCmd)
	registerVerboseFlag(stopCmd, commonFlags)
	registerDeploymentDirFlag(stopCmd, commonFlags)
	registerOutputFlags(stopCmd, commonFlags)
	rootCmd.AddCommand(stopCmd)
}
