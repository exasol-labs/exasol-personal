// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const removeCmdShortDesc = `Remove a local deployment directory without destroying resources`

const removeCmdLongDesc = removeCmdShortDesc + `

This is a recovery command for cases where deployment resources were already deleted manually,
or where you no longer have access to destroy them through the launcher.

WARNING: This command removes the local deployment directory. It does not destroy
deployment resources and can make launcher-based cleanup impossible if resources still exist.
`

var removeCmd = &cobra.Command{
	Use:     "remove",
	Short:   removeCmdShortDesc,
	Long:    removeCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupLifecycle,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		proceed, err := confirmAction(
			rootOpts.ApprovalMode(),
			func() bool {
				deployment := commonFlags.Deployment()

				return askForUserConfirmation(removeConfirmationPrompt(deployment.Root()))
			},
		)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}

		return deploy.RemoveLocalDeploymentDirectory(cmd.Context(), commonFlags.Deployment())
	},
}

func removeConfirmationPrompt(deploymentDir string) string {
	return "WARNING: This removes the local deployment directory. " +
		"It does not destroy deployment resources. Continue only if the " +
		"resources were already deleted manually or you accept that " +
		"launcher-based cleanup may no longer be possible.\n\n" +
		"Local deployment directory to remove:\n" +
		deploymentDir + "\n\n" +
		"Proceed with local removal? [y/N]"
}

// nolint: gochecknoinits
func init() {
	registerDeploymentDirFlag(removeCmd, commonFlags)
	rootCmd.AddCommand(removeCmd)
}
