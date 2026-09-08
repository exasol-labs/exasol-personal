# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Helper utilities for integration tests."""

import json
import os
import subprocess
import sys
from pathlib import Path
from subprocess import CompletedProcess
from typing import Any

import pytest


def launcher_home_overrides(home: Path) -> dict[str, str]:
    """Environment overrides that move every launcher-owned location into home.

    The launcher resolves its deployments, configuration, and cache locations
    from platform-specific variables, so overriding the home variables alone
    would leave a command reading and writing the real user's configuration
    and cache directories.
    """
    return {
        "HOME": str(home),
        "USERPROFILE": str(home),
        "HOMEDRIVE": "",
        "HOMEPATH": "",
        "XDG_CONFIG_HOME": str(home / ".config"),
        "XDG_CACHE_HOME": str(home / ".cache"),
        "APPDATA": str(home / "AppData" / "Roaming"),
        "LOCALAPPDATA": str(home / "AppData" / "Local"),
    }


def env_with_home(home: Path) -> dict[str, str]:
    """Build a full environment whose launcher-owned locations live inside home."""
    env = os.environ.copy()
    env.update(launcher_home_overrides(home))
    return env


def deployments_root(home: Path) -> Path:
    """Where the launcher keeps managed deployments below an isolated home."""
    if sys.platform == "win32":
        return home / "AppData" / "Roaming" / "exasol" / "launcher" / "deployments"
    if sys.platform == "darwin":
        return (
            home
            / "Library"
            / "Application Support"
            / "exasol"
            / "launcher"
            / "deployments"
        )

    return home / ".exasol" / "launcher" / "deployments"


def run_command(
    command: list[str], env: dict[str, str] | None = None
) -> CompletedProcess[str]:
    """Run CLI commands in integration tests.

    Output is captured, so an unexpected failure would otherwise report only an
    exit code. The captured streams are attached as exception notes, which
    pytest prints with the traceback, so the launcher's own error is visible
    without re-running the command by hand.
    """
    try:
        return subprocess.run(
            command,
            capture_output=True,
            text=True,
            # The launcher writes UTF-8 on every platform, including the Unicode
            # box drawing in rendered tables. Without this, Windows decodes with
            # the locale code page and a table crashes the reader thread, which
            # surfaces as an empty stdout rather than a decode error.
            encoding="utf-8",
            check=True,
            env=env if env is not None else os.environ.copy(),
        )
    except subprocess.CalledProcessError as error:
        error.add_note(f"command: {' '.join(command)}")
        error.add_note(f"stdout:\n{error.stdout or '<empty>'}")
        error.add_note(f"stderr:\n{error.stderr or '<empty>'}")
        raise


def first_preset_id_or_skip(exasol_path: str, preset_type: str) -> str:
    """Return the first embedded preset ID for a given type, or skip if none exist."""
    result = run_command([exasol_path, "presets", "list", "--json"])
    data = json.loads(result.stdout)
    presets_list = data.get(preset_type)
    if not isinstance(presets_list, list) or len(presets_list) == 0:
        pytest.skip(f"no presets found for type {preset_type!r}")

    first_preset: dict[str, Any] = next(iter(presets_list), {})
    preset_id = first_preset.get("id")
    if not isinstance(preset_id, str) or preset_id.strip() == "":
        pytest.skip(f"first preset in type {preset_type!r} has no id")

    return preset_id


def first_infrastructure_preset_id_or_skip(exasol_path: str) -> str:
    return first_preset_id_or_skip(exasol_path, "infrastructures")


def first_installation_preset_id_or_skip(exasol_path: str) -> str:
    return first_preset_id_or_skip(exasol_path, "installations")


def preset_id_or_skip(exasol_path: str, preset_type: str, preset_id: str) -> str:
    """Return an embedded preset ID if present, or skip."""
    result = run_command([exasol_path, "presets", "list", "--json"])
    data = json.loads(result.stdout)
    presets_list = data.get(preset_type)
    if not isinstance(presets_list, list):
        pytest.skip(f"no presets found for type {preset_type!r}")

    for preset in presets_list:
        if preset.get("id") == preset_id:
            return preset_id

    pytest.skip(f"preset {preset_id!r} not found for type {preset_type!r}")


def installation_preset_id_or_skip(exasol_path: str, preset_id: str) -> str:
    return preset_id_or_skip(exasol_path, "installations", preset_id)


def preset_description_or_skip(
    exasol_path: str, preset_type: str, preset_id: str
) -> str:
    """Return an embedded preset's description as reported by the preset listing."""
    result = run_command([exasol_path, "presets", "list", "--json"])
    data = json.loads(result.stdout)
    presets_list = data.get(preset_type)
    if not isinstance(presets_list, list):
        pytest.skip(f"no presets found for type {preset_type!r}")

    for preset in presets_list:
        if preset.get("id") != preset_id:
            continue
        description = preset.get("description")
        if isinstance(description, str) and description.strip() != "":
            return description
        pytest.skip(f"preset {preset_id!r} reports no description")

    pytest.skip(f"preset {preset_id!r} not found for type {preset_type!r}")


def compatible_preset_pair_or_skip(exasol_path: str) -> tuple[str, str]:
    """Return a preset pair the generic compatibility matrix reports as compatible.

    The pair is read from the generic help's matrix rather than from preset-specific
    help, so a test asserting preset-specific output does not source its own input
    from the output it verifies.
    """
    result = run_command([exasol_path, "install", "--help"])
    lines = result.stdout.splitlines()
    for index, line in enumerate(lines):
        if "Compatibility matrix" not in line:
            continue
        installation_ids = lines[index + 1].split()[1:]
        for row in lines[index + 2 :]:
            cells = row.split()
            if len(cells) != len(installation_ids) + 1:
                break
            for installation_id, cell in zip(installation_ids, cells[1:], strict=True):
                if cell == "yes":
                    return cells[0], installation_id

    pytest.skip("the compatibility matrix reports no compatible preset pair")


def export_preset(
    exasol_path: str, preset_id: str, preset_type: str, to_dir: str
) -> None:
    """Export a preset to a directory (used to test preset path argument variants)."""
    run_command(
        [
            exasol_path,
            "presets",
            "export",
            preset_id,
            "--type",
            preset_type,
            "--to",
            to_dir,
        ]
    )
