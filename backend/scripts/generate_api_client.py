from __future__ import annotations

import os
import shutil
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

BACKEND = Path(__file__).resolve().parents[1]
ROOT = BACKEND.parent
FRONTEND = ROOT / "frontend"


def free_loopback_port() -> int:
    with socket.socket() as server:
        server.bind(("127.0.0.1", 0))
        return int(server.getsockname()[1])


def wait_until_ready(url: str, process: subprocess.Popen[str] | None = None) -> None:
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        if process is not None and process.poll() is not None:
            raise RuntimeError(f"OpenAPI server exited with status {process.returncode}")
        try:
            with urllib.request.urlopen(url, timeout=0.5) as response:
                if response.status == 200:
                    return
        except (OSError, urllib.error.URLError):
            time.sleep(0.05)
    raise RuntimeError(f"OpenAPI URL did not become ready: {url}")


def generate(url: str) -> int:
    pnpm = shutil.which("pnpm")
    if pnpm is None:
        raise RuntimeError("pnpm is required")
    result = subprocess.run(
        [pnpm, "dlx", "node@24.18.0", "scripts/generate-openapi.mjs"],
        cwd=FRONTEND,
        env={**os.environ, "LANVERSE_OPENAPI_URL": url},
        check=False,
    )
    return result.returncode


def main() -> int:
    configured_url = os.environ.get("LANVERSE_OPENAPI_URL")
    if configured_url:
        wait_until_ready(configured_url)
        return generate(configured_url)

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
            "main:create_app",
            "--factory",
            "--host",
            "127.0.0.1",
            "--port",
            str(port),
        ],
        cwd=BACKEND,
        env=environment,
        text=True,
    )
    try:
        wait_until_ready(url, process)
        return generate(url)
    finally:
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)


if __name__ == "__main__":
    raise SystemExit(main())
