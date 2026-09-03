// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localports"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	nanoImageOverridePathEnv  = "EXASOL_LOCAL_NANO_IMAGE_OVERRIDE_PATH"
	exasolNanoImageResourceID = "exasol-nano-image"
	loadedImagePrefix         = "Loaded image:"
	dataDirMode               = 0o750
	nanoInternalDBPort        = 8563
	nanoShmSize               = "512mb"
	nanoPIDsLimit             = "-1"
	nanoSecurityOpt           = "unmask=ALL"
	nanoRestartPolicy         = "always"
	// nanoDataMountPath is where Nano expects its persistent data inside the
	// container; the target adds SELinux relabelling for the bind mount.
	nanoDataMountPath        = "/exa"
	nanoDataMountTarget      = nanoDataMountPath + ":Z"
	podmanDiagnosticsTimeout = 10 * time.Second
	nanoVersionCheckDefault  = 86400
	nanoVersionCheckMin      = 60
	nanoVersionCheckMax      = 604800
	nanoVersionCheckRetryMax = 86400
)

type PodmanInstall struct {
	deployment    config.DeploymentDir
	environment   ExecutionEnvironment
	resolveImage  func(context.Context) (RuntimePath, error)
	slcStagingDir RuntimePath
	slcStatusPath RuntimePath
	portAvailable func(string, int) bool
}

func NewPodmanInstall(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
	runtimeExec []string,
	slcStagingDir string,
	slcStatusPath string,
) *PodmanInstall {
	directEnvironment := NewDirectExecutionEnvironment(runtimeExec)
	resolveImage := func(ctx context.Context) (RuntimePath, error) {
		path, err := ResolveNanoImage(ctx, manager)
		if err != nil {
			return RuntimePath{}, err
		}

		return IdentityRuntimePath(path), nil
	}

	return NewPodmanInstallWithEnvironment(
		deployment,
		directEnvironment,
		resolveImage,
		IdentityRuntimePath(slcStagingDir),
		IdentityRuntimePath(slcStatusPath),
	)
}

// ResolveNanoImage materializes the platform-specific image archive on the host.
// Runtimes that execute Podman elsewhere can stage and map this host path before
// passing it back to PodmanInstall.
func ResolveNanoImage(
	ctx context.Context,
	manager *runtimeartifacts.Manager,
) (string, error) {
	if override := strings.TrimSpace(os.Getenv(nanoImageOverridePathEnv)); override != "" {
		return override, nil
	}
	if manager == nil {
		return "", errors.New("nano runtime artifact manager is required")
	}

	return manager.Request(ctx, exasolNanoImageResourceID)
}

func NewPodmanInstallWithEnvironment(
	deployment config.DeploymentDir,
	environment ExecutionEnvironment,
	resolveImage func(context.Context) (RuntimePath, error),
	slcStagingDir RuntimePath,
	slcStatusPath RuntimePath,
) *PodmanInstall {
	return &PodmanInstall{
		deployment:    deployment,
		environment:   environment,
		resolveImage:  resolveImage,
		slcStagingDir: slcStagingDir,
		slcStatusPath: slcStatusPath,
		portAvailable: localports.IsAvailable,
	}
}

func (install *PodmanInstall) Start(
	ctx context.Context,
	out, outErr io.Writer,
	startConfig StartConfig,
) error {
	if err := install.validateStartConfig(startConfig); err != nil {
		return err
	}

	containerName, err := getContainerName(install.deployment)
	if err != nil {
		return err
	}
	if len(startConfig.LegacyContainerNames) > 0 {
		if err := install.adoptLegacyContainerName(
			ctx, out, outErr, containerName, startConfig.LegacyContainerNames,
		); err != nil {
			return err
		}
	}
	containerStatus, err := install.inspectContainerStatus(ctx, outErr, containerName)
	if err != nil {
		return err
	}
	migrated, err := install.migrateOverlayDataIfNeeded(
		ctx,
		out,
		outErr,
		containerName,
		startConfig.DataDir,
		containerStatus,
	)
	if err != nil {
		return err
	}
	if migrated {
		containerStatus = podmanContainerStatus{}
	}
	recovered, err := install.recoverIncompleteInitialCreate(
		ctx, out, outErr, containerName, startConfig.DataDir, containerStatus,
	)
	if err != nil {
		return err
	}
	if recovered {
		containerStatus = podmanContainerStatus{}
	}
	if containerStatus.Running {
		return nil
	}

	if err := install.environment.MkdirAll(ctx, startConfig.DataDir, dataDirMode); err != nil {
		return fmt.Errorf("failed to create Nano data directory %s: %w", startConfig.DataDir, err)
	}
	freshDeployment, err := install.isFreshDeployment(ctx, startConfig.DataDir)
	if err != nil {
		return err
	}
	if freshDeployment {
		if err := install.removeStaleNanoTLSFiles(ctx, startConfig.DataDir); err != nil {
			return err
		}
	}

	imagePath, err := install.resolveImagePath(ctx)
	if err != nil {
		return install.failureWithDiagnostics(
			ctx, outErr, containerName, fmt.Errorf("failed to resolve Nano image: %w", err),
		)
	}
	loadOutput, err := install.runPodmanOutput(
		ctx, out, outErr, "load", "-i", imagePath.RuntimePath,
	)
	if err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName,
			fmt.Errorf("failed to load Nano image %s: %w", imagePath.RuntimePath, err))
	}
	loadedImage, err := parseLoadedImage(loadOutput)
	if err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName, err)
	}

	availableSLCs, err := install.materializeSLCs(ctx, out, outErr, containerName, startConfig.SLCs)
	if err != nil {
		return err
	}
	install.pruneUnreferencedSLCImages(ctx, outErr, startConfig.SLCs)
	if err := install.environment.Sync(ctx, out, outErr); err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName,
			fmt.Errorf("failed to synchronize local runtime storage before starting Nano: %w", err))
	}

	args := []string{
		"run", "-d", "--replace",
		"--name", containerName,
		"--shm-size=" + nanoShmSize,
		"--pids-limit=" + nanoPIDsLimit,
		"--security-opt", nanoSecurityOpt,
		"--restart", nanoRestartPolicy,
		"-p", podmanDBPortMapping(startConfig),
		"-v", startConfig.DataDir + ":" + nanoDataMountTarget,
	}
	for _, slc := range availableSLCs {
		args = append(args,
			"--mount",
			fmt.Sprintf("type=image,source=%s,destination=%s", slc.Image, slc.Target),
		)
	}
	if startConfig.VersionCheck.Enabled {
		args = append(args, "-e", "VERSION_CHECK_IDENTITY="+startConfig.VersionCheck.Identity)
	}
	args = append(args, loadedImage, "init")
	if freshDeployment && len(startConfig.InitParams) > 0 {
		args = append(args, "params="+strings.Join(startConfig.InitParams, " "))
	}
	args = append(args, nanoVersionCheckInitArgs(startConfig.VersionCheck)...)
	var startDiagnostic bytes.Buffer
	startErrOutput := io.Writer(&startDiagnostic)
	if outErr != nil {
		startErrOutput = io.MultiWriter(outErr, &startDiagnostic)
	}
	if err := install.runPodman(ctx, out, startErrOutput, args...); err != nil {
		failure := fmt.Errorf("failed to start Nano container %s: %w", containerName, err)
		failure = classifyPodmanStartBindFailure(
			ctx,
			install,
			outErr,
			containerName,
			startConfig,
			failure,
			startDiagnostic.String(),
		)

		return install.failureWithDiagnostics(ctx, outErr, containerName,
			failure)
	}

	return nil
}

func classifyPodmanStartBindFailure(
	ctx context.Context,
	install *PodmanInstall,
	outErr io.Writer,
	containerName string,
	startConfig StartConfig,
	failure error,
	diagnostic string,
) error {
	if startConfig.ContainerDBBindHost == "" {
		return failure
	}

	status, err := install.inspectContainerStatus(ctx, outErr, containerName)
	if err != nil || status.Running {
		return failure
	}
	available := install.portAvailable
	if available == nil {
		available = localports.IsAvailable
	}

	return localports.ClassifyBindFailure(
		"db",
		startConfig.ContainerDBPort,
		failure,
		diagnostic,
		func() bool {
			return available(startConfig.ContainerDBBindHost, startConfig.ContainerDBPort)
		},
	)
}

func podmanDBPortMapping(startConfig StartConfig) string {
	ports := fmt.Sprintf("%d:%d", startConfig.ContainerDBPort, nanoInternalDBPort)
	if startConfig.ContainerDBBindHost == "" {
		return ports
	}

	return startConfig.ContainerDBBindHost + ":" + ports
}

func (install *PodmanInstall) Stop(ctx context.Context, out, outErr io.Writer) error {
	containerName, err := getContainerName(install.deployment)
	if err != nil {
		return err
	}
	if err := install.runPodman(
		ctx,
		out,
		outErr,
		"rm",
		"--force",
		"--ignore",
		containerName,
	); err != nil {
		return fmt.Errorf("failed to remove Nano container %s: %w", containerName, err)
	}

	return nil
}

func (install *PodmanInstall) Status(
	ctx context.Context,
	_, outErr io.Writer,
) (*InstallStatus, error) {
	containerName, err := getContainerName(install.deployment)
	if err != nil {
		return nil, err
	}
	running, err := install.containerRunning(ctx, outErr, containerName)
	if err != nil {
		return nil, err
	}

	return &InstallStatus{Running: running}, nil
}

func (install *PodmanInstall) Destroy(ctx context.Context, out, outErr io.Writer) error {
	return install.Stop(ctx, out, outErr)
}

func (install *PodmanInstall) adoptLegacyContainerName(
	ctx context.Context,
	out, outErr io.Writer,
	containerName string,
	legacyNames []string,
) error {
	exists, err := install.containerExists(ctx, outErr, containerName)
	if err != nil || exists {
		return err
	}
	for _, legacyName := range legacyNames {
		legacyName = strings.TrimSpace(legacyName)
		if legacyName == "" || legacyName == containerName {
			continue
		}
		exists, err := install.containerExists(ctx, outErr, legacyName)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := install.runPodman(
			ctx, out, outErr, "rename", legacyName, containerName,
		); err != nil {
			return install.failureWithDiagnostics(
				ctx,
				outErr,
				legacyName,
				fmt.Errorf(
					"failed to adopt legacy Nano container %s as %s: %w",
					legacyName,
					containerName,
					err,
				),
			)
		}

		return nil
	}

	return nil
}

func (install *PodmanInstall) failureWithDiagnostics(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
	failure error,
) error {
	install.writePodmanDiagnostics(ctx, outErr, containerName)

	return failure
}

func (install *PodmanInstall) writePodmanDiagnostics(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) {
	diagnosticOut := outErr
	if diagnosticOut == nil {
		diagnosticOut = io.Discard
	}
	diagnosticCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		podmanDiagnosticsTimeout,
	)
	defer cancel()

	_, _ = fmt.Fprintf(diagnosticOut, "Podman diagnostics for %s:\n", containerName)
	commands := [][]string{
		{"info"},
		{"ps", "-a"},
		{"container", "inspect", containerName},
		{"logs", containerName},
	}
	for _, command := range commands {
		_, _ = fmt.Fprintf(diagnosticOut, "$ podman %s\n", strings.Join(command, " "))
		if err := install.runPodman(
			diagnosticCtx, diagnosticOut, diagnosticOut, command...,
		); err != nil {
			_, _ = fmt.Fprintf(diagnosticOut, "diagnostic command failed: %v\n", err)
		}
	}
}

func (install *PodmanInstall) validateStartConfig(startConfig StartConfig) error {
	if startConfig.ContainerDBPort <= 0 || startConfig.ContainerDBPort > 65535 {
		return fmt.Errorf("invalid Nano published DB port %d", startConfig.ContainerDBPort)
	}
	if strings.TrimSpace(startConfig.DataDir) == "" {
		return errors.New("nano data directory is required")
	}
	if startConfig.VersionCheck.Enabled {
		for name, value := range map[string]string{
			"URL":              startConfig.VersionCheck.URL,
			"identity":         startConfig.VersionCheck.Identity,
			"operating system": startConfig.VersionCheck.OperatingSystem,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("nano version-check %s is required when enabled", name)
			}
		}
	}
	for index, slc := range startConfig.SLCs {
		if strings.TrimSpace(slc.Image) == "" {
			return fmt.Errorf("nano SLC %d image is required", index)
		}
		if strings.TrimSpace(slc.Target) == "" {
			return fmt.Errorf("nano SLC %d target is required", index)
		}
	}
	if startConfig.SLCs != nil {
		if strings.TrimSpace(install.slcStatusPath.RuntimePath) == "" {
			return errors.New("SLC status path is required for SLC-aware startup")
		}
		if strings.TrimSpace(install.slcStagingDir.RuntimePath) == "" {
			return errors.New("SLC staging directory is required for SLC-aware startup")
		}
	}

	return nil
}

func nanoVersionCheckInitArgs(versionCheck VersionCheckConfig) []string {
	if !versionCheck.Enabled {
		return []string{"VERSION_CHECK_ENABLED=0"}
	}
	interval := versionCheck.IntervalSeconds
	if interval == 0 {
		interval = nanoVersionCheckDefault
	}
	interval = clamp(interval, nanoVersionCheckMin, nanoVersionCheckMax)
	retryInterval := clamp(interval, nanoVersionCheckMin, nanoVersionCheckRetryMax)

	return []string{
		"VERSION_CHECK_ENABLED=1",
		"VERSION_CHECK_ENDPOINT=" + versionCheck.URL,
		fmt.Sprintf("VERSION_CHECK_INTERVAL_SEC=%d", interval),
		fmt.Sprintf("VERSION_CHECK_RETRY_INTERVAL_SEC=%d", retryInterval),
		"VERSION_CHECK_OPERATING_SYSTEM=" + versionCheck.OperatingSystem,
	}
}

func clamp(value, minimum, maximum int) int {
	return min(max(value, minimum), maximum)
}

func (install *PodmanInstall) isFreshDeployment(
	ctx context.Context,
	dataDir string,
) (bool, error) {
	configPath := filepath.Join(dataDir, "exasol.conf")
	exists, err := install.environment.PathExists(ctx, configPath)
	if err != nil {
		return false, fmt.Errorf("failed to inspect Nano configuration %s: %w", configPath, err)
	}

	return !exists, nil
}

func parseLoadedImage(output string) (string, error) {
	var loadedImage string
	for line := range strings.SplitSeq(output, "\n") {
		_, image, found := strings.Cut(line, loadedImagePrefix)
		if !found {
			continue
		}
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		if loadedImage != "" && loadedImage != image {
			return "", errors.New("podman load reported multiple Nano images")
		}
		loadedImage = image
	}
	if loadedImage == "" {
		return "", errors.New("could not determine the image reference reported by podman load")
	}

	return loadedImage, nil
}

func (install *PodmanInstall) containerRunning(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (bool, error) {
	status, err := install.inspectContainerStatus(ctx, outErr, containerName)
	return status.Running, err
}

type podmanContainerStatus struct {
	Exists  bool
	Running bool
}

func (install *PodmanInstall) inspectContainerStatus(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (podmanContainerStatus, error) {
	exists, err := install.containerExists(ctx, outErr, containerName)
	if err != nil || !exists {
		return podmanContainerStatus{}, err
	}
	output, err := install.runPodmanOutput(
		ctx,
		nil,
		outErr,
		"container",
		"inspect",
		"--format",
		"{{.State.Running}}",
		containerName,
	)
	if err != nil {
		return podmanContainerStatus{}, fmt.Errorf(
			"failed to inspect Nano container %s: %w", containerName, err,
		)
	}
	running, err := strconv.ParseBool(strings.TrimSpace(output))
	if err != nil {
		return podmanContainerStatus{}, fmt.Errorf(
			"failed to parse running state for Nano container %s from %q: %w",
			containerName,
			strings.TrimSpace(output),
			err,
		)
	}

	return podmanContainerStatus{Exists: true, Running: running}, nil
}

func (install *PodmanInstall) containerExists(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (bool, error) {
	err := install.runPodman(
		ctx,
		nil,
		outErr,
		"container",
		"exists",
		containerName,
	)
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("failed to find Nano container %s: %w", containerName, err)
}

func (install *PodmanInstall) resolveImagePath(ctx context.Context) (RuntimePath, error) {
	if install.resolveImage == nil {
		return RuntimePath{}, errors.New("nano runtime artifact resolver is required")
	}
	path, err := install.resolveImage(ctx)
	if err != nil {
		return RuntimePath{}, err
	}
	if strings.TrimSpace(path.RuntimePath) == "" {
		return RuntimePath{}, errors.New("nano runtime artifact path is required")
	}

	return path, nil
}

func (install *PodmanInstall) runPodman(
	ctx context.Context,
	out, outErr io.Writer,
	arg ...string,
) error {
	return install.environment.Run(ctx, nil, out, outErr, append([]string{"podman"}, arg...)...)
}

func (install *PodmanInstall) runPodmanOutput(
	ctx context.Context,
	out, outErr io.Writer,
	arg ...string,
) (string, error) {
	var stdout bytes.Buffer
	if out == nil {
		out = &stdout
	} else {
		out = io.MultiWriter(out, &stdout)
	}
	err := install.environment.Run(
		ctx, nil, out, outErr, append([]string{"podman"}, arg...)...,
	)

	return stdout.String(), err
}

func ContainerName(deployment config.DeploymentDir) (string, error) {
	launcherState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return "", err
	}
	deploymentID := strings.TrimSpace(launcherState.DeploymentId)
	if deploymentID == "" {
		return "", errors.New("deployment state is missing deployment ID")
	}

	return "exasol-db-" + deploymentID, nil
}

func getContainerName(deployment config.DeploymentDir) (string, error) {
	return ContainerName(deployment)
}
