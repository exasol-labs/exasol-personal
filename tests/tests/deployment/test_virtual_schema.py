# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""End-to-end coverage for a JDBC-based Virtual Schema on a local deployment.

The test starts an ephemeral PostgreSQL container. Adapter artifacts are downloaded
from the pinned test configuration and checksum-verified before the test runs.
"""

from __future__ import annotations

import hashlib
import json
import platform
import re
import shlex
import shutil
import subprocess
import time
import urllib.request
import uuid
from collections.abc import Callable
from contextlib import suppress
from dataclasses import dataclass
from functools import partial
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

from framework.deployment import Deployment
from framework.launcher import DeploymentConfig, Launcher
from tests.testcase_helpers import run_in_local_vm

if TYPE_CHECKING:
    from collections.abc import Iterator

pytestmark = [pytest.mark.installation_e2e, pytest.mark.local_e2e]

_ARTIFACT_CONFIG = Path(__file__).parent / "assets/virtual_schema_artifacts.json"
_POSTGRES_DATABASE = "vs_test"
_POSTGRES_USER = "vs_test"
_POSTGRES_PASSWORD = "vs_test"  # noqa: S105 - disposable test-only credential
# Enable TLS connections because we've had bugs with this before and it's
# likely what's actually used by people running postgres.
_POSTGRES_TLS_SETUP = """\
set -eu
openssl req -x509 -newkey rsa:2048 -sha256 -days 1 -nodes \
  -subj /CN=localhost \
  -addext subjectAltName=DNS:localhost,IP:127.0.0.1 \
  -keyout /tmp/postgres.key -out /tmp/postgres.crt
chown postgres:postgres /tmp/postgres.key /tmp/postgres.crt
chmod 0600 /tmp/postgres.key
exec docker-entrypoint.sh postgres -c timezone=UTC -c ssl=on \
  -c ssl_cert_file=/tmp/postgres.crt \
  -c ssl_key_file=/tmp/postgres.key
"""


PodmanRunner = Callable[[list[str], str | None], subprocess.CompletedProcess[str]]


@dataclass(frozen=True)
class PostgresSource:
    podman: PodmanRunner
    container: str
    runtime_host: str
    port: int
    database: str
    user: str
    password: str
    schema: str


@dataclass(frozen=True)
class VirtualSchemaArtifacts:
    adapter: Path
    driver: Path


@dataclass(frozen=True)
class ArtifactSpec:
    filename: str
    url: str
    sha256: str


def _run_postgres(source: PostgresSource, sql: str, *args: str) -> str:
    result = source.podman(
        [
            "exec",
            "--interactive",
            source.container,
            "psql",
            "--no-password",
            "--username",
            source.user,
            "--dbname",
            source.database,
            *args,
        ],
        sql,
    )
    return result.stdout


def _sql_identifier(value: str) -> str:
    return '"' + value.replace('"', '""') + '"'


def _sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def _run_in_local_vm(
    deployment: Deployment,
    command: str,
    input_text: str | None = None,
) -> subprocess.CompletedProcess[str]:
    return run_in_local_vm(
        deployment.launcher.launcher_path,
        deployment.deployment_dir.name,
        command,
        input_text,
    )


def _run_podman(
    args: list[str],
    input_text: str | None,
    *,
    deployment: Deployment | None = None,
    executable: str | None = None,
) -> subprocess.CompletedProcess[str]:
    if platform.system() == "Darwin":
        if deployment is None:
            pytest.fail("macOS Podman execution requires a deployment")
        return _run_in_local_vm(
            deployment,
            shlex.join(["podman", *args]),
            input_text,
        )
    if executable is None:
        pytest.fail("Linux Podman execution requires an executable")
    return subprocess.run(
        [executable, *args],
        check=True,
        capture_output=True,
        text=True,
        input=input_text,
    )


def _container_runtime(deployment: Deployment) -> PodmanRunner:
    if platform.system() == "Darwin":
        try:
            _run_podman(["--version"], None, deployment=deployment)
        except subprocess.CalledProcessError:
            pytest.skip("podman is required inside the local Exasol VM")
        return partial(_run_podman, deployment=deployment)
    executable = shutil.which("podman")
    if executable is not None:
        return partial(_run_podman, executable=executable)
    pytest.skip("podman is required for the Virtual Schema smoke test")


def _wait_for_postgres(source: PostgresSource) -> None:
    for _ in range(60):
        try:
            _run_postgres(source, "SELECT 1;", "--file", "-")
        except subprocess.CalledProcessError:
            pass
        else:
            return
        time.sleep(1)
    message = "PostgreSQL container did not become ready"
    raise AssertionError(message)


def _deployment_container(deployment: Deployment) -> str:
    deployment_file = Path(deployment.deployment_dir.name) / "deployment.json"
    deployment_data = json.loads(deployment_file.read_text(encoding="utf-8"))
    deployment_id = deployment_data.get("deploymentId")
    if not isinstance(deployment_id, str) or not deployment_id:
        pytest.fail("Local deployment output does not contain a deployment ID")
    return "exasol-db-" + deployment_id


def _container_ip(podman: PodmanRunner, container: str) -> str:
    marker = "VIRTUAL_SCHEMA_POSTGRES_IP="
    result = podman(
        [
            "inspect",
            container,
            "--format",
            marker + "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
        ],
        None,
    )
    match = re.search(
        rf"{re.escape(marker)}((?:\d{{1,3}}\.){{3}}\d{{1,3}})", result.stdout
    )
    if match is None:
        pytest.fail(f"Podman did not report an IP address for {container}")
    return match.group(1)


@pytest.fixture(scope="module", name="_local_infra")
def local_infra(infra: str) -> None:
    """Gate Virtual Schema smoke-test resources to local deployments."""
    if infra != "local":
        pytest.skip("Virtual Schema smoke test is local-only")


@pytest.fixture(scope="module")
def postgres_source(
    prepared_virtual_schema_deployment: Deployment,
    _local_infra: None,
) -> Iterator[PostgresSource]:
    """Start PostgreSQL and create a private schema for the Virtual Schema test."""
    podman = _container_runtime(prepared_virtual_schema_deployment)
    config = json.loads(_ARTIFACT_CONFIG.read_text(encoding="utf-8"))
    container = "exasol-vs-postgres-" + uuid.uuid4().hex[:12]
    postgres_image = config["postgres_image"]
    deployment_container = _deployment_container(prepared_virtual_schema_deployment)
    network: str | None = None
    schema_created = False
    try:
        if platform.system() == "Darwin":
            network = "exasol-vs-network-" + uuid.uuid4().hex[:12]
            podman(["network", "create", network], None)
            podman(["network", "connect", network, deployment_container], None)
            postgres_network = network
        else:
            postgres_network = "container:" + deployment_container
        run_args = [
            "run",
            "--detach",
            "--rm",
            "--name",
            container,
            "--network",
            postgres_network,
        ]
        run_args.extend(
            [
                "--env",
                f"POSTGRES_DB={_POSTGRES_DATABASE}",
                "--env",
                f"POSTGRES_USER={_POSTGRES_USER}",
                "--env",
                f"POSTGRES_PASSWORD={_POSTGRES_PASSWORD}",
                postgres_image,
                "bash",
                "-euc",
                _POSTGRES_TLS_SETUP,
            ]
        )
        podman(run_args, None)
        source = PostgresSource(
            podman=podman,
            container=container,
            runtime_host=(
                _container_ip(podman, container)
                if platform.system() == "Darwin"
                else "127.0.0.1"
            ),
            port=5432,
            database=_POSTGRES_DATABASE,
            user=_POSTGRES_USER,
            password=_POSTGRES_PASSWORD,
            schema="vs_smoke_" + uuid.uuid4().hex[:12],
        )
        schema = _sql_identifier(source.schema)
        _wait_for_postgres(source)
        ssl = _run_postgres(
            source,
            "SHOW ssl;",
            "--tuples-only",
            "--no-align",
            "--file",
            "-",
        )
        assert ssl.strip() == "on"
        _run_postgres(
            source,
            f"CREATE SCHEMA {schema};"  # noqa: S608 - schema is generated by this test
            f"CREATE TABLE {schema}.source_data (id INTEGER PRIMARY KEY, payload TEXT);"
            f"INSERT INTO {schema}.source_data VALUES (1, 'before-refresh');",
            "--file",
            "-",
        )
        schema_created = True
        yield source
    finally:
        try:
            if schema_created:
                with suppress(subprocess.CalledProcessError):
                    _run_postgres(
                        source,
                        f"DROP SCHEMA IF EXISTS {schema} CASCADE;",
                        "--file",
                        "-",
                    )
        finally:
            with suppress(subprocess.CalledProcessError):
                podman(["rm", "--force", container], None)
            if network is not None:
                with suppress(subprocess.CalledProcessError):
                    podman(
                        ["network", "disconnect", network, deployment_container], None
                    )
                with suppress(subprocess.CalledProcessError):
                    podman(["network", "rm", network], None)


def _download_artifact(spec: ArtifactSpec, target: Path) -> Path:
    digest = hashlib.sha256()
    with (
        urllib.request.urlopen(spec.url, timeout=60) as response,  # noqa: S310 - pinned HTTPS URL from test config
        target.open("wb") as output,
    ):
        while chunk := response.read(1024 * 1024):
            output.write(chunk)
            digest.update(chunk)
    actual = digest.hexdigest()
    if actual != spec.sha256:
        target.unlink(missing_ok=True)
        message = (
            f"checksum mismatch for {spec.filename}: "
            f"expected {spec.sha256}, got {actual}"
        )
        raise AssertionError(message)
    return target


@pytest.fixture(scope="module")
def virtual_schema_artifacts(
    tmp_path_factory: pytest.TempPathFactory,
    _local_infra: None,
) -> VirtualSchemaArtifacts:
    config = json.loads(_ARTIFACT_CONFIG.read_text(encoding="utf-8"))
    artifact_dir = tmp_path_factory.mktemp("virtual-schema-artifacts")

    specs = {name: ArtifactSpec(**config[name]) for name in ("adapter", "driver")}
    paths = {
        name: _download_artifact(spec, artifact_dir / spec.filename)
        for name, spec in specs.items()
    }
    return VirtualSchemaArtifacts(adapter=paths["adapter"], driver=paths["driver"])


@pytest.fixture(scope="module")
def virtual_schema_deployment(
    exasol_path: str,
    _local_infra: None,
) -> Iterator[Deployment]:
    if platform.system() == "Windows":
        pytest.skip("Virtual Schema smoke test is not supported on Windows")

    deployment = Deployment(
        Launcher(exasol_path),
        config=DeploymentConfig(infra="local"),
    )
    try:
        deployment.deploy()
        yield deployment
    finally:
        deployment.cleanup()


def _write_settings(path: Path, driver: Path) -> None:
    path.write_text(
        f"""DRIVERNAME=POSTGRESQL
PREFIX=jdbc:postgresql:
DRIVERMAIN=org.postgresql.Driver
FETCHSIZE=100000
INSERTSIZE=-1
JAR={driver.name}

""",
        encoding="utf-8",
    )


def _stage_artifacts(deployment: Deployment, artifacts: VirtualSchemaArtifacts) -> None:
    deployment_dir = Path(deployment.deployment_dir.name)
    runtime_relative = Path("local/runtime/exa")
    bucketfs = deployment_dir / runtime_relative / "bucketfs/bfsdefault/default/vs"
    jdbc = deployment_dir / runtime_relative / "jdbc/POSTGRESQL"
    settings_name = "settings.cfg"

    bucketfs.mkdir(parents=True, exist_ok=True)
    jdbc.mkdir(parents=True, exist_ok=True)
    for artifact in (artifacts.adapter, artifacts.driver):
        shutil.copy2(artifact, bucketfs / artifact.name)
    shutil.copy2(artifacts.driver, jdbc / artifacts.driver.name)
    _write_settings(jdbc / settings_name, artifacts.driver)


@pytest.fixture(scope="module")
def prepared_virtual_schema_deployment(
    virtual_schema_deployment: Deployment,
    virtual_schema_artifacts: VirtualSchemaArtifacts,
) -> Deployment:
    """Install Java and restart Exasol before starting PostgreSQL."""
    virtual_schema_deployment.launcher.run_command(
        "slc",
        virtual_schema_deployment.deployment_dir.name,
        "install",
        "java",
        "--no-restart",
    )
    _stage_artifacts(virtual_schema_deployment, virtual_schema_artifacts)
    assert virtual_schema_deployment.stop().returncode == 0
    assert virtual_schema_deployment.start().returncode == 0
    return virtual_schema_deployment


def test_postgresql_virtual_schema_create_query_and_refresh(
    postgres_source: PostgresSource,
    virtual_schema_artifacts: VirtualSchemaArtifacts,
    prepared_virtual_schema_deployment: Deployment,
) -> None:
    """A JDBC adapter can be registered, queried, and refreshed locally."""
    source_schema = _sql_identifier(postgres_source.schema)
    exasol_schema = "VS_SMOKE"
    connection_name = "VS_SMOKE_POSTGRES_CONN"
    adapter_name = "POSTGRES_ADAPTER"
    virtual_schema_name = "VS_SMOKE_POSTGRES"
    adapter_path = (
        "/buckets/bfsdefault/default/vs/" + virtual_schema_artifacts.adapter.name
    )
    driver_path = (
        "/buckets/bfsdefault/default/vs/" + virtual_schema_artifacts.driver.name
    )
    postgres_url = (
        f"jdbc:postgresql://{postgres_source.runtime_host}:"
        f"{postgres_source.port}/{postgres_source.database}"
        "?options=-c%20TimeZone%3DUTC&sslmode=require"
    )
    connection_sql = (
        f"CREATE OR REPLACE CONNECTION {connection_name} "
        f"TO {_sql_literal(postgres_url)} "
        f"USER {_sql_literal(postgres_source.user)} "
        f"IDENTIFIED BY {_sql_literal(postgres_source.password)};"
    )

    # When: create the connection, adapter script, and Virtual Schema through SQL.
    create_sql = "\n".join(
        [
            f"DROP SCHEMA IF EXISTS {exasol_schema} CASCADE;",
            f"CREATE SCHEMA {exasol_schema};",
            connection_sql,
            f"CREATE OR REPLACE JAVA ADAPTER SCRIPT {exasol_schema}.{adapter_name} AS",
            "  %scriptclass com.exasol.adapter.RequestDispatcher;",
            "  %jvmoption -Duser.timezone=UTC;",
            f"  %jar {adapter_path};",
            f"  %jar {driver_path};",
            "/",
            f"CREATE VIRTUAL SCHEMA {virtual_schema_name}",
            f"  USING {exasol_schema}.{adapter_name}",
            f"  WITH CONNECTION_NAME = '{connection_name}'",
            f"       SCHEMA_NAME = '{postgres_source.schema}';",
            f"SELECT id, payload FROM {virtual_schema_name}.source_data ORDER BY id;",  # noqa: S608 - test-generated identifier
        ]
    )
    result = prepared_virtual_schema_deployment.connect(
        input=create_sql, capture_output=True
    )

    # Then: the source row is visible through Exasol.
    assert "before-refresh" in result.stdout

    # Given: the source schema gains a new table after Virtual Schema creation.
    _run_postgres(
        postgres_source,
        f"CREATE TABLE {source_schema}.refreshed_data "  # noqa: S608 - test-generated identifier
        "(id INTEGER PRIMARY KEY, payload TEXT);"
        f"INSERT INTO {source_schema}.refreshed_data VALUES (2, 'after-refresh');",
        "--file",
        "-",
    )

    # When / Then: refresh exposes the new source table through SQL.
    refresh_sql = (
        f"ALTER VIRTUAL SCHEMA {virtual_schema_name} REFRESH;"  # noqa: S608
        f"SELECT id, payload FROM {virtual_schema_name}.refreshed_data ORDER BY id;"
    )
    refreshed = prepared_virtual_schema_deployment.connect(
        input=refresh_sql, capture_output=True
    )
    assert "after-refresh" in refreshed.stdout
