from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

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
    complete_script_adaptation_task,
    fail_script_adaptation_task,
    lock_task,
    mark_script_adaptation_task_unknown,
    start_script_adaptation_task,
)
from app.modules.scripts import (
    AdaptationInput,
    AdaptationInputChanged,
    ScriptAdaptationProviderError,
    ScriptAdaptationProviderResult,
    prepare_adaptation_input,
    record_adaptation_error,
    record_adaptation_result,
)

IO_SCRIPT_ADAPTATION_CONSUMER = "lanverse.io.script-adaptation.v1"
ConsumerResult = Literal["completed", "duplicate", "rejected"]


@dataclass(frozen=True, slots=True)
class PreparedScriptAdaptation:
    delivery_id: UUID
    adaptation_input: AdaptationInput


PreparationResult = ConsumerResult | PreparedScriptAdaptation


async def _mark_interrupted_unknown(
    session: AsyncSession,
    delivery: InboxDelivery,
    *,
    now: datetime,
) -> ConsumerResult:
    if delivery.task_id is None:
        return reject_inbox_delivery(delivery, error_code="task_not_found", now=now)
    task = await lock_task(session, delivery.task_id)
    if task is None:
        return reject_inbox_delivery(delivery, error_code="task_not_found", now=now)
    await mark_script_adaptation_task_unknown(
        session,
        task.id,
        now=now,
        trace_id=delivery.trace_id,
    )
    await record_adaptation_error(
        session,
        run_id=task.request_id,
        task_id=task.id,
        error_code="ai_result_unknown",
        unknown=True,
    )
    complete_inbox_delivery(delivery, now=now, last_error="ai_result_unknown")
    await session.flush()
    return "completed"


async def prepare_script_adaptation(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    configured: bool,
) -> PreparationResult:
    now = datetime.now(UTC)
    delivery, is_first = await start_inbox_delivery(
        session,
        envelope,
        IO_SCRIPT_ADAPTATION_CONSUMER,
        now,
    )
    if not is_first:
        if delivery.status == "processing":
            return await _mark_interrupted_unknown(session, delivery, now=now)
        return "duplicate"
    task = await lock_task(session, envelope.aggregate_id)
    if task is None:
        return reject_inbox_delivery(delivery, error_code="task_not_found", now=now)
    delivery.task_id = task.id
    if task.workspace_id != envelope.workspace_id:
        return reject_inbox_delivery(delivery, error_code="workspace_mismatch", now=now)
    if envelope.event_type != "script_adaptation.requested":
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_message_type",
            now=now,
        )
    if envelope.schema_version != 1:
        await fail_script_adaptation_task(
            session,
            task.id,
            error_code="unsupported_message_schema",
            error_summary="Message schema is not supported",
            next_action="contact_support",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_adaptation_error(
            session,
            run_id=task.request_id,
            task_id=task.id,
            error_code="unsupported_message_schema",
        )
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_message_schema",
            now=now,
        )
    if envelope.payload != {"task_id": str(envelope.aggregate_id)}:
        await fail_script_adaptation_task(
            session,
            task.id,
            error_code="invalid_message_payload",
            error_summary="Message payload does not match the task",
            next_action="contact_support",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_adaptation_error(
            session,
            run_id=task.request_id,
            task_id=task.id,
            error_code="invalid_message_payload",
        )
        return reject_inbox_delivery(
            delivery,
            error_code="invalid_message_payload",
            now=now,
        )
    if task.task_type != "script_adaptation" or task.request_type != "adaptation_run":
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_task_type",
            now=now,
        )
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    if not configured:
        await fail_script_adaptation_task(
            session,
            task.id,
            error_code="ai_service_unavailable",
            error_summary="AI script adaptation service is not configured",
            next_action="configure_ai_service",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_adaptation_error(
            session,
            run_id=task.request_id,
            task_id=task.id,
            error_code="ai_service_unavailable",
        )
        complete_inbox_delivery(delivery, now=now, last_error="ai_service_unavailable")
        await session.flush()
        return "completed"
    changed = await start_script_adaptation_task(
        session,
        task.id,
        now=now,
        trace_id=envelope.trace_id,
    )
    if not changed:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    try:
        adaptation_input = await prepare_adaptation_input(
            session,
            run_id=task.request_id,
            task_id=task.id,
        )
    except AdaptationInputChanged:
        await fail_script_adaptation_task(
            session,
            task.id,
            error_code="input_version_changed",
            error_summary="Current script version changed before adaptation",
            next_action="start_new_adaptation",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_adaptation_error(
            session,
            run_id=task.request_id,
            task_id=task.id,
            error_code="input_version_changed",
        )
        complete_inbox_delivery(delivery, now=now, last_error="input_version_changed")
        await session.flush()
        return "completed"
    if adaptation_input is None:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    await session.flush()
    return PreparedScriptAdaptation(
        delivery_id=delivery.id,
        adaptation_input=adaptation_input,
    )


async def finalize_script_adaptation_success(
    session: AsyncSession,
    prepared: PreparedScriptAdaptation,
    result: ScriptAdaptationProviderResult | dict[str, object],
) -> ConsumerResult:
    delivery = await lock_inbox_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    now = datetime.now(UTC)
    try:
        await record_adaptation_result(
            session,
            run_id=prepared.adaptation_input.run_id,
            task_id=prepared.adaptation_input.task_id,
            result=result,
        )
    except (ApiError, AdaptationInputChanged) as result_error:
        code = (
            "input_version_changed"
            if isinstance(result_error, AdaptationInputChanged)
            else "ai_output_invalid"
        )
        await fail_script_adaptation_task(
            session,
            prepared.adaptation_input.task_id,
            error_code=code,
            error_summary="AI adaptation result could not be accepted",
            next_action="start_new_adaptation",
            retryable=False,
            now=now,
            trace_id=delivery.trace_id,
        )
        await record_adaptation_error(
            session,
            run_id=prepared.adaptation_input.run_id,
            task_id=prepared.adaptation_input.task_id,
            error_code=code,
        )
        complete_inbox_delivery(delivery, now=now, last_error=code)
        await session.flush()
        return "completed"
    await complete_script_adaptation_task(
        session,
        prepared.adaptation_input.task_id,
        now=now,
        trace_id=delivery.trace_id,
    )
    complete_inbox_delivery(delivery, now=now)
    await session.flush()
    return "completed"


async def finalize_script_adaptation_failure(
    session: AsyncSession,
    prepared: PreparedScriptAdaptation,
    error: ScriptAdaptationProviderError,
) -> ConsumerResult:
    delivery = await lock_inbox_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    now = datetime.now(UTC)
    unknown = error.outcome == "unknown"
    if unknown:
        await mark_script_adaptation_task_unknown(
            session,
            prepared.adaptation_input.task_id,
            now=now,
            trace_id=delivery.trace_id,
        )
    else:
        await fail_script_adaptation_task(
            session,
            prepared.adaptation_input.task_id,
            error_code=error.code,
            error_summary=error.summary,
            next_action=error.next_action,
            retryable=error.retryable,
            now=now,
            trace_id=delivery.trace_id,
        )
    await record_adaptation_error(
        session,
        run_id=prepared.adaptation_input.run_id,
        task_id=prepared.adaptation_input.task_id,
        error_code=error.code,
        unknown=unknown,
    )
    complete_inbox_delivery(delivery, now=now, last_error=error.code)
    await session.flush()
    return "completed"
