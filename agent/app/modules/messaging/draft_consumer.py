from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import Literal
from uuid import UUID

from pydantic import ValidationError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.errors import ApiError
from app.modules.messaging.contracts import MessageEnvelope
from app.modules.messaging.delivery import (
    complete_inbox_delivery,
    lock_inbox_delivery,
    reject_inbox_delivery,
    start_inbox_delivery,
)
from app.modules.messaging.models import InboxDelivery
from app.modules.production import (
    fail_storyboard_draft_task,
    lock_task,
    mark_storyboard_draft_task_unknown,
    start_storyboard_draft_task,
)
from app.modules.storyboards import (
    StoryboardDraftInput,
    StoryboardDraftInputChanged,
    StoryboardDraftLeaseActive,
    StoryboardDraftLeaseLost,
    StoryboardDraftProviderError,
    fence_draft_run,
    prepare_draft_input,
    record_draft_error,
    record_draft_result,
)

IO_STORYBOARD_DRAFT_CONSUMER = "lanverse.io.storyboard-draft.v1"
STORYBOARD_DRAFT_LEASE_DURATION = timedelta(minutes=5)
ConsumerResult = Literal["completed", "duplicate", "rejected"]


@dataclass(frozen=True, slots=True)
class PreparedStoryboardDraft:
    delivery_id: UUID
    draft_input: StoryboardDraftInput
    run_token: UUID
    lease_expires_at: datetime


PreparationResult = ConsumerResult | Literal["lease_active"] | PreparedStoryboardDraft


async def _complete_input_changed(
    session: AsyncSession,
    delivery: InboxDelivery,
    *,
    task_id: UUID,
    batch_id: UUID,
    trace_id: str,
    now: datetime,
) -> ConsumerResult:
    await fail_storyboard_draft_task(
        session,
        task_id,
        error_code="input_version_changed",
        error_summary="Storyboard draft input changed before generation",
        next_action="create_new_storyboard_draft_batch",
        now=now,
        trace_id=trace_id,
    )
    await record_draft_error(
        session,
        batch_id=batch_id,
        task_id=task_id,
        error_code="input_version_changed",
    )
    complete_inbox_delivery(delivery, now=now, last_error="input_version_changed")
    await session.flush()
    return "completed"


async def _resume_interrupted_draft(
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
    if task.task_type != "storyboard_draft" or task.request_type != "storyboard_draft_batch":
        return reject_inbox_delivery(delivery, error_code="unsupported_task_type", now=now)
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    if task.status != "running":
        return reject_inbox_delivery(delivery, error_code="invalid_task_state", now=now)
    if not configured:
        await fail_storyboard_draft_task(
            session,
            task.id,
            error_code="ai_service_unavailable",
            error_summary="AI storyboard draft service is not configured",
            next_action="configure_ai_service",
            now=now,
            trace_id=delivery.trace_id,
        )
        await record_draft_error(
            session,
            batch_id=task.request_id,
            task_id=task.id,
            error_code="ai_service_unavailable",
        )
        complete_inbox_delivery(delivery, now=now, last_error="ai_service_unavailable")
        await session.flush()
        return "completed"
    run_token = uuid7()
    lease_expires_at = now + STORYBOARD_DRAFT_LEASE_DURATION
    try:
        draft_input = await prepare_draft_input(
            session,
            batch_id=task.request_id,
            task_id=task.id,
            run_token=run_token,
            lease_expires_at=lease_expires_at,
            now=now,
        )
    except StoryboardDraftLeaseActive:
        return "lease_active"
    except StoryboardDraftInputChanged:
        return await _complete_input_changed(
            session,
            delivery,
            task_id=task.id,
            batch_id=task.request_id,
            trace_id=delivery.trace_id,
            now=now,
        )
    await session.flush()
    return PreparedStoryboardDraft(
        delivery_id=delivery.id,
        draft_input=draft_input,
        run_token=run_token,
        lease_expires_at=lease_expires_at,
    )


async def prepare_storyboard_draft(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    configured: bool,
) -> PreparationResult:
    now = datetime.now(UTC)
    delivery, is_first = await start_inbox_delivery(
        session,
        envelope,
        IO_STORYBOARD_DRAFT_CONSUMER,
        now,
    )
    if not is_first:
        if delivery.status == "processing":
            return await _resume_interrupted_draft(
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
    if envelope.event_type != "storyboard_draft.requested":
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_message_type",
            now=now,
        )
    if envelope.schema_version != 1:
        await fail_storyboard_draft_task(
            session,
            task.id,
            error_code="unsupported_message_schema",
            error_summary="Message schema is not supported",
            next_action="contact_support",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_draft_error(
            session,
            batch_id=task.request_id,
            task_id=task.id,
            error_code="unsupported_message_schema",
        )
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_message_schema",
            now=now,
        )
    if envelope.payload != {"task_id": str(envelope.aggregate_id)}:
        await fail_storyboard_draft_task(
            session,
            task.id,
            error_code="invalid_message_payload",
            error_summary="Message payload does not match the task",
            next_action="contact_support",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_draft_error(
            session,
            batch_id=task.request_id,
            task_id=task.id,
            error_code="invalid_message_payload",
        )
        return reject_inbox_delivery(delivery, error_code="invalid_message_payload", now=now)
    if task.task_type != "storyboard_draft" or task.request_type != "storyboard_draft_batch":
        return reject_inbox_delivery(delivery, error_code="unsupported_task_type", now=now)
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    if not configured:
        await fail_storyboard_draft_task(
            session,
            task.id,
            error_code="ai_service_unavailable",
            error_summary="AI storyboard draft service is not configured",
            next_action="configure_ai_service",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_draft_error(
            session,
            batch_id=task.request_id,
            task_id=task.id,
            error_code="ai_service_unavailable",
        )
        complete_inbox_delivery(delivery, now=now, last_error="ai_service_unavailable")
        await session.flush()
        return "completed"
    changed = await start_storyboard_draft_task(
        session,
        task.id,
        now=now,
        trace_id=envelope.trace_id,
    )
    if not changed:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    run_token = uuid7()
    lease_expires_at = now + STORYBOARD_DRAFT_LEASE_DURATION
    try:
        draft_input = await prepare_draft_input(
            session,
            batch_id=task.request_id,
            task_id=task.id,
            run_token=run_token,
            lease_expires_at=lease_expires_at,
            now=now,
        )
    except StoryboardDraftLeaseActive:
        return "lease_active"
    except StoryboardDraftInputChanged:
        return await _complete_input_changed(
            session,
            delivery,
            task_id=task.id,
            batch_id=task.request_id,
            trace_id=envelope.trace_id,
            now=now,
        )
    await session.flush()
    return PreparedStoryboardDraft(
        delivery_id=delivery.id,
        draft_input=draft_input,
        run_token=run_token,
        lease_expires_at=lease_expires_at,
    )


async def finalize_storyboard_draft_success(
    session: AsyncSession,
    prepared: PreparedStoryboardDraft,
    result: dict[str, object],
) -> ConsumerResult:
    delivery = await lock_inbox_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    if not await fence_draft_run(
        session,
        batch_id=prepared.draft_input.batch_id,
        task_id=prepared.draft_input.task_id,
        run_token=prepared.run_token,
    ):
        raise StoryboardDraftLeaseLost("Storyboard draft result no longer owns an active lease")
    now = datetime.now(UTC)
    try:
        await record_draft_result(
            session,
            batch_id=prepared.draft_input.batch_id,
            result=result,
        )
    except (ApiError, ValidationError, AttributeError):
        await fail_storyboard_draft_task(
            session,
            prepared.draft_input.task_id,
            error_code="ai_output_invalid",
            error_summary="AI storyboard draft result could not be accepted",
            next_action="create_new_storyboard_draft_batch",
            now=now,
            trace_id=delivery.trace_id,
        )
        await record_draft_error(
            session,
            batch_id=prepared.draft_input.batch_id,
            task_id=prepared.draft_input.task_id,
            error_code="ai_output_invalid",
        )
        complete_inbox_delivery(delivery, now=now, last_error="ai_output_invalid")
        await session.flush()
        return "completed"
    complete_inbox_delivery(delivery, now=now)
    await session.flush()
    return "completed"


async def finalize_storyboard_draft_failure(
    session: AsyncSession,
    prepared: PreparedStoryboardDraft,
    error: StoryboardDraftProviderError,
) -> ConsumerResult:
    delivery = await lock_inbox_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    if not await fence_draft_run(
        session,
        batch_id=prepared.draft_input.batch_id,
        task_id=prepared.draft_input.task_id,
        run_token=prepared.run_token,
    ):
        raise StoryboardDraftLeaseLost("Storyboard draft failure no longer owns an active lease")
    now = datetime.now(UTC)
    unknown = error.outcome == "unknown"
    if unknown:
        await mark_storyboard_draft_task_unknown(
            session,
            prepared.draft_input.task_id,
            now=now,
            trace_id=delivery.trace_id,
        )
    else:
        await fail_storyboard_draft_task(
            session,
            prepared.draft_input.task_id,
            error_code=error.code,
            error_summary=error.summary,
            next_action=error.next_action,
            retryable=error.retryable,
            now=now,
            trace_id=delivery.trace_id,
        )
    await record_draft_error(
        session,
        batch_id=prepared.draft_input.batch_id,
        task_id=prepared.draft_input.task_id,
        error_code=error.code,
        unknown=unknown,
    )
    complete_inbox_delivery(delivery, now=now, last_error=error.code)
    await session.flush()
    return "completed"
