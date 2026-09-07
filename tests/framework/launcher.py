# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import logging
import os
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from subprocess import CompletedProcess, Popen
from typing import Unpack

from framework.types import SubprocessRunKwargs


@dataclass
class DeploymentConfig:
    """Configuration for deployment initialization."""

    infra: str = "aws"
    cluster_size: int = 1
    instance_type: str | None = None
    data_volume_size: int | None = None
    db_password: str | None = None
    adminui_password: str | None = None
    stackit_project_id: str | None = None
    location: str | None = None


class Launcher:
    """Launcher provides Python bindings for the Exasol launcher."""

    def __init__(
        self,
        launcher_path: str,
    ) -> None:
        self.launcher_path = launcher_path

    def run_command(
        self,
        command: str,
        deployment_dir: str,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> CompletedProcess[str]:
        logging.info("Running launcher command: %s", command)

        return subprocess.run(  # type: ignore[call-overload,no-any-return]
            [
                self.launcher_path,
                command,
                *args,
                "--deployment-dir",
                deployment_dir,
            ],
            text=True,
            encoding="utf-8",
            check=True,
            **kwargs,
        )

    def start_command(
        self,
        command: str,
        deployment_dir: str,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> Popen[str]:
        logging.info("Running launcher command: %s", command)

        platform_kwargs: dict[str, object] = {}
        if sys.platform.startswith("win"):
            # Own process group so a CTRL_BREAK_EVENT can target this process
            # without also hitting the test runner.
            platform_kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP

        return subprocess.Popen(  # type: ignore[call-overload,no-any-return]
            [
                self.launcher_path,
                command,
                *args,
                "--deployment-dir",
                deployment_dir,
            ],
            **{
                "text": True,
                "encoding": "utf-8",
                **platform_kwargs,
                **kwargs,
            },
        )

    def _build_init_args(self, config: DeploymentConfig) -> list[str]:
        init_args = [config.infra]
        if config.infra != "local":
            init_args.extend(["--cluster-size", str(config.cluster_size)])
        if config.instance_type is not None:
            init_args.extend(["--instance-type", config.instance_type])
        if config.data_volume_size is not None:
            init_args.extend(["--data-volume-size", str(config.data_volume_size)])
        if config.db_password is not None:
            init_args.extend(["--db-password", config.db_password])
        if config.adminui_password is not None:
            init_args.extend(["--adminui-password", config.adminui_password])
        if config.stackit_project_id is not None:
            init_args.extend(["--project-id", config.stackit_project_id])
        if config.infra == "azure":
            location = config.location
            if location is None:
                location = os.getenv("TF_VAR_LOCATION") or os.getenv("AZURE_LOCATION")
            if location is not None and location.strip() != "":
                init_args.extend(["--location", location])
        return init_args

    def init(
        self,
        deployment_dir: str,
        *args: str,
        config: DeploymentConfig | None = None,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> CompletedProcess[str]:
        if config is None:
            config = DeploymentConfig()

        return self.run_command(
            "init",
            deployment_dir,
            *self._build_init_args(config),
            *args,
            **kwargs,
        )

    def deploy(
        self,
        deployment_dir: str,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> CompletedProcess[str]:
        return self.run_command("deploy", deployment_dir, *args, **kwargs)

    def deploy_no_block(
        self,
        deployment_dir: str,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> Popen[str]:
        return self.start_command("deploy", deployment_dir, *args, **kwargs)

    def start_no_block(
        self,
        deployment_dir: str,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> Popen[str]:
        return self.start_command("start", deployment_dir, *args, **kwargs)

    def stop_no_block(
        self,
        deployment_dir: str,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> Popen[str]:
        return self.start_command("stop", deployment_dir, *args, **kwargs)

    def destroy(
        self,
        deployment_dir: str,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> CompletedProcess[str]:
        return self.run_command("destroy", deployment_dir, *args, **kwargs)

    def connect(
        self,
        deployment_dir: str,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> CompletedProcess[str]:
        return self.run_command("connect", deployment_dir, *args, **kwargs)

    def start(
        self,
        deployment_dir: str,
        *args: str,
    ) -> CompletedProcess[str]:
        return self.run_command("start", deployment_dir, *args)

    def stop(
        self,
        deployment_dir: str,
        *args: str,
    ) -> CompletedProcess[str]:
        return self.run_command("stop", deployment_dir, *args)

    def info(
        self,
        deployment_dir: str,
        *args: str,
    ) -> CompletedProcess[str]:
        return self.run_command(
            "info",
            deployment_dir,
            *args,
            capture_output=True,
        )

    def status(
        self,
        deployment_dir: str,
        *args: str,
    ) -> CompletedProcess[str]:
        return self.run_command(
            "status",
            deployment_dir,
            "--json",
            *args,
            capture_output=True,
        )

    def status_value(
        self,
        deployment_dir: str,
    ) -> str:
        """Return the `status` field reported by `exasol status --json`."""
        status_result = self.status(
            deployment_dir=deployment_dir,
        )

        if status_result.returncode != 0:
            msg = "Failed to get status"
            raise RuntimeError(msg)

        status_data = json.loads(status_result.stdout)

        return str(status_data["status"])

    def has_status(
        self,
        deployment_dir: str,
        expected_status: str,
    ) -> bool:
        actual_status = self.status_value(deployment_dir)

        if actual_status == expected_status:
            return True

        logging.info(
            "expected status %s, got status %s",
            expected_status,
            actual_status,
        )

        return False

    def has_no_deployment(
        self,
        deployment_dir: str,
    ) -> bool:
        deployment_info_path = Path(deployment_dir) / "deployment-info.txt"
        info_result = self.info(deployment_dir)

        if info_result.returncode != 0:
            logging.error(
                "Info command failed with return code: %s", info_result.returncode
            )
            return False

        if "Deployment State: initialized" not in info_result.stdout:
            logging.error(
                "Info stdout does not report initialized state: %s",
                info_result.stdout,
            )
            return False

        if "Ready for deployment" not in info_result.stderr:
            logging.error(
                "Info stderr does not contain ready-for-deployment guidance.\n"
                "stdout: %s\nstderr: %s",
                info_result.stdout,
                info_result.stderr,
            )
            return False

        if deployment_info_path.exists():
            logging.error(
                "deployment-info.txt file still exists at: %s", deployment_info_path
            )
            return False

        logging.info("Verified no deployment exists")
        return True
