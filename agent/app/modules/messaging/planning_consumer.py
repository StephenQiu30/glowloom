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
    fail_episode_planning_task,
    lock_task,
    mark_episode_planning_task_unknown,
    start_episode_planning_task,
)
from app.modules.scripts import (
    EpisodePlanningInput,
    EpisodePlanningProviderError,
    EpisodePlanningProviderResult,
    get_episode_planning_input,
    record_episode_planning_error,
    record_episode_planning_result,
)

IO_EPISODE_PLANNING_CONSUMER = "lanverse.io.episode-planning.v1"
ConsumerResult = Literal["completed", "duplicate", "rejected"]


@dataclass(frozen=True, slots=True)
class PreparedEpisodePlanning:
    delivery_id: UUID
    planning_input: EpisodePlanningInput


PreparationResult = ConsumerResult | PreparedEpisodePlanning


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
    await mark_episode_planning_task_unknown(
        session,
        task.id,
        now=now,
        trace_id=delivery.trace_id,
    )
    await record_episode_planning_error(
        session,
        task.request_id,
        error_code="ai_result_unknown",
    )
    complete_inbox_delivery(delivery, now=now, last_error="ai_result_unknown")
    await session.flush()
    return "completed"


async def prepare_episode_planning(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    configured: bool,
) -> PreparationResult:
    now = datetime.now(UTC)
    delivery, is_first = await start_inbox_delivery(
        session,
        envelope,
        IO_EPISODE_PLANNING_CONSUMER,
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
    if envelope.event_type != "episode_planning.requested":
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_message_type",
            now=now,
        )
    if envelope.schema_version != 1:
        await fail_episode_planning_task(
            session,
            task.id,
            error_code="unsupported_message_schema",
            error_summary="Message schema is not supported",
            next_action="contact_support",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_episode_planning_error(
            session,
            task.request_id,
            error_code="unsupported_message_schema",
        )
        return reject_inbox_delivery(
            delivery,
            error_code="unsupported_message_schema",
            now=now,
        )
    if envelope.payload != {"task_id": str(envelope.aggregate_id)}:
        await fail_episode_planning_task(
            session,
            task.id,
            error_code="invalid_message_payload",
            error_summary="Message payload does not match the task",
            next_action="contact_support",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_episode_planning_error(
            session,
            task.request_id,
            error_code="invalid_message_payload",
        )
        return reject_inbox_delivery(
            delivery,
            error_code="invalid_message_payload",
            now=now,
        )
    if task.task_type != "episode_planning" or task.request_type != "episode_plan":
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
        await fail_episode_planning_task(
            session,
            task.id,
            error_code="ai_service_unavailable",
            error_summary="AI episode planning service is not configured",
            next_action="configure_ai_service",
            now=now,
            trace_id=envelope.trace_id,
        )
        await record_episode_planning_error(
            session,
            task.request_id,
            error_code="ai_service_unavailable",
        )
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    changed = await start_episode_planning_task(
        session,
        task.id,
        now=now,
        trace_id=envelope.trace_id,
    )
    if not changed:
        complete_inbox_delivery(delivery, now=now)
        await session.flush()
        return "completed"
    planning_input = await get_episode_planning_input(
        session,
        task.request_id,
        task.id,
    )
    await session.flush()
    return PreparedEpisodePlanning(
        delivery_id=delivery.id,
        planning_input=planning_input,
    )


async def finalize_episode_planning_success(
    session: AsyncSession,
    prepared: PreparedEpisodePlanning,
    result: EpisodePlanningProviderResult,
) -> ConsumerResult:
    delivery = await lock_inbox_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    now = datetime.now(UTC)
    try:
        await record_episode_planning_result(
            session,
            prepared.planning_input,
            result,
            trace_id=delivery.trace_id,
        )
    except ApiError as error:
        await fail_episode_planning_task(
            session,
            prepared.planning_input.task_id,
            error_code="ai_output_invalid",
            error_summary=error.message,
            next_action="start_new_episode_plan",
            retryable=False,
            now=now,
            trace_id=delivery.trace_id,
        )
        await record_episode_planning_error(
            session,
            prepared.planning_input.plan_id,
            error_code="ai_output_invalid",
        )
        complete_inbox_delivery(delivery, now=now, last_error="ai_output_invalid")
        await session.flush()
        return "completed"
    complete_inbox_delivery(delivery, now=now)
    await session.flush()
    return "completed"


async def finalize_episode_planning_failure(
    session: AsyncSession,
    prepared: PreparedEpisodePlanning,
    error: EpisodePlanningProviderError,
) -> ConsumerResult:
    delivery = await lock_inbox_delivery(session, prepared.delivery_id)
    if delivery.status != "processing":
        return "duplicate"
    now = datetime.now(UTC)
    if error.outcome == "unknown":
        await mark_episode_planning_task_unknown(
            session,
            prepared.planning_input.task_id,
            now=now,
            trace_id=delivery.trace_id,
        )
    else:
        await fail_episode_planning_task(
            session,
            prepared.planning_input.task_id,
            error_code=error.code,
            error_summary=error.summary,
            next_action=error.next_action,
            retryable=error.retryable,
            now=now,
            trace_id=delivery.trace_id,
        )
    await record_episode_planning_error(
        session,
        prepared.planning_input.plan_id,
        error_code=error.code,
    )
    complete_inbox_delivery(delivery, now=now, last_error=error.code)
    await session.flush()
    return "completed"
