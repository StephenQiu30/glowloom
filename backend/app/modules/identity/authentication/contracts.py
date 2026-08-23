from dataclasses import dataclass
from typing import Protocol
from uuid import UUID


class AuthSessionUnavailableError(RuntimeError):
    """Raised when the persistent authentication session store is unavailable."""


@dataclass(frozen=True, slots=True)
class AuthSession:
    user_id: UUID
    token_version: int
    refresh_token: str


class AuthSessionStore(Protocol):
    async def create_session(
        self,
        user_id: UUID,
        token_version: int,
        *,
        ttl_seconds: int,
    ) -> str: ...

    async def rotate_session(
        self,
        refresh_token: str,
        *,
        ttl_seconds: int,
    ) -> AuthSession | None: ...

    async def revoke_session(self, refresh_token: str) -> None: ...

    async def close(self) -> None: ...
