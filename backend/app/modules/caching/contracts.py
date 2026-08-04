import re
from dataclasses import dataclass
from enum import StrEnum
from typing import Literal, Protocol
from uuid import UUID


class CacheNamespace(StrEnum):
    WORKSPACE_DETAIL = "workspace_detail"


_KEY_SEGMENT = re.compile(r"^[a-zA-Z0-9_-]{1,128}$")


@dataclass(frozen=True, slots=True)
class CacheKey:
    namespace: CacheNamespace
    scope: str
    identity: str
    revision: str

    def __post_init__(self) -> None:
        for name, value in (
            ("scope", self.scope),
            ("identity", self.identity),
            ("revision", self.revision),
        ):
            if not _KEY_SEGMENT.fullmatch(value):
                raise ValueError(f"cache key {name} is invalid")


class CacheUnavailableError(RuntimeError):
    pass


class CachePort(Protocol):
    async def get(self, key: CacheKey) -> bytes | None: ...

    async def set(
        self,
        key: CacheKey,
        value: bytes,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> None: ...

    async def set_revision(
        self,
        key: CacheKey,
        revision: int,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> bool: ...

    async def delete(self, key: CacheKey) -> None: ...

    async def invalidate_namespace(self, namespace: CacheNamespace) -> int: ...

    async def close(self) -> None: ...


HighCostGuardOutcome = Literal[
    "allowed",
    "duplicate",
    "workspace_limit",
    "global_limit",
    "idempotency_conflict",
]


@dataclass(frozen=True, slots=True)
class HighCostGuardRequest:
    workspace_id: UUID
    idempotency_digest: str
    request_hash: str
    workspace_limit: int
    global_limit: int
    window_seconds: int
    idempotency_ttl_seconds: int

    def __post_init__(self) -> None:
        for name, value in (
            ("idempotency_digest", self.idempotency_digest),
            ("request_hash", self.request_hash),
        ):
            if not re.fullmatch(r"[0-9a-f]{64}", value):
                raise ValueError(f"high cost guard {name} is invalid")
        if self.workspace_limit < 1 or self.global_limit < self.workspace_limit:
            raise ValueError("high cost guard limits are invalid")
        if self.window_seconds < 1:
            raise ValueError("high cost guard window is invalid")
        if self.idempotency_ttl_seconds < self.window_seconds:
            raise ValueError("high cost guard idempotency TTL is invalid")


@dataclass(frozen=True, slots=True)
class HighCostGuardResult:
    allowed: bool
    outcome: HighCostGuardOutcome
    retry_after_seconds: int | None


class HighCostGuardPort(Protocol):
    async def authorize_high_cost(
        self,
        request: HighCostGuardRequest,
    ) -> HighCostGuardResult: ...
