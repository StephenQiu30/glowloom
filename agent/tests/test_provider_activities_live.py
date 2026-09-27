import asyncio
import os
from datetime import timedelta
from pathlib import Path
from typing import Any
from uuid import uuid4

import pytest
from redis.asyncio import Redis
from temporalio import workflow
from temporalio.client import Client
from temporalio.worker import Worker

from app.harness.mock_model import MockStructuredClient
from app.harness.router import ModelRouter, Price
from app.harness.skills import SkillRegistry
from app.providers.mock import MockProvider
from app.worker.provider_activities import create_mock_worker
from app.worker.skill_activities import create_skill_worker


@workflow.defn
class MockActivityProbe:
    @workflow.run
    async def run(self, activity_queue: str, skill_queue: str, request_key: str) -> dict[str, Any]:
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
        skill = await workflow.execute_activity(
            "llm.run_skill",
            {
                "skill_key": "mock.echo",
                "skill_version": "1.0.0",
                "model_key": "mock.structured",
                "inputs": {"value": request_key},
                "budget": {
                    "max_tokens": 4096,
                    "max_cost_micros": 0,
                    "deadline": (workflow.now() + timedelta(minutes=1)).isoformat(),
                },
            },
            task_queue=skill_queue,
            start_to_close_timeout=timedelta(seconds=10),
        )
        moderation = await workflow.execute_activity(
            "moderation.check",
            {
                "operation_id": request_key,
                "output_id": f"{request_key}-output",
                "adapter_key": "mock",
                "kind": "video",
                "asset_id": f"{request_key}-asset",
                "mock_status": "rejected",
                "mock_labels": ["fixture.policy"],
            },
            task_queue=skill_queue,
            start_to_close_timeout=timedelta(seconds=10),
        )
        return {"submit": submitted, "query": queried, "skill": skill, "moderation": moderation}


def test_local_temporal_routes_to_mock_provider_activity() -> None:
    temporal_addr = os.getenv("LV_TEST_TEMPORAL_ADDR")
    namespace = os.getenv("LV_TEST_TEMPORAL_NAMESPACE")
    redis_url = os.getenv("LV_TEST_REDIS_URL")
    if not all((temporal_addr, namespace, redis_url)):
        pytest.skip("local Temporal and Redis test addresses are not configured")
    assert temporal_addr is not None and namespace is not None and redis_url is not None
    registry = SkillRegistry.load(Path(__file__).resolve().parents[1] / "skills")

    async def run() -> None:
        client = await Client.connect(temporal_addr, namespace=namespace)
        token = uuid4().hex
        flow_queue = f"lanverse-test-flow-{token}"
        activity_queue = f"lanverse-test-agent-mock-{token}"
        skill_queue = f"lanverse-test-agent-skill-{token}"
        router = ModelRouter(MockStructuredClient(), {"mock.structured": Price(0, 0)})
        async with (
            Redis.from_url(redis_url, decode_responses=True) as store,
            create_mock_worker(
                client,
                MockProvider(store, task_ttl_seconds=60, key_prefix="lanverse:mock-provider:test"),
                task_queue=activity_queue,
            ),
            create_skill_worker(client, registry, router, task_queue=skill_queue),
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
                    args=[activity_queue, skill_queue, f"lanverse-mock-activity-{token}"],
                    id=f"lanverse-mock-activity-{token}",
                    task_queue=flow_queue,
                ),
                timeout=30,
            )
        submitted = result["submit"]
        queried = result["query"]
        skill = result["skill"]
        assert isinstance(submitted, dict) and submitted["outcome"] == "unknown"
        assert isinstance(queried, dict) and queried["state"] == "succeeded"
        assert isinstance(skill, dict)
        assert skill["result"] == {"value": f"lanverse-mock-activity-{token}"}
        assert result["moderation"] == {
            "status": "rejected",
            "labels": ["fixture.policy"],
            "provider": "mock",
        }

    asyncio.run(run())
