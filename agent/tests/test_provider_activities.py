import asyncio

import pytest
from fakeredis.aioredis import FakeRedis
from temporalio.exceptions import ApplicationError
from temporalio.testing import ActivityEnvironment

from app.providers.mock import MockProvider
from app.worker.provider_activities import MockProviderActivities


def submit_input(**overrides: object) -> dict[str, object]:
    payload: dict[str, object] = {
        "operation_id": "op-1",
        "provider_request_key": "request-1",
        "adapter_key": "mock",
        "provider_model_id": "mock-video",
        "capability": "video.generate",
        "mode": "omni_reference",
        "params": {"mock_outcome": "unknown", "mock_delay_ms": 1000},
        "inputs": [],
        "output_count": 1,
    }
    payload.update(overrides)
    return payload


def test_activity_submit_unknown_then_query_and_cancel() -> None:
    async def run() -> None:
        async with FakeRedis(decode_responses=True) as store:
            activities = MockProviderActivities(MockProvider(store))
            env = ActivityEnvironment()
            submitted = await env.run(activities.submit, submit_input())
            assert submitted["outcome"] == "unknown"
            assert submitted["provider_task_id"] is None

            found = await env.run(activities.query, {"provider_request_key": "request-1"})
            assert found["state"] == "pending"
            assert found["result_urls"] == []
            assert found["provider_task_id"].startswith("mock-")

            replay = await env.run(activities.submit, submit_input())
            assert replay["outcome"] == "accepted"
            cancelled = await env.run(
                activities.cancel, {"provider_task_id": replay["provider_task_id"]}
            )
            assert cancelled == {"outcome": "cancelled"}
            found = await env.run(
                activities.query, {"provider_task_id": replay["provider_task_id"]}
            )
            assert found["state"] == "failed"
            assert found["error"]["code"] == "cancelled"

    asyncio.run(run())


def test_activity_rejects_invalid_contract_without_sensitive_detail() -> None:
    async def run() -> None:
        async with FakeRedis(decode_responses=True) as store:
            activities = MockProviderActivities(MockProvider(store))
            env = ActivityEnvironment()
            with pytest.raises(ApplicationError) as exc:
                await env.run(activities.submit, submit_input(adapter_key="unknown"))
            assert exc.value.type == "invalid_activity_input"
            assert exc.value.non_retryable

            with pytest.raises(ApplicationError) as exc:
                await env.run(
                    activities.query,
                    {"provider_task_id": "task-id", "provider_request_key": "request-key"},
                )
            assert exc.value.type == "invalid_activity_input"

    asyncio.run(run())
