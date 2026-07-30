import asyncio
from uuid import UUID

import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.identity import ActorContext
from app.modules.identity.models import UserAccount, Workspace
from app.modules.messaging import envelope_from_event
from app.modules.messaging.consumer import IO_SCRIPT_EXTRACTION_CONSUMER, consume_envelope
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production import ScriptExtractionTaskCommand, create_script_extraction_task
from app.modules.production.models import Task


async def _task_and_event(
    factory: async_sessionmaker[AsyncSession],
) -> tuple[UUID, OutboxEvent]:
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
                        email_normalized=f"consumer-{user_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Consumer Fixture",
                    ),
                    Workspace(id=workspace_id, name="Consumer Fixture"),
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
                    input_hash="b" * 64,
                    idempotency_key=f"consumer-{uuid7()}",
                ),
                trace_id="consumer-contract-trace",
            )
            task_id = task.id
        event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.aggregate_id == task_id)
        )
        assert event is not None
        session.expunge(event)
    return task_id, event


@pytest.mark.asyncio
async def test_duplicate_delivery_has_one_business_effect(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    task_id, event = await _task_and_event(session_factory)
    envelope = envelope_from_event(event)

    async with session_factory() as session:
        async with session.begin():
            assert (
                await consume_envelope(
                    session,
                    envelope,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )
                == "completed"
            )
    async with session_factory() as session:
        async with session.begin():
            assert (
                await consume_envelope(
                    session,
                    envelope,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )
                == "duplicate"
            )

    async with session_factory() as session:
        task = await session.get(Task, task_id)
        inbox = await session.scalar(select(InboxDelivery))
        count = await session.scalar(select(func.count()).select_from(InboxDelivery))
        assert task is not None
        assert task.status == "failed"
        assert task.progress_stage == "blocked"
        assert task.error_code == "ai_service_unavailable"
        assert task.error_retryable is False
        assert task.error_summary == "AI extraction service is not configured"
        assert task.next_action == "configure_ai_service"
        assert task.revision == 2
        assert inbox is not None
        assert inbox.event_id == envelope.event_id
        assert inbox.consumer_name == IO_SCRIPT_EXTRACTION_CONSUMER
        assert inbox.task_id == task_id
        assert inbox.status == "completed"
        assert inbox.attempt_count == 2
        assert inbox.last_error is None
        assert count == 1


@pytest.mark.asyncio
async def test_concurrent_duplicate_delivery_is_serialized_by_inbox_key(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    task_id, event = await _task_and_event(session_factory)
    envelope = envelope_from_event(event)

    async def deliver() -> str:
        async with session_factory() as session:
            async with session.begin():
                return await consume_envelope(
                    session,
                    envelope,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )

    results = await asyncio.gather(deliver(), deliver())
    assert sorted(results) == ["completed", "duplicate"]

    async with session_factory() as session:
        task = await session.get(Task, task_id)
        inbox = await session.scalar(select(InboxDelivery))
        assert task is not None
        assert task.status == "failed"
        assert task.revision == 2
        assert inbox is not None
        assert inbox.status == "completed"
        assert inbox.attempt_count == 2


@pytest.mark.asyncio
async def test_database_rollback_leaves_message_safe_to_redeliver(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    task_id, event = await _task_and_event(session_factory)
    envelope = envelope_from_event(event)

    with pytest.raises(RuntimeError, match="worker stopped before commit"):
        async with session_factory() as session:
            async with session.begin():
                await consume_envelope(
                    session,
                    envelope,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )
                raise RuntimeError("worker stopped before commit")

    async with session_factory() as session:
        task = await session.get(Task, task_id)
        count = await session.scalar(select(func.count()).select_from(InboxDelivery))
        assert task is not None
        assert task.status == "queued"
        assert task.revision == 1
        assert count == 0

    async with session_factory() as session:
        async with session.begin():
            assert (
                await consume_envelope(
                    session,
                    envelope,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )
                == "completed"
            )

    async with session_factory() as session:
        task = await session.get(Task, task_id)
        count = await session.scalar(select(func.count()).select_from(InboxDelivery))
        assert task is not None
        assert task.status == "failed"
        assert task.revision == 2
        assert count == 1


@pytest.mark.asyncio
async def test_unknown_schema_is_persisted_as_rejected(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    task_id, event = await _task_and_event(session_factory)
    envelope = envelope_from_event(event).model_copy(update={"schema_version": 99})

    async with session_factory() as session:
        async with session.begin():
            result = await consume_envelope(
                session,
                envelope,
                consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
            )
            assert result == "rejected"

    async with session_factory() as session:
        task = await session.get(Task, task_id)
        inbox = await session.scalar(select(InboxDelivery))
        assert task is not None
        assert task.status == "failed"
        assert task.error_code == "unsupported_message_schema"
        assert task.error_retryable is False
        assert task.next_action == "contact_support"
        assert task.revision == 2
        assert inbox is not None
        assert inbox.status == "rejected"
        assert inbox.last_error == "unsupported_message_schema"


@pytest.mark.asyncio
async def test_late_event_for_terminal_task_is_acknowledged_without_reexecution(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    task_id, event = await _task_and_event(session_factory)
    first = envelope_from_event(event)
    late = first.model_copy(update={"event_id": uuid7()})

    async with session_factory() as session:
        async with session.begin():
            await consume_envelope(
                session,
                first,
                consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
            )
    async with session_factory() as session:
        async with session.begin():
            assert (
                await consume_envelope(
                    session,
                    late,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )
                == "completed"
            )

    async with session_factory() as session:
        task = await session.get(Task, task_id)
        deliveries = list(
            await session.scalars(
                select(InboxDelivery).order_by(InboxDelivery.received_at)
            )
        )
        assert task is not None
        assert task.status == "failed"
        assert task.revision == 2
        assert [item.status for item in deliveries] == ["completed", "completed"]
