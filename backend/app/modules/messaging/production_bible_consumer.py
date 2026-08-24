from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import Literal
from uuid import UUID

from pydantic import ValidationError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.messaging.contracts import MessageEnvelope
from app.modules.messaging.delivery import (
    complete_inbox_delivery,
    lock_inbox_delivery,
    reject_inbox_delivery,
    start_inbox_delivery,
)
from app.modules.messaging.models import InboxDelivery
from app.modules.production import (
    complete_production_bible_task,
    fail_production_bible_task,
    lock_task,
    mark_production_bible_task_unknown,
    start_production_bible_task,
)
from app.modules.scripts import (
    ProductionBibleInput,
    ProductionBibleInputChanged,
    ProductionBibleLeaseActive,
    ProductionBibleLeaseLost,
    ProductionBibleProviderError,
    ProductionBibleProviderResult,
    fence_bible_run,
    prepare_bible_input,
    record_bible_error,
    record_bible_result,
)

IO_PRODUCTION_BIBLE_CONSUMER = "lanverse.io.production-bible.v1"
PRODUCTION_BIBLE_LEASE_DURATION = timedelta(minutes=5)
ConsumerResult = Literal["completed", "duplicate", "rejected"]


@dataclass(frozen=True, slots=True)
class PreparedProductionBible:
    delivery_id: UUID
    bible_input: ProductionBibleInput
    run_token: UUID
    lease_expires_at: datetime


PreparationResult = ConsumerResult | Literal["lease_active"] | PreparedProductionBible


async def _complete_input_changed(
    session: AsyncSession,
    delivery: InboxDelivery,
    *,
    task_id: UUID,
    bible_id: UUID,
    now: datetime,
) -> ConsumerResult:
    await fail_production_bible_task(
        session,
        task_id,
        error_code="input_version_changed",
        error_summary="Production Bible input changed before analysis",
        next_action="start_new_production_bible",
        now=now,
        trace_id=delivery.trace_id,
    )
    await record_bible_error(
        session,
        bible_id=bible_id,
        task_id=task_id,
        error_code="input_version_changed",
        unknown=False,
    )
    complete_inbox_delivery(delivery, now=now, last_error="input_version_changed")
    await session.flush()
    return "completed"


async def _claim_bible(
    session: AsyncSession,
    delivery: InboxDelivery,
    *,
    task_id: UUID,
    bible_id: UUID,
    now: datetime,
) -> PreparationResult:
    run_token = uuid7()
    lease_expires_at = now + PRODUCTION_BIBLE_LEASE_DURATION
    try:
        bible_input = await prepare_bible_input(
            session,
            bible_id=bible_id,
            task_id=task_id,
            run_token=run_token,
            lease_expires_at=lease_expires_at,
            now=now,
        )
    except ProductionBibleLeaseActive:
        return "lease_active"
    except ProductionBibleInputChanged:
        return await _complete_input_changed(
            session,
            delivery,
            task_id=task_id,
            bible_id=bible_id,
            now=now,
        )
    await session.flush()
    return PreparedProductionBible(
        delivery_id=delivery.id,
        bible_input=bible_input,
        run_token=run_token,
        lease_expires_at=lease_expires_at,
    )


async def _resume_processing(
    session: AsyncSession,
    delivery: InboxDelivery,
    *,
    configured: bool,
    now: datetime,
) -> PreparationResult:
    if delivery.task_id is None:
        return reject_inbox_delivery(delivery, error_code="task_not_found", now=now)
    task = await lock_task(session, delivery.task_id)
    if task is None:
        return reject_inbox_delivery(delivery, error_code="task_not_found", now=now)
    if task.task_type != "production_bible" or task.request_type != "production_bible":
        return reject_inbox_delivery(delivery, error_code="unsupported_task_type", now=now)
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    if task.status != "running":
        return reject_inbox_delivery(delivery, error_code="invalid_task_state", now=now)
    if not configured:
        return await _fail_unavailable(session, delivery, task.id, task.request_id, now=now)
    return await _claim_bible(
        session,
        delivery,
        task_id=task.id,
        bible_id=task.request_id,
        now=now,
    )


async def _fail_unavailable(
    session: AsyncSession,
    delivery: InboxDelivery,
    task_id: UUID,
    bible_id: UUID,
    *,
    now: datetime,
) -> ConsumerResult:
    await fail_production_bible_task(
        session,
        task_id,
        error_code="ai_service_unavailable",
        error_summary="Local Codex Production Bible service is unavailable",
        next_action="configure_ai_service",
        now=now,
        trace_id=delivery.trace_id,
    )
    await record_bible_error(
        session,
        bible_id=bible_id,
        task_id=task_id,
        error_code="ai_service_unavailable",
        unknown=False,
    )
    complete_inbox_delivery(delivery, now=now, last_error="ai_service_unavailable")
    await session.flush()
    return "completed"


async def prepare_production_bible(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    configured: bool,
) -> PreparationResult:
    now = datetime.now(UTC)
    delivery, is_first = await start_inbox_delivery(
        session,
        envelope,
        IO_PRODUCTION_BIBLE_CONSUMER,
        now,
    )
    if not is_first:
        if delivery.status == "processing":
            return await _resume_processing(
                session,
                delivery,
                configured=configured,
                now=now,
            )
        return "duplicate"
    task = await lock_task(session, envelope.aggregate_id)
    if task is None:
        return reject_inbox_delivery(delivery, error_code="task_not_found", now=now)
    delivery.task_id = task.id
    if task.workspace_id != envelope.workspace_id:
        return reject_inbox_delivery(delivery, error_code="workspace_mismatch", now=now)
    if envelope.event_type != "production_bible.requested":
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_message_type",
            now=now,
        )
    if envelope.schema_version != 1:
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_message_schema",
            now=now,
        )
    if envelope.payload != {"task_id": str(envelope.aggregate_id)}:
        return reject_inbox_delivery(
            delivery,
            error_code="invalid_message_payload",
            now=now,
        )
    if task.task_type != "production_bible" or task.request_type != "production_bible":
        return reject_inbox_delivery(delivery, error_code="unsupported_task_type", now=now)
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    if not configured:
        return await _fail_unavailable(session, delivery, task.id, task.request_id, now=now)
    changed = await start_production_bible_task(
        session,
        task.id,
        now=now,
        trace_id=envelope.trace_id,
    )
    if not changed:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    return await _claim_bible(
        session,
        delivery,
        task_id=task.id,
        bible_id=task.request_id,
        now=now,
    )


async def finalize_production_bible_success(
    session: AsyncSession,
    prepared: PreparedProductionBible,
    result: ProductionBibleProviderResult | dict[str, object],
) -> ConsumerResult:
    delivery = await lock_inbox_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    if not await fence_bible_run(
        session,
        bible_id=prepared.bible_input.bible_id,
        task_id=prepared.bible_input.task_id,
        run_token=prepared.run_token,
    ):
        raise ProductionBibleLeaseLost(
            "Production Bible result no longer owns an active lease"
        )
    now = datetime.now(UTC)
    try:
        await record_bible_result(
            session,
            bible_id=prepared.bible_input.bible_id,
            task_id=prepared.bible_input.task_id,
            run_token=prepared.run_token,
            result=result,
        )
    except (ValidationError, ValueError, ProductionBibleInputChanged):
        await fail_production_bible_task(
            session,
            prepared.bible_input.task_id,
            error_code="ai_output_invalid",
            error_summary="Production Bible result could not be accepted",
            next_action="start_new_production_bible",
            now=now,
            trace_id=delivery.trace_id,
        )
        await record_bible_error(
            session,
            bible_id=prepared.bible_input.bible_id,
            task_id=prepared.bible_input.task_id,
            error_code="ai_output_invalid",
            unknown=False,
        )
        complete_inbox_delivery(delivery, now=now, last_error="ai_output_invalid")
        await session.flush()
        return "completed"
    await complete_production_bible_task(
        session,
        prepared.bible_input.task_id,
        now=now,
        trace_id=delivery.trace_id,
    )
    complete_inbox_delivery(delivery, now=now)
    await session.flush()
    return "completed"


async def finalize_production_bible_failure(
    session: AsyncSession,
    prepared: PreparedProductionBible,
    error: ProductionBibleProviderError,
) -> ConsumerResult:
    delivery = await lock_inbox_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    if not await fence_bible_run(
        session,
        bible_id=prepared.bible_input.bible_id,
        task_id=prepared.bible_input.task_id,
        run_token=prepared.run_token,
    ):
        raise ProductionBibleLeaseLost(
            "Production Bible failure no longer owns an active lease"
        )
    now = datetime.now(UTC)
    unknown = error.outcome == "unknown"
    if unknown:
        await mark_production_bible_task_unknown(
            session,
            prepared.bible_input.task_id,
            now=now,
            trace_id=delivery.trace_id,
            error_code=error.code,
            error_summary=error.summary,
            retryable=error.retryable,
            next_action=error.next_action,
        )
    else:
        await fail_production_bible_task(
            session,
            prepared.bible_input.task_id,
            error_code=error.code,
            error_summary=error.summary,
            next_action=error.next_action,
            retryable=error.retryable,
            now=now,
            trace_id=delivery.trace_id,
        )
    await record_bible_error(
        session,
        bible_id=prepared.bible_input.bible_id,
        task_id=prepared.bible_input.task_id,
        error_code=error.code,
        unknown=unknown,
    )
    complete_inbox_delivery(delivery, now=now, last_error=error.code)
    await session.flush()
    return "completed"
