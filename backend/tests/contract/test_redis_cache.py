import asyncio
import os
from collections.abc import AsyncIterator
from typing import cast
from uuid import UUID, uuid4

import pytest
from prometheus_client import generate_latest
from redis.asyncio import Redis

from app.core.config import Settings
from app.integrations.redis import RedisCache
from app.modules.caching.contracts import (
    CacheKey,
    CacheNamespace,
    CacheUnavailableError,
    HighCostGuardRequest,
)


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_REDIS_CONTRACT") != "1",
    reason="set LANVERSE_RUN_REDIS_CONTRACT=1 with the configured Redis running",
)
@pytest.mark.asyncio
async def test_real_redis_cache_ttl_namespace_isolation_and_disconnect_fallback() -> None:
    settings = Settings()
    environment = f"contract-{uuid4().hex}"
    cache = RedisCache(settings.redis_url, environment=environment)
    scope = str(uuid4())
    revision_key = CacheKey(
        namespace=CacheNamespace.WORKSPACE_DETAIL,
        scope=scope,
        identity="revision",
        revision="current",
    )
    projection_key = CacheKey(
        namespace=CacheNamespace.WORKSPACE_DETAIL,
        scope=scope,
        identity="workspace",
        revision="r1",
    )
    adjacent_environment = f"{environment}-adjacent"
    adjacent_key = (
        f"lanverse:{adjacent_environment}:workspace_detail:v1:{scope}:workspace:r1"
    )
    observer = Redis.from_url(  # pyright: ignore[reportUnknownMemberType]
        settings.redis_url, decode_responses=False
    )
    try:
        assert await cache.get(revision_key) is None
        assert await cache.set_revision(
            revision_key,
            1,
            ttl_seconds=1,
            jitter_seconds=0,
        )
        assert await cache.get(revision_key) == b"1"
        await asyncio.sleep(1.1)
        assert await cache.get(revision_key) is None

        assert await cache.set_revision(
            revision_key,
            2,
            ttl_seconds=30,
            jitter_seconds=0,
        )
        assert not await cache.set_revision(
            revision_key,
            1,
            ttl_seconds=30,
            jitter_seconds=0,
        )
        assert await cache.get(revision_key) == b"2"
        await cache.set(
            projection_key,
            b'{"revision":1}',
            ttl_seconds=30,
            jitter_seconds=0,
        )
        await observer.set(adjacent_key, b"untouched", ex=30)
        assert await cache.invalidate_namespace(CacheNamespace.WORKSPACE_DETAIL) == 2
        assert await cache.get(revision_key) is None
        assert await observer.get(adjacent_key) == b"untouched"
        metrics = generate_latest().decode("utf-8")
        assert (
            'lanverse_cache_operations_total{namespace="workspace_detail",'
            'operation="get",result="hit"}' in metrics
        )
        assert scope not in metrics
    finally:
        await cache.invalidate_namespace(CacheNamespace.WORKSPACE_DETAIL)
        await observer.delete(adjacent_key)
        await observer.aclose()
        await cache.close()

    unavailable = RedisCache("redis://127.0.0.1:1/0", environment=environment)
    try:
        with pytest.raises(CacheUnavailableError):
            await unavailable.get(revision_key)
    finally:
        await unavailable.close()


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_REDIS_CONTRACT") != "1",
    reason="set LANVERSE_RUN_REDIS_CONTRACT=1 with the configured Redis running",
)
@pytest.mark.asyncio
async def test_real_redis_high_cost_guard_is_atomic_deduplicated_and_fail_closed() -> None:
    settings = Settings()
    environment = f"contract-high-cost-{uuid4().hex}"
    guard = RedisCache(settings.redis_url, environment=environment)
    observer = Redis.from_url(  # pyright: ignore[reportUnknownMemberType]
        settings.redis_url, decode_responses=False
    )
    workspace_a = uuid4()
    workspace_b = uuid4()

    def request(
        workspace_id: UUID,
        number: int,
        *,
        request_hash: str | None = None,
    ) -> HighCostGuardRequest:
        return HighCostGuardRequest(
            workspace_id=workspace_id,
            idempotency_digest=f"{number:064x}",
            request_hash=request_hash or f"{number + 1000:064x}",
            workspace_limit=3,
            global_limit=5,
            window_seconds=1,
            idempotency_ttl_seconds=10,
        )

    try:
        first = await guard.authorize_high_cost(request(workspace_a, 1))
        duplicate = await guard.authorize_high_cost(request(workspace_a, 1))
        conflict = await guard.authorize_high_cost(
            request(workspace_a, 1, request_hash=f"{9999:064x}")
        )
        assert first.outcome == "allowed"
        assert duplicate.outcome == "duplicate"
        assert duplicate.allowed is True
        assert conflict.outcome == "idempotency_conflict"
        assert conflict.allowed is False

        workspace_a_results = await asyncio.gather(
            *(guard.authorize_high_cost(request(workspace_a, number)) for number in range(2, 12))
        )
        assert sum(item.outcome == "allowed" for item in workspace_a_results) == 2
        assert sum(item.outcome == "workspace_limit" for item in workspace_a_results) == 8

        workspace_b_results = await asyncio.gather(
            *(guard.authorize_high_cost(request(workspace_b, number)) for number in range(20, 30))
        )
        assert sum(item.outcome == "allowed" for item in workspace_b_results) == 2
        assert sum(item.outcome == "global_limit" for item in workspace_b_results) == 8

        await asyncio.sleep(1.1)
        after_window = await guard.authorize_high_cost(request(workspace_a, 40))
        assert after_window.outcome == "allowed"

        metrics = generate_latest().decode("utf-8")
        assert "lanverse_high_cost_guard_decisions_total" in metrics
        assert str(workspace_a) not in metrics
        assert f"{1:064x}" not in metrics
    finally:
        scan = cast(
            AsyncIterator[bytes],
            observer.scan_iter(  # pyright: ignore[reportUnknownMemberType]
                match=f"lanverse:{environment}:high_cost_guard:v1:*",
                count=100,
            ),
        )
        keys = [
            item
            async for item in scan
        ]
        if keys:
            await observer.delete(*keys)
        await observer.aclose()
        await guard.close()

    unavailable = RedisCache("redis://127.0.0.1:1/0", environment=environment)
    try:
        with pytest.raises(CacheUnavailableError):
            await unavailable.authorize_high_cost(request(workspace_a, 50))
    finally:
        await unavailable.close()
