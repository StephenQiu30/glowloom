from datetime import datetime, timedelta
from typing import Protocol
from uuid import UUID

from sqlalchemy import and_, or_, select
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.messaging.contracts import MessageEnvelope, OutboxEventCommand
from app.modules.messaging.models import OutboxEvent


class MessagePublisher(Protocol):
    async def publish(self, envelope: MessageEnvelope, routing_key: str) -> None: ...


async def enqueue_outbox_event(
    session: AsyncSession,
    command: OutboxEventCommand,
) -> UUID:
    event_id = uuid7()
    session.add(
        OutboxEvent(
            id=event_id,
            workspace_id=command.workspace_id,
            event_type=command.event_type,
            schema_version=command.schema_version,
            aggregate_type=command.aggregate_type,
            aggregate_id=command.aggregate_id,
            routing_key=command.routing_key,
            payload=command.payload,
            trace_id=command.trace_id,
            causation_event_id=command.causation_event_id,
            status="pending",
            attempt_count=0,
            available_at=command.available_at,
            occurred_at=command.occurred_at,
            created_at=command.occurred_at,
        )
    )
    await session.flush()
    return event_id


def envelope_from_event(event: OutboxEvent) -> MessageEnvelope:
    return MessageEnvelope(
        event_id=event.id,
        event_type=event.event_type,
        schema_version=event.schema_version,
        aggregate_id=event.aggregate_id,
        workspace_id=event.workspace_id,
        occurred_at=event.occurred_at,
        trace_id=event.trace_id,
        causation_event_id=event.causation_event_id,
        payload={key: str(value) for key, value in event.payload.items()},
    )


async def claim_outbox_events(
    session: AsyncSession,
    *,
    publisher_id: str,
    now: datetime,
    batch_size: int,
    claim_timeout: timedelta,
) -> list[OutboxEvent]:
    expired_before = now - claim_timeout
    rows = await session.scalars(
        select(OutboxEvent)
        .where(
            or_(
                and_(
                    OutboxEvent.status == "pending",
                    OutboxEvent.available_at <= now,
                ),
                and_(
                    OutboxEvent.status == "claimed",
                    or_(
                        OutboxEvent.claimed_at.is_(None),
                        OutboxEvent.claimed_at <= expired_before,
                    ),
                ),
            )
        )
        .order_by(OutboxEvent.available_at, OutboxEvent.id)
        .limit(batch_size)
        .with_for_update(skip_locked=True)
    )
    events = list(rows)
    for event in events:
        event.status = "claimed"
        event.attempt_count += 1
        event.claimed_at = now
        event.claimed_by = publisher_id
    await session.flush()
    return events


async def mark_outbox_published(
    session: AsyncSession,
    event_id: UUID,
    *,
    publisher_id: str,
    now: datetime,
) -> None:
    event = await session.scalar(
        select(OutboxEvent).where(OutboxEvent.id == event_id).with_for_update()
    )
    if event is None or event.status != "claimed" or event.claimed_by != publisher_id:
        raise RuntimeError("outbox claim is no longer owned")
    event.status = "published"
    event.published_at = now
    event.claimed_at = None
    event.claimed_by = None
    event.last_error = None
    await session.flush()


async def release_outbox_for_retry(
    session: AsyncSession,
    event_id: UUID,
    *,
    publisher_id: str,
    now: datetime,
    error: Exception,
) -> None:
    event = await session.scalar(
        select(OutboxEvent).where(OutboxEvent.id == event_id).with_for_update()
    )
    if event is None or event.status != "claimed" or event.claimed_by != publisher_id:
        raise RuntimeError("outbox claim is no longer owned")
    backoff_seconds = min(2**event.attempt_count, 60)
    event.status = "pending"
    event.available_at = now + timedelta(seconds=backoff_seconds)
    event.claimed_at = None
    event.claimed_by = None
    event.last_error = type(error).__name__
    await session.flush()
