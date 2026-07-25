from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
COMPOSE = ROOT / "docker-compose.yml"

RUNTIME_ENV = {
    "DATABASE_URL": "postgresql://lanverse:secret@127.0.0.1:5432/lanverse",
    "MINIO_ENDPOINT": "127.0.0.1:9000",
    "MINIO_PUBLIC_ENDPOINT": "http://127.0.0.1:9000",
    "MINIO_BUCKET": "lanverse",
    "MINIO_ACCESS_KEY": "test-access-key",
    "MINIO_SECRET_KEY": "test-secret-key",
}


def compose_config() -> dict[str, object]:
    result = subprocess.run(
        ["docker", "compose", "-f", str(COMPOSE), "config", "--format", "json"],
        check=False,
        capture_output=True,
        text=True,
        env={**os.environ, **RUNTIME_ENV},
    )
    assert result.returncode == 0, result.stderr
    return json.loads(result.stdout)


def services(config: dict[str, object]) -> dict[str, dict[str, object]]:
    value = config["services"]
    assert isinstance(value, dict)
    return value


def test_compose_has_only_application_runtime_units() -> None:
    configured = services(compose_config())
    assert set(configured) == {"frontend", "backend-api", "backend-worker"}


def test_only_http_services_publish_loopback_ports() -> None:
    configured = services(compose_config())
    assert configured["backend-worker"].get("ports", []) == []

    for name in ("frontend", "backend-api"):
        ports = configured[name].get("ports")
        assert isinstance(ports, list) and len(ports) == 1
        assert ports[0]["host_ip"] == "127.0.0.1"


def test_backend_units_reuse_external_postgres_and_minio_settings() -> None:
    configured = services(compose_config())
    required = set(RUNTIME_ENV)

    for name in ("backend-api", "backend-worker"):
        environment = configured[name].get("environment")
        assert isinstance(environment, dict)
        assert required <= set(environment)
