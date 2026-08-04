from datetime import datetime
from typing import Literal
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.messaging.contracts import MessageEnvelope
from app.modules.messaging.models import InboxDelivery

InboxResult = Literal["completed", "duplicate", "rejected"]


async def start_inbox_delivery(
    session: AsyncSession,
    envelope: MessageEnvelope,
    consumer_name: str,
    now: datetime,
) -> tuple[InboxDelivery, bool]:
    delivery_id = uuid7()
    inserted_id = await session.scalar(
        insert(InboxDelivery)
        .values(
            id=delivery_id,
            workspace_id=envelope.workspace_id,
            event_id=envelope.event_id,
            event_type=envelope.event_type,
            consumer_name=consumer_name,
            trace_id=envelope.trace_id,
            status="processing",
            attempt_count=1,
            received_at=now,
        )
        .on_conflict_do_nothing(constraint="uq_sys_inbox_event_consumer")
        .returning(InboxDelivery.id)
    )
    if inserted_id is None:
        existing = await session.scalar(
            select(InboxDelivery)
            .where(
                InboxDelivery.event_id == envelope.event_id,
                InboxDelivery.consumer_name == consumer_name,
            )
            .with_for_update()
        )
        if existing is None:
            raise RuntimeError("inbox delivery is unavailable")
        existing.attempt_count += 1
        await session.flush()
        return existing, False

    delivery = await session.scalar(
        select(InboxDelivery)
        .where(InboxDelivery.id == inserted_id)
        .with_for_update()
    )
    if delivery is None:
        raise RuntimeError("inbox delivery is unavailable")
    return delivery, True


def reject_inbox_delivery(
    delivery: InboxDelivery,
    *,
    error_code: str,
    now: datetime,
) -> InboxResult:
    delivery.status = "rejected"
    delivery.last_error = error_code
    delivery.processed_at = now
    return "rejected"


def mark_inbox_delivery_manual_attention(
    delivery: InboxDelivery,
    *,
    error_code: str,
    now: datetime,
) -> InboxResult:
    delivery.status = "manual_attention"
    delivery.last_error = error_code
    delivery.processed_at = now
    return "rejected"


def complete_inbox_delivery(
    delivery: InboxDelivery,
    *,
    now: datetime,
    last_error: str | None = None,
) -> None:
    delivery.status = "completed"
    delivery.last_error = last_error
    delivery.processed_at = now


async def lock_inbox_delivery(
    session: AsyncSession,
    delivery_id: UUID,
) -> InboxDelivery:
    delivery = await session.scalar(
        select(InboxDelivery)
        .where(InboxDelivery.id == delivery_id)
        .with_for_update()
    )
    if delivery is None:
        raise RuntimeError("inbox delivery is unavailable")
    return delivery
