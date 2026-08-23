import logging
from collections.abc import Awaitable, Callable
from typing import cast
from uuid import UUID

import pytest
from prometheus_client import generate_latest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.identity import ActorContext
from app.modules.identity.models import UserAccount, Workspace
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production import ScriptExtractionTaskCommand, create_script_extraction_task
from app.modules.production.models import Task
from app.runtime.workers import io as io_worker


async def _task_and_body(
    factory: async_sessionmaker[AsyncSession],
) -> tuple[UUID, bytes]:
    user_id = uuid7()
    workspace_id = uuid7()
    actor = ActorContext(
        user_id=user_id,
        workspace_id=workspace_id,
        membership_id=uuid7(),
        role="owner",
        workspace_status="active",
    )
    async with factory() as session:
        async with session.begin():
            session.add_all(
                (
                    UserAccount(
                        id=user_id,
                        email_normalized=f"worker-{user_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Worker Fixture",
                    ),
                    Workspace(id=workspace_id, name="Worker Fixture"),
                )
            )
            task = await create_script_extraction_task(
                session,
                actor,
                ScriptExtractionTaskCommand(
                    workspace_id=workspace_id,
                    episode_id=uuid7(),
                    request_id=uuid7(),
                    input_version_id=uuid7(),
                    input_hash="c" * 64,
                    idempotency_key=f"worker-{uuid7()}",
                ),
                trace_id="worker-transaction-trace",
            )
            task_id = task.id
        event = await session.scalar(select(OutboxEvent).where(OutboxEvent.aggregate_id == task_id))
        assert event is not None
        body = envelope_from_event(event).model_dump_json().encode("utf-8")
    return task_id, body


class RecordingMessage:
    def __init__(
        self,
        body: bytes,
        *,
        on_ack: Callable[[], Awaitable[None]] | None = None,
    ) -> None:
        self.body = body
        self.on_ack = on_ack
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        if self.on_ack is not None:
            await self.on_ack()
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


@pytest.mark.asyncio
async def test_worker_ack_happens_after_database_commit(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    task_id, body = await _task_and_body(session_factory)

    async def assert_committed() -> None:
        async with session_factory() as session:
            task = await session.get(Task, task_id)
            inbox = await session.scalar(select(InboxDelivery))
            assert task is not None
            assert task.status == "failed"
            assert task.revision == 2
            assert inbox is not None
            assert inbox.status == "completed"

    message = RecordingMessage(body, on_ack=assert_committed)
    result = await io_worker.process_incoming_message(message, session_factory)

    assert result == "completed"
    assert message.ack_count == 1
    assert message.nack_requeues == []


@pytest.mark.asyncio
async def test_worker_nacks_database_failure_without_persisting_result(
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    caplog.set_level(logging.INFO, logger="lanverse.worker")
    task_id, body = await _task_and_body(session_factory)

    async def fail_processing(*_: object, **__: object) -> None:
        raise RuntimeError("synthetic database stage failure")

    monkeypatch.setattr(io_worker, "consume_envelope", fail_processing)
    message = RecordingMessage(body)
    result = await io_worker.process_incoming_message(message, session_factory)

    assert result == "requeued"
    assert message.ack_count == 0
    assert message.nack_requeues == [True]
    async with session_factory() as session:
        task = await session.get(Task, task_id)
        count = await session.scalar(select(func.count()).select_from(InboxDelivery))
        assert task is not None
        assert task.status == "queued"
        assert task.revision == 1
        assert count == 0
    failed_record = next(
        record
        for record in caplog.records
        if getattr(record, "event_name", None) == "message.consume.failed"
    )
    context = cast(dict[str, object], failed_record.__dict__["context"])
    assert context["topic"] == "lanverse.io.v1"
    assert context["event_type"] == "script_extraction.requested"
    assert context["result"] == "requeued"
    assert context["retryable"] is True
    assert "synthetic database stage failure" not in str(context)


@pytest.mark.asyncio
async def test_worker_acknowledges_unparseable_or_oversized_poison_message(
    session_factory: async_sessionmaker[AsyncSession],
    caplog: pytest.LogCaptureFixture,
) -> None:
    caplog.set_level(logging.INFO, logger="lanverse.worker")
    malformed = RecordingMessage(b"not-json-and-no-stable-message-identity")
    oversized = RecordingMessage(b"x" * (io_worker.MAX_MESSAGE_BYTES + 1))

    malformed_result = await io_worker.process_incoming_message(malformed, session_factory)
    oversized_result = await io_worker.process_incoming_message(oversized, session_factory)

    assert malformed_result == "rejected"
    assert oversized_result == "rejected"
    assert malformed.ack_count == 1
    assert oversized.ack_count == 1
    assert malformed.nack_requeues == []
    assert oversized.nack_requeues == []
    rejected_records = [
        record
        for record in caplog.records
        if getattr(record, "event_name", None) == "message.consume.failed"
    ]
    assert len(rejected_records) == 2
    assert all(
        cast(dict[str, object], record.__dict__["context"])["event_type"] == "invalid"
        for record in rejected_records
    )
    assert "not-json-and-no-stable-message-identity" not in str(rejected_records)
    rendered_metrics = generate_latest().decode("utf-8")
    assert (
        'lanverse_message_results_total{event_type="invalid",result="rejected",topic="lanverse.io.v1"}'
    ) in rendered_metrics
    assert "not-json-and-no-stable-message-identity" not in rendered_metrics
