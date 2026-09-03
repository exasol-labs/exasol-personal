// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const deployCmdShortDesc = `Deploy using an existing deployment directory`

const deployCmdLongDesc = deployCmdShortDesc + `

Once a deployment is complete, Terraform state files will be stored in the deployment directory.
Do not delete the deployment directory until the ` + "`destroy`" + ` command has been executed.
`

var deployCmd = &cobra.Command{
	Use:     "deploy",
	Short:   deployCmdShortDesc,
	Long:    deployCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupEssential,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
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
			addLocalPortRecoveryCallToAction(err)

			return err
		}

		return addConnectionInstructionsTerminalOutput(deployment)
	},
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(deployCmd)
	requireInitializedDeploymentDir(deployCmd)
	requireDeploymentFileLogging(deployCmd)
	registerDeploymentDirFlag(deployCmd, commonFlags)
	registerVerboseFlag(deployCmd, commonFlags)
	registerDeployFlags(deployCmd, commonFlags)
	rootCmd.AddCommand(deployCmd)
}
