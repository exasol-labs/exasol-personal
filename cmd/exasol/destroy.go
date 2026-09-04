// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const destroyCmdShortDesc = `Destroy a deployment`

// nolint: revive
const destroyCmdLongDesc = destroyCmdShortDesc + `

Destroying a deployment deletes all resources - including all data.
If you want to retain any data, make sure you've created and moved backups to another safe location.
By default, local deployment files are kept so the same deployment can be inspected or recreated.
Pass --remove to remove the local deployment directory after deployment resources have been destroyed.
`

var destroyOpts = struct {
	Remove bool
}{}

func registerDestroyFlags(cmd *cobra.Command) {
	registerVerboseFlag(cmd, commonFlags)
	// nolint: revive
	cmd.Flags().BoolVar(&destroyOpts.Remove,
		"remove",
		false,
		"Remove the local deployment directory after deployment resources are successfully destroyed")
}

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Short:   destroyCmdShortDesc,
	Long:    destroyCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupLifecycle,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		deployment := commonFlags.Deployment()

		proceed, err := confirmAction(
			rootOpts.ApprovalMode(),
			func() bool {
				removalTarget := ""
				if destroyOpts.Remove {
					removalTarget = deployment.Root()
				}

				return askForUserConfirmation(destroyConfirmationPrompt(removalTarget))
			},
		)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}

		if err := deploy.Destroy(
			cmd.Context(),
			deployment,
			commonFlags.DeployVerbose,
		); err != nil {
			return err
		}
		if destroyOpts.Remove {
			return deploy.RemoveLocalDeploymentDirectory(cmd.Context(), deployment)
		}

		return nil
	},
}

func destroyConfirmationPrompt(localRemovalTarget string) string {
	prompt := "WARNING: Destroying a deployment " +
		"is an irreversible operation, " +
		"and removes all deployment resources " +
		"- including all data."
	if localRemovalTarget != "" {
		prompt += "\n\nLocal deployment directory to remove after destroy:\n" + localRemovalTarget
	}
	prompt += "\n\nProceed with destroy? [y/N]"

	return prompt
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(destroyCmd)
	requireInitializedDeploymentDir(destroyCmd)
	requireDeploymentFileLogging(destroyCmd)
	registerDeploymentDirFlag(destroyCmd, commonFlags)
	registerDestroyFlags(destroyCmd)
	rootCmd.AddCommand(destroyCmd)
}
