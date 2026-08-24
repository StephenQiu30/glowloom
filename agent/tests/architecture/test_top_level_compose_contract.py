import json
import os
import shutil
import subprocess
from pathlib import Path
from typing import cast

import pytest

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
BASE_COMPOSE = REPOSITORY_ROOT / "docker-compose.yml"
ENVIRONMENT_COMPOSE = REPOSITORY_ROOT / "docker-compose-env.yml"
PRODUCTION_COMPOSE = REPOSITORY_ROOT / "docker-compose-prod.yml"

CURRENT_RUNTIME_SERVICES = {
    "frontend",
    "backend",
    "agent-init",
    "agent-api",
    "schedule-dispatcher",
    "outbox-publisher",
    "io-worker",
    "media-worker",
    "postgres",
    "redis",
    "minio",
    "minio-init",
    "kafka",
    "kafka-init",
}
STATEFUL_SERVICES = {"postgres", "redis", "minio", "kafka"}
INTERNAL_PORT_SERVICES = STATEFUL_SERVICES | {
    "minio-init",
    "kafka-init",
    "agent-init",
    "agent-api",
}


def _compose_config(*files: Path) -> dict[str, object]:
    if shutil.which("docker") is None:
        pytest.skip("Docker CLI is required for the Compose architecture contract")
    environment = os.environ.copy()
    environment.update(
        {
            "COMPOSE_PROJECT_NAME": "lanverse-contract-test",
            "LANVERSE_BACKEND_IMAGE": "ghcr.io/example/lanverse-backend:test",
            "LANVERSE_AGENT_IMAGE": "ghcr.io/example/lanverse-agent:test",
            "LANVERSE_FRONTEND_IMAGE": "ghcr.io/example/lanverse-frontend:test",
            "LANVERSE_CORS_ORIGINS": '["https://app.example.com"]',
            "POSTGRES_PASSWORD": "contract-test-postgres",
            "MINIO_ACCESS_KEY": "contract-test-minio",
            "MINIO_SECRET_KEY": "contract-test-minio-secret",
            "JWT_SECRET_KEY": "contract-test-jwt-secret-that-is-long-enough",
            "EMAIL_VERIFICATION_HMAC_SECRET": (
                "contract-test-registration-secret-that-is-long-enough"
            ),
        }
    )
    command = [
        "docker",
        "compose",
        "--env-file",
        str(REPOSITORY_ROOT / ".env.example"),
    ]
    for compose_file in files:
        command.extend(("-f", str(compose_file)))
    command.extend(("config", "--format", "json"))
    completed = subprocess.run(
        command,
        cwd=REPOSITORY_ROOT,
        env=environment,
        check=False,
        capture_output=True,
        text=True,
    )
    assert completed.returncode == 0, completed.stderr
    parsed: object = json.loads(completed.stdout)
    assert isinstance(parsed, dict)
    return cast(dict[str, object], parsed)


def _service_map(config: dict[str, object]) -> dict[str, dict[str, object]]:
    services = config["services"]
    assert isinstance(services, dict)
    return cast(dict[str, dict[str, object]], services)


def test_compose_files_define_the_current_runnable_top_level_slice() -> None:
    assert BASE_COMPOSE.is_file()
    assert ENVIRONMENT_COMPOSE.is_file()
    assert PRODUCTION_COMPOSE.is_file()

    config = _compose_config(BASE_COMPOSE, ENVIRONMENT_COMPOSE)
    services = _service_map(config)
    assert set(services) == CURRENT_RUNTIME_SERVICES

    commands: dict[str, str] = {}
    for name, service in services.items():
        command = service.get("command")
        if isinstance(command, list):
            commands[name] = " ".join(str(item) for item in cast(list[object], command))
        elif isinstance(command, str):
            commands[name] = command
        else:
            commands[name] = ""
    assert "app.runtime.workers.scheduler schedule" in commands["schedule-dispatcher"]
    assert "app.runtime.workers.scheduler outbox" in commands["outbox-publisher"]
    assert commands["schedule-dispatcher"] != commands["outbox-publisher"]

    networks = cast(dict[str, dict[str, object]], config["networks"])
    assert networks["internal"]["internal"] is True

    volumes = cast(dict[str, object], config["volumes"])
    assert {"postgres-data", "redis-data", "minio-data", "kafka-data"} <= set(volumes)

    for service_name in STATEFUL_SERVICES:
        service = services[service_name]
        assert isinstance(service, dict)
        assert "healthcheck" in service
        assert service.get("volumes"), service_name


def test_production_override_uses_published_images_and_hides_internal_ports() -> None:
    config = _compose_config(
        BASE_COMPOSE,
        ENVIRONMENT_COMPOSE,
        PRODUCTION_COMPOSE,
    )
    services = _service_map(config)

    for service_name in (
        "frontend",
        "backend",
        "agent-api",
        "schedule-dispatcher",
        "outbox-publisher",
        "io-worker",
        "media-worker",
    ):
        service = services[service_name]
        assert isinstance(service, dict)
        assert "build" not in service
        assert service["restart"] == "unless-stopped"

    agent_init = services["agent-init"]
    assert isinstance(agent_init, dict)
    assert "build" not in agent_init
    assert agent_init["restart"] == "on-failure"

    assert services["frontend"]["image"] == "ghcr.io/example/lanverse-frontend:test"
    assert services["backend"]["image"] == "ghcr.io/example/lanverse-backend:test"
    assert services["agent-api"]["image"] == "ghcr.io/example/lanverse-agent:test"
    for service_name in INTERNAL_PORT_SERVICES:
        service = services[service_name]
        assert isinstance(service, dict)
        assert "ports" not in service
