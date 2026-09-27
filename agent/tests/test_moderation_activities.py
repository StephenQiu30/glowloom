import asyncio

import pytest
from temporalio.exceptions import ApplicationError
from temporalio.testing import ActivityEnvironment

from app.moderation.mock import MockModerationActivities


def media_input(**overrides: object) -> dict[str, object]:
    payload: dict[str, object] = {
        "operation_id": "op-1",
        "output_id": "output-1",
        "adapter_key": "mock",
        "kind": "video",
        "asset_id": "asset-1",
        "mock_status": "passed",
    }
    payload.update(overrides)
    return payload


def test_mock_moderation_requires_explicit_decision_and_never_reads_media() -> None:
    async def run() -> None:
        activities = MockModerationActivities()
        env = ActivityEnvironment()

        passed = await env.run(
            activities.check,
            media_input(media_url="http://127.0.0.1:1/unreachable?token=private"),
        )
        assert passed == {"status": "passed", "labels": [], "provider": "mock"}

        rejected = await env.run(
            activities.check,
            media_input(mock_status="rejected", mock_labels=["fixture.policy"]),
        )
        assert rejected == {
            "status": "rejected",
            "labels": ["fixture.policy"],
            "provider": "mock",
        }

        rejected_without_label = await env.run(
            activities.check,
            media_input(mock_status="rejected"),
        )
        assert rejected_without_label["labels"] == ["mock_rejected"]

        text_result = await env.run(
            activities.check,
            media_input(kind="text", asset_id=None, text="fixture text"),
        )
        assert text_result["status"] == "passed"

    asyncio.run(run())


@pytest.mark.parametrize(
    "overrides",
    [
        {"adapter_key": "ark"},
        {"mock_status": None},
        {"operation_id": ""},
        {"output_id": ""},
        {"asset_id": ""},
        {"kind": "unknown"},
        {"kind": "text", "asset_id": None},
        {"mock_labels": [""]},
        {"unexpected": "value"},
    ],
)
def test_mock_moderation_fails_closed(overrides: dict[str, object]) -> None:
    async def run() -> None:
        activities = MockModerationActivities()
        env = ActivityEnvironment()
        with pytest.raises(ApplicationError) as exc:
            await env.run(activities.check, media_input(**overrides))
        assert exc.value.type == "invalid_activity_input"
        assert exc.value.non_retryable
        assert "private" not in str(exc.value)

    asyncio.run(run())
