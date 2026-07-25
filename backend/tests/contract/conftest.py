from __future__ import annotations

import os
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from collections.abc import Iterator
from pathlib import Path

import pytest

BACKEND = Path(__file__).resolve().parents[2]


def free_loopback_port() -> int:
    with socket.socket() as server:
        server.bind(("127.0.0.1", 0))
        return int(server.getsockname()[1])


@pytest.fixture
def live_openapi_url() -> Iterator[str]:
    port = free_loopback_port()
    url = f"http://127.0.0.1:{port}/openapi.json"
    environment = os.environ.copy()
    environment.pop("DATABASE_URL", None)
    environment.pop("LANVERSE_DATABASE_URL", None)
    environment.update({"LANVERSE_ENVIRONMENT": "test", "LANVERSE_DOCS_ENABLED": "true"})
    process = subprocess.Popen(
        [
            sys.executable,
            "-m",
            "uvicorn",
            "lanverse.main:create_app",
            "--factory",
            "--host",
            "127.0.0.1",
            "--port",
            str(port),
        ],
        cwd=BACKEND,
        env=environment,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    try:
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if process.poll() is not None:
                _, stderr = process.communicate()
                pytest.fail(f"OpenAPI test server exited early: {stderr}")
            try:
                with urllib.request.urlopen(url, timeout=0.5) as response:
                    if response.status == 200:
                        break
            except (OSError, urllib.error.URLError):
                time.sleep(0.05)
        else:
            pytest.fail("OpenAPI test server did not become ready")
        yield url
    finally:
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
