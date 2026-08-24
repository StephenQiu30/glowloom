import asyncio
from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID

import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

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
from app.runtime.workers import io as io_worker
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


class SimulatedWorkerCrash(BaseException):
    pass


class InterruptingDrafter(StoryboardDraftProvider):
    async def draft(self, value: StoryboardDraftInput) -> dict[str, object]:
        del value
        raise SimulatedWorkerCrash


class BlockingDrafter(StoryboardDraftProvider):
    def __init__(self, result: dict[str, object]) -> None:
        self.result = result
        self.entered = asyncio.Event()
        self.release = asyncio.Event()

    async def draft(self, value: StoryboardDraftInput) -> dict[str, object]:
        del value
        self.entered.set()
        await self.release.wait()
        return self.result


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
        assert stored_batch.agent_run_token is None
        assert stored_batch.agent_lease_expires_at is None
        assert task is not None
        assert task.status == "unknown"


@pytest.mark.asyncio
async def test_worker_resumes_interrupted_storyboard_harness_from_same_delivery(
    client: Any,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _project, episode, script = await published_episode(
        client,
        session_factory,
        email="draft-worker-resume@example.com",
    )
    batch = await create_batch_fixture(
        client,
        headers=headers,
        episode=episode,
        version_id=script["version"]["id"],
        key="draft-worker-resume",
    )
    body = await _message_body(session_factory, batch["task_id"])

    with pytest.raises(SimulatedWorkerCrash):
        await io_worker.process_incoming_message(
            RecordingMessage(body),
            session_factory,
            storyboard_drafter=InterruptingDrafter(),
        )

    async with session_factory() as session, session.begin():
        interrupted_batch = await session.get(StoryboardDraftBatch, UUID(batch["id"]))
        assert interrupted_batch is not None
        assert interrupted_batch.agent_run_token is not None
        interrupted_batch.agent_lease_expires_at = datetime.now(UTC) - timedelta(seconds=1)

    resumed_drafter = RecordingDrafter(provider_result(script["structure"]).model_dump(mode="json"))
    resumed_message = RecordingMessage(body)
    outcome = await io_worker.process_incoming_message(
        resumed_message,
        session_factory,
        storyboard_drafter=resumed_drafter,
    )

    assert outcome == "completed"
    assert resumed_message.ack_count == 1
    assert len(resumed_drafter.inputs) == 1
    async with session_factory() as session:
        stored_batch = await session.get(StoryboardDraftBatch, UUID(batch["id"]))
        task = await session.get(Task, UUID(batch["task_id"]))
        delivery = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == UUID(batch["task_id"]))
        )
        assert stored_batch is not None and stored_batch.status == "needs_review"
        assert task is not None and task.status == "succeeded"
        assert delivery is not None and delivery.status == "completed"
        assert delivery.attempt_count == 2


@pytest.mark.asyncio
async def test_worker_requeues_redelivery_while_storyboard_run_lease_is_active(
    client: Any,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(io_worker, "STORYBOARD_DRAFT_LEASE_HEARTBEAT_SECONDS", 0.01)
    headers, _project, episode, script = await published_episode(
        client,
        session_factory,
        email="draft-worker-active-lease@example.com",
    )
    batch = await create_batch_fixture(
        client,
        headers=headers,
        episode=episode,
        version_id=script["version"]["id"],
        key="draft-worker-active-lease",
    )
    body = await _message_body(session_factory, batch["task_id"])
    provider_payload = provider_result(script["structure"]).model_dump(mode="json")
    blocking = BlockingDrafter(provider_payload)
    first_message = RecordingMessage(body)
    first_run = asyncio.create_task(
        io_worker.process_incoming_message(
            first_message,
            session_factory,
            storyboard_drafter=blocking,
        )
    )
    await asyncio.wait_for(blocking.entered.wait(), timeout=2)
    async with session_factory() as session:
        running_batch = await session.get(StoryboardDraftBatch, UUID(batch["id"]))
        assert running_batch is not None
        initial_expiry = running_batch.agent_lease_expires_at
        assert initial_expiry is not None
    await asyncio.sleep(0.05)
    async with session_factory() as session:
        renewed_batch = await session.get(StoryboardDraftBatch, UUID(batch["id"]))
        assert renewed_batch is not None
        assert renewed_batch.agent_lease_expires_at is not None
        assert renewed_batch.agent_lease_expires_at > initial_expiry

    duplicate_drafter = RecordingDrafter(provider_payload)
    redelivery = RecordingMessage(body)
    try:
        outcome = await io_worker.process_incoming_message(
            redelivery,
            session_factory,
            storyboard_drafter=duplicate_drafter,
        )

        assert outcome == "requeued"
        assert redelivery.ack_count == 0
        assert redelivery.nack_requeues == [True]
        assert not duplicate_drafter.inputs
    finally:
        blocking.release.set()

    assert await first_run == "completed"
    async with session_factory() as session:
        completed_batch = await session.get(StoryboardDraftBatch, UUID(batch["id"]))
        assert completed_batch is not None
        assert completed_batch.agent_run_token is None
        assert completed_batch.agent_lease_expires_at is None
