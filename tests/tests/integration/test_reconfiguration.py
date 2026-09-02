# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import shutil
import subprocess
from pathlib import Path
from subprocess import CalledProcessError

import pytest

from .helpers import (
    first_infrastructure_preset_id_or_skip,
    run_command,
)

PRESET_FIXTURES_DIR = Path(__file__).resolve().parents[1] / "fixtures" / "presets"

CONFIGURABLE_INFRASTRUCTURE_DISPLAY_NAME = "Configurable Test Infrastructure"
CONFIGURABLE_INFRASTRUCTURE_DESCRIPTION = "configurable test infrastructure"
CONFIGURABLE_INSTALLATION_DISPLAY_NAME = "Empty Test Installation"
CONFIGURABLE_INSTALLATION_DESCRIPTION = "empty test installation"


def _configurable_active_configuration(
    infra_dir: Path,
    install_dir: Path,
    infrastructure_options: dict[str, object],
) -> dict[str, object]:
    return {
        "infrastructure": {
            "identity": {
                "selector": f"path:{infra_dir}",
                "kind": "path",
                "path": str(infra_dir),
                "displayName": CONFIGURABLE_INFRASTRUCTURE_DISPLAY_NAME,
                "description": CONFIGURABLE_INFRASTRUCTURE_DESCRIPTION,
            },
            "options": infrastructure_options,
        },
        "installation": {
            "identity": {
                "selector": f"path:{install_dir}",
                "kind": "path",
                "path": str(install_dir),
                "displayName": CONFIGURABLE_INSTALLATION_DISPLAY_NAME,
                "description": CONFIGURABLE_INSTALLATION_DESCRIPTION,
            },
            "options": {},
        },
    }


def _infrastructure_presets_or_skip(exasol_path: str, count: int) -> list[str]:
    result = run_command([exasol_path, "presets", "list", "--json"])
    data = json.loads(result.stdout)
    presets = data.get("infrastructures")
    if not isinstance(presets, list) or len(presets) < count:
        pytest.skip(f"need at least {count} infrastructure presets")

    ids: list[str] = []
    for preset in presets[:count]:
        preset_id = preset.get("id")
        if not isinstance(preset_id, str):
            pytest.skip("infrastructure preset list contains invalid IDs")
        ids.append(preset_id)

    return ids


def _copy_preset_fixture(fixture_name: str, target_dir: Path) -> tuple[Path, Path]:
    shutil.copytree(PRESET_FIXTURES_DIR / fixture_name, target_dir)

    return target_dir / "infra", target_dir / "install"


def _copy_configurable_no_resource_presets(tmp_path: Path) -> tuple[Path, Path]:
    return _copy_preset_fixture(
        "configurable-no-resource",
        tmp_path / "configurable-no-resource",
    )


def _set_workflow_state(
    deployment_dir: Path, workflow_state: dict[str, object]
) -> None:
    launcher_state_path = deployment_dir / ".exasolLauncherState.json"
    state = json.loads(launcher_state_path.read_text())
    state["currentWorkflowState"] = workflow_state
    launcher_state_path.write_text(json.dumps(state))


def _init_stopped_local_deployment(
    exasol_path: str, deployment_dir: Path, port: int
) -> None:
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--ports",
            f"db:{port}",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    _set_workflow_state(deployment_dir, {"stopped": {}})


def test_stopped_local_configuration_can_be_set_and_reset(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a stopped local deployment with an explicit database port
    deployment_dir = tmp_path / "deployment"
    _init_stopped_local_deployment(exasol_path, deployment_dir, 18563)

    # When the port is changed while stopped
    set_result = run_command(
        [
            exasol_path,
            "config",
            "set",
            "--ports",
            "db:28563",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then the concrete value is persisted and restart guidance is shown
    assert "ports = db:28563" in set_result.stdout
    assert "run `exasol start` to apply these changes" in set_result.stderr
    assert "exasol deploy" not in set_result.stderr

    # When the port is reset
    reset_result = run_command(
        [
            exasol_path,
            "config",
            "reset",
            "ports",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then a concrete automatic value is persisted and state stays stopped
    infrastructure = _get_active_configuration(exasol_path, deployment_dir)[
        "infrastructure"
    ]
    assert isinstance(infrastructure, dict)
    options = infrastructure["options"]
    assert isinstance(options, dict)
    port_mapping = options["ports"]
    assert isinstance(port_mapping, str)
    assert port_mapping.startswith("db:")
    assert int(port_mapping.removeprefix("db:")) > 0
    assert "run `exasol start` to apply these changes" in reset_result.stderr
    info_result = run_command(
        [exasol_path, "info", "--json", "--deployment-dir", str(deployment_dir)]
    )
    assert json.loads(info_result.stdout)["deploymentState"] == "stopped"


def test_running_local_configuration_requires_stop(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a local deployment recorded as running
    deployment_dir = tmp_path / "deployment"
    _init_stopped_local_deployment(exasol_path, deployment_dir, 18563)
    _set_workflow_state(deployment_dir, {"running": {}})

    # When configuration is changed
    result = subprocess.run(
        [
            exasol_path,
            "config",
            "set",
            "--ports",
            "db:28563",
            "--deployment-dir",
            str(deployment_dir),
        ],
        text=True,
        capture_output=True,
        check=False,
    )

    # Then the command preserves configuration and directs the user to stop
    assert result.returncode != 0
    assert "exasol stop" in result.stderr
    infrastructure = _get_active_configuration(exasol_path, deployment_dir)[
        "infrastructure"
    ]
    assert isinstance(infrastructure, dict)
    options = infrastructure["options"]
    assert isinstance(options, dict)
    assert options["ports"] == "db:18563"


def _get_active_configuration(
    exasol_path: str,
    deployment_dir: Path,
    *option_names: str,
) -> dict[str, object]:
    result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            *option_names,
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    data = json.loads(result.stdout)
    assert isinstance(data, dict)

    return {str(key): value for key, value in data.items()}


def _copy_named_no_resource_presets(
    tmp_path: Path,
    directory_name: str,
    infrastructure_name: str,
    installation_name: str,
) -> tuple[Path, Path]:
    infra_dir, install_dir = _copy_preset_fixture(
        "named-no-resource-template",
        tmp_path / directory_name,
    )
    infrastructure_manifest = infra_dir / "infrastructure.yaml"
    infrastructure_manifest.write_text(
        infrastructure_manifest.read_text(encoding="utf-8").replace(
            "{{INFRASTRUCTURE_NAME}}",
            infrastructure_name,
        ),
        encoding="utf-8",
    )
    installation_manifest = install_dir / "installation.yaml"
    installation_manifest.write_text(
        installation_manifest.read_text(encoding="utf-8").replace(
            "{{INSTALLATION_NAME}}",
            installation_name,
        ),
        encoding="utf-8",
    )

    return infra_dir, install_dir


def test_config_set_updates_same_preset_variables(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an initialized deployment
    deployment_dir = tmp_path / "deployment"
    infra_dir, install_dir = _copy_configurable_no_resource_presets(tmp_path)
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--cluster-size",
            "2",
            "--instance-type",
            "custom-instance",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    state_file = deployment_dir / "infrastructure" / "terraform.tfstate"
    state_file.write_text("partial state")

    # When config set updates one same-preset parameter
    result = run_command(
        [
            exasol_path,
            "config",
            "set",
            "--cluster-size",
            "3",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then it succeeds and preserves local infrastructure state. The effective
    # configuration is primary output on stdout; the apply guidance is call-to-action
    # output on stderr, shown for text output (interactive or not).
    assert result.returncode == 0
    assert "cluster-size = 3" in result.stdout
    assert "run `exasol deploy` to apply these changes" in result.stderr
    assert state_file.read_text() == "partial state"
    assert _get_active_configuration(
        exasol_path,
        deployment_dir,
        "cluster-size",
        "instance-type",
    ) == _configurable_active_configuration(
        infra_dir,
        install_dir,
        {
            "cluster-size": 3,
            "instance-type": "custom-instance",
        },
    )


def test_init_updates_same_preset_variables(exasol_path: str, tmp_path: Path) -> None:
    # Given an initialized deployment with custom configuration
    deployment_dir = tmp_path / "deployment"
    infra_dir, install_dir = _copy_configurable_no_resource_presets(tmp_path)
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--cluster-size",
            "2",
            "--instance-type",
            "custom-instance",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When init is rerun with the same presets and one changed option
    result = run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--cluster-size",
            "3",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # Then it patches only the supplied option. The effective configuration is
    # primary output on stdout; the apply guidance is call-to-action output on
    # stderr, shown for text output (interactive or not).
    assert result.returncode == 0
    assert "cluster-size = 3" in result.stdout
    assert "run `exasol deploy` to apply these changes" in result.stderr
    assert _get_active_configuration(
        exasol_path,
        deployment_dir,
        "cluster-size",
        "instance-type",
    ) == _configurable_active_configuration(
        infra_dir,
        install_dir,
        {
            "cluster-size": 3,
            "instance-type": "custom-instance",
        },
    )


def test_config_get_outputs_active_configuration(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an initialized deployment with custom configuration
    deployment_dir = tmp_path / "deployment"
    infra_dir, install_dir = _copy_configurable_no_resource_presets(tmp_path)
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--cluster-size",
            "5",
            "--instance-type",
            "custom-instance",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When the active configuration is queried as JSON
    result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "cluster-size",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then only the requested active value is printed
    assert result.returncode == 0
    assert json.loads(result.stdout) == _configurable_active_configuration(
        infra_dir,
        install_dir,
        {"cluster-size": 5},
    )


def test_config_reset_restores_selected_defaults(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an initialized deployment with custom configuration
    deployment_dir = tmp_path / "deployment"
    infra_dir, install_dir = _copy_configurable_no_resource_presets(tmp_path)
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--cluster-size",
            "5",
            "--instance-type",
            "custom-instance",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When one option is reset
    result = run_command(
        [
            exasol_path,
            "config",
            "reset",
            "cluster-size",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then only that option returns to its preset default
    assert result.returncode == 0
    assert _get_active_configuration(
        exasol_path,
        deployment_dir,
        "cluster-size",
        "instance-type",
    ) == _configurable_active_configuration(
        infra_dir,
        install_dir,
        {
            "cluster-size": 2,
            "instance-type": "custom-instance",
        },
    )


def test_config_reset_all_restores_all_defaults(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an initialized deployment with custom configuration
    deployment_dir = tmp_path / "deployment"
    infra_dir, install_dir = _copy_configurable_no_resource_presets(tmp_path)
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--cluster-size",
            "5",
            "--instance-type",
            "custom-instance",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When all options are reset
    result = run_command(
        [
            exasol_path,
            "config",
            "reset",
            "--all",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then all options return to preset defaults
    assert result.returncode == 0
    assert _get_active_configuration(
        exasol_path,
        deployment_dir,
        "cluster-size",
        "instance-type",
    ) == _configurable_active_configuration(
        infra_dir,
        install_dir,
        {
            "cluster-size": 2,
            "instance-type": "default-instance",
        },
    )


def test_config_set_refuses_running_deployment(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an initialized deployment that is already running
    deployment_dir = tmp_path / "deployment"
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    run_command(
        [
            exasol_path,
            "init",
            infra_id,
            "--cluster-size",
            "2",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    _set_workflow_state(deployment_dir, {"running": {}})

    # When config set is requested
    args = [
        exasol_path,
        "config",
        "set",
        "--cluster-size",
        "3",
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it tells the user to destroy before changing configuration
    stderr = exc.value.stderr.lower()
    assert "deployment may already have resources" in stderr
    assert "exasol destroy" in stderr


@pytest.mark.parametrize(
    "workflow_state",
    [
        pytest.param(
            {"deploymentFailed": {"error": "boom"}},
            id="deployment_failed",
        ),
        pytest.param(
            {"interrupted": {"interruptedDuringOperation": "deploy"}},
            id="interrupted_during_deploy",
        ),
        pytest.param(
            {"interrupted": {"interruptedDuringOperation": "destroy"}},
            id="interrupted_during_destroy",
        ),
    ],
)
def test_config_set_refuses_state_with_possible_resources(
    exasol_path: str,
    tmp_path: Path,
    workflow_state: dict[str, object],
) -> None:
    # Given an initialized deployment whose previous operation failed or was
    # interrupted, so deployment resources may already exist
    deployment_dir = tmp_path / "deployment"
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    run_command(
        [
            exasol_path,
            "init",
            infra_id,
            "--cluster-size",
            "2",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    _set_workflow_state(deployment_dir, workflow_state)

    # When config set is requested
    args = [
        exasol_path,
        "config",
        "set",
        "--cluster-size",
        "3",
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it refuses the configuration change with destroy guidance
    stderr = exc.value.stderr.lower()
    assert "deployment may already have resources" in stderr
    assert "exasol destroy" in stderr


def test_install_updates_same_preset_configuration_before_retry(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment initialized from custom presets with configurable variables
    infra_dir, install_dir = _copy_configurable_no_resource_presets(tmp_path)
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--cluster-size",
            "2",
            "--instance-type",
            "custom-instance",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    state_path = deployment_dir / "infrastructure" / "terraform.tfstate"
    state_path.write_text("partial state")

    # When the same install command is rerun with updated configuration
    result = subprocess.run(
        [
            exasol_path,
            "install",
            str(infra_dir),
            str(install_dir),
            "--cluster-size",
            "4",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ],
        capture_output=True,
        text=True,
        check=False,
    )

    # Then install applies configuration while preserving deployment state
    assert result.returncode != 0
    assert state_path.read_text() == "partial state"
    assert _get_active_configuration(
        exasol_path,
        deployment_dir,
        "cluster-size",
        "instance-type",
    ) == _configurable_active_configuration(
        infra_dir,
        install_dir,
        {
            "cluster-size": 4,
            "instance-type": "custom-instance",
        },
    )


def test_install_refuses_same_preset_configuration_change_for_running_deployment(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment initialized from custom presets and marked as running
    infra_dir, install_dir = _copy_configurable_no_resource_presets(tmp_path)
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--cluster-size",
            "2",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    _set_workflow_state(deployment_dir, {"running": {}})

    # When install is rerun with changed same-preset configuration
    args = [
        exasol_path,
        "install",
        str(infra_dir),
        str(install_dir),
        "--cluster-size",
        "3",
        "--deployment-dir",
        str(deployment_dir),
        "--no-launcher-version-check",
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it refuses the configuration change without destroying or mutating state
    stderr = exc.value.stderr.lower()
    assert "deployment may already have resources" in stderr
    assert "exasol destroy" in stderr
    assert _get_active_configuration(
        exasol_path,
        deployment_dir,
        "cluster-size",
    ) == _configurable_active_configuration(
        infra_dir,
        install_dir,
        {"cluster-size": 2},
    )
    updated_state = json.loads(
        (deployment_dir / ".exasolLauncherState.json").read_text()
    )
    assert "running" in updated_state["currentWorkflowState"]


def test_init_refuses_different_preset_without_remove(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment initialized with one preset
    first_preset, second_preset = _infrastructure_presets_or_skip(exasol_path, 2)
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            first_preset,
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When init is requested with a different preset
    args = [
        exasol_path,
        "init",
        second_preset,
        "--deployment-dir",
        str(deployment_dir),
        "--no-launcher-version-check",
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it fails before replacing local state
    assert exc.value.returncode != 0
    stderr = exc.value.stderr.lower()
    assert "different presets" in stderr
    assert "exasol remove" in stderr


def test_install_refuses_different_preset_without_removing_local_state(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment initialized with a different preset identity
    infra_dir, install_dir = _copy_configurable_no_resource_presets(tmp_path)
    old_infra_dir, old_install_dir = _copy_named_no_resource_presets(
        tmp_path,
        "old",
        "Old Infrastructure",
        "Old Installation",
    )
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            str(old_infra_dir),
            str(old_install_dir),
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    old_local = deployment_dir / "old-local.txt"
    old_local.write_text("old")

    # When install is requested with a different preset
    args = [
        exasol_path,
        "install",
        str(infra_dir),
        str(install_dir),
        "--deployment-dir",
        str(deployment_dir),
        "--no-launcher-version-check",
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it fails before removing or reconfiguring local state
    assert exc.value.returncode != 0
    stderr = exc.value.stderr.lower()
    assert "different presets" in stderr
    assert "exasol remove" in stderr
    assert old_local.exists()


def test_destroy_remove_removes_local_deployment_directory(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a minimal initialized deployment whose backend has no external resources
    infra_dir, install_dir = _copy_named_no_resource_presets(
        tmp_path,
        "destroy-remove",
        "Destroy Infrastructure",
        "Destroy Installation",
    )
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    (deployment_dir / "local.txt").write_text("local state")

    # When destroy is run with local cleanup enabled
    result = run_command(
        [
            exasol_path,
            "destroy",
            "--remove",
            "--auto-approve",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then the local deployment directory is removed after successful destroy
    assert result.returncode == 0
    assert not deployment_dir.exists()


def test_remove_removes_local_deployment_directory_without_destroy(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a local deployment directory that should be abandoned
    infra_dir, install_dir = _copy_named_no_resource_presets(
        tmp_path,
        "remove-only",
        "Remove Infrastructure",
        "Remove Installation",
    )
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    (deployment_dir / "leftover.txt").write_text("leftover")

    # When the recovery command is run
    result = run_command(
        [
            exasol_path,
            "remove",
            "--auto-approve",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then the local deployment directory is removed
    # without destroying deployment resources
    assert result.returncode == 0
    assert not deployment_dir.exists()


def test_remove_refuses_non_deployment_directory(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a local directory that is not a deployment directory
    deployment_dir = tmp_path / "not-a-deployment"
    deployment_dir.mkdir()
    local_file = deployment_dir / "leftover.txt"
    local_file.write_text("leftover")

    # When the recovery command is run
    args = [
        exasol_path,
        "remove",
        "--auto-approve",
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it refuses to remove unrelated local files
    assert exc.value.returncode != 0
    assert "not an exasol personal deployment directory" in exc.value.stderr.lower()
    assert local_file.exists()


def test_install_retries_same_preset_after_failed_state(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment initialized from custom no-resource presets
    infra_dir, install_dir = _copy_named_no_resource_presets(
        tmp_path,
        "retry",
        "Retry Infrastructure",
        "Retry Installation",
    )
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    state_path = deployment_dir / "infrastructure" / "terraform.tfstate"
    state_path.write_text("partial state")
    launcher_state_path = deployment_dir / ".exasolLauncherState.json"
    _set_workflow_state(deployment_dir, {"deploymentFailed": {"error": "failed"}})

    # When the same install command is rerun
    result = subprocess.run(
        [
            exasol_path,
            "install",
            str(infra_dir),
            str(install_dir),
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ],
        capture_output=True,
        text=True,
        check=False,
    )

    # Then it preserves local state and completes deployment retry
    assert result.returncode != 0
    assert "deployment info file not found" in result.stderr.lower()
    assert state_path.read_text() == "partial state"
    updated_state = json.loads(launcher_state_path.read_text())
    assert "running" in updated_state["currentWorkflowState"]
