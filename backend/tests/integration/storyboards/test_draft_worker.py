from typing import Any
from uuid import UUID

import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app import io_worker
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production.models import Task
from app.modules.storyboards import (
    StoryboardDraftInput,
    StoryboardDraftProvider,
    StoryboardDraftProviderError,
)
from app.modules.storyboards.drafts.models import StoryboardDraftBatch
from app.modules.storyboards.models import Shot
from tests.integration.storyboards.test_draft_batches import (
    create_batch_fixture,
    provider_result,
    published_episode,
)


class RecordingMessage:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


class RecordingDrafter(StoryboardDraftProvider):
    def __init__(self, result: dict[str, object]) -> None:
        self.result = result
        self.inputs: list[StoryboardDraftInput] = []

    async def draft(self, value: StoryboardDraftInput) -> dict[str, object]:
        self.inputs.append(value)
        return self.result


class UnknownDrafter(StoryboardDraftProvider):
    async def draft(self, value: StoryboardDraftInput) -> dict[str, object]:
        raise StoryboardDraftProviderError(
            outcome="unknown",
            code="ai_result_unknown",
            summary="Provider outcome is unknown",
            retryable=False,
            next_action="create_new_storyboard_draft_batch",
        )


async def _message_body(
    factory: async_sessionmaker[AsyncSession],
    task_id: str,
) -> bytes:
    async with factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.aggregate_id == UUID(task_id))
        )
        assert event is not None
        return envelope_from_event(event).model_dump_json().encode("utf-8")


@pytest.mark.asyncio
async def test_worker_persists_reviewable_drafts_without_writing_shots(
    client: Any,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _project, episode, script = await published_episode(
        client,
        session_factory,
        email="draft-worker@example.com",
    )
    batch = await create_batch_fixture(
        client,
        headers=headers,
        episode=episode,
        version_id=script["version"]["id"],
        key="draft-worker",
    )
    result = provider_result(script["structure"])
    drafter = RecordingDrafter(result.model_dump(mode="json"))
    message = RecordingMessage(await _message_body(session_factory, batch["task_id"]))

    first = await io_worker.process_incoming_message(
        message,
        session_factory,
        storyboard_drafter=drafter,
    )
    replay = RecordingMessage(message.body)
    second = await io_worker.process_incoming_message(
        replay,
        session_factory,
        storyboard_drafter=drafter,
    )

    assert first == "completed"
    assert second == "duplicate"
    assert message.ack_count == 1
    assert replay.ack_count == 1
    assert len(drafter.inputs) == 1
    assert drafter.inputs[0].batch_id == UUID(batch["id"])
    assert drafter.inputs[0].units
    async with session_factory() as session:
        stored_batch = await session.get(StoryboardDraftBatch, UUID(batch["id"]))
        task = await session.get(Task, UUID(batch["task_id"]))
        shot_count = await session.scalar(select(func.count()).select_from(Shot))
        delivery = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == UUID(batch["task_id"]))
        )
        assert stored_batch is not None
        assert stored_batch.status == "needs_review"
        assert task is not None
        assert task.status == "succeeded"
        assert shot_count == 0
        assert delivery is not None
        assert delivery.status == "completed"


@pytest.mark.asyncio
async def test_worker_records_unknown_without_partial_drafts(
    client: Any,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _project, episode, script = await published_episode(
        client,
        session_factory,
        email="draft-worker-unknown@example.com",
    )
    batch = await create_batch_fixture(
        client,
        headers=headers,
        episode=episode,
        version_id=script["version"]["id"],
        key="draft-worker-unknown",
    )
    message = RecordingMessage(await _message_body(session_factory, batch["task_id"]))

    outcome = await io_worker.process_incoming_message(
        message,
        session_factory,
        storyboard_drafter=UnknownDrafter(),
    )

    assert outcome == "completed"
    assert message.ack_count == 1
    async with session_factory() as session:
        stored_batch = await session.get(StoryboardDraftBatch, UUID(batch["id"]))
        task = await session.get(Task, UUID(batch["task_id"]))
        assert stored_batch is not None
        assert stored_batch.status == "unknown"
        assert stored_batch.error_code == "ai_result_unknown"
        assert task is not None
        assert task.status == "unknown"
