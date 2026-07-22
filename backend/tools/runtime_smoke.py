from __future__ import annotations

import json
import os
import signal
import subprocess
import sys
import tempfile
import time
import urllib.request
from base64 import b64encode
from pathlib import Path
from typing import IO
from urllib.parse import unquote, urlparse


ROOT = Path(__file__).resolve().parents[2]
BACKEND = ROOT / "backend"
SOURCE = BACKEND / "src"
BROKER_URL = "amqp://thief:thief_local@localhost:5672//"


def wait_for_json(url: str, expected: dict[str, str], timeout: float = 30) -> None:
    deadline = time.monotonic() + timeout
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=2) as response:
                payload = json.load(response)
            if payload == expected:
                return
            raise RuntimeError(f"unexpected response from {url}: {payload}")
        except Exception as error:  # noqa: BLE001
            last_error = error
            time.sleep(0.25)
    raise RuntimeError(f"timed out waiting for {url}: {last_error}")


def wait_for_worker(env: dict[str, str], timeout: float = 30) -> None:
    broker = urlparse(env["THIEF_RABBITMQ_URL"])
    credentials = f"{unquote(broker.username or '')}:{unquote(broker.password or '')}"
    authorization = b64encode(credentials.encode()).decode()
    management_url = env["THIEF_RABBITMQ_MANAGEMENT_URL"].rstrip("/")
    request = urllib.request.Request(
        f"{management_url}/api/queues/%2F/generation",
        headers={"Authorization": f"Basic {authorization}"},
    )
    deadline = time.monotonic() + timeout
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(request, timeout=2) as response:
                queue = json.load(response)
            if queue.get("consumers", 0) >= 1:
                return
            raise RuntimeError("generation queue has no consumers")
        except Exception as error:  # noqa: BLE001
            last_error = error
            time.sleep(0.25)
    raise RuntimeError(f"timed out waiting for worker: {last_error}")


def start_process(
    command: list[str], env: dict[str, str], log: IO[str]
) -> subprocess.Popen[str]:
    return subprocess.Popen(
        command,
        cwd=ROOT,
        env=env,
        stderr=subprocess.STDOUT,
        stdout=log,
        start_new_session=True,
        text=True,
    )


def stop_process(process: subprocess.Popen[str]) -> None:
    if process.poll() is not None:
        return
    os.killpg(process.pid, signal.SIGTERM)
    try:
        process.wait(timeout=10)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait(timeout=5)


def main() -> int:
    env = os.environ.copy()
    env.update(
        {
            "PYTHONPATH": str(SOURCE),
            "THIEF_ENV": "production",
            "THIEF_RABBITMQ_MANAGEMENT_URL": os.getenv(
                "THIEF_RABBITMQ_MANAGEMENT_URL", "http://localhost:15672"
            ),
            "THIEF_RABBITMQ_URL": os.getenv("THIEF_RABBITMQ_URL", BROKER_URL),
        }
    )
    processes: dict[str, subprocess.Popen[str]] = {}

    with tempfile.TemporaryDirectory(prefix="thief-runtime-") as temporary_dir:
        temporary = Path(temporary_dir)
        logs = {
            service: (temporary / f"{service}.log").open("w+", encoding="utf-8")
            for service in ("web", "api", "worker", "scheduler")
        }
        commands = {
            "web": ["pnpm", "--dir", "frontend", "start", "--port", "3000"],
            "api": [
                sys.executable,
                "-m",
                "uvicorn",
                "thief.api.app:app",
                "--app-dir",
                str(SOURCE),
                "--host",
                "127.0.0.1",
                "--port",
                "8000",
            ],
            "worker": [
                sys.executable,
                "-m",
                "celery",
                "-A",
                "thief.worker:app",
                "worker",
                "--loglevel=WARNING",
                "--pool=solo",
                "--queues=generation",
                "--hostname=thief-smoke@%h",
                "--without-gossip",
                "--without-heartbeat",
                "--without-mingle",
            ],
            "scheduler": [
                sys.executable,
                "-m",
                "celery",
                "-A",
                "thief.scheduler:app",
                "beat",
                "--loglevel=WARNING",
                f"--pidfile={temporary / 'beat.pid'}",
                f"--schedule={temporary / 'celerybeat-schedule'}",
            ],
        }

        try:
            for service, command in commands.items():
                processes[service] = start_process(command, env, logs[service])

            wait_for_json(
                "http://127.0.0.1:3000/health",
                {"service": "web", "status": "ok"},
            )
            for path in ("live", "ready"):
                wait_for_json(
                    f"http://127.0.0.1:8000/health/{path}",
                    {"service": "api", "status": "ok"},
                )

            wait_for_worker(env)
            if processes["scheduler"].poll() is not None:
                raise RuntimeError("scheduler exited during smoke test")

            print(
                json.dumps(
                    {
                        "api": "ok",
                        "scheduler": "ok",
                        "web": "ok",
                        "worker": "ok",
                    },
                    sort_keys=True,
                )
            )
            return 0
        except Exception:
            for service, log in logs.items():
                log.flush()
                log.seek(0)
                print(f"[{service}]\n{log.read()}", file=sys.stderr)
            raise
        finally:
            for process in reversed(processes.values()):
                stop_process(process)
            for log in logs.values():
                log.close()


if __name__ == "__main__":
    raise SystemExit(main())
