import secrets
from uuid import UUID

from app.modules.identity.authentication.contracts import AuthSession


class MemoryAuthSessionStore:
    def __init__(self) -> None:
        self._sessions: dict[str, tuple[UUID, int]] = {}

    async def create_session(
        self,
        user_id: UUID,
        token_version: int,
        *,
        ttl_seconds: int,
    ) -> str:
        del ttl_seconds
        token = secrets.token_urlsafe(48)
        self._sessions[token] = (user_id, token_version)
        return token

    async def rotate_session(
        self,
        refresh_token: str,
        *,
        ttl_seconds: int,
    ) -> AuthSession | None:
        del ttl_seconds
        session = self._sessions.pop(refresh_token, None)
        if session is None:
            return None
        next_token = secrets.token_urlsafe(48)
        self._sessions[next_token] = session
        return AuthSession(
            user_id=session[0],
            token_version=session[1],
            refresh_token=next_token,
        )

    async def revoke_session(self, refresh_token: str) -> None:
        self._sessions.pop(refresh_token, None)

    async def close(self) -> None:
        self._sessions.clear()
