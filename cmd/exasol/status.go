// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const statusCmdShortDesc = `Get the status of a deployment`

const (
	defaultStatusTimeoutSeconds int64 = 5
	maxStatusTimeoutSeconds           = int64(1<<63-1) / int64(time.Second)
)

const statusCmdLongDesc = statusCmdShortDesc + `

Display the status of the current deployment.

	The possible values for the ` + "`status`" + ` field are:

	- ` + deploy.StatusNotInitialized + `
	- ` + deploy.StatusInitialized + `
	- ` + deploy.StatusOperationInProgress + `
	- ` + deploy.StatusInterrupted + `
	- ` + deploy.StatusStopped + `
	- ` + deploy.StatusRunning + `
	- ` + deploy.StatusDeploymentFailed + `
	- ` + deploy.StatusDatabaseConnectionFailed + `
	- ` + deploy.StatusDatabaseReady + `
`

var statusOpts = struct {
	unsafe         bool
	timeoutSeconds int64
}{}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: statusCmdShortDesc,
	Long:  statusCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		ctx, cancel, err := contextWithStatusTimeout(cmd.Context(), statusOpts.timeoutSeconds)
		if err != nil {
			return err
		}
		defer cancel()

		deployment := commonFlags.Deployment()
		var status *deploy.StatusOutput

		if statusOpts.unsafe {
			slog.Debug("acquiring deployment status without lock")
			status, err = deploy.StatusUnsafe(ctx, deployment)
		} else {
			slog.Debug("acquiring deployment status with lock")
			status, err = deploy.Status(ctx, deployment)
		}
		if err != nil {
			return err
		}

		var output string
		if commonFlags.OutputJson {
			output, err = formatStatusJSON(*status)
			if err != nil {
				return err
			}
		} else {
			output = formatStatusText(*status)
		}
		addTerminalOutput(output)

		return nil
	},
}

func contextWithStatusTimeout(
	parent context.Context,
	timeoutSeconds int64,
) (context.Context, context.CancelFunc, error) {
	if timeoutSeconds <= 0 {
		return nil, nil, fmt.Errorf("--timeout must be positive, got %d", timeoutSeconds)
	}
	if timeoutSeconds > maxStatusTimeoutSeconds {
		return nil, nil, fmt.Errorf("--timeout is too large, got %d", timeoutSeconds)
	}

	timeout := time.Duration(timeoutSeconds) * time.Second

	ctx, cancel := context.WithTimeout(parent, timeout)

	return ctx, cancel, nil
}

func formatStatusJSON(status deploy.StatusOutput) (string, error) {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func formatStatusText(status deploy.StatusOutput) string {
	var builder strings.Builder
	_, _ = fmt.Fprintf(&builder, "Deployment directory: %s\n", status.DeploymentDir)
	_, _ = fmt.Fprintf(&builder, "Status: %s\n", status.Status)
	if status.Message != "" {
		_, _ = fmt.Fprintf(&builder, "Message: %s\n", status.Message)
	}

	return strings.TrimRight(builder.String(), "\n")
}

func registerStatusFlags() {
	statusCmd.Flags().BoolVar(
		&statusOpts.unsafe,
		"unsafe", false,
		"Try to read the deployment folder state even if it is locked. May fail.",
	)
	statusCmd.Flags().Int64Var(
		&statusOpts.timeoutSeconds,
		"timeout", defaultStatusTimeoutSeconds,
		"Maximum number of seconds to wait for the complete status check",
	)
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(statusCmd)
	registerStatusFlags()
	registerDeploymentDirFlag(statusCmd, commonFlags)
	registerOutputFlags(statusCmd, commonFlags)
	rootCmd.AddCommand(statusCmd)
}
