# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests specific to local deployments."""

import json
import socket
from collections.abc import Iterator
from pathlib import Path
from subprocess import CalledProcessError
from typing import Final

import pytest

from framework.deployment import Deployment, StatusStopped
from framework.launcher import DeploymentConfig, Launcher

AUTOMATIC_BLOCKED_PORTS: Final = (8563, 8564)
EXPECTED_AUTOMATIC_PORT: Final = 8565


def _configured_db_port(deployment: Deployment) -> int:
    result = deployment.launcher.run_command(
        "config",
        deployment.deployment_dir.name,
        "get",
        "ports",
        "--json",
        capture_output=True,
    )
    data = json.loads(result.stdout)
    mapping = data["infrastructure"]["options"]["ports"]
    service, separator, raw_port = mapping.partition(":")
    assert (service, separator) == ("db", ":")

    return int(raw_port)


def _reported_db_port(deployment: Deployment) -> int:
    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"

    return int(json.loads(deployment_json.read_text())["connection"]["dbPort"])


def _reserve_port(port: int = 0) -> socket.socket:
    listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    listener.bind(("127.0.0.1", port))
    listener.listen()

    return listener


def _verify_occupied_port_recovery(deployment: Deployment) -> None:
    # Given a stopped deployment is reconfigured to an occupied port
    deployment.stop()
    conflict = _reserve_port()
    try:
        conflict_port = int(conflict.getsockname()[1])
        set_result = deployment.launcher.run_command(
            "config",
            deployment.deployment_dir.name,
            "set",
            "--ports",
            f"db:{conflict_port}",
            capture_output=True,
        )
        assert "exasol start" in set_result.stderr

        # When start reaches the authoritative runtime bind
        with pytest.raises(CalledProcessError) as captured:
            deployment.launcher.run_command(
                "start",
                deployment.deployment_dir.name,
                "--auto-approve",
                capture_output=True,
            )

        # Then it stays stopped and reports both replacement commands
        assert deployment.has_status(StatusStopped)
        assert _configured_db_port(deployment) == conflict_port
        expected_error = (
            f'local service "db" cannot bind configured host port {conflict_port}'
        )
        assert expected_error in captured.value.stderr
        assert "exasol config set --ports db:<available-port>" in captured.value.stderr
        assert "exasol config set --ports auto" in captured.value.stderr
    finally:
        conflict.close()

    # When automatic replacement is selected after releasing the conflict
    reset_result = deployment.launcher.run_command(
        "config",
        deployment.deployment_dir.name,
        "set",
        "--ports",
        "auto",
        capture_output=True,
    )
    replacement_port = _configured_db_port(deployment)
    assert replacement_port > 0
    assert "exasol start" in reset_result.stderr
    deployment.start()

    # Then the replacement endpoint is persisted and reachable
    assert _reported_db_port(deployment) == replacement_port
    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout


@pytest.fixture
def local_ports_deployment(
    exasol_path: str,
    infra: str,
) -> Iterator[tuple[Deployment, int]]:
    if infra != "local":
        pytest.skip("ports override is local-only")

    custom_db_port: Final = 9564
    config = DeploymentConfig(infra="local")

    deployment = Deployment(
        Launcher(exasol_path),
        "--ports",
        f"db:{custom_db_port}",
        config=config,
    )
    try:
        deployment.deploy()
        yield deployment, custom_db_port
    finally:
        deployment.cleanup()


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_ports_override_sets_db_port(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """--ports db:<port> passes the port to the selected local runtime.

    The DB is reachable on the specified port.
    """
    deployment, custom_db_port = local_ports_deployment

    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"
    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_ports_override_stable_across_restarts(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """Port assignments from --ports db:<port> survive a stop/start cycle.

    The custom DB port must remain unchanged in deployment.json and the DB
    must be reachable on that port after the local runtime is restarted.
    """
    deployment, custom_db_port = local_ports_deployment

    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"

    stop_result = deployment.stop()
    assert stop_result.returncode == 0

    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    start_result = deployment.start()
    assert start_result.returncode == 0

    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_static_local_port_selection_reconfiguration_and_recovery(
    exasol_path: str,
    infra: str,
) -> None:
    """Automatic ports are deterministic, stable, configurable, and recoverable."""
    if infra != "local":
        pytest.skip("static local port lifecycle is local-only")

    # Given the database default and its successor are occupied during init
    reservations: list[socket.socket] = []
    deployment: Deployment | None = None
    try:
        try:
            reservations = [_reserve_port(port) for port in AUTOMATIC_BLOCKED_PORTS]
            probe = _reserve_port(EXPECTED_AUTOMATIC_PORT)
            probe.close()
        except OSError as exc:
            for reservation in reservations:
                reservation.close()
            pytest.skip(f"required deterministic test ports are unavailable: {exc}")

        # When a local deployment is initialized with automatic ports
        deployment = Deployment(
            Launcher(exasol_path),
            config=DeploymentConfig(infra="local"),
        )

        # Then allocation advances deterministically and persists the concrete value
        assert _configured_db_port(deployment) == EXPECTED_AUTOMATIC_PORT
        for reservation in reservations:
            reservation.close()
        reservations.clear()

        # When the deployment is started, stopped, and started again
        deployment.deploy()
        assert _reported_db_port(deployment) == EXPECTED_AUTOMATIC_PORT
        deployment.stop()
        assert deployment.has_status(StatusStopped)
        deployment.start()

        # Then its runtime endpoint remains stable
        assert _configured_db_port(deployment) == EXPECTED_AUTOMATIC_PORT
        assert _reported_db_port(deployment) == EXPECTED_AUTOMATIC_PORT

        _verify_occupied_port_recovery(deployment)
    finally:
        for reservation in reservations:
            reservation.close()
        if deployment is not None:
            deployment.cleanup()
