from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.messaging.contracts import MessageEnvelope
from app.modules.messaging.delivery import (
    InboxResult,
    complete_inbox_delivery,
    lock_inbox_delivery,
    mark_inbox_delivery_manual_attention,
    reject_inbox_delivery,
    start_inbox_delivery,
)
from app.modules.messaging.models import InboxDelivery
from app.modules.production import (
    GenerationProtocolErrorCode,
    PreparedGenerationAttempt,
    TaskContext,
    fail_generation_attempt_without_provider,
    fail_generation_protocol_message,
    lock_task,
    prepare_generation_attempt,
)

IO_GENERATION_CONSUMER = "lanverse.io.generation.v1"
GENERATION_TASK_TYPES = frozenset({"image_generation", "video_generation"})
TERMINAL_TASK_STATUSES = frozenset(
    {"succeeded", "failed", "cancelled", "unknown"}
)
PROVIDER_UNAVAILABLE_CODE = "provider_dispatch_unavailable"


@dataclass(frozen=True, slots=True)
class PreparedGenerationDispatch:
    delivery_id: UUID
    workspace_id: UUID
    attempt: PreparedGenerationAttempt


GenerationPreparationResult = InboxResult | PreparedGenerationDispatch


async def prepare_generation_dispatch(
    session: AsyncSession,
    envelope: MessageEnvelope,
) -> GenerationPreparationResult:
    now = datetime.now(UTC)
    delivery, is_first_delivery = await start_inbox_delivery(
        session,
        envelope,
        IO_GENERATION_CONSUMER,
        now,
    )
    if not is_first_delivery and delivery.status != "processing":
        return "duplicate"
    return await _prepare_or_resume_dispatch(
        session,
        delivery,
        envelope,
        now=now,
    )


async def finalize_generation_dispatch_unavailable(
    session: AsyncSession,
    prepared: PreparedGenerationDispatch,
) -> InboxResult:
    delivery = await lock_inbox_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    if delivery.task_id != prepared.attempt.task_id:
        raise RuntimeError("generation delivery task changed")
    now = datetime.now(UTC)
    await fail_generation_attempt_without_provider(
        session,
        prepared.attempt,
        workspace_id=prepared.workspace_id,
        trace_id=delivery.trace_id,
        now=now,
    )
    complete_inbox_delivery(
        delivery,
        now=now,
        last_error=PROVIDER_UNAVAILABLE_CODE,
    )
    await session.flush()
    return "completed"


async def _prepare_or_resume_dispatch(
    session: AsyncSession,
    delivery: InboxDelivery,
    envelope: MessageEnvelope,
    *,
    now: datetime,
) -> GenerationPreparationResult:
    task = await lock_task(session, envelope.aggregate_id)
    if task is None:
        return reject_inbox_delivery(
            delivery,
            error_code="task_not_found",
            now=now,
        )
    if task.workspace_id != envelope.workspace_id:
        return reject_inbox_delivery(
            delivery,
            error_code="workspace_mismatch",
            now=now,
        )
    delivery.task_id = task.id
    invalid = await _validate_generation_envelope(
        session,
        delivery,
        envelope,
        task,
        now=now,
    )
    if invalid is not None:
        return invalid
    if task.status in TERMINAL_TASK_STATUSES:
        complete_inbox_delivery(delivery, now=now, last_error=None)
        await session.flush()
        return "completed"
    if task.status not in {"queued", "running"}:
        return reject_inbox_delivery(
            delivery,
            error_code="generation_execution_state_conflict",
            now=now,
        )
    attempt = await prepare_generation_attempt(
        session,
        workspace_id=task.workspace_id,
        task_id=task.id,
        trace_id=delivery.trace_id,
        now=now,
    )
    return PreparedGenerationDispatch(
        delivery_id=delivery.id,
        workspace_id=task.workspace_id,
        attempt=attempt,
    )


async def _validate_generation_envelope(
    session: AsyncSession,
    delivery: InboxDelivery,
    envelope: MessageEnvelope,
    task: TaskContext,
    *,
    now: datetime,
) -> InboxResult | None:
    if envelope.event_type != "generation.requested":
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_message_type",
            now=now,
        )
    if task.task_type not in GENERATION_TASK_TYPES or task.request_type != "generation_request":
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_task_type",
            now=now,
        )
    protocol_error: GenerationProtocolErrorCode | None = None
    if envelope.schema_version != 1:
        protocol_error = "unsupported_message_schema"
    elif envelope.payload != {"task_id": str(envelope.aggregate_id)}:
        protocol_error = "invalid_message_payload"
    if protocol_error is not None:
        changed = await fail_generation_protocol_message(
            session,
            workspace_id=task.workspace_id,
            task_id=task.id,
            event_id=envelope.event_id,
            error_code=protocol_error,
            trace_id=delivery.trace_id,
            now=now,
        )
        if changed:
            return mark_inbox_delivery_manual_attention(
                delivery,
                error_code=protocol_error,
                now=now,
            )
        return reject_inbox_delivery(
            delivery,
            error_code=protocol_error,
            now=now,
        )
    return None
