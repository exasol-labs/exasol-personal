# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import subprocess
from pathlib import Path

import pytest

from .helpers import (
    deployments_root,
    env_with_home,
    first_infrastructure_preset_id_or_skip,
    run_command,
)


def _deployment_dir_logged(stderr: str, deployment_dir: Path, source: str) -> bool:
    for line in stderr.splitlines():
        try:
            decoded: object = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(decoded, dict):
            continue
        if decoded.get("msg") != "using deployment directory":
            continue
        if decoded.get("source") != source:
            continue

        logged_path = decoded.get("path")
        if isinstance(logged_path, str) and Path(logged_path) == deployment_dir:
            return True

    return False


def _assert_deployment_dir_logged(
    stderr: str, deployment_dir: Path, source: str
) -> None:
    assert _deployment_dir_logged(stderr, deployment_dir, source), (
        f"expected {source} deployment directory {str(deployment_dir)!r} "
        f"in debug stderr logs:\n{stderr}"
    )


def _infrastructure_presets_or_skip(exasol_path: str, count: int) -> list[str]:
    result = run_command([exasol_path, "presets", "list", "--json"])
    data = json.loads(result.stdout)
    infra_presets = data.get("infrastructures")
    if not isinstance(infra_presets, list) or len(infra_presets) < count:
        pytest.skip(f"need at least {count} infrastructure presets")

    ids: list[str] = []
    for preset in infra_presets[:count]:
        preset_id = preset.get("id")
        if not isinstance(preset_id, str):
            pytest.skip("infrastructure preset list contains invalid IDs")
        ids.append(preset_id)

    return ids


def test_status_uses_default_deployment_dir_without_corrupting_json(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a home directory without a default deployment directory
    home = tmp_path / "home"
    cwd = tmp_path / "work"
    home.mkdir()
    cwd.mkdir()
    default_dir = deployments_root(home) / "default"
    launcher = str(Path(exasol_path).resolve())

    # When status is invoked outside a deployment directory
    result = subprocess.run(
        [launcher, "--log-level", "debug", "status", "--json"],
        cwd=cwd,
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then stdout stays parseable JSON and status reports the default directory
    data = json.loads(result.stdout)
    assert data["status"] == "not_initialized"
    assert data["deploymentDir"] == str(default_dir)
    _assert_deployment_dir_logged(result.stderr, default_dir, "default")
    assert f"Using default deployment directory: {default_dir}" in result.stderr


def test_status_uses_named_deployment_dir_without_corrupting_json(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a home directory without a named deployment directory
    home = tmp_path / "home"
    cwd = tmp_path / "work"
    home.mkdir()
    cwd.mkdir()
    named_dir = deployments_root(home) / "staging"
    launcher = str(Path(exasol_path).resolve())

    # When status is invoked with --deployment and no --deployment-dir
    result = subprocess.run(
        [
            launcher,
            "--log-level",
            "debug",
            "status",
            "--json",
            "--deployment",
            "staging",
        ],
        cwd=cwd,
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then stdout stays parseable JSON and status reports the named directory
    data = json.loads(result.stdout)
    assert data["status"] == "not_initialized"
    assert data["deploymentDir"] == str(named_dir)
    _assert_deployment_dir_logged(result.stderr, named_dir, "named")
    assert f'Using named deployment directory "staging": {named_dir}' in result.stderr


def test_status_reports_uninitialized_named_deployment_dir(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a home directory without a named deployment directory
    home = tmp_path / "home"
    home.mkdir()
    named_dir = deployments_root(home) / "staging"
    launcher = str(Path(exasol_path).resolve())

    # When status is invoked with --deployment
    result = subprocess.run(
        [launcher, "status", "--json", "--deployment", "staging"],
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then it succeeds and reports not_initialized for the resolved named directory
    data = json.loads(result.stdout)
    assert data["status"] == "not_initialized"
    assert data["deploymentDir"] == str(named_dir)


def test_named_deployment_dir_wins_over_current_directory(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given the current working directory is itself a different recognized deployment
    home = tmp_path / "home"
    home.mkdir()
    current_dir = tmp_path / "current"
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    launcher = str(Path(exasol_path).resolve())
    run_command(
        [
            launcher,
            "init",
            infra_id,
            "--deployment-dir",
            str(current_dir),
            "--no-launcher-version-check",
        ],
        env=env_with_home(home),
    )
    named_dir = deployments_root(home) / "staging"

    # When status is invoked with --deployment from inside the current deployment
    result = subprocess.run(
        [launcher, "status", "--json", "--deployment", "staging"],
        cwd=current_dir,
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then the named deployment directory wins, not the current directory
    data = json.loads(result.stdout)
    assert data["deploymentDir"] == str(named_dir)


def test_deployment_dir_and_deployment_are_mutually_exclusive_before_any_side_effect(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a fresh home directory with no deployments yet
    home = tmp_path / "home"
    home.mkdir()
    launcher = str(Path(exasol_path).resolve())

    # When both --deployment-dir and --deployment are passed
    result = subprocess.run(
        [
            launcher,
            "status",
            "--deployment-dir",
            str(tmp_path / "explicit"),
            "--deployment",
            "staging",
        ],
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=False,
    )

    # Then the command fails naming both flags, and no side effect (deployment
    # directory creation, log file, resolution) ever happened
    assert result.returncode != 0
    assert "deployment-dir" in result.stderr
    assert "were all set" in result.stderr
    assert not deployments_root(home).exists()


def test_deployment_shorthand_wins_over_current_directory(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given the current working directory is itself a different recognized deployment
    home = tmp_path / "home"
    home.mkdir()
    current_dir = tmp_path / "current"
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    launcher = str(Path(exasol_path).resolve())
    run_command(
        [
            launcher,
            "init",
            infra_id,
            "--deployment-dir",
            str(current_dir),
            "--no-launcher-version-check",
        ],
        env=env_with_home(home),
    )
    named_dir = deployments_root(home) / "staging"

    # When status is invoked with -d from inside the current deployment directory
    result = subprocess.run(
        [launcher, "status", "--json", "-d", "staging"],
        cwd=current_dir,
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then the named deployment directory wins, not the current directory
    data = json.loads(result.stdout)
    assert data["deploymentDir"] == str(named_dir)


def test_deployment_flag_rejects_invalid_characters(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a home directory
    home = tmp_path / "home"
    home.mkdir()
    launcher = str(Path(exasol_path).resolve())

    # When --deployment is passed a value with unsafe characters
    result = subprocess.run(
        [launcher, "status", "--deployment", "bad/name"],
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=False,
    )

    # Then the command fails and does not create anything under the deployments tree
    assert result.returncode != 0
    assert "invalid deployment name" in result.stderr
    assert not deployments_root(home).exists()


def test_status_reports_uninitialized_explicit_deployment_dir(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an explicit uninitialized deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    launcher = str(Path(exasol_path).resolve())

    # When status is invoked for that directory
    result = subprocess.run(
        [launcher, "status", "--json", "--deployment-dir", str(deployment_dir)],
        capture_output=True,
        text=True,
        check=True,
    )

    # Then it succeeds and reports not_initialized
    data = json.loads(result.stdout)
    assert data["status"] == "not_initialized"
    assert data["deploymentDir"] == str(deployment_dir)


def test_status_debug_logs_current_deployment_dir(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given the current working directory is a recognized deployment directory
    deployment_dir = tmp_path / "deployment"
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    launcher = str(Path(exasol_path).resolve())
    subprocess.run(
        [
            launcher,
            "init",
            infra_id,
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ],
        capture_output=True,
        text=True,
        check=True,
    )

    # When status is invoked without an explicit deployment directory
    result = subprocess.run(
        [launcher, "--log-level", "debug", "status", "--json"],
        cwd=deployment_dir,
        capture_output=True,
        text=True,
        check=True,
    )

    # Then the current deployment directory is logged and reported
    data = json.loads(result.stdout)
    assert data["deploymentDir"] == str(deployment_dir)
    _assert_deployment_dir_logged(result.stderr, deployment_dir, "current")


def test_init_creates_default_deployment_dir(exasol_path: str, tmp_path: Path) -> None:
    # Given no deployment directory flag and no recognized current deployment directory
    home = tmp_path / "home"
    cwd = tmp_path / "work"
    home.mkdir()
    cwd.mkdir()
    default_dir = deployments_root(home) / "default"
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    launcher = str(Path(exasol_path).resolve())

    # When init is invoked
    result = subprocess.run(
        [
            launcher,
            "--log-level",
            "debug",
            "init",
            infra_id,
            "--no-launcher-version-check",
        ],
        cwd=cwd,
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then init creates and initializes the default deployment directory
    assert (default_dir / ".exasolLauncherState.json").exists()
    _assert_deployment_dir_logged(result.stderr, default_dir, "default")


def test_init_creates_named_deployment_dir(exasol_path: str, tmp_path: Path) -> None:
    # Given a --deployment flag and no recognized current deployment directory
    home = tmp_path / "home"
    home.mkdir()
    named_dir = deployments_root(home) / "staging"
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    launcher = str(Path(exasol_path).resolve())

    # When init is invoked with --deployment
    result = subprocess.run(
        [
            launcher,
            "--log-level",
            "debug",
            "init",
            infra_id,
            "--deployment",
            "staging",
            "--no-launcher-version-check",
        ],
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then init creates and initializes the named deployment directory
    assert (named_dir / ".exasolLauncherState.json").exists()
    _assert_deployment_dir_logged(result.stderr, named_dir, "named")


def test_init_refuses_different_preset_in_named_deployment_dir(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a named deployment initialized with one preset
    home = tmp_path / "home"
    home.mkdir()
    named_dir = deployments_root(home) / "staging"
    first_preset, second_preset = _infrastructure_presets_or_skip(exasol_path, 2)
    launcher = str(Path(exasol_path).resolve())
    run_command(
        [
            launcher,
            "init",
            first_preset,
            "--deployment",
            "staging",
            "--no-launcher-version-check",
        ],
        env=env_with_home(home),
    )

    # When init is requested again with a different preset for the same name
    result = subprocess.run(
        [
            launcher,
            "init",
            second_preset,
            "--deployment",
            "staging",
            "--no-launcher-version-check",
        ],
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=False,
    )

    # Then it fails before replacing local state
    assert result.returncode != 0
    stderr = result.stderr.lower()
    assert "different presets" in stderr
    assert "exasol remove" in stderr
    assert (named_dir / ".exasolLauncherState.json").exists()


def test_info_reports_uninitialized_resolved_default_dir(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given no initialized deployment is available in the current or default directory
    home = tmp_path / "home"
    cwd = tmp_path / "work"
    home.mkdir()
    cwd.mkdir()
    default_dir = deployments_root(home) / "default"
    launcher = str(Path(exasol_path).resolve())

    # When info is invoked without an explicit deployment directory
    result = subprocess.run(
        [launcher, "info"],
        cwd=cwd,
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then info reports the resolved default directory on stdout and guides the user
    # on stderr; next-step guidance is shown for text output (interactive or not).
    assert str(default_dir) in result.stdout
    assert "No Exasol Personal deployment exists" in result.stderr
    assert "exasol install <infra preset>" in result.stderr


@pytest.mark.launcher_tests
def test_status_reports_resolved_default_dir(exasol_path: str, tmp_path: Path) -> None:
    # Given a home without a default deployment and an empty non-deployment cwd
    home = tmp_path / "home"
    cwd = tmp_path / "work"
    home.mkdir()
    cwd.mkdir()
    default_dir = deployments_root(home) / "default"

    # When status runs outside any deployment directory
    command = [str(Path(exasol_path).resolve()), "status", "--json"]
    result = subprocess.run(
        command,
        cwd=cwd,
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then it reports not_initialized against the resolved default dir
    data = json.loads(result.stdout)
    assert data["status"] == "not_initialized"
    assert data["deploymentDir"] == str(default_dir)


@pytest.mark.launcher_tests
def test_init_without_flag_uses_default_dir(exasol_path: str, tmp_path: Path) -> None:
    # Given a home and cwd with no recognized deployment directory
    home = tmp_path / "home"
    cwd = tmp_path / "work"
    home.mkdir()
    cwd.mkdir()
    default_dir = deployments_root(home) / "default"

    # When init runs with no --deployment-dir
    command = [
        str(Path(exasol_path).resolve()),
        "init",
        "aws",
        "--no-launcher-version-check",
    ]
    subprocess.run(
        command,
        cwd=cwd,
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then the default directory is created and initialized
    assert (default_dir / ".exasolLauncherState.json").exists()


@pytest.mark.launcher_tests
def test_status_resolves_current_deployment_dir(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment directory initialized in place
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    launcher = str(Path(exasol_path).resolve())
    init_command = [
        launcher,
        "init",
        "aws",
        "--deployment-dir",
        str(deployment_dir),
        "--no-launcher-version-check",
    ]
    subprocess.run(
        init_command,
        capture_output=True,
        text=True,
        check=True,
    )

    # When status runs with cwd inside the deployment (no flag)
    status_command = [launcher, "status", "--json"]
    from_cwd = subprocess.run(
        status_command,
        cwd=deployment_dir,
        capture_output=True,
        text=True,
        check=True,
    )
    assert json.loads(from_cwd.stdout)["deploymentDir"] == str(deployment_dir)

    # When status targets cwd explicitly with --deployment-dir .
    explicit_command = [launcher, "status", "--json", "--deployment-dir", "."]
    explicit = subprocess.run(
        explicit_command,
        cwd=deployment_dir,
        capture_output=True,
        text=True,
        check=True,
    )
    assert json.loads(explicit.stdout)["deploymentDir"] == str(deployment_dir)
