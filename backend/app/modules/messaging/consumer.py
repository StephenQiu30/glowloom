from datetime import UTC, datetime
from typing import Literal

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.messaging.models import InboxDelivery
from app.modules.messaging.schemas import MessageEnvelope
from app.modules.production.models import Task
from app.modules.production.service import fail_script_extraction_task
from app.modules.scripts import service as scripts_service

IO_SCRIPT_EXTRACTION_CONSUMER = "lanverse.io.script-extraction.v1"
ConsumerResult = Literal["completed", "duplicate", "rejected"]


async def _start_delivery(
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


def _reject(
    delivery: InboxDelivery,
    *,
    error_code: str,
    now: datetime,
) -> ConsumerResult:
    delivery.status = "rejected"
    delivery.last_error = error_code
    delivery.processed_at = now
    return "rejected"


async def consume_envelope(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    consumer_name: str,
) -> ConsumerResult:
    now = datetime.now(UTC)
    delivery, is_first_delivery = await _start_delivery(
        session, envelope, consumer_name, now
    )
    if not is_first_delivery:
        return "duplicate"

    task = await session.scalar(
        select(Task).where(Task.id == envelope.aggregate_id).with_for_update()
    )
    if task is None:
        return _reject(delivery, error_code="task_not_found", now=now)
    if task.workspace_id != envelope.workspace_id:
        return _reject(delivery, error_code="workspace_mismatch", now=now)
    delivery.task_id = task.id

    if envelope.event_type != "script_extraction.requested":
        return _reject(delivery, error_code="unsupported_message_type", now=now)
    if envelope.schema_version != 1:
        changed = fail_script_extraction_task(
            task,
            error_code="unsupported_message_schema",
            error_summary="Message schema is not supported",
            next_action="contact_support",
            now=now,
        )
        if changed:
            await scripts_service.synchronize_extraction_batch_status(
                session, task.request_id, "failed", now=now
            )
        return _reject(
            delivery,
            error_code="unsupported_message_schema",
            now=now,
        )
    if envelope.payload != {"task_id": str(envelope.aggregate_id)}:
        changed = fail_script_extraction_task(
            task,
            error_code="invalid_message_payload",
            error_summary="Message payload does not match the task",
            next_action="contact_support",
            now=now,
        )
        if changed:
            await scripts_service.synchronize_extraction_batch_status(
                session, task.request_id, "failed", now=now
            )
        return _reject(delivery, error_code="invalid_message_payload", now=now)
    if task.task_type != "script_extraction" or task.request_type != "extraction_batch":
        return _reject(delivery, error_code="unsupported_task_type", now=now)

    changed = fail_script_extraction_task(
        task,
        error_code="ai_service_unavailable",
        error_summary="AI extraction service is not configured",
        next_action="configure_ai_service",
        now=now,
    )
    if changed:
        await scripts_service.synchronize_extraction_batch_status(
            session, task.request_id, "failed", now=now
        )
    delivery.status = "completed"
    delivery.last_error = None
    delivery.processed_at = now
    await session.flush()
    return "completed"
