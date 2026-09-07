# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests for the built-in (official catalog) and custom script language containers.

The custom container is supplied by the runner via EXASOL_TEST_CUSTOM_SLC_FILE or
EXASOL_TEST_CUSTOM_SLC_URL; the custom tests skip if neither is set, so no large
container is hard-coded into the suite. Both are passed as `--source`, which accepts a
local path or an https URL.
"""

import json
import os
import textwrap
from collections.abc import Iterator
from pathlib import Path
from subprocess import CalledProcessError, CompletedProcess
from typing import Any, Final

import pytest

from framework.deployment import Deployment
from framework.launcher import DeploymentConfig, Launcher

PYTHON_ALIAS: Final = "PYTHON3"

UNKNOWN_ALIAS: Final = "invalid-test-slc-alias"

CUSTOM_SLC_FILE_ENV: Final = "EXASOL_TEST_CUSTOM_SLC_FILE"
CUSTOM_SLC_URL_ENV: Final = "EXASOL_TEST_CUSTOM_SLC_URL"
CUSTOM_ALIAS: Final = "MYPY3"


@pytest.fixture(scope="module")
def slc_deployment(exasol_path: str, infra: str) -> Iterator[Deployment]:
    if infra != "local":
        pytest.skip("SLC is currently supported only on local deployments")

    deployment = Deployment(Launcher(exasol_path), config=DeploymentConfig(infra=infra))
    try:
        deployment.deploy()
        yield deployment
    finally:
        deployment.cleanup()


def _slc(
    deployment: Deployment, *args: str, capture: bool = False
) -> CompletedProcess[str]:
    return deployment.launcher.run_command(
        "slc",
        deployment.deployment_dir.name,
        *args,
        capture_output=capture,
    )


def _slc_statuses(deployment: Deployment) -> list[dict[str, Any]]:
    """Return the parsed `slc list --json` status entries."""
    result = _slc(deployment, "list", "--json", capture=True)
    statuses: list[dict[str, Any]] = json.loads(result.stdout)
    return statuses


def _official_statuses(deployment: Deployment) -> list[dict[str, Any]]:
    """Return only the catalog (official) entries of `slc list --json`."""
    return [s for s in _slc_statuses(deployment) if s.get("type") == "official"]


def _custom_statuses(deployment: Deployment) -> list[dict[str, Any]]:
    """Return only the user-supplied (custom) entries of `slc list --json`."""
    return [s for s in _slc_statuses(deployment) if s.get("type") == "custom"]


def _custom_status(deployment: Deployment, alias: str) -> dict[str, Any] | None:
    """Return the custom entry for an alias, or None when it is not installed."""
    for status in _custom_statuses(deployment):
        if status["alias"].casefold() == alias.casefold():
            return status
    return None


def _status_for_alias(deployment: Deployment, alias: str) -> dict[str, Any]:
    """Return the unique catalog entry declaring an SLC alias."""
    matches = [
        status
        for status in _official_statuses(deployment)
        if any(
            candidate.casefold() == alias.casefold() for candidate in status["aliases"]
        )
    ]
    assert len(matches) == 1, f"expected exactly one catalog SLC for alias {alias!r}"
    return matches[0]


def _is_alias_installed(deployment: Deployment, alias: str) -> bool:
    """Report whether the catalog SLC declaring an alias is marked installed."""
    return bool(_status_for_alias(deployment, alias)["installed"])


def _run_scalar_udf(deployment: Deployment, alias: str, schema: str) -> str:
    """Create and run a trivial scalar UDF; return the connect stdout."""
    script = textwrap.dedent(
        f"""\
        DROP SCHEMA IF EXISTS {schema} CASCADE;
        CREATE SCHEMA {schema};
        OPEN SCHEMA {schema};
        CREATE OR REPLACE {alias} SCALAR SCRIPT hello() RETURNS VARCHAR(10) AS
        def run(ctx):
            return 'hi'
        /
        SELECT hello();
        """
    )
    return deployment.connect(input=script, capture_output=True).stdout


def _assert_database_responds(deployment: Deployment) -> None:
    """Assert that the database remains reachable independently of SLC availability."""
    result = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in result.stdout


def _assert_python_udf_is_unavailable(deployment: Deployment, schema: str) -> None:
    """Assert that the database is reachable but Python UDFs are unavailable."""
    _assert_database_responds(deployment)
    assert "hi" not in _run_scalar_udf(deployment, PYTHON_ALIAS, schema)


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_list_reports_catalog_containers(slc_deployment: Deployment) -> None:
    """`slc list` reports the catalog in text and JSON, whatever is installed."""
    # When / Then: text listing shows the table and the Python alias.
    result = _slc(slc_deployment, "list", capture=True)
    assert result.stdout.splitlines()[0].split() == [
        "ALIASES",
        "FLAVOR",
        "VERSION",
        "INSTALLED",
    ]
    assert PYTHON_ALIAS in result.stdout
    assert "Prefer an alias from the ALIASES column" in result.stderr

    # When / Then: JSON listing carries the documented fields for every entry.
    statuses = _official_statuses(slc_deployment)
    assert statuses, "catalog listing should not be empty on a supported platform"
    required_fields = {"language", "flavor", "version", "aliases", "installed"}
    for status in statuses:
        assert required_fields <= status.keys()
        assert isinstance(status["installed"], bool)

    python = _status_for_alias(slc_deployment, PYTHON_ALIAS)
    assert PYTHON_ALIAS in python["aliases"]


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_install_rejects_unknown_alias(slc_deployment: Deployment) -> None:
    """An unknown alias fails before any restart, leaving install state unchanged."""
    # Given: the current install state (order-independent on the shared deployment).
    was_installed = _is_alias_installed(slc_deployment, PYTHON_ALIAS)

    # When: installing an unknown alias.
    with pytest.raises(CalledProcessError) as exc_info:
        _slc(slc_deployment, "install", UNKNOWN_ALIAS, "--auto-approve", capture=True)

    # Then: the error names the failure and valid aliases, and nothing changed.
    stderr = exc_info.value.stderr or ""
    assert "unknown SLC identifier" in stderr
    assert PYTHON_ALIAS in stderr
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS) == was_installed


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_remove_when_not_installed_is_noop(slc_deployment: Deployment) -> None:
    """Removing an SLC that is not installed succeeds as a no-op without restarting."""
    # Given: R is never installed by this suite (so the outcome is order-independent).
    r_alias: Final = "R"

    # When / Then: removing it succeeds and reports nothing to remove.
    result = _slc(slc_deployment, "remove", r_alias, capture=True)
    assert "nothing to remove" in result.stdout


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_official_slc_install_runs_udf(slc_deployment: Deployment) -> None:
    """Installing an official SLC makes its UDFs runnable; reinstalling is a no-op."""
    # Given: the flavor advertised by `slc list` for the Python SLC.
    python_flavor = str(_status_for_alias(slc_deployment, PYTHON_ALIAS)["flavor"])

    # When / Then: installing by flavor marks it installed and a Python UDF runs.
    _slc(slc_deployment, "install", python_flavor, "--auto-approve")
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    assert "hi" in _run_scalar_udf(slc_deployment, PYTHON_ALIAS, "slc_e2e_official")

    # When / Then: reinstalling the same alias is idempotent, without a restart.
    reinstall = _slc(
        slc_deployment, "install", PYTHON_ALIAS, "--auto-approve", capture=True
    )
    assert "already installed and up to date" in reinstall.stdout
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_official_slc_remove_uninstalls_language(slc_deployment: Deployment) -> None:
    """Removing an installed official SLC clears its status and makes its UDFs fail."""
    # Given: the Python SLC is installed (idempotent setup) and its UDFs run.
    _slc(slc_deployment, "install", PYTHON_ALIAS, "--auto-approve")
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    assert "hi" in _run_scalar_udf(slc_deployment, PYTHON_ALIAS, "slc_e2e_remove")

    # When: removing it (restarts the database, unmounting the language).
    _slc(slc_deployment, "remove", PYTHON_ALIAS, "--auto-approve")

    # Then: it is no longer installed and its UDFs can no longer run.
    assert not _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    _assert_python_udf_is_unavailable(slc_deployment, "slc_e2e_remove")


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_install_no_restart_activates_on_next_start(
    slc_deployment: Deployment,
) -> None:
    """`--no-restart` records the SLC without applying it; the next start mounts it."""
    # Given: the Python SLC is neither recorded nor mounted (database left running).
    _slc(slc_deployment, "remove", PYTHON_ALIAS, "--auto-approve")

    # When: installing with --no-restart against the running database.
    result = _slc(slc_deployment, "install", PYTHON_ALIAS, "--no-restart", capture=True)

    # Then: it is recorded as installed but deferred, and not yet usable.
    assert "next start" in result.stdout
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    _assert_python_udf_is_unavailable(slc_deployment, "slc_e2e_defer")

    # When / Then: a restart applies the recorded state, mounting the SLC.
    slc_deployment.stop()
    slc_deployment.start()
    assert "hi" in _run_scalar_udf(slc_deployment, PYTHON_ALIAS, "slc_e2e_defer")


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_update_when_current_is_noop(slc_deployment: Deployment) -> None:
    """Updating an SLC already at the catalog version is a no-op and stays usable."""
    # Given: the Python SLC is installed at the catalog version.
    _slc(slc_deployment, "install", PYTHON_ALIAS, "--auto-approve")
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)

    # When / Then: updating reports no change and the language is still usable.
    result = _slc(
        slc_deployment, "update", PYTHON_ALIAS, "--auto-approve", capture=True
    )
    assert "already up to date" in result.stdout
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    assert "hi" in _run_scalar_udf(slc_deployment, PYTHON_ALIAS, "slc_e2e_update")


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_custom_slc_rejects_invalid_input(slc_deployment: Deployment) -> None:
    """Bad alias, language, or source are rejected without touching the database."""
    # Given: invocations that must never reach the deployment.
    invalid = [
        ["--alias", CUSTOM_ALIAS, "--language", "python"],
        ["--source", "c.tar.gz", "--alias", "123bad", "--language", "python"],
        ["--source", "c.tar.gz", "--alias", CUSTOM_ALIAS, "--language", "cobol/"],
        [
            "--source",
            "http://example.com/c.tar.gz",
            "--alias",
            CUSTOM_ALIAS,
            "--language",
            "python",
        ],
    ]

    for args in invalid:
        # When / Then: the command fails.
        with pytest.raises(CalledProcessError):
            _slc(slc_deployment, "custom", "install", *args, capture=True)

    # Then: the database is untouched and nothing was recorded.
    _assert_database_responds(slc_deployment)
    assert _custom_status(slc_deployment, CUSTOM_ALIAS) is None


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_custom_slc_rejects_invalid_container(
    slc_deployment: Deployment, tmp_path: Path
) -> None:
    """A file that is not a script language container fails before any restart."""
    # Given: a file that is not a container archive.
    source = tmp_path / "not-a-container.tar.gz"
    source.write_text("this is not a container")

    # When: installing it.
    with pytest.raises(CalledProcessError) as exc_info:
        _slc(
            slc_deployment,
            "custom",
            "install",
            "--source",
            str(source),
            "--alias",
            CUSTOM_ALIAS,
            "--language",
            "python",
            "--auto-approve",
            capture=True,
        )

    # Then: the failure names the container, and the deployment is unchanged.
    assert "container" in (exc_info.value.stderr or "")
    _assert_database_responds(slc_deployment)
    assert _custom_status(slc_deployment, CUSTOM_ALIAS) is None


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_custom_slc_install_runs_udf(slc_deployment: Deployment) -> None:
    """Installing a custom container makes its UDFs runnable and lists it as active."""
    # When: installing it under a custom alias (this restarts the database).
    _install_custom(slc_deployment, "--auto-approve")

    # Then: the alias is listed as an available custom entry.
    status = _custom_status(slc_deployment, CUSTOM_ALIAS)
    assert status is not None
    assert status["language"] == "python"
    assert status["available"] is True

    # And: the text listing shows it under its own section.
    text = _slc(slc_deployment, "list", capture=True).stdout
    assert CUSTOM_ALIAS in text
    assert "CUSTOM ALIAS" in text

    # And: a UDF written against the custom alias runs.
    assert "hi" in _run_scalar_udf(slc_deployment, CUSTOM_ALIAS, "slc_e2e_custom")


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_custom_slc_reinstall_and_update_are_noops(slc_deployment: Deployment) -> None:
    """Re-supplying identical content changes nothing, for both install and update."""
    # Given: the container is installed (idempotent setup).
    _install_custom(slc_deployment, "--auto-approve")

    # When / Then: installing the same content again is a no-op.
    reinstall = _install_custom(slc_deployment, "--auto-approve", capture=True)
    assert "Nothing to do" in reinstall.stdout

    # When / Then: updating to the same content is a no-op too.
    update = _slc(
        slc_deployment,
        "custom",
        "update",
        "--source",
        _custom_slc_source(),
        "--alias",
        CUSTOM_ALIAS,
        "--auto-approve",
        capture=True,
    )
    assert "up to date" in update.stdout

    # And: it is still usable.
    assert "hi" in _run_scalar_udf(slc_deployment, CUSTOM_ALIAS, "slc_e2e_custom_noop")


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_remove_handles_a_custom_alias(slc_deployment: Deployment) -> None:
    """The unified `slc remove` removes a custom container, not only official ones."""
    # Given: the custom container is installed (idempotent setup).
    _install_custom(slc_deployment, "--auto-approve")

    # When: removing it through the top-level command.
    _slc(slc_deployment, "remove", CUSTOM_ALIAS, "--auto-approve")

    # Then: it is gone and its UDFs no longer run.
    assert _custom_status(slc_deployment, CUSTOM_ALIAS) is None
    assert "hi" not in _run_scalar_udf(
        slc_deployment, CUSTOM_ALIAS, "slc_e2e_custom_unified"
    )


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_custom_slc_remove_uninstalls_language(slc_deployment: Deployment) -> None:
    """Removing a custom container clears its alias and makes its UDFs fail."""
    # Given: the container is installed and usable (idempotent setup).
    _install_custom(slc_deployment, "--auto-approve")
    assert "hi" in _run_scalar_udf(
        slc_deployment, CUSTOM_ALIAS, "slc_e2e_custom_remove"
    )

    # When: removing it.
    _slc(slc_deployment, "custom", "remove", CUSTOM_ALIAS)

    # Then: it is gone from the listing and its UDFs no longer run.
    assert _custom_status(slc_deployment, CUSTOM_ALIAS) is None
    assert CUSTOM_ALIAS not in _slc(slc_deployment, "list", capture=True).stdout
    _assert_database_responds(slc_deployment)
    assert "hi" not in _run_scalar_udf(
        slc_deployment, CUSTOM_ALIAS, "slc_e2e_custom_remove"
    )

    # When / Then: removing it again is a no-op.
    again = _slc(slc_deployment, "custom", "remove", CUSTOM_ALIAS, capture=True)
    assert "nothing to remove" in again.stdout


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_custom_slc_no_restart_activates_on_next_start(
    slc_deployment: Deployment,
) -> None:
    """`--no-restart` stages the container; the next start mounts and activates it."""
    # Given: no custom container is installed (removal is idempotent).
    _slc(slc_deployment, "custom", "remove", CUSTOM_ALIAS)

    # When: installing with --no-restart against the running database.
    result = _install_custom(slc_deployment, "--no-restart", capture=True)

    # Then: it is recorded but not yet available, and the database is untouched.
    assert "next start" in result.stdout
    status = _custom_status(slc_deployment, CUSTOM_ALIAS)
    assert status is not None
    assert status["available"] is False
    _assert_database_responds(slc_deployment)

    # When: restarting.
    slc_deployment.stop()
    slc_deployment.start()

    # Then: the container is mounted and its alias activated, with no user action.
    activated = _custom_status(slc_deployment, CUSTOM_ALIAS)
    assert activated is not None
    assert activated["available"] is True
    assert "hi" in _run_scalar_udf(slc_deployment, CUSTOM_ALIAS, "slc_e2e_custom_defer")

    _slc(slc_deployment, "custom", "remove", CUSTOM_ALIAS)


def _install_custom(
    deployment: Deployment, *extra: str, capture: bool = False
) -> CompletedProcess[str]:
    """Install the runner-supplied custom container under CUSTOM_ALIAS (idempotent)."""
    return _slc(
        deployment,
        "custom",
        "install",
        "--source",
        _custom_slc_source(),
        "--alias",
        CUSTOM_ALIAS,
        "--language",
        "python",
        *extra,
        capture=capture,
    )


def _custom_slc_source() -> str:
    """Return the runner-supplied custom container, or skip when none is configured."""
    source = os.environ.get(CUSTOM_SLC_FILE_ENV) or os.environ.get(CUSTOM_SLC_URL_ENV)
    if not source:
        pytest.skip(
            f"set {CUSTOM_SLC_FILE_ENV} or {CUSTOM_SLC_URL_ENV} to a standard python "
            "container to run the custom SLC e2e"
        )
    return source
