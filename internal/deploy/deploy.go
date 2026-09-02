// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localports"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/remote"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/gobwas/glob"
)

const (
	cloudDeployFailureResourceHint = "Deployment may have created cloud resources " +
		"that can incur costs. " +
		"Fix the problem and run `deploy` again, or run `destroy` to clean up."
	localDeployFailureResourceHint = "Deployment may have created local VM runtime files. " +
		"Fix the problem and run `deploy` again, or run `destroy` to clean up."
)

func appendDeployFailureHint(
	deployment config.DeploymentDir,
	err error,
) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf(
		"%w\n\nInspect launcher logs at %s for details. %s%s",
		err,
		deployment.Resolve("deployment.log"),
		deployFailureResourceHint(deployment),
		deployFailureResourceHintSuffix(deployment),
	)
}

func deployFailureResourceHint(deployment config.DeploymentDir) string {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return cloudDeployFailureResourceHint
	}
	backend, err := resolveBackendKind(manifest)
	if err != nil {
		return cloudDeployFailureResourceHint
	}
	if backend == backendTypeLocal {
		return localDeployFailureResourceHint
	}

	return cloudDeployFailureResourceHint
}

func deployFailureResourceHintSuffix(deployment config.DeploymentDir) string {
	if _, statErr := os.Stat(deployment.NodeDetailsPath()); statErr == nil {
		return fmt.Sprintf(
			" Additional deployment diagnostics are stored in %s.",
			deployment.NodeDetailsPath(),
		)
	}

	return ""
}

//nolint:revive
func Deploy(
	ctx context.Context,
	deployment config.DeploymentDir,
	verbose bool,
	options DeployOptions,
) error {
	slog.Debug("Running deploy")

	// Execute according to infrastructure/installation manifests instead of exasolConfig.yaml
	return deployFromManifests(ctx, deployment, verbose, options)
}

func WorkflowStatePermitsDeploy(
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
) error {
	// Check we are in state init, inprogress or deploymentInterrupted.
	// - We are allowed to retry deployment when in the interrupted state, because
	//   tofu apply is idempotent
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		slog.Error("failed to read workflow state")
		return err
	}

	switch state := workflowState.(type) {
	case *config.WorkflowStateInitialized,
		*config.WorkflowStateDeploymentFailed,
		*config.WorkflowStateRunning:
		return nil

	case *config.WorkflowStateOperationInProgress:
		if state.Operation == config.DeployOperation {
			return nil
		}

		return newBlockedStateError(deployment, ErrUnspportedOperation)

	case *config.WorkflowStateInterrupted:
		if state.InterruptedDuringOperation == config.DeployOperation {
			slog.Debug("deploying in workflow state `deploymentInterrupted`")
			return nil
		}
	default:
	}

	return newBlockedStateError(deployment, ErrUnexpectedDeploymentStatus)
}

//nolint:revive
func deployFromManifests(
	ctx context.Context,
	deployment config.DeploymentDir,
	verbose bool,
	options DeployOptions,
) error {
	return withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			return deployLocked(ctx, deployment, verbose, options)
		})
}

//nolint:revive // verbose mirrors the command-level --verbose flag.
func deployLocked(
	ctx context.Context,
	deployment config.DeploymentDir,
	verbose bool,
	options DeployOptions,
) error {
	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		slog.Error("failed to read exasol personal state")
		return err
	}

	if err := WorkflowStatePermitsDeploy(exasolState, deployment); err != nil {
		return err
	}
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		return err
	}
	_, restoreInitializedOnPortConflict := workflowState.(*config.WorkflowStateInitialized)
	infrastructureManifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return err
	}
	backend, err := newDeploymentBackend(ctx, deployment, infrastructureManifest)
	if err != nil {
		return err
	}
	var externalCommandOutput io.Writer
	if verbose {
		externalCommandOutput = os.Stderr
	}
	// Prepare before recording an operation in progress: a declined or
	// failed prerequisite must leave the deployment retryable rather than
	// stranded mid-operation.
	if err := backend.Prepare(
		ctx, externalCommandOutput, externalCommandOutput, options.RuntimePreparation,
	); err != nil {
		return err
	}

	// Set the workflowstate to deployment in-progress
	if err := exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
		Operation: config.DeployOperation,
	}, deployment); err != nil {
		slog.Error("failed to set workflow state to in-progress", "error", err.Error())
	}

	return runDeployBackend(
		ctx,
		exasolState,
		deployment,
		backend,
		externalCommandOutput,
		options,
		restoreInitializedOnPortConflict,
	)
}

// runDeployBackend registers the interruption signal handler, runs the
// manifest-driven deploy and installation phases, and commits the final
// state. It is split out from deployLocked so the signal-handler lifetime
// (registered right before the backend calls, unregistered right after)
// stays scoped to a single, easy-to-audit block.
//
//nolint:revive // verbose mirrors the command-level --verbose flag.
func runDeployBackend(
	ctx context.Context,
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
	backend deploymentBackend,
	externalCommandOutput io.Writer,
	options DeployOptions,
	restoreInitializedOnPortConflict bool,
) error {
	// Register signal handler for catching interruptions and set state
	// in case of interruption
	unregister, _ := util.RegisterOnceSignalHandler(func() {
		slog.Warn("Deployment interrupted")
		if err := exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateInterrupted{
			Error:                      "Deployment interrupted via signal",
			InterruptedDuringOperation: config.DeployOperation,
		}, deployment); err != nil {
			slog.Error("failed to set workflow state to in-progress", "error", err.Error())
		}
	})

	// Fallback cleanup
	defer unregister()

	installManifest, err := config.ReadInstallManifest(deployment)
	if err != nil {
		return err
	}

	if err := backend.Deploy(
		ctx,
		externalCommandOutput,
		externalCommandOutput,
		options,
	); err != nil {
		unregister()
		_, unavailable := localports.AsUnavailable(err)
		if unavailable && restoreInitializedOnPortConflict {
			return restoreStateAfterUnavailableLocalPort(
				exasolState,
				deployment,
				&config.WorkflowStateInitialized{},
				err,
			)
		}

		return recordDeployFailure(exasolState, deployment, err)
	}

	// Installation phase (remoteExec tasks)
	if err := runInstallSteps(ctx, deployment, installManifest,
		externalCommandOutput, externalCommandOutput); err != nil {
		unregister()

		return recordDeployFailure(exasolState, deployment, err)
	}

	// Stop handling interrupts before committing success state
	unregister()

	return finalizeSuccessfulDeploy(ctx, exasolState, deployment)
}

// recordDeployFailure appends diagnostic hints to err and persists the
// deployment-failed workflow state before returning the enriched error.
func recordDeployFailure(
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
	err error,
) error {
	deployErr := appendDeployFailureHint(deployment, err)
	if stateErr := exasolState.SetWorkflowStateAndWrite(
		&config.WorkflowStateDeploymentFailed{
			Error: deployErr.Error(),
		},
		deployment,
	); stateErr != nil {
		slog.Warn("failed to persist deployment failure state", "error", stateErr)
	}

	return deployErr
}

// finalizeSuccessfulDeploy commits the running workflow state and writes the
// connection instructions after a successful deploy and installation phase.
func finalizeSuccessfulDeploy(
	ctx context.Context,
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
) error {
	if err := exasolState.SetWorkflowStateAndWrite(
		&config.WorkflowStateRunning{},
		deployment,
	); err != nil {
		slog.Error("failed to write workflow state")
		return err
	}

	reconcileCustomSLCsAfterStart(ctx, deployment)

	connectionInstructions, err := getConnectionInstructionsTextUnsafe(ctx, deployment)
	if err != nil {
		slog.Error("failed to collect connection instructions")
		return err
	}
	if err := writeConnectionInstructionsFile(deployment, connectionInstructions); err != nil {
		slog.Error("failed to write connection instructions")
		return err
	}

	slog.Info("Completed deploying")

	return nil
}

type nodeLookupImpl struct {
	deployment config.DeploymentDir
}

var _ task_runner.NodeFinder = &nodeLookupImpl{}

func NewNodeLookup(deployment config.DeploymentDir) task_runner.NodeFinder {
	return &nodeLookupImpl{
		deployment: deployment,
	}
}

func (s *nodeLookupImpl) Find(
	nodeNameGlob string,
) ([]task_runner.RunScriptNode, error) {
	nodeListBuilder, err := newNodeListBuilder(s.deployment)
	if err != nil {
		return nil, util.LoggedError(err, "failed to create node list builder")
	}

	nodes, err := nodeListBuilder.BuildForNodeGlob(nodeNameGlob)
	if err != nil {
		return nil, util.LoggedError(err, "failed to build node list", "node", nodeNameGlob)
	}

	return nodes, nil
}

type nodeListBuilder struct {
	deployment  config.DeploymentDir
	nodeDetails *config.NodeDetails
}

func newNodeListBuilder(deployment config.DeploymentDir) (*nodeListBuilder, error) {
	nodeDetails, err := config.ReadNodeDetails(deployment)
	if err != nil {
		return nil, err
	}

	return &nodeListBuilder{
		deployment:  deployment,
		nodeDetails: nodeDetails,
	}, nil
}

func (builder *nodeListBuilder) BuildForNodeGlob(
	nodeGlob string,
) ([]task_runner.RunScriptNode, error) {
	search, err := glob.Compile(nodeGlob)
	if err != nil {
		return nil, err
	}

	result := []task_runner.RunScriptNode{}

	for _, nodeName := range builder.nodeDetails.ListNodes() {
		if search.Match(nodeName) {
			remoteConn, err := builder.getRemote(nodeName)
			if err != nil {
				return nil, err
			}

			result = append(result, task_runner.RunScriptNode{
				Name:              nodeName,
				ConnectionOptions: remoteConn,
			})
		}
	}

	return result, nil
}

func (builder *nodeListBuilder) getRemote(node string) (*remote.SSHConnectionOptions, error) {
	sshDetails, err := builder.nodeDetails.GetSSHDetails(node, builder.deployment)
	if err != nil {
		return nil, err
	}

	keyFilePath := sshDetails.KeyFile
	keyData, err := os.ReadFile(keyFilePath)
	if err != nil {
		return nil, fmt.Errorf("%w: could not read SSH key file %s", err, keyFilePath)
	}

	return &remote.SSHConnectionOptions{
		Host: sshDetails.Host,
		User: sshDetails.User,
		Port: sshDetails.Port,
		Key:  keyData,
	}, nil
}

func runInstallSteps(
	ctx context.Context,
	deployment config.DeploymentDir,
	im *presets.InstallManifest,
	out, outErr io.Writer,
) error {
	tasks := buildInstallTasks(im)
	if len(tasks) == 0 {
		slog.Info("no installation steps defined; skipping")
		return nil
	}

	return task_runner.NewTaskRunner(
		&task_runner.LocalCommandRunnerImpl{},
		&task_runner.RemoteScriptRunnerImpl{},
		NewNodeLookup(deployment),
	).RunTasks(ctx, tasks, deployment, out, outErr)
}

func buildInstallTasks(installManifest *presets.InstallManifest) []config.Task {
	if installManifest == nil {
		return nil
	}

	tasks := []config.Task{}
	for _, step := range installManifest.Install {
		if step.RemoteExec != nil {
			remoteExec := *step.RemoteExec
			if remoteExec.Filename != "" {
				remoteExec.Filename = filepath.Join("installation", remoteExec.Filename)
			}
			tasks = append(tasks, config.Task{RemoteExec: &remoteExec})
		}
		if step.LocalCommand != nil {
			localCommand := *step.LocalCommand
			tasks = append(tasks, config.Task{LocalCommand: &localCommand})
		}
	}

	return tasks
}
