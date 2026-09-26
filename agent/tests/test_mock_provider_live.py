import asyncio
import os
from uuid import uuid4

import pytest
from redis.asyncio import Redis

from app.providers.mock import MockProvider, SubmitRequest, TaskRef


def test_redis_keeps_unknown_task_across_clients() -> None:
    redis_url = os.getenv("LV_TEST_REDIS_URL")
    if not redis_url:
        pytest.skip("LV_TEST_REDIS_URL is not configured")

    async def run() -> None:
        request_key = f"lanverse-mock-test-{uuid4().hex}"
        req = SubmitRequest(
            operation_id=f"op-{uuid4().hex}",
            request_key=request_key,
            provider_model_id="mock-video",
            capability="video.generate",
            mode="omni_reference",
            params={"mock_outcome": "unknown"},
            inputs=[],
            output_count=1,
        )
        async with Redis.from_url(redis_url, decode_responses=True) as first:
            provider = MockProvider(
                first, task_ttl_seconds=60, key_prefix="lanverse:mock-provider:test"
            )
            submitted = await provider.submit(req)
            assert submitted.outcome == "unknown"

        async with Redis.from_url(redis_url, decode_responses=True) as second:
            restarted = MockProvider(
                second, task_ttl_seconds=60, key_prefix="lanverse:mock-provider:test"
            )
            result = await restarted.query(TaskRef(request_key=request_key))
            assert result.state == "succeeded"

    asyncio.run(run())
