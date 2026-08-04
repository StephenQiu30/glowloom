import secrets
from collections.abc import AsyncIterator
from time import perf_counter
from typing import cast

from redis.asyncio import Redis
from redis.exceptions import RedisError

from app.modules.caching.contracts import (
    CacheKey,
    CacheNamespace,
    CacheUnavailableError,
    HighCostGuardOutcome,
    HighCostGuardRequest,
    HighCostGuardResult,
)
from app.modules.caching.metrics import (
    CACHE_DURATION,
    CACHE_OPERATIONS,
    HIGH_COST_GUARD_DECISIONS,
    HIGH_COST_GUARD_DURATION,
)


class RedisCache:
    _SET_MONOTONIC_REVISION = """
local current = redis.call('GET', KEYS[1])
if current and tonumber(current) > tonumber(ARGV[1]) then
  return 0
end
redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
return 1
"""
    _AUTHORIZE_HIGH_COST = """
local existing = redis.call('GET', KEYS[3])
if existing then
  local request_ttl = redis.call('TTL', KEYS[3])
  if existing == ARGV[1] then
    return {1, 'duplicate', request_ttl}
  end
  return {0, 'idempotency_conflict', request_ttl}
end

local global_count = tonumber(redis.call('GET', KEYS[1]) or '0')
if global_count >= tonumber(ARGV[2]) then
  return {0, 'global_limit', redis.call('TTL', KEYS[1])}
end
local workspace_count = tonumber(redis.call('GET', KEYS[2]) or '0')
if workspace_count >= tonumber(ARGV[3]) then
  return {0, 'workspace_limit', redis.call('TTL', KEYS[2])}
end

redis.call('SET', KEYS[3], ARGV[1], 'EX', ARGV[5])
local next_global = redis.call('INCR', KEYS[1])
if next_global == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[4])
end
local next_workspace = redis.call('INCR', KEYS[2])
if next_workspace == 1 then
  redis.call('EXPIRE', KEYS[2], ARGV[4])
end
return {1, 'allowed', 0}
"""

    def __init__(
        self,
        url: str,
        *,
        environment: str,
        socket_timeout_seconds: float = 0.25,
    ) -> None:
        if not environment or any(character in environment for character in ":*?[]"):
            raise ValueError("cache environment is invalid")
        self._environment = environment
        self._client = Redis.from_url(  # pyright: ignore[reportUnknownMemberType]
            url,
            decode_responses=False,
            socket_connect_timeout=socket_timeout_seconds,
            socket_timeout=socket_timeout_seconds,
            health_check_interval=30,
        )

    def _key(self, key: CacheKey) -> str:
        return (
            f"lanverse:{self._environment}:{key.namespace.value}:v1:"
            f"{key.scope}:{key.identity}:{key.revision}"
        )

    def _namespace_pattern(self, namespace: CacheNamespace) -> str:
        return f"lanverse:{self._environment}:{namespace.value}:v1:*"

    def _high_cost_key(self, scope: str, identity: str) -> str:
        return (
            f"lanverse:{self._environment}:high_cost_guard:v1:"
            f"{scope}:{identity}"
        )

    @staticmethod
    def _unavailable(error: Exception) -> CacheUnavailableError:
        return CacheUnavailableError("cache dependency is unavailable")

    async def get(self, key: CacheKey) -> bytes | None:
        started = perf_counter()
        result = "error"
        try:
            value = await self._client.get(self._key(key))
            if value is None:
                result = "miss"
                return None
            if not isinstance(value, bytes):
                raise CacheUnavailableError("cache returned a non-binary value")
            result = "hit"
            return value
        except CacheUnavailableError:
            raise
        except (RedisError, OSError, TimeoutError) as error:
            raise self._unavailable(error) from error
        finally:
            CACHE_OPERATIONS.labels(
                namespace=key.namespace.value,
                operation="get",
                result=result,
            ).inc()
            CACHE_DURATION.labels(
                namespace=key.namespace.value,
                operation="get",
            ).observe(perf_counter() - started)

    async def set(
        self,
        key: CacheKey,
        value: bytes,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> None:
        if ttl_seconds <= 0 or jitter_seconds < 0 or jitter_seconds >= ttl_seconds:
            raise ValueError("cache TTL policy is invalid")
        started = perf_counter()
        result = "error"
        ttl = ttl_seconds + (
            secrets.randbelow(jitter_seconds + 1) if jitter_seconds else 0
        )
        try:
            stored = await self._client.set(self._key(key), value, ex=ttl)
            if not stored:
                raise CacheUnavailableError("cache set was not acknowledged")
            result = "success"
        except CacheUnavailableError:
            raise
        except (RedisError, OSError, TimeoutError) as error:
            raise self._unavailable(error) from error
        finally:
            CACHE_OPERATIONS.labels(
                namespace=key.namespace.value,
                operation="set",
                result=result,
            ).inc()
            CACHE_DURATION.labels(
                namespace=key.namespace.value,
                operation="set",
            ).observe(perf_counter() - started)

    async def delete(self, key: CacheKey) -> None:
        started = perf_counter()
        result = "error"
        try:
            await self._client.delete(self._key(key))
            result = "success"
        except (RedisError, OSError, TimeoutError) as error:
            raise self._unavailable(error) from error
        finally:
            CACHE_OPERATIONS.labels(
                namespace=key.namespace.value,
                operation="delete",
                result=result,
            ).inc()
            CACHE_DURATION.labels(
                namespace=key.namespace.value,
                operation="delete",
            ).observe(perf_counter() - started)

    async def set_revision(
        self,
        key: CacheKey,
        revision: int,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> bool:
        if revision < 1:
            raise ValueError("cache revision is invalid")
        if ttl_seconds <= 0 or jitter_seconds < 0 or jitter_seconds >= ttl_seconds:
            raise ValueError("cache TTL policy is invalid")
        started = perf_counter()
        result = "error"
        ttl = ttl_seconds + (
            secrets.randbelow(jitter_seconds + 1) if jitter_seconds else 0
        )
        try:
            raw_result = await self._client.eval(  # pyright: ignore[reportUnknownMemberType]
                self._SET_MONOTONIC_REVISION,
                1,
                self._key(key),
                revision,
                ttl,
            )
            accepted = int(raw_result) == 1
            result = "stored" if accepted else "older_rejected"
            return accepted
        except (RedisError, OSError, TimeoutError) as error:
            raise self._unavailable(error) from error
        finally:
            CACHE_OPERATIONS.labels(
                namespace=key.namespace.value,
                operation="set_revision",
                result=result,
            ).inc()
            CACHE_DURATION.labels(
                namespace=key.namespace.value,
                operation="set_revision",
            ).observe(perf_counter() - started)

    async def invalidate_namespace(self, namespace: CacheNamespace) -> int:
        started = perf_counter()
        result = "error"
        removed = 0
        try:
            batch: list[bytes] = []
            keys = cast(
                AsyncIterator[bytes],
                self._client.scan_iter(  # pyright: ignore[reportUnknownMemberType]
                    match=self._namespace_pattern(namespace), count=100
                ),
            )
            async for raw_key in keys:
                batch.append(raw_key)
                if len(batch) == 100:
                    removed += int(await self._client.delete(*batch))
                    batch.clear()
            if batch:
                removed += int(await self._client.delete(*batch))
            result = "success"
            return removed
        except (RedisError, OSError, TimeoutError) as error:
            raise self._unavailable(error) from error
        finally:
            CACHE_OPERATIONS.labels(
                namespace=namespace.value,
                operation="invalidate_namespace",
                result=result,
            ).inc()
            CACHE_DURATION.labels(
                namespace=namespace.value,
                operation="invalidate_namespace",
            ).observe(perf_counter() - started)

    async def authorize_high_cost(
        self,
        request: HighCostGuardRequest,
    ) -> HighCostGuardResult:
        started = perf_counter()
        result = "dependency_unavailable"
        try:
            raw_result = cast(
                object,
                await self._client.eval(  # pyright: ignore[reportUnknownMemberType]
                    self._AUTHORIZE_HIGH_COST,
                    3,
                    self._high_cost_key("global", "window"),
                    self._high_cost_key("workspace", str(request.workspace_id)),
                    self._high_cost_key("request", request.idempotency_digest),
                    request.request_hash,
                    request.global_limit,
                    request.workspace_limit,
                    request.window_seconds,
                    request.idempotency_ttl_seconds,
                ),
            )
            decision = self._parse_high_cost_result(raw_result)
            result = decision.outcome
            return decision
        except CacheUnavailableError:
            raise
        except (RedisError, OSError, TimeoutError) as error:
            raise self._unavailable(error) from error
        finally:
            HIGH_COST_GUARD_DECISIONS.labels(result=result).inc()
            HIGH_COST_GUARD_DURATION.observe(perf_counter() - started)

    @staticmethod
    def _parse_high_cost_result(raw_result: object) -> HighCostGuardResult:
        if not isinstance(raw_result, (list, tuple)):
            raise CacheUnavailableError("cache returned an invalid guard decision")
        parts = cast(list[object] | tuple[object, ...], raw_result)
        if len(parts) != 3:
            raise CacheUnavailableError("cache returned an invalid guard decision")
        try:
            allowed = int(cast(int | bytes | str, parts[0])) == 1
            outcome_raw = parts[1]
            outcome = (
                outcome_raw.decode("utf-8")
                if isinstance(outcome_raw, bytes)
                else str(outcome_raw)
            )
            retry_after_raw = int(cast(int | bytes | str, parts[2]))
        except (TypeError, ValueError, UnicodeDecodeError) as error:
            raise CacheUnavailableError("cache returned an invalid guard decision") from error
        valid_outcomes = {
            "allowed",
            "duplicate",
            "workspace_limit",
            "global_limit",
            "idempotency_conflict",
        }
        if outcome not in valid_outcomes:
            raise CacheUnavailableError("cache returned an invalid guard outcome")
        should_allow = outcome in {"allowed", "duplicate"}
        if allowed is not should_allow:
            raise CacheUnavailableError("cache returned an inconsistent guard decision")
        retry_after = None if should_allow else max(1, retry_after_raw)
        return HighCostGuardResult(
            allowed=allowed,
            outcome=cast(HighCostGuardOutcome, outcome),
            retry_after_seconds=retry_after,
        )

    async def close(self) -> None:
        await self._client.aclose()


async def redis_ping(url: str) -> None:
    # redis-py leaves connection keyword arguments untyped in its public stub.
    client = Redis.from_url(  # pyright: ignore[reportUnknownMemberType]
        url,
        socket_connect_timeout=1,
        socket_timeout=1,
    )
    try:
        # The ping return is bool at runtime but its stub exposes unknown kwargs.
        if not await client.ping():  # pyright: ignore[reportUnknownMemberType]
            raise ConnectionError("redis ping returned false")
    finally:
        await client.aclose()
