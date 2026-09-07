from __future__ import annotations

import os
from dataclasses import dataclass, field
from urllib.parse import urlsplit


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
        if not host or (not tls and host not in {"localhost", "127.0.0.1", "::1"}):
            raise ValueError("non-loopback Temporal connections require TLS")
        namespace = os.getenv("CREATION_TEMPORAL_NAMESPACE", "default")
        queue = os.getenv("CREATION_TASK_QUEUE", "lanverse-creation-text")
        if not namespace.strip() or not queue.strip() or len(queue.encode()) > 255:
            raise ValueError("creation Temporal namespace and queue are required")
        return cls(database_url(), secret, address, namespace, queue, tls)
