from __future__ import annotations

from dataclasses import dataclass, field
from urllib.parse import urlsplit

from pydantic import Field, TypeAdapter
from pydantic_settings import BaseSettings, SettingsConfigDict

from app.text_contract.source import Digest


class CreationEnvironment(BaseSettings):
    """Environment parsing only; validated runtime values are injected below."""

    model_config = SettingsConfigDict(env_prefix="CREATION_", case_sensitive=False)
    database_url: str = Field(default="", repr=False)
    agent_secret: str = Field(default="", repr=False)
    execution_secret: str = Field(default="", validation_alias="AGENT_EXECUTION_SECRET", repr=False)
    temporal_address: str = "127.0.0.1:7233"
    temporal_namespace: str = "default"
    task_queue: str = "lanverse-creation-text"
    temporal_tls: str = "false"
    docker_network: str = "false"
    platform_url: str = ""
    text_release_hash: str = ""
    call_limit: str = "1000"
    invocation_timeout_seconds: str = "600"


def docker_network(value: str | None = None) -> bool:
    value = CreationEnvironment().docker_network if value is None else value
    if value not in {"true", "false"}:
        raise ValueError("CREATION_DOCKER_NETWORK must be true or false")
    return value == "true"


def database_url(value: str | None = None) -> str:
    value = CreationEnvironment().database_url if value is None else value
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
        environment = CreationEnvironment()
        secret = environment.agent_secret
        if len(secret.encode()) < 32 or secret == environment.execution_secret:
            raise ValueError(
                "CREATION_AGENT_SECRET requires an independent key of at least 32 bytes"
            )
        address = environment.temporal_address
        tls_value = environment.temporal_tls
        if tls_value not in {"true", "false"}:
            raise ValueError("CREATION_TEMPORAL_TLS must be true or false")
        tls = tls_value == "true"
        host = urlsplit("http://" + address).hostname
        docker_temporal = docker_network(environment.docker_network) and address in {
            "host.docker.internal:7233",
            "temporal:7233",
        }
        if not host or (
            not tls and host not in {"localhost", "127.0.0.1", "::1"} and not docker_temporal
        ):
            raise ValueError("non-loopback Temporal connections require TLS")
        namespace = environment.temporal_namespace
        queue = environment.task_queue
        if not namespace.strip() or not queue.strip() or len(queue.encode()) > 255:
            raise ValueError("creation Temporal namespace and queue are required")
        return cls(database_url(environment.database_url), secret, address, namespace, queue, tls)


def trusted_url(name: str = "CREATION_PLATFORM_URL") -> str:
    environment = CreationEnvironment()
    if name != "CREATION_PLATFORM_URL":
        raise ValueError("unsupported creation peer")
    value = environment.platform_url
    parsed = urlsplit(value)
    docker_origins = {
        "CREATION_PLATFORM_URL": "http://backend:8686",
    }
    docker_peer = docker_network(environment.docker_network) and value.rstrip(
        "/"
    ) == docker_origins.get(name)
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
    release_hash: str
    call_limit: int
    invocation_timeout_seconds: int

    @classmethod
    def from_environment(cls, base: Settings | None = None) -> WorkerSettings:
        base = base or Settings.from_environment()
        environment = CreationEnvironment()
        release = TypeAdapter[str](Digest).validate_python(environment.text_release_hash)
        limit = int(environment.call_limit)
        if not 1 <= limit <= 1000:
            raise ValueError("CREATION_CALL_LIMIT must be between 1 and 1000")
        deadline = int(environment.invocation_timeout_seconds)
        if not 1 <= deadline <= 900:
            raise ValueError("CREATION_INVOCATION_TIMEOUT_SECONDS must be between 1 and 900")
        return cls(
            base,
            trusted_url("CREATION_PLATFORM_URL"),
            release,
            limit,
            deadline,
        )
