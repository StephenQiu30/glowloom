import hmac
import re
from typing import cast

from redis.asyncio import Redis
from redis.exceptions import RedisError

from app.modules.identity.registration_verifications.contracts import (
    ChallengeReservation,
    ConfirmationResult,
    ConfirmationStatus,
    RegistrationVerificationUnavailableError,
    ReservationReason,
)

_DIGEST = re.compile(r"^[0-9a-f]{64}$")


class RedisRegistrationVerificationStore:
    _RESERVE_CHALLENGE = """
local cooldown_ttl = redis.call('TTL', KEYS[2])
if cooldown_ttl > 0 then
  return {0, 'email_cooldown', cooldown_ttl}
end

local source_count = tonumber(redis.call('GET', KEYS[3]) or '0')
if source_count >= tonumber(ARGV[5]) then
  local source_ttl = redis.call('TTL', KEYS[3])
  return {0, 'source_limit', math.max(source_ttl, 1)}
end

redis.call('HSET', KEYS[1], 'digest', ARGV[1], 'remaining', ARGV[4])
redis.call('EXPIRE', KEYS[1], ARGV[2])
redis.call('SET', KEYS[2], '1', 'EX', ARGV[3])
local next_source_count = redis.call('INCR', KEYS[3])
if next_source_count == 1 then
  redis.call('EXPIRE', KEYS[3], ARGV[6])
end
return {1, 'accepted', ARGV[3]}
"""
    _DISCARD_CHALLENGE = """
local current = redis.call('HGET', KEYS[1], 'digest')
if current and current == ARGV[1] then
  redis.call('DEL', KEYS[1])
  return 1
end
return 0
"""
    _ABANDON_CHALLENGE = """
local current = redis.call('HGET', KEYS[1], 'digest')
if current and current == ARGV[1] then
  redis.call('DEL', KEYS[1], KEYS[2])
  return 1
end
return 0
"""
    _CONFIRM_CHALLENGE = """
local current = redis.call('HGET', KEYS[1], 'digest')
if not current then
  return {'expired', 0}
end
if current ~= ARGV[1] then
  return {'changed', 0}
end
if ARGV[2] == '1' then
  local stored = redis.call('SET', KEYS[2], ARGV[3], 'EX', ARGV[4], 'NX')
  if not stored then
    return {'ticket_collision', 0}
  end
  redis.call('DEL', KEYS[1])
  return {'confirmed', 0}
end
local remaining = redis.call('HINCRBY', KEYS[1], 'remaining', -1)
if remaining <= 0 then
  redis.call('DEL', KEYS[1])
  return {'expired', 0}
end
return {'invalid', remaining}
"""
    _CONSUME_TICKET = """
local email = redis.call('GET', KEYS[1])
if not email then
  return false
end
redis.call('DEL', KEYS[1])
return email
"""

    def __init__(
        self,
        url: str,
        *,
        environment: str,
        socket_timeout_seconds: float = 0.25,
    ) -> None:
        if not environment or any(character in environment for character in ":*?[]"):
            raise ValueError("registration verification environment is invalid")
        self._environment = environment
        self._client = Redis.from_url(  # pyright: ignore[reportUnknownMemberType]
            url,
            decode_responses=False,
            socket_connect_timeout=socket_timeout_seconds,
            socket_timeout=socket_timeout_seconds,
            health_check_interval=30,
        )

    def _key(self, kind: str, identity: str) -> str:
        if not _DIGEST.fullmatch(identity):
            raise ValueError("registration verification key digest is invalid")
        return (
            f"lanverse:{self._environment}:identity:registration_verification:v1:{kind}:{identity}"
        )

    @staticmethod
    def _decode(value: object) -> str:
        if isinstance(value, bytes):
            return value.decode("utf-8")
        return str(value)

    @staticmethod
    def _unavailable(error: Exception) -> RegistrationVerificationUnavailableError:
        return RegistrationVerificationUnavailableError(
            "registration verification dependency is unavailable"
        )

    async def reserve_challenge(
        self,
        *,
        email_key: str,
        source_key: str,
        code_digest: str,
        challenge_ttl_seconds: int,
        resend_seconds: int,
        max_attempts: int,
        source_window_seconds: int,
        source_limit: int,
    ) -> ChallengeReservation:
        if not _DIGEST.fullmatch(code_digest):
            raise ValueError("registration code digest is invalid")
        try:
            raw = cast(
                list[object],
                await self._client.eval(  # pyright: ignore[reportUnknownMemberType]
                    self._RESERVE_CHALLENGE,
                    3,
                    self._key("challenge", email_key),
                    self._key("cooldown", email_key),
                    self._key("source", source_key),
                    code_digest,
                    challenge_ttl_seconds,
                    resend_seconds,
                    max_attempts,
                    source_limit,
                    source_window_seconds,
                ),
            )
            accepted = int(cast(int | bytes | str, raw[0])) == 1
            reason = cast(ReservationReason, self._decode(raw[1]))
            retry_after = max(1, int(cast(int | bytes | str, raw[2])))
            if reason not in {"accepted", "email_cooldown", "source_limit"}:
                raise RegistrationVerificationUnavailableError(
                    "registration verification dependency is unavailable"
                )
            return ChallengeReservation(
                accepted=accepted,
                retry_after_seconds=retry_after,
                reason=reason,
            )
        except RegistrationVerificationUnavailableError:
            raise
        except (RedisError, OSError, TimeoutError, TypeError, ValueError, IndexError) as error:
            raise self._unavailable(error) from error

    async def discard_challenge(
        self,
        *,
        email_key: str,
        code_digest: str,
    ) -> None:
        await self._remove_challenge(
            script=self._DISCARD_CHALLENGE,
            email_key=email_key,
            code_digest=code_digest,
            include_cooldown=False,
        )

    async def abandon_challenge(
        self,
        *,
        email_key: str,
        code_digest: str,
    ) -> None:
        await self._remove_challenge(
            script=self._ABANDON_CHALLENGE,
            email_key=email_key,
            code_digest=code_digest,
            include_cooldown=True,
        )

    async def _remove_challenge(
        self,
        *,
        script: str,
        email_key: str,
        code_digest: str,
        include_cooldown: bool,
    ) -> None:
        try:
            keys = [self._key("challenge", email_key)]
            if include_cooldown:
                keys.append(self._key("cooldown", email_key))
            await self._client.eval(  # pyright: ignore[reportUnknownMemberType]
                script,
                len(keys),
                *keys,
                code_digest,
            )
        except (RedisError, OSError, TimeoutError) as error:
            raise self._unavailable(error) from error

    async def confirm_challenge(
        self,
        *,
        email_key: str,
        candidate_digest: str,
        ticket_digest: str,
        email: str,
        ticket_ttl_seconds: int,
    ) -> ConfirmationResult:
        challenge_key = self._key("challenge", email_key)
        ticket_key = self._key("ticket", ticket_digest)
        try:
            for _ in range(3):
                observed_raw = await self._client.hget(  # pyright: ignore[reportUnknownMemberType]
                    challenge_key, "digest"
                )
                if observed_raw is None:
                    return ConfirmationResult(status="expired", remaining_attempts=0)
                if not isinstance(observed_raw, bytes):
                    raise RegistrationVerificationUnavailableError(
                        "registration verification dependency is unavailable"
                    )
                observed = observed_raw.decode("ascii")
                matched = hmac.compare_digest(observed, candidate_digest)
                raw = cast(
                    list[object],
                    await self._client.eval(  # pyright: ignore[reportUnknownMemberType]
                        self._CONFIRM_CHALLENGE,
                        2,
                        challenge_key,
                        ticket_key,
                        observed,
                        1 if matched else 0,
                        email,
                        ticket_ttl_seconds,
                    ),
                )
                status = self._decode(raw[0])
                if status == "changed":
                    continue
                if status == "ticket_collision":
                    raise RegistrationVerificationUnavailableError(
                        "registration verification dependency is unavailable"
                    )
                if status not in {"confirmed", "invalid", "expired"}:
                    raise RegistrationVerificationUnavailableError(
                        "registration verification dependency is unavailable"
                    )
                return ConfirmationResult(
                    status=cast(ConfirmationStatus, status),
                    remaining_attempts=max(0, int(cast(int | bytes | str, raw[1]))),
                )
            raise RegistrationVerificationUnavailableError(
                "registration verification dependency is unavailable"
            )
        except RegistrationVerificationUnavailableError:
            raise
        except (RedisError, OSError, TimeoutError, TypeError, ValueError, IndexError) as error:
            raise self._unavailable(error) from error

    async def consume_ticket(self, *, ticket_digest: str) -> str | None:
        try:
            raw = await self._client.eval(  # pyright: ignore[reportUnknownMemberType]
                self._CONSUME_TICKET,
                1,
                self._key("ticket", ticket_digest),
            )
            if raw is None:
                return None
            if isinstance(raw, bytes):
                return raw.decode("utf-8")
            if raw is False:
                return None
            raise RegistrationVerificationUnavailableError(
                "registration verification dependency is unavailable"
            )
        except RegistrationVerificationUnavailableError:
            raise
        except (RedisError, OSError, TimeoutError, UnicodeDecodeError) as error:
            raise self._unavailable(error) from error

    async def close(self) -> None:
        await self._client.aclose()
