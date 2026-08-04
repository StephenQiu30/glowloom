import logging
from datetime import UTC, datetime, timedelta
from typing import cast
from uuid import UUID

import pytest
from prometheus_client import generate_latest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.identity import ActorContext
from app.modules.identity.models import UserAccount, Workspace
from app.modules.messaging import (
    MessageEnvelope,
    claim_outbox_events,
)
from app.modules.messaging.models import OutboxEvent
from app.modules.production import ScriptExtractionTaskCommand, create_script_extraction_task
from app.scheduler import publish_outbox_batch


async def _actor(session_factory: async_sessionmaker[AsyncSession]) -> ActorContext:
    user_id = uuid7()
    workspace_id = uuid7()
    async with session_factory() as session:
        async with session.begin():
            session.add_all(
                (
                    UserAccount(
                        id=user_id,
                        email_normalized=f"publisher-{user_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Publisher Fixture",
                    ),
                    Workspace(id=workspace_id, name="Publisher Fixture"),
                )
            )
    return ActorContext(
        user_id=user_id,
        workspace_id=workspace_id,
        membership_id=uuid7(),
        role="owner",
        workspace_status="active",
    )


async def _task(
    session_factory: async_sessionmaker[AsyncSession],
    actor: ActorContext,
    *,
    idempotency_key: str,
) -> UUID:
    async with session_factory() as session:
        async with session.begin():
            task = await create_script_extraction_task(
                session,
                actor,
                ScriptExtractionTaskCommand(
                    workspace_id=actor.workspace_id,
                    episode_id=uuid7(),
                    request_id=uuid7(),
                    input_version_id=uuid7(),
                    input_hash="a" * 64,
                    idempotency_key=idempotency_key,
                ),
                trace_id=f"trace-{idempotency_key}",
            )
    return task.id


class RecordingPublisher:
    def __init__(self, failed_task_id: UUID | None = None) -> None:
        self.failed_task_id = failed_task_id
        self.messages: list[tuple[MessageEnvelope, str]] = []

    async def publish(self, envelope: MessageEnvelope, routing_key: str) -> None:
        self.messages.append((envelope, routing_key))
        if envelope.aggregate_id == self.failed_task_id:
            raise RuntimeError("synthetic-secret-must-not-be-stored")


@pytest.mark.asyncio
async def test_claim_is_exclusive_and_expired_claim_can_be_recovered(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    actor = await _actor(session_factory)
    await _task(session_factory, actor, idempotency_key="claim-once")
    now = datetime.now(UTC)

    async with session_factory() as session:
        async with session.begin():
            first = await claim_outbox_events(
                session,
                publisher_id="publisher-a",
                now=now,
                batch_size=10,
                claim_timeout=timedelta(seconds=60),
            )
        assert len(first) == 1
        assert first[0].status == "claimed"
        assert first[0].claimed_by == "publisher-a"
        assert first[0].attempt_count == 1

    async with session_factory() as session:
        async with session.begin():
            second = await claim_outbox_events(
                session,
                publisher_id="publisher-b",
                now=now + timedelta(seconds=30),
                batch_size=10,
                claim_timeout=timedelta(seconds=60),
            )
        assert second == []

        async with session.begin():
            recovered = await claim_outbox_events(
                session,
                publisher_id="publisher-b",
                now=now + timedelta(seconds=61),
                batch_size=10,
                claim_timeout=timedelta(seconds=60),
            )
        assert len(recovered) == 1
        assert recovered[0].id == first[0].id
        assert recovered[0].claimed_by == "publisher-b"
        assert recovered[0].attempt_count == 2


@pytest.mark.asyncio
async def test_publish_batch_persists_confirm_and_sanitized_retry(
    session_factory: async_sessionmaker[AsyncSession],
    caplog: pytest.LogCaptureFixture,
) -> None:
    caplog.set_level(logging.INFO, logger="lanverse.outbox")
    actor = await _actor(session_factory)
    succeeded_task_id = await _task(session_factory, actor, idempotency_key="publish-success")
    failed_task_id = await _task(session_factory, actor, idempotency_key="publish-failure")
    publisher = RecordingPublisher(failed_task_id)
    published = await publish_outbox_batch(
        session_factory,
        publisher,
        publisher_id="publisher-test",
        batch_size=10,
        claim_timeout=timedelta(seconds=60),
    )
    assert published == 1
    assert len(publisher.messages) == 2
    for envelope, routing_key in publisher.messages:
        assert routing_key == "io.script.extract"
        assert envelope.event_type == "script_extraction.requested"
        assert envelope.schema_version == 1
        assert envelope.payload == {"task_id": str(envelope.aggregate_id)}
        assert envelope.workspace_id == actor.workspace_id

    async with session_factory() as session:
        events = list(await session.scalars(select(OutboxEvent).order_by(OutboxEvent.id)))
    succeeded = next(event for event in events if event.aggregate_id == succeeded_task_id)
    failed = next(event for event in events if event.aggregate_id == failed_task_id)
    assert succeeded.status == "published"
    assert succeeded.attempt_count == 1
    assert succeeded.published_at is not None
    assert succeeded.last_error is None
    assert failed.status == "pending"
    assert failed.attempt_count == 1
    assert failed.available_at > failed.created_at
    assert failed.last_error == "RuntimeError"
    assert "synthetic-secret" not in failed.last_error

    records = [
        record
        for record in caplog.records
        if getattr(record, "event_name", "").startswith("outbox.publish.")
    ]
    assert [record.__dict__["event_name"] for record in records] == [
        "outbox.publish.completed",
        "outbox.publish.failed",
    ]
    failed_context = cast(dict[str, object], records[-1].__dict__["context"])
    assert failed_context["result"] == "retry_scheduled"
    assert failed_context["retryable"] is True
    assert failed_context["error_type"] == "RuntimeError"
    assert "synthetic-secret" not in str(failed_context)

    rendered_metrics = generate_latest().decode("utf-8")
    assert (
        'lanverse_outbox_publish_results_total{event_type="script_extraction.requested",'
        'queue="lanverse.io",result="published"}'
    ) in rendered_metrics
    assert (
        'lanverse_outbox_publish_results_total{event_type="script_extraction.requested",'
        'queue="lanverse.io",result="retry_scheduled"}'
    ) in rendered_metrics
    assert "synthetic-secret" not in rendered_metrics
