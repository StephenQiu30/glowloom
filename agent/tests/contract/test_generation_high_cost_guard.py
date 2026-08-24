import hashlib
import os
from collections.abc import AsyncIterator
from typing import cast
from uuid import uuid4

import httpx
import pytest
from fastapi import FastAPI
from redis.asyncio import Redis
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings
from app.integrations.redis import RedisCache
from app.modules.caching.dependencies import get_high_cost_guard
from tests.integration.test_generation_requests_api import (
    create_ready_generation_shot,
    generation_fact_counts,
    seed_active_capability,
)


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_REDIS_CONTRACT") != "1",
    reason="set LANVERSE_RUN_REDIS_CONTRACT=1 with the configured Redis running",
)
@pytest.mark.asyncio
async def test_real_redis_guards_generation_submission_and_postgresql_replay(
    app: FastAPI,
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    environment = f"contract-generation-guard-{uuid4().hex}"
    guard = RedisCache(test_settings.redis_url, environment=environment)
    unavailable = RedisCache("redis://127.0.0.1:1/0", environment=environment)
    observer = Redis.from_url(  # pyright: ignore[reportUnknownMemberType]
        test_settings.redis_url,
        decode_responses=False,
    )
    app.dependency_overrides[get_high_cost_guard] = lambda: guard
    try:
        headers, _, refs, shot_facts = await create_ready_generation_shot(
            client,
            session_factory,
        )
        workspace_id = refs["workspace_id"]
        capability = await seed_active_capability(session_factory)
        preflight_payload = {
            "workspace_id": str(workspace_id),
            "shot_spec_version_id": shot_facts["spec_version"]["id"],
            "capability_id": str(capability.id),
            "parameters": {"resolution": "1080p"},
        }
        preflight = await client.post(
            f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
            headers=headers,
            json=preflight_payload,
        )
        assert preflight.status_code == 200
        preflight_result = preflight.json()["data"]
        idempotency_key = "real-redis-generation-submit"
        submission = {
            **preflight_payload,
            "preflight_hash": preflight_result["preflight_hash"],
            "preflight_expires_at": preflight_result["expires_at"],
            "warning_acknowledgements": ["STYLE_REFERENCE_MISSING"],
            "high_cost_confirmed": True,
            "idempotency_key": idempotency_key,
        }

        created = await client.post(
            f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
            headers=headers,
            json=submission,
        )
        assert created.status_code == 201
        created_data = created.json()["data"]
        assert created_data["replayed"] is False
        assert await generation_fact_counts(session_factory) == (1, 1, 1, 1, 1)

        key_prefix = f"lanverse:{environment}:high_cost_guard:v1"
        global_key = f"{key_prefix}:global:window"
        workspace_key = f"{key_prefix}:workspace:{workspace_id}"
        request_digest = hashlib.sha256(idempotency_key.encode("utf-8")).hexdigest()
        request_key = f"{key_prefix}:request:{request_digest}"
        assert await observer.get(global_key) == b"1"
        assert await observer.get(workspace_key) == b"1"
        assert await observer.get(request_key) == created_data["request"]["input_hash"].encode(
            "utf-8"
        )

        replayed = await client.post(
            f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
            headers=headers,
            json=submission,
        )
        assert replayed.status_code == 201
        assert replayed.json()["data"]["replayed"] is True
        assert await observer.get(global_key) == b"1"
        assert await observer.get(workspace_key) == b"1"
        assert await generation_fact_counts(session_factory) == (1, 1, 1, 1, 1)

        refreshed_preflight = await client.post(
            f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
            headers=headers,
            json=preflight_payload,
        )
        assert refreshed_preflight.status_code == 200
        refreshed_result = refreshed_preflight.json()["data"]
        app.dependency_overrides[get_high_cost_guard] = lambda: unavailable
        dependency_failure = await client.post(
            f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
            headers=headers,
            json={
                **submission,
                "preflight_hash": refreshed_result["preflight_hash"],
                "preflight_expires_at": refreshed_result["expires_at"],
                "idempotency_key": "real-redis-outage",
            },
        )
        assert dependency_failure.status_code == 503
        assert dependency_failure.json()["error"]["code"] == "dependency_unavailable"
        assert await generation_fact_counts(session_factory) == (1, 1, 1, 1, 1)
    finally:
        scan = cast(
            AsyncIterator[bytes],
            observer.scan_iter(  # pyright: ignore[reportUnknownMemberType]
                match=f"lanverse:{environment}:high_cost_guard:v1:*",
                count=100,
            ),
        )
        keys = [item async for item in scan]
        if keys:
            await observer.delete(*keys)
        await observer.aclose()
        await guard.close()
        await unavailable.close()
