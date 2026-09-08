from __future__ import annotations

import os
from dataclasses import dataclass, field
from urllib.parse import urlsplit

from pydantic import TypeAdapter

from app.text_contract.source import Digest


def docker_network() -> bool:
    value = os.getenv("CREATION_DOCKER_NETWORK", "false")
    if value not in {"true", "false"}:
        raise ValueError("CREATION_DOCKER_NETWORK must be true or false")
    return value == "true"


def database_url() -> str:
    value = os.getenv("CREATION_DATABASE_URL", "")
    parsed = urlsplit(value)
    if (
        parsed.scheme not in {"postgresql", "postgres"}
        or not parsed.hostname
        or not parsed.path.strip("/")
    ):
        raise ValueError("CREATION_DATABASE_URL must identify a dedicated PostgreSQL database")
    return value


@dataclass(frozen=True)
class Settings:
    database_url: str = field(repr=False)
    secret: str = field(repr=False)
    temporal_address: str
    temporal_namespace: str
    task_queue: str
    temporal_tls: bool

    @classmethod
    def from_environment(cls) -> Settings:
        secret = os.getenv("CREATION_AGENT_SECRET", "")
        if len(secret.encode()) < 32 or secret == os.getenv("AGENT_EXECUTION_SECRET"):
            raise ValueError(
                "CREATION_AGENT_SECRET requires an independent key of at least 32 bytes"
            )
        address = os.getenv("CREATION_TEMPORAL_ADDRESS", "127.0.0.1:7233")
        tls_value = os.getenv("CREATION_TEMPORAL_TLS", "false")
        if tls_value not in {"true", "false"}:
            raise ValueError("CREATION_TEMPORAL_TLS must be true or false")
        tls = tls_value == "true"
        host = urlsplit("http://" + address).hostname
        docker_temporal = docker_network() and address == "host.docker.internal:7233"
        if not host or (
            not tls and host not in {"localhost", "127.0.0.1", "::1"} and not docker_temporal
        ):
            raise ValueError("non-loopback Temporal connections require TLS")
        namespace = os.getenv("CREATION_TEMPORAL_NAMESPACE", "default")
        queue = os.getenv("CREATION_TASK_QUEUE", "lanverse-creation-text")
        if not namespace.strip() or not queue.strip() or len(queue.encode()) > 255:
            raise ValueError("creation Temporal namespace and queue are required")
        return cls(database_url(), secret, address, namespace, queue, tls)


def trusted_url(name: str) -> str:
    value = os.getenv(name, "")
    parsed = urlsplit(value)
    docker_origins = {
        "CREATION_PLATFORM_URL": "http://backend:8686",
        "CREATION_HARNESS_URL": "http://agent:8787",
    }
    docker_peer = docker_network() and value.rstrip("/") == docker_origins.get(name)
    if (
        not parsed.hostname
        or parsed.username
        or parsed.password
        or parsed.query
        or parsed.fragment
        or parsed.path not in {"", "/"}
        or parsed.scheme not in {"http", "https"}
        or (
            parsed.scheme == "http"
            and parsed.hostname not in {"127.0.0.1", "localhost", "::1"}
            and not docker_peer
        )
    ):
        raise ValueError(f"{name} requires an HTTPS origin (HTTP is loopback-only)")
    return value.rstrip("/")


@dataclass(frozen=True)
class WorkerSettings:
    base: Settings
    platform_url: str
    harness_url: str
    harness_secret: str = field(repr=False)
    release_hash: str
    call_limit: int
    invocation_timeout_seconds: int

    @classmethod
    def from_environment(cls) -> WorkerSettings:
        base = Settings.from_environment()
        secret = os.getenv("CREATION_HARNESS_SECRET", "")
        if len(secret.encode()) < 32 or secret == base.secret:
            raise ValueError("CREATION_HARNESS_SECRET requires an independent 32-byte key")
        release = TypeAdapter[str](Digest).validate_python(
            os.getenv("CREATION_TEXT_RELEASE_HASH", "")
        )
        limit = int(os.getenv("CREATION_CALL_LIMIT", "1000"))
        if not 1 <= limit <= 1000:
            raise ValueError("CREATION_CALL_LIMIT must be between 1 and 1000")
        deadline = int(os.getenv("CREATION_INVOCATION_TIMEOUT_SECONDS", "600"))
        if not 1 <= deadline <= 900:
            raise ValueError("CREATION_INVOCATION_TIMEOUT_SECONDS must be between 1 and 900")
        return cls(
            base,
            trusted_url("CREATION_PLATFORM_URL"),
            trusted_url("CREATION_HARNESS_URL"),
            secret,
            release,
            limit,
            deadline,
        )
