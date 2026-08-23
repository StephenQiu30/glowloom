import hashlib
import json
import secrets
from uuid import UUID

from redis.asyncio import Redis
from redis.exceptions import RedisError

from app.modules.identity.authentication.contracts import (
    AuthSession,
    AuthSessionUnavailableError,
)


class RedisAuthSessionStore:
    _ROTATE_SESSION = """
local current = redis.call('GET', KEYS[1])
if not current then
  return false
end
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], current, 'EX', ARGV[1])
return current
"""

    def __init__(
        self,
        url: str,
        *,
        environment: str,
        socket_timeout_seconds: float = 0.25,
    ) -> None:
        if not environment or any(character in environment for character in ":*?[]"):
            raise ValueError("auth session environment is invalid")
        self._environment = environment
        self._client = Redis.from_url(  # pyright: ignore[reportUnknownMemberType]
            url,
            decode_responses=False,
            socket_connect_timeout=socket_timeout_seconds,
            socket_timeout=socket_timeout_seconds,
            health_check_interval=30,
        )

    def _key(self, refresh_token: str) -> str:
        if len(refresh_token) < 32:
            raise ValueError("refresh token is invalid")
        digest = hashlib.sha256(refresh_token.encode("utf-8")).hexdigest()
        return f"lanverse:{self._environment}:identity:auth_session:v1:{digest}"

    @staticmethod
    def _payload(user_id: UUID, token_version: int) -> bytes:
        return json.dumps(
            {"user_id": str(user_id), "token_version": token_version},
            separators=(",", ":"),
        ).encode("utf-8")

    @staticmethod
    def _decode(payload: object, refresh_token: str) -> AuthSession:
        if not isinstance(payload, bytes):
            raise AuthSessionUnavailableError("auth session payload is invalid")
        try:
            data = json.loads(payload)
            return AuthSession(
                user_id=UUID(str(data["user_id"])),
                token_version=int(data["token_version"]),
                refresh_token=refresh_token,
            )
        except (KeyError, TypeError, ValueError, json.JSONDecodeError) as error:
            raise AuthSessionUnavailableError("auth session payload is invalid") from error

    @staticmethod
    def _unavailable(error: Exception) -> AuthSessionUnavailableError:
        return AuthSessionUnavailableError("authentication session dependency is unavailable")

    async def create_session(
        self,
        user_id: UUID,
        token_version: int,
        *,
        ttl_seconds: int,
    ) -> str:
        if token_version < 1 or ttl_seconds < 1:
            raise ValueError("auth session policy is invalid")
        refresh_token = secrets.token_urlsafe(48)
        try:
            stored = await self._client.set(  # pyright: ignore[reportUnknownMemberType]
                self._key(refresh_token),
                self._payload(user_id, token_version),
                ex=ttl_seconds,
                nx=True,
            )
            if not stored:
                raise AuthSessionUnavailableError("auth session key collision")
            return refresh_token
        except AuthSessionUnavailableError:
            raise
        except (RedisError, OSError, TimeoutError) as error:
            raise self._unavailable(error) from error

    async def rotate_session(
        self,
        refresh_token: str,
        *,
        ttl_seconds: int,
    ) -> AuthSession | None:
        if ttl_seconds < 1:
            raise ValueError("auth session policy is invalid")
        next_refresh_token = secrets.token_urlsafe(48)
        try:
            raw_payload = await self._client.eval(  # pyright: ignore[reportUnknownMemberType]
                self._ROTATE_SESSION,
                2,
                self._key(refresh_token),
                self._key(next_refresh_token),
                ttl_seconds,
            )
            if raw_payload is False or raw_payload is None:
                return None
            session = self._decode(raw_payload, next_refresh_token)
            return session
        except AuthSessionUnavailableError:
            raise
        except (RedisError, OSError, TimeoutError, TypeError, ValueError) as error:
            raise self._unavailable(error) from error

    async def revoke_session(self, refresh_token: str) -> None:
        if len(refresh_token) < 32:
            return
        try:
            await self._client.delete(self._key(refresh_token))
        except (RedisError, OSError, TimeoutError) as error:
            raise self._unavailable(error) from error

    async def close(self) -> None:
        await self._client.aclose()
