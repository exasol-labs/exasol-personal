// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/slc"
)

// Internal escape hatch for fake local-runner integration tests.
const localSkipDatabaseWaitEnv = "EXASOL_LOCAL_SKIP_DB_WAIT"

// JDBC IMPORT/EXPORT expects the Java runtime at this stable path. The flavor-specific
// mount remains available for Java UDF aliases.
const currentJavaMountTarget = slc.SLCMountRoot + "/current-java"

func startPreparedLocalRuntime(
	ctx context.Context,
	runtime localruntime.Runtime,
	runtimeConfig localRuntimeConfig,
	waitTimeoutSeconds int,
	out, outErr io.Writer,
) error {
	localConfig, err := toLocalRuntimeConfig(runtime.Deployment(), runtimeConfig)
	if err != nil {
		return err
	}
	if err := runtime.Start(ctx, out, outErr, localConfig); err != nil {
		return diagnoseLocalFailure(ctx, runtime, err)
	}

	endpoint, err := runtime.ReadEndpoints()
	if err != nil {
		return err
	}

	return writeLocalRuntimeArtifactsAndWait(
		ctx, runtime, endpoint, waitTimeoutSeconds, out, outErr,
	)
}

// Must run after the caller commits its workflow state: that write serialises a copy read
// before the restart, so an earlier activation would be overwritten.
func reconcileCustomSLCsAfterStart(ctx context.Context, deployment config.DeploymentDir) {
	if !isLocalDeployment(deployment) {
		return
	}
	if err := reconcileCustomSLCActivation(ctx, deployment); err != nil {
		slog.Warn("failed to activate a custom script language container", "error", err)
	}
}

// localRuntimeLiveness answers "is the local runtime running?", including the
// case where the probe cannot answer at all. That case must stay distinct from
// stopped: treating an unanswerable probe as stopped leaves a deployment in a
// state that blocks its own recovery.
type localRuntimeLiveness int

const (
	localRuntimeLivenessUnknown localRuntimeLiveness = iota
	localRuntimeStopped
	localRuntimeRunning
)

// probeLocalRuntimeLiveness asks the selected local runtime whether it is
// running. It reports the same answer on every platform, so callers need no
// per-runtime knowledge to tell a clean failure from an indeterminate one.
func probeLocalRuntimeLiveness(
	ctx context.Context,
	deployment config.DeploymentDir,
) localRuntimeLiveness {
	if !isLocalDeployment(deployment) {
		return localRuntimeLivenessUnknown
	}

	manager := runtimeartifacts.FromContext(ctx)

	selectedRuntime, err := newLocalRuntime(deployment, manager)
	if err != nil {
		slog.Warn("could not select local runtime to probe its status", "error", err)
		return localRuntimeLivenessUnknown
	}
	// Make the runtime observable first. A Podman machine stopped outside the
	// launcher leaves Status unable to answer, and a swallowed status error
	// used to leave the stale "running" state in place - which then blocked
	// the very start that would have restarted the machine. Best-effort: if
	// this fails, Status is still worth attempting.
	if err := selectedRuntime.EnsureQueryable(ctx, os.Stderr, os.Stderr); err != nil {
		slog.Warn("could not make local runtime queryable", "error", err)
	}

	runtimeStatus, err := selectedRuntime.Status(ctx)
	if err != nil {
		slog.Warn("could not determine local runtime status", "error", err)
		return localRuntimeLivenessUnknown
	}

	if runtimeStatus.Running {
		return localRuntimeRunning
	}

	return localRuntimeStopped
}

// stopLocalRuntimeAfterFailure stops a local runtime left running by a failed
// lifecycle operation, so the state the launcher records and the machine the
// user retries against agree.
func stopLocalRuntimeAfterFailure(
	ctx context.Context,
	deployment config.DeploymentDir,
) error {
	selectedRuntime, err := newLocalRuntime(deployment, runtimeartifacts.FromContext(ctx))
	if err != nil {
		return err
	}

	return selectedRuntime.Stop(ctx, os.Stderr, os.Stderr)
}

// reconcileLocalVMState corrects stale workflow state after an unclean local
// runtime shutdown. The historical name is retained for compatibility with callers.
//
// Only reconciles Running→Stopped. Stopped→Running is not corrected, as a VM
// running outside the launcher's knowledge is an externally-caused inconsistency
// that should surface as an error rather than be silently accepted.
//
// Errors from the runtime status check are logged and swallowed; reconciliation is
// best-effort and must not block the caller's primary operation.
// The caller must already hold the exclusive deployment lock.
func reconcileLocalVMState(
	ctx context.Context,
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
) error {
	if !isLocalDeployment(deployment) {
		return nil
	}

	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		slog.Debug("could not read workflow state during reconciliation", "error", err)
		return nil
	}

	if _, ok := workflowState.(*config.WorkflowStateRunning); !ok {
		return nil
	}

	if probeLocalRuntimeLiveness(ctx, deployment) != localRuntimeStopped {
		return nil
	}

	slog.Info("local runtime is not running; correcting workflow state to stopped")

	return exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateStopped{}, deployment)
}

func isLocalDeployment(deployment config.DeploymentDir) bool {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return false
	}

	kind, err := resolveBackendKind(manifest)

	return err == nil && kind == backendTypeLocal
}

func stopLocalRuntime(
	ctx context.Context,
	runtime localruntime.Runtime,
	out, outErr io.Writer,
) error {
	if err := runtime.Stop(ctx, out, outErr); err != nil {
		return err
	}

	return updateLocalDeploymentArtifactState(runtime.Deployment(), StatusStopped)
}

func destroyLocalRuntime(
	ctx context.Context,
	runtime localruntime.Runtime,
	out, outErr io.Writer,
) error {
	if err := runtime.Destroy(ctx, out, outErr); err != nil {
		return err
	}

	deployment := runtime.Deployment()
	for _, path := range []string{
		deployment.NodeDetailsPath(),
		deployment.SecretsPath(),
		deployment.ConnectionInstructionsPath(),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove local deployment artifact %s: %w", path, err)
		}
	}

	return nil
}

func toLocalRuntimeConfig(
	deployment config.DeploymentDir,
	runtimeConfig localRuntimeConfig,
) (localruntime.VMConfig, error) {
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return localruntime.VMConfig{}, fmt.Errorf("failed to read local startup settings: %w", err)
	}
	versionDetails := GetVersionCheckDetails(deployment)
	versionCheck := localinstall.VersionCheckConfig{
		Enabled:         state.VersionCheckEnabled,
		URL:             versionDetails.URL,
		Identity:        strings.TrimSpace(state.ClusterIdentity),
		OperatingSystem: versionDetails.OperatingSystem,
	}
	if versionCheck.Enabled && versionCheck.Identity == "" {
		return localruntime.VMConfig{}, errors.New("deployment state is missing cluster identity")
	}

	totalSLCs := len(state.InstalledSLCs) + len(state.InstalledCustomSLCs)
	slcs := make([]localinstall.SLCConfig, 0, totalSLCs)
	for _, installed := range state.InstalledSLCs {
		slcs = appendOfficialSLCConfigs(slcs, installed)
	}
	for _, installed := range state.InstalledCustomSLCs {
		slcs = append(slcs, localinstall.SLCConfig{
			Image:   installed.Image,
			Target:  installed.Target,
			Package: installed.Package,
		})
	}

	return localruntime.VMConfig{
		RuntimeConfig: localruntime.RuntimeConfig{
			Ports:        runtimeConfig.ports,
			VersionCheck: versionCheck,
			SLCs:         slcs,
		},
		CPUCount:   runtimeConfig.cpuCount,
		MemoryMB:   runtimeConfig.memoryMB,
		DataSizeGB: runtimeConfig.dataSizeGB,
	}, nil
}

func appendOfficialSLCConfigs(
	configs []localinstall.SLCConfig,
	installed config.InstalledSLC,
) []localinstall.SLCConfig {
	configs = append(configs, localinstall.SLCConfig{
		Image:  installed.Image,
		Target: installed.Target,
	})
	if isJavaSLC(installed) {
		configs = append(configs, localinstall.SLCConfig{
			Image:  installed.Image,
			Target: currentJavaMountTarget,
		})
	}

	return configs
}

func isJavaSLC(installed config.InstalledSLC) bool {
	for _, alias := range installed.Aliases {
		if strings.EqualFold(alias, "JAVA") {
			return true
		}
	}

	return false
}

func writeLocalRuntimeArtifactsAndWait(
	ctx context.Context,
	runtime localruntime.Runtime,
	endpoint *localruntime.VMRuntimeEndpoint,
	waitTimeoutSeconds int,
	out, outErr io.Writer,
) error {
	if err := writeLocalDeploymentArtifacts(runtime.Deployment(), endpoint); err != nil {
		return err
	}
	if os.Getenv(localSkipDatabaseWaitEnv) != "" {
		return nil
	}

	waitTimeoutSeconds = boundedLocalDatabaseStartedTimeout(waitTimeoutSeconds)

	return waitForLocalDatabaseAndSync(
		ctx,
		runtime,
		waitTimeoutSeconds,
		out,
		outErr,
		WaitForLocalDatabaseStarted,
	)
}

func boundedLocalDatabaseStartedTimeout(waitTimeoutSeconds int) int {
	if waitTimeoutSeconds <= 0 || waitTimeoutSeconds > LocalDatabaseStartedDefaultTimeoutSeconds {
		return LocalDatabaseStartedDefaultTimeoutSeconds
	}

	return waitTimeoutSeconds
}

func waitForLocalDatabaseAndSync(
	ctx context.Context,
	runtime localruntime.Runtime,
	waitTimeoutSeconds int,
	out, outErr io.Writer,
	waitForDatabase func(context.Context, localruntime.Runtime) error,
) error {
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(waitTimeoutSeconds)*time.Second)
	defer cancel()

	if err := waitForDatabase(waitCtx, runtime); err != nil {
		return err
	}

	return runtime.WorkaroundNanoStartupDurability(ctx, out, outErr)
}

func writeLocalDeploymentArtifacts(
	deployment config.DeploymentDir,
	endpoint *localruntime.VMRuntimeEndpoint,
) error {
	if endpoint == nil {
		return errors.New("local runtime endpoint state is required")
	}
	if endpoint.DBPort <= 0 || endpoint.DBPort > 65535 {
		return fmt.Errorf("local runtime database port is invalid: %d", endpoint.DBPort)
	}

	deploymentID := "local"
	if launcherState, err := config.ReadExasolPersonalState(deployment); err == nil {
		if strings.TrimSpace(launcherState.DeploymentId) != "" {
			deploymentID = launcherState.DeploymentId
		}
	}

	info := &config.DeploymentInfo{
		Backend:         localDeploymentBackend,
		DeploymentId:    deploymentID,
		DeploymentState: StatusRunning,
		ClusterSize:     1,
		ClusterState:    StatusRunning,
		InstanceType:    "exasol-local",
		Connection: &config.DeploymentConnection{
			Host:                       localDeploymentPublicHost,
			DisplayHost:                localDeploymentPublicHost,
			PublicIp:                   localDeploymentPublicHost,
			DBPort:                     endpoint.DBPort,
			Username:                   localDBUser,
			InsecureSkipCertValidation: true,
			ShellSupported:             endpoint.ShellSupported,
		},
	}
	if err := config.WriteDeploymentInfo(deployment.Root(), info); err != nil {
		return err
	}

	return config.WriteSecrets(deployment.Root(), &config.Secrets{
		DbPassword: localDBPassword,
	})
}

func updateLocalDeploymentArtifactState(deployment config.DeploymentDir, state string) error {
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		return fmt.Errorf("failed to read local deployment info after state change: %w", err)
	}

	info.DeploymentState = state
	info.ClusterState = state

	return config.WriteDeploymentInfo(deployment.Root(), info)
}
