import asyncio
import os
from datetime import timedelta
from typing import Any
from uuid import uuid4

import pytest
from redis.asyncio import Redis
from temporalio import workflow
from temporalio.client import Client
from temporalio.worker import Worker

from app.providers.mock import MockProvider
from app.worker.provider_activities import create_mock_worker


@workflow.defn
class MockActivityProbe:
    @workflow.run
    async def run(self, activity_queue: str, request_key: str) -> dict[str, Any]:
        submitted = await workflow.execute_activity(
            "provider.submit",
            {
                "operation_id": request_key,
                "provider_request_key": request_key,
                "adapter_key": "mock",
                "provider_model_id": "mock-video",
                "capability": "video.generate",
                "mode": "omni_reference",
                "params": {"mock_outcome": "unknown"},
                "inputs": [],
                "output_count": 1,
            },
            task_queue=activity_queue,
            start_to_close_timeout=timedelta(seconds=10),
        )
        queried = await workflow.execute_activity(
            "provider.query",
            {"provider_request_key": request_key},
            task_queue=activity_queue,
            start_to_close_timeout=timedelta(seconds=10),
        )
        return {"submit": submitted, "query": queried}


def test_local_temporal_routes_to_mock_provider_activity() -> None:
    temporal_addr = os.getenv("LV_TEST_TEMPORAL_ADDR")
    namespace = os.getenv("LV_TEST_TEMPORAL_NAMESPACE")
    redis_url = os.getenv("LV_TEST_REDIS_URL")
    if not all((temporal_addr, namespace, redis_url)):
        pytest.skip("local Temporal and Redis test addresses are not configured")
    assert temporal_addr is not None and namespace is not None and redis_url is not None

    async def run() -> None:
        client = await Client.connect(temporal_addr, namespace=namespace)
        token = uuid4().hex
        flow_queue = f"lanverse-test-flow-{token}"
        activity_queue = f"lanverse-test-agent-mock-{token}"
        async with (
            Redis.from_url(redis_url, decode_responses=True) as store,
            create_mock_worker(
                client,
                MockProvider(store, task_ttl_seconds=60, key_prefix="lanverse:mock-provider:test"),
                task_queue=activity_queue,
            ),
            Worker(
                client,
                task_queue=flow_queue,
                workflows=[MockActivityProbe],
                activities=[],
            ),
        ):
            result = await asyncio.wait_for(
                client.execute_workflow(
                    MockActivityProbe.run,
                    args=[activity_queue, f"lanverse-mock-activity-{token}"],
                    id=f"lanverse-mock-activity-{token}",
                    task_queue=flow_queue,
                ),
                timeout=30,
            )
        submitted = result["submit"]
        queried = result["query"]
        assert isinstance(submitted, dict) and submitted["outcome"] == "unknown"
        assert isinstance(queried, dict) and queried["state"] == "succeeded"

    asyncio.run(run())
