import asyncio
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any

import pytest
from temporalio.exceptions import ApplicationError
from temporalio.testing import ActivityEnvironment

from app.harness.mock_model import MockStructuredClient
from app.harness.router import ModelRouter, Price
from app.harness.skills import SkillRegistry
from app.worker.skill_activities import SkillActivities


def test_mock_skill_activity_returns_structured_result_and_safe_trace() -> None:
    registry = SkillRegistry.load(Path(__file__).resolve().parents[1] / "skills")

    async def run() -> None:
        activities = SkillActivities(
            registry,
            ModelRouter(MockStructuredClient(), {"mock.structured": Price(0, 0)}),
        )
        env = ActivityEnvironment()
        payload: dict[str, Any] = {
            "skill_key": "mock.echo",
            "skill_version": "1.0.0",
            "model_key": "mock.structured",
            "inputs": {"value": "private text"},
            "budget": {
                "max_tokens": 4096,
                "max_cost_micros": 0,
                "deadline": (datetime.now(UTC) + timedelta(minutes=1)).isoformat(),
            },
        }
        result = await env.run(activities.run, payload)
        assert result["result"] == {"value": "private text"}
        assert result["cost_micros"] == 0
        assert len(result["trace_steps"]) == 1
        assert "private text" not in str(result["trace"])

        with pytest.raises(ApplicationError) as exc:
            await env.run(activities.run, {**payload, "skill_version": "2.0.0"})
        assert exc.value.type == "skill_version_unavailable"
        assert exc.value.non_retryable

        with pytest.raises(ApplicationError) as exc:
            await env.run(
                activities.run,
                {
                    **payload,
                    "budget": {
                        **payload["budget"],
                        "deadline": (datetime.now(UTC) - timedelta(seconds=1)).isoformat(),
                    },
                },
            )
        assert exc.value.type == "budget_exceeded"
        assert exc.value.details[0]["reason"] == "deadline"
        assert "private text" not in str(exc.value.details)

    asyncio.run(run())
