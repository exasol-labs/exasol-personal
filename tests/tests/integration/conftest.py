# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Shared fixtures for integration tests."""

import subprocess
import time
from collections.abc import Iterator
from pathlib import Path

import pytest
import requests

from .helpers import launcher_home_overrides


@pytest.fixture(scope="session", autouse=True)
def isolated_launcher_home(tmp_path_factory: pytest.TempPathFactory) -> Iterator[Path]:
    """Move launcher-owned locations into a throwaway home for the whole suite.

    Every launcher command runs startup path migration, so a suite that
    inherited the real environment would migrate and delete the developer's
    own deployments, SQL history, and cache. Tests that build their own
    environment still override these per command; this only makes the default
    safe.
    """
    home = tmp_path_factory.mktemp("launcher-home")
    with pytest.MonkeyPatch.context() as patch:
        for key, value in launcher_home_overrides(home).items():
            patch.setenv(key, value)

        yield home


@pytest.fixture
def mock_version_server() -> Iterator[str]:
    """Start mock version server and return its base URL."""
    port = "18080"
    mock_server_path = Path(__file__).parent.parent.parent / "mock_version_server.py"

    # Start the mock server (stdout/stderr suppressed)
    process = subprocess.Popen(
        ["python3", str(mock_server_path), "-port", port],  # noqa: S607
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    base_url = f"http://localhost:{port}"
    set_data_endpoint = (
        f"{base_url}/set-package-data"  # Configure mock server with test data
    )
    test_data = {
        "latestVersion": {
            "version": "9.9.9",
            "filename": "exasol-9.9.9.tar.gz",
            "url": "https://example.com/exasol-9.9.9.tar.gz",
            "size": 1234567890,
            "sha256": (
                "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
            ),
            "operatingSystem": "Linux",
            "architecture": "x86_64",
        }
    }

    # Wait for server to start by polling with POST requests
    max_attempts = 50  # 50 * 0.2s = 10 seconds max
    last_error = None
    for _attempt in range(max_attempts):
        try:
            # Check if process is still running
            if process.poll() is not None:
                stdout_bytes, stderr_bytes = process.communicate()
                msg = (
                    f"Mock server process exited unexpectedly\n"
                    f"Stdout: {stdout_bytes.decode('utf-8', errors='replace')}\n"
                    f"Stderr: {stderr_bytes.decode('utf-8', errors='replace')}"
                )
                raise RuntimeError(msg)

            response = requests.post(
                set_data_endpoint,
                json=test_data,
                timeout=1,
            )
            response.raise_for_status()
            break
        except requests.exceptions.RequestException as e:
            last_error = e
            time.sleep(0.2)
    else:
        stdout: str
        stderr: str
        if process.poll() is None:
            stdout_bytes, stderr_bytes = process.communicate(timeout=1)
            stdout = stdout_bytes.decode("utf-8", errors="replace")
            stderr = stderr_bytes.decode("utf-8", errors="replace")
        else:
            stdout = ""
            stderr = ""
        process.terminate()
        error_msg = (
            f"Mock server failed to start within timeout. "
            f"Last error: {last_error}\nStdout: {stdout}\nStderr: {stderr}"
        )
        raise RuntimeError(error_msg)

    try:
        yield base_url
    finally:
        # Clean up
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()


def get_version_check_count(base_url: str) -> int:
    """Get and reset the version check count from the mock server."""
    count_endpoint = f"{base_url}/version-check-count"
    response = requests.get(count_endpoint, timeout=1)
    response.raise_for_status()
    data = response.json()
    return int(data["count"])
