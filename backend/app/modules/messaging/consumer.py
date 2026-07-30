from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.messaging.contracts import MessageEnvelope
from app.modules.messaging.models import InboxDelivery
from app.modules.production import (
    fail_script_extraction_task,
    lock_task,
    mark_script_extraction_task_unknown,
    start_script_extraction_task,
)
from app.modules.scripts import (
    ScriptExtractionInput,
    ScriptExtractionProviderError,
    ScriptExtractionResult,
    get_script_extraction_input,
    record_extraction_result,
    synchronize_extraction_batch_status,
)

IO_SCRIPT_EXTRACTION_CONSUMER = "lanverse.io.script-extraction.v1"
ConsumerResult = Literal["completed", "duplicate", "rejected"]


@dataclass(frozen=True, slots=True)
class PreparedScriptExtraction:
    delivery_id: UUID
    extraction_input: ScriptExtractionInput


PreparationResult = ConsumerResult | PreparedScriptExtraction


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


def _complete_delivery(
    delivery: InboxDelivery,
    *,
    now: datetime,
    last_error: str | None = None,
) -> None:
    delivery.status = "completed"
    delivery.last_error = last_error
    delivery.processed_at = now


async def _lock_delivery(
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


async def _mark_interrupted_delivery_unknown(
    session: AsyncSession,
    delivery: InboxDelivery,
    *,
    now: datetime,
) -> ConsumerResult:
    if delivery.task_id is None:
        return _reject(delivery, error_code="task_not_found", now=now)
    task = await lock_task(session, delivery.task_id)
    if task is None:
        return _reject(delivery, error_code="task_not_found", now=now)
    changed = await mark_script_extraction_task_unknown(
        session,
        task.id,
        now=now,
    )
    if changed:
        await synchronize_extraction_batch_status(
            session,
            task.request_id,
            "unknown",
            now=now,
        )
    _complete_delivery(delivery, now=now, last_error="ai_result_unknown")
    await session.flush()
    return "completed"


async def _prepare_envelope(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    consumer_name: str,
    extraction_configured: bool,
) -> PreparationResult:
    now = datetime.now(UTC)
    delivery, is_first_delivery = await _start_delivery(
        session, envelope, consumer_name, now
    )
    if not is_first_delivery:
        if delivery.status == "processing":
            return await _mark_interrupted_delivery_unknown(
                session,
                delivery,
                now=now,
            )
        return "duplicate"

    task = await lock_task(session, envelope.aggregate_id)
    if task is None:
        return _reject(delivery, error_code="task_not_found", now=now)
    if task.workspace_id != envelope.workspace_id:
        return _reject(delivery, error_code="workspace_mismatch", now=now)
    delivery.task_id = task.id

    if envelope.event_type != "script_extraction.requested":
        return _reject(delivery, error_code="unsupported_message_type", now=now)
    if envelope.schema_version != 1:
        changed = await fail_script_extraction_task(
            session,
            task.id,
            error_code="unsupported_message_schema",
            error_summary="Message schema is not supported",
            next_action="contact_support",
            now=now,
        )
        if changed:
            await synchronize_extraction_batch_status(
                session, task.request_id, "failed", now=now
            )
        return _reject(
            delivery,
            error_code="unsupported_message_schema",
            now=now,
        )
    if envelope.payload != {"task_id": str(envelope.aggregate_id)}:
        changed = await fail_script_extraction_task(
            session,
            task.id,
            error_code="invalid_message_payload",
            error_summary="Message payload does not match the task",
            next_action="contact_support",
            now=now,
        )
        if changed:
            await synchronize_extraction_batch_status(
                session, task.request_id, "failed", now=now
            )
        return _reject(delivery, error_code="invalid_message_payload", now=now)
    if task.task_type != "script_extraction" or task.request_type != "extraction_batch":
        return _reject(delivery, error_code="unsupported_task_type", now=now)
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        _complete_delivery(delivery, now=now)
        await session.flush()
        return "completed"

    if not extraction_configured:
        changed = await fail_script_extraction_task(
            session,
            task.id,
            error_code="ai_service_unavailable",
            error_summary="AI extraction service is not configured",
            next_action="configure_ai_service",
            now=now,
        )
        if changed:
            await synchronize_extraction_batch_status(
                session, task.request_id, "failed", now=now
            )
        _complete_delivery(delivery, now=now)
        await session.flush()
        return "completed"

    changed = await start_script_extraction_task(session, task.id, now=now)
    if not changed:
        _complete_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    await synchronize_extraction_batch_status(
        session,
        task.request_id,
        "running",
        now=now,
    )
    extraction_input = await get_script_extraction_input(
        session,
        task.request_id,
        task.id,
    )
    await session.flush()
    return PreparedScriptExtraction(
        delivery_id=delivery.id,
        extraction_input=extraction_input,
    )


async def consume_envelope(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    consumer_name: str,
) -> ConsumerResult:
    result = await _prepare_envelope(
        session,
        envelope,
        consumer_name=consumer_name,
        extraction_configured=False,
    )
    if isinstance(result, PreparedScriptExtraction):
        raise RuntimeError("unconfigured consumer prepared an extraction")
    return result


async def prepare_configured_extraction(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    consumer_name: str,
) -> PreparationResult:
    return await _prepare_envelope(
        session,
        envelope,
        consumer_name=consumer_name,
        extraction_configured=True,
    )


async def finalize_extraction_success(
    session: AsyncSession,
    prepared: PreparedScriptExtraction,
    result: ScriptExtractionResult,
) -> ConsumerResult:
    delivery = await _lock_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    await record_extraction_result(
        session,
        prepared.extraction_input.batch_id,
        result,
    )
    _complete_delivery(delivery, now=datetime.now(UTC))
    await session.flush()
    return "completed"


async def finalize_extraction_failure(
    session: AsyncSession,
    prepared: PreparedScriptExtraction,
    error: ScriptExtractionProviderError,
) -> ConsumerResult:
    delivery = await _lock_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    now = datetime.now(UTC)
    if error.outcome == "unknown":
        changed = await mark_script_extraction_task_unknown(
            session,
            prepared.extraction_input.task_id,
            now=now,
        )
        status: Literal["failed", "unknown"] = "unknown"
    else:
        changed = await fail_script_extraction_task(
            session,
            prepared.extraction_input.task_id,
            error_code=error.code,
            error_summary=error.summary,
            next_action=error.next_action,
            retryable=error.retryable,
            now=now,
        )
        status = "failed"
    if changed:
        await synchronize_extraction_batch_status(
            session,
            prepared.extraction_input.batch_id,
            status,
            now=now,
        )
    _complete_delivery(delivery, now=now, last_error=error.code)
    await session.flush()
    return "completed"
