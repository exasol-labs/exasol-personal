# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import os
import platform
import shutil
import subprocess
import sys
from pathlib import Path
from subprocess import CalledProcessError
from typing import Final

import pytest

from .helpers import (
    compatible_preset_pair_or_skip,
    export_preset,
    first_infrastructure_preset_id_or_skip,
    preset_description_or_skip,
    run_command,
)

LOCAL_TEST_DB_PORT = 28563
LOCAL_MINIMUM_MEMORY_MB = 4096
LOCAL_MACHINE = platform.machine().lower()
IS_MACOS_APPLE_SILICON = sys.platform == "darwin" and LOCAL_MACHINE in {
    "arm64",
    "aarch64",
}
IS_LINUX_LOCAL_PLATFORM = sys.platform.startswith("linux") and LOCAL_MACHINE in {
    "amd64",
    "x86_64",
    "arm64",
    "aarch64",
}
IS_WINDOWS_LOCAL_PLATFORM = sys.platform.startswith("win") and LOCAL_MACHINE in {
    "amd64",
    "x86_64",
}
# Linux and Windows both run the database directly through host Podman, so
# they share the "no VM sizing" configuration surface.
IS_HOST_LOCAL_PLATFORM = IS_LINUX_LOCAL_PLATFORM or IS_WINDOWS_LOCAL_PLATFORM
IS_SUPPORTED_LOCAL_PLATFORM = IS_MACOS_APPLE_SILICON or IS_HOST_LOCAL_PLATFORM
LOCAL_VM_SIZING_FLAGS = {"--cpu-count", "--memory-mb", "--data-size-gb"}


def assert_lifecycle_json_signal(
    stdout: str, deployment_state: str, *, database_ready: bool
) -> None:
    assert json.loads(stdout) == {
        "deploymentState": deployment_state,
        "databaseReady": database_ready,
    }


def _assert_running_start_guidance(
    exasol_path: str,
    deployment_dir: Path,
    env: dict[str, str],
) -> None:
    start_result = run_command(
        [
            exasol_path,
            "start",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert start_result.returncode == 0
    assert "deployment state is running, but the database is not ready" in (
        start_result.stderr
    )
    assert "exasol status" in start_result.stderr
    assert "exasol stop" in start_result.stderr
    assert "exasol start" in start_result.stderr


def _assert_already_stopped_guidance(
    exasol_path: str,
    deployment_dir: Path,
    env: dict[str, str],
) -> None:
    second_stop_result = run_command(
        [
            exasol_path,
            "stop",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert second_stop_result.returncode == 0
    assert "deployment is already stopped" in second_stop_result.stderr


def _assert_local_deployment_artifacts(deployment_dir: Path) -> None:
    version_check_marker = (
        deployment_dir / "local" / "runtime" / "start-version-check-enabled"
    )
    assert version_check_marker.read_text() == "false"

    deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
    assert deployment_data["backend"] == "local"
    assert "nodes" not in deployment_data
    assert deployment_data["connection"]["host"] == "127.0.0.1"
    assert deployment_data["connection"]["dbPort"] == LOCAL_TEST_DB_PORT
    assert "adminUi" not in deployment_data["connection"]
    assert "uiPort" not in deployment_data["connection"]
    assert deployment_data["connection"]["insecureSkipCertValidation"] is True

    secrets_data = json.loads((deployment_dir / "secrets.json").read_text())
    assert secrets_data["dbPassword"] == "exasol"
    assert "adminUiPassword" not in secrets_data


def _assert_local_info(
    exasol_path: str,
    deployment_dir: Path,
    env: dict[str, str],
) -> None:
    info_result = run_command(
        [
            exasol_path,
            "info",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    info_data = json.loads(info_result.stdout)
    assert info_data["deploymentState"] == "running"
    assert info_data["connection"]["backend"] == "local"
    assert info_data["connection"]["dbPort"] == LOCAL_TEST_DB_PORT
    assert "adminUi" not in info_data["connection"]
    assert "uiPort" not in info_data["connection"]


def _assert_database_connection_failed_status(
    exasol_path: str,
    deployment_dir: Path,
    env: dict[str, str],
) -> None:
    status_result = run_command(
        [
            exasol_path,
            "status",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    status_data = json.loads(status_result.stdout)
    assert status_data["status"] == "database_connection_failed"


def test_install_requires_infra_preset_arg(exasol_path: str) -> None:
    # Given the install command

    # When it is invoked without arguments
    with pytest.raises(CalledProcessError) as exc:
        run_command([exasol_path, "install"])

    # Then it fails because the required infra preset argument is missing
    assert exc.value.returncode != 0
    assert (
        "requires" in (exc.value.stderr or "").lower()
        or "accepts" in (exc.value.stderr or "").lower()
    )


def test_install_help(exasol_path: str) -> None:
    # Given the install command

    # When help is invoked
    result = run_command([exasol_path, "install", "--help"])
    output: str = result.stdout.strip()

    # Then the output explains the command
    assert "Initialize, apply configuration, and deploy Exasol in one step" in output

    # Then I see which preset names I can pass
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    assert "Available infrastructure presets:" in output
    assert infra_id in output
    assert "Available installation presets:" in output
    assert "exasol presets" in output

    # Then I see how the presets can be combined
    assert "Compatibility matrix" in output


def test_install_help_describes_the_selected_infrastructure_preset(
    exasol_path: str,
) -> None:
    # Given an infrastructure preset
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    description = preset_description_or_skip(exasol_path, "infrastructures", infra_id)

    # When help is invoked for that preset
    result = run_command([exasol_path, "install", infra_id, "--help"])
    output: str = result.stdout.strip()

    # Then the help describes the selected preset and how to combine it
    assert f"Infrastructure preset `{infra_id}`:" in output
    assert description in output
    assert "Compatible installation presets:" in output

    # Then the help exemplifies the command with the selected preset
    assert f"exasol install {infra_id}" in output

    # Then the help does not repeat the overview of every preset
    assert "Available infrastructure presets:" not in output
    assert "Compatibility matrix" not in output

    # Then preset discovery stays reachable
    assert "exasol presets" in output


def test_install_help_describes_both_selected_presets(exasol_path: str) -> None:
    # Given a compatible infrastructure and installation preset pair
    infra_id, install_id = compatible_preset_pair_or_skip(exasol_path)
    install_description = preset_description_or_skip(
        exasol_path, "installations", install_id
    )

    # When help is invoked for both presets
    result = run_command([exasol_path, "install", infra_id, install_id, "--help"])
    output: str = result.stdout.strip()

    # Then the help describes both selected presets
    assert f"Infrastructure preset `{infra_id}`:" in output
    assert f"Installation preset `{install_id}`:" in output
    assert install_description in output

    # Then the help exemplifies the selected pair
    assert f"exasol install {infra_id} {install_id}" in output

    # Then the help does not repeat the overview of every preset
    assert "Available installation presets:" not in output
    assert "Compatibility matrix" not in output


def test_install_help_describes_a_preset_selected_by_path(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an infrastructure preset exported to a directory
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    description = preset_description_or_skip(exasol_path, "infrastructures", infra_id)
    infra_dir = tmp_path / "infra_export"
    infra_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(infra_dir))

    # When help is invoked with that directory as the preset argument
    result = run_command([exasol_path, "install", str(infra_dir), "--help"])
    output: str = result.stdout.strip()

    # Then the help identifies the preset by its path and describes it
    assert f"Infrastructure preset `{infra_dir}`:" in output
    assert description in output
    assert "Compatible installation presets:" in output

    # Then the help does not repeat the overview of every preset
    assert "Available infrastructure presets:" not in output


def test_install_help_quotes_a_preset_path_containing_spaces(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an infrastructure preset exported to a directory whose path contains a space
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    infra_dir = tmp_path / "my preset"
    infra_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(infra_dir))

    # When help is invoked with that directory as the preset argument
    result = run_command([exasol_path, "install", str(infra_dir), "--help"])
    output: str = result.stdout.strip()

    # Then the examples keep the path as a single argument
    assert f'exasol install "{infra_dir}"' in output

    # Then no example splits the path across two arguments
    assert f"exasol install {infra_dir}\n" not in output


def test_install_help_keeps_the_overview_for_an_unresolvable_installation_preset(
    exasol_path: str,
) -> None:
    # Given a resolvable infrastructure preset, and an installation preset
    # argument that names nothing
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When help is invoked for both
    result = run_command([exasol_path, "install", infra_id, "no-such-preset", "--help"])
    output: str = result.stdout.strip()

    # Then the overview of every preset is shown, not a description of one of them
    assert "Available infrastructure presets:" in output
    assert "Compatibility matrix" in output

    # Then no example advertises the preset that could not be described
    assert "no-such-preset" not in output


def test_install_help_keeps_the_overview_for_an_unresolvable_preset(
    exasol_path: str,
) -> None:
    # Given a preset argument naming neither an embedded preset nor a preset directory

    # When help is invoked for it
    result = run_command([exasol_path, "install", "no-such-preset", "--help"])
    output: str = result.stdout.strip()

    # Then help is printed with the overview of every preset
    assert "Available infrastructure presets:" in output
    assert "Available installation presets:" in output
    assert "Compatibility matrix" in output


def test_install_executes_init_step(exasol_path: str, tmp_path: Path) -> None:
    # Given a non-empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    (deployment_dir / "somefile.txt").write_text("x")

    # Given an infrastructure preset ID
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When the install command is invoked
    args = [
        exasol_path,
        "install",
        infra_id,
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as excinfo:
        run_command(args)

    # Then it fails during initialization (proving init ran)
    assert excinfo.value.returncode != 0
    stderr = (excinfo.value.stderr or "").lower()
    assert "initialization failed" in stderr
    assert "deployment directory is not empty" in stderr


@pytest.mark.skipif(
    IS_SUPPORTED_LOCAL_PLATFORM,
    reason="local deployments are supported on this platform",
)
def test_init_local_rejects_unsupported_platform_before_writing_files(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an empty deployment directory on an unsupported local platform
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked for the local preset
    args = [
        exasol_path,
        "init",
        "local",
        "--deployment-dir",
        str(deployment_dir),
        "--no-launcher-version-check",
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it fails before writing deployment state
    stderr = exc.value.stderr.lower()
    assert (
        "local deployments are only supported on macos apple silicon, "
        "linux amd64/arm64, and windows amd64" in stderr
    )
    assert list(deployment_dir.iterdir()) == []


@pytest.mark.skipif(
    not IS_HOST_LOCAL_PLATFORM,
    reason="host configuration is only exposed on supported host platforms",
)
def test_init_local_host_help_exposes_only_host_parameters(exasol_path: str) -> None:
    # Given the local preset on a supported direct-host platform

    # When init help is requested
    result = run_command([exasol_path, "init", "local", "--help"])

    # Then the host port option is exposed without VM sizing options
    assert "--ports" in result.stdout
    for flag in LOCAL_VM_SIZING_FLAGS:
        assert flag not in result.stdout


@pytest.mark.skipif(
    not IS_HOST_LOCAL_PLATFORM,
    reason="host configuration is only exposed on supported host platforms",
)
def test_init_local_host_configuration_omits_vm_sizing(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an initialized local deployment on a direct-host platform
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When the active configuration is queried
    result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then only the host-relevant local option is reported
    options = json.loads(result.stdout)["infrastructure"]["options"]
    assert set(options) == {"ports"}
    service, separator, raw_port = options["ports"].partition(":")
    assert service == "db"
    assert separator == ":"
    assert int(raw_port) > 0


@pytest.mark.skipif(
    not IS_MACOS_APPLE_SILICON,
    reason="local memory sizing applies only to the macOS VM runtime",
)
def test_init_local_accepts_explicit_minimum_memory(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a local macOS VM deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with the minimum supported memory
    result = run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--memory-mb",
            "4096",
        ]
    )

    # Then the deployment is initialized with that value
    assert result.returncode == 0
    config_result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "memory-mb",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    config_data = json.loads(config_result.stdout)
    configured_memory_mb = config_data["infrastructure"]["options"]["memory-mb"]
    assert configured_memory_mb == LOCAL_MINIMUM_MEMORY_MB


@pytest.mark.skipif(
    not IS_MACOS_APPLE_SILICON,
    reason="local memory sizing applies only to the macOS VM runtime",
)
def test_init_local_rejects_memory_below_minimum(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a local macOS VM deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked below the supported minimum memory
    args = [
        exasol_path,
        "init",
        "local",
        "--deployment-dir",
        str(deployment_dir),
        "--memory-mb",
        "4095",
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then the user sees the minimum-memory validation message
    assert (
        "local memory-mb must be at least 4096 mb" in (exc.value.stderr or "").lower()
    )
    # Then validation happened before extraction, leaving the directory empty
    # so a corrected retry is not blocked by leftover preset files.
    assert list(deployment_dir.iterdir()) == []


@pytest.mark.skipif(
    not IS_MACOS_APPLE_SILICON,
    reason="fake local runner exercises the macOS VM runtime",
)
def test_deploy_local_with_fake_runner_override(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a fake local runner that implements the runner contract without a VM
    runner = tmp_path / "fake-mac-runner-aarch64"
    runner.write_text(
        """#!/usr/bin/env sh
set -eu
case "$1" in
  version)
    # A version far ahead of anything real, so version reconciliation
    # never mistakes this fake runner for a downgrade.
    printf 'v999.0.0\n'
    ;;
  init)
    mkdir -p vm vm-shared
    ;;
  start)
    shift
    version_check_enabled=""
    while [ "$#" -gt 3 ]; do
      case "$1" in
        --version-check-enabled=*)
          version_check_enabled="${1#*=}"
          shift
          ;;
        --version-check-enabled)
          version_check_enabled="$2"
          shift 2
          ;;
        --version-check-url|--version-check-identity|--version-check-interval-seconds|--ports)
          shift 2
          ;;
        *)
          echo "unexpected start flag: $1" >&2
          exit 3
          ;;
      esac
    done
    if [ "${version_check_enabled}" != "false" ]; then
      echo "expected disabled version check, got: ${version_check_enabled}" >&2
      exit 3
    fi
    printf '%s' "${version_check_enabled}" > start-version-check-enabled
    if [ "$1" != "2" ] || [ "$2" != "2048" ] || [ "$3" != "100" ]; then
      echo "unexpected sizing args: $*" >&2
      exit 3
    fi
    mkdir -p vm vm-shared
    cat > vm-state.json <<'JSON'
{"vm_name":"exasol-local-vm","vm_ip":"192.168.64.2","ports":{"ssh":20022,"db":28563,"ui":0}}
JSON
    ;;
  stop)
    exit 0
    ;;
  *)
    echo "unexpected command: $1" >&2
    exit 2
    ;;
esac
"""
    )
    runner.chmod(0o700)

    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    env = {
        **os.environ,
        "EXASOL_LOCAL_SKIP_DB_WAIT": "1",
        # exasol-local-runner is embed-only, so this override injects the
        # fake executable without replacing the embedded artifact.
        "EXASOL_LOCAL_RUNNER_OVERRIDE_PATH": str(runner),
    }

    # Given a local deployment directory
    init_result = run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ],
        env=env,
    )
    assert init_result.returncode == 0
    config_result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "memory-mb",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    expected_memory_mb = json.loads(config_result.stdout)["infrastructure"]["options"][
        "memory-mb"
    ]

    runner.write_text(runner.read_text().replace('"2048"', str(expected_memory_mb)))

    # When local deploy is invoked
    result = run_command(
        [
            exasol_path,
            "deploy",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )

    # Then the deployment is initialized and local connection artifacts are written
    assert result.returncode == 0
    _assert_local_deployment_artifacts(deployment_dir)

    # Then info can consume the local artifacts without cloud-specific assumptions
    _assert_local_info(exasol_path, deployment_dir, env)

    # Then status also handles the local artifacts even though the fake DB is absent
    _assert_database_connection_failed_status(exasol_path, deployment_dir, env)

    # Then start on a running deployment gives recovery guidance instead of failing
    _assert_running_start_guidance(exasol_path, deployment_dir, env)

    # Then stop updates persisted state and emits a JSON ready signal
    stop_result = run_command(
        [
            exasol_path,
            "stop",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert stop_result.returncode == 0
    assert_lifecycle_json_signal(stop_result.stdout, "stopped", database_ready=False)
    deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
    assert deployment_data["deploymentState"] == "stopped"
    assert deployment_data["clusterState"] == "stopped"

    # Then stopping an already stopped deployment is an idempotent no-op
    _assert_already_stopped_guidance(exasol_path, deployment_dir, env)

    # Then start emits only the JSON ready signal on stdout
    start_result = run_command(
        [
            exasol_path,
            "start",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert start_result.returncode == 0
    assert_lifecycle_json_signal(start_result.stdout, "running", database_ready=True)
    assert "Connection Instructions" not in start_result.stdout


def _windows_podman_path() -> Path:
    """Resolve podman.exe at the location winget installs RedHat.Podman to."""
    return Path(os.environ["PROGRAMFILES"]) / "RedHat" / "Podman" / "podman.exe"


def _podman_query(podman_path: Path, *args: str) -> str:
    """Run a read-only podman query outside the launcher, for verification."""
    return subprocess.run(
        [str(podman_path), *args],
        capture_output=True,
        text=True,
        check=True,
    ).stdout


def _run_podman(podman_path: Path, *args: str) -> None:
    subprocess.run(
        [str(podman_path), *args],
        capture_output=True,
        text=True,
        check=True,
    )


def _windows_machine_state(podman_path: Path) -> str:
    return (
        _podman_query(
            podman_path,
            "machine",
            "inspect",
            "--format",
            "{{.State}}",
            "podman-machine-default",
        )
        .strip()
        .lower()
    )


def _windows_container_names(podman_path: Path) -> list[str]:
    return _podman_query(podman_path, "ps", "-a", "--format", "{{.Names}}").splitlines()


def _assert_windows_host_data_passthrough(
    podman_path: Path,
    container_name: str,
    host_exa: Path,
    tmp_path: Path,
) -> None:
    mounts = json.loads(
        _podman_query(
            podman_path,
            "inspect",
            "--format",
            "{{json .Mounts}}",
            container_name,
        )
    )
    exa_mounts = [mount for mount in mounts if mount["Destination"] == "/exa"]
    assert len(exa_mounts) == 1
    assert exa_mounts[0]["Type"] == "bind"
    assert host_exa.is_dir()

    host_file = host_exa / "host-visible"
    host_file.write_text("from-host")
    host_roundtrip = tmp_path / "host-roundtrip"
    _run_podman(
        podman_path,
        "cp",
        f"{container_name}:/exa/host-visible",
        str(host_roundtrip),
    )
    assert host_roundtrip.read_text() == "from-host"

    guest_source = tmp_path / "guest-source"
    guest_source.write_text("from-guest")
    _run_podman(
        podman_path,
        "cp",
        str(guest_source),
        f"{container_name}:/exa/guest-visible",
    )
    assert (host_exa / "guest-visible").read_text() == "from-guest"


# The flags the Windows lifecycle test drives, kept next to it so the two stay
# in step. Appending --help makes the launcher parse flags and exit without
# side effects, so an unknown flag surfaces as a non-zero exit anywhere.
WINDOWS_LIFECYCLE_FLAG_PROBES: Final = [
    (
        "install",
        ["local", "--auto-approve", "--no-launcher-version-check", "--verbose"],
    ),
    ("connect", ["--csv", "--command", "SELECT 1;"]),
    ("stop", ["--json"]),
    ("start", ["--json", "--auto-approve"]),
    ("destroy", ["--auto-approve", "--verbose"]),
]


@pytest.mark.parametrize(("command", "args"), WINDOWS_LIFECYCLE_FLAG_PROBES)
def test_windows_lifecycle_flags_are_accepted(
    exasol_path: str, command: str, args: list[str]
) -> None:
    """Every flag the Windows lifecycle test uses must exist.

    That test is skipped everywhere but Windows, so a flag that does not exist
    is invisible until CI runs it. This probe runs on all platforms.
    """
    # When the flags are parsed and the command exits at --help
    result = run_command([exasol_path, command, *args, "--help"])

    # Then parsing succeeded
    assert result.returncode == 0


@pytest.mark.skipif(
    not IS_WINDOWS_LOCAL_PLATFORM,
    reason="the real Windows host lifecycle runs only on Windows amd64",
)
def test_install_local_windows_lifecycle(exasol_path: str, tmp_path: Path) -> None:
    """Exercise Windows Podman preparation, persistence, and destroy cleanup."""
    deployment_dir = tmp_path / "windows-local-deployment"
    state_path = deployment_dir / ".exasolLauncherState.json"
    destroyed = False

    # Given no Podman on PATH, so the winget install and registered-PATH
    # refresh stay covered by the standard Windows job rather than being
    # skipped because CI pre-installed Podman.
    assert shutil.which("podman") is None

    try:
        # When the deployment is installed unattended
        install_result = run_command(
            [
                exasol_path,
                "install",
                "local",
                "--deployment-dir",
                str(deployment_dir),
                "--auto-approve",
                # --no-launcher-version-check also disables the local database
                # version check. --no-db-version-check is a variable of the
                # cloud installation preset and is not a flag of `install local`.
                "--no-launcher-version-check",
                "--verbose",
            ]
        )
        assert install_result.returncode == 0

        # Then Podman was installed and its machine is running
        podman_path = _windows_podman_path()
        assert podman_path.is_file()
        assert _windows_machine_state(podman_path) == "running"

        deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
        container_name = f"exasol-db-{deployment_data['deploymentId']}"
        host_exa = deployment_dir / "local" / "runtime" / "exa"

        # Then Nano uses one host bind at /exa with visibility in both directions
        _assert_windows_host_data_passthrough(
            podman_path, container_name, host_exa, tmp_path
        )

        # Then the database accepts SQL over the published loopback port
        run_command(
            [
                exasol_path,
                "connect",
                "--deployment-dir",
                str(deployment_dir),
                "--command",
                # A fresh database has no schema open, so one must be created
                # and opened before any table can be referenced.
                (
                    "CREATE SCHEMA WINDOWS_LIFECYCLE; "
                    "OPEN SCHEMA WINDOWS_LIFECYCLE; "
                    "CREATE TABLE SMOKE (ID DECIMAL(18,0)); "
                    "INSERT INTO SMOKE VALUES 424242;"
                ),
            ]
        )

        # When the deployment is stopped
        stop_result = run_command(
            [
                exasol_path,
                "stop",
                "--json",
                "--deployment-dir",
                str(deployment_dir),
            ]
        )
        assert_lifecycle_json_signal(
            stop_result.stdout, "stopped", database_ready=False
        )
        assert container_name not in _windows_container_names(podman_path)

        # When it is started again
        start_result = run_command(
            [
                exasol_path,
                "start",
                "--json",
                "--deployment-dir",
                str(deployment_dir),
                "--auto-approve",
            ]
        )
        assert_lifecycle_json_signal(
            start_result.stdout, "running", database_ready=True
        )

        # Then the mounted data survived the restart
        persisted = run_command(
            [
                exasol_path,
                "connect",
                "--csv",
                "--deployment-dir",
                str(deployment_dir),
                "--command",
                "SELECT ID FROM WINDOWS_LIFECYCLE.SMOKE;",
            ]
        )
        assert "424242" in persisted.stdout

        # When the Podman machine is stopped underneath the running deployment,
        # the container survives without a clean stop — the launcher's own
        # `stop` removes it, so only an out-of-band shutdown leaves one behind.
        subprocess.run(
            [str(podman_path), "machine", "stop"],
            capture_output=True,
            text=True,
            check=True,
        )

        # Then start recovers on its own: it restarts the machine, reconciles
        # the stale running state, and reuses the existing data rather than
        # mistaking the surviving container for a legacy one to migrate.
        recovered = run_command(
            [
                exasol_path,
                "start",
                "--json",
                "--deployment-dir",
                str(deployment_dir),
                "--auto-approve",
            ]
        )
        assert_lifecycle_json_signal(recovered.stdout, "running", database_ready=True)

        survived = run_command(
            [
                exasol_path,
                "connect",
                "--csv",
                "--deployment-dir",
                str(deployment_dir),
                "--command",
                "SELECT ID FROM WINDOWS_LIFECYCLE.SMOKE;",
            ]
        )
        assert "424242" in survived.stdout

        # When the deployment is destroyed
        run_command(
            [
                exasol_path,
                "destroy",
                "--deployment-dir",
                str(deployment_dir),
                "--auto-approve",
                "--verbose",
            ]
        )
        destroyed = True

        # Then the container and local runtime files are gone
        assert container_name not in _windows_container_names(podman_path)
        assert not (deployment_dir / "local").exists()
        assert not host_exa.exists()

        # Then the shared Podman machine is left running: it is host-wide
        # state the launcher borrows, not deployment state it owns.
        assert _windows_machine_state(podman_path) == "running"
    finally:
        if not destroyed and state_path.exists():
            subprocess.run(
                [
                    exasol_path,
                    "destroy",
                    "--deployment-dir",
                    str(deployment_dir),
                    "--auto-approve",
                    "--verbose",
                ],
                capture_output=True,
                text=True,
                check=False,
            )
