import hashlib
from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import Capability, actor_context
from app.modules.production import generation_repository as repository
from app.modules.production.generation_presenters import (
    cost_entry_response,
    reservation_response,
)
from app.modules.production.generation_schemas import (
    GenerationTaskCancellationRequest,
    GenerationTaskCancellationResponse,
)
from app.modules.production.models import CostEntry, GenerationRequest, Reservation, Task
from app.modules.production.service import task_response

_GENERATION_TASK_TYPES = frozenset({"image_generation", "video_generation"})


async def cancel_queued_generation_task(
    session: AsyncSession,
    claims: AccessTokenClaims,
    task_id: UUID,
    request: GenerationTaskCancellationRequest,
    *,
    trace_id: str,
) -> GenerationTaskCancellationResponse:
    async with session.begin():
        actor = await actor_context(
            session,
            claims,
            request.workspace_id,
            Capability.GENERATION_SUBMIT,
        )
        task = await repository.find_generation_task_for_cancellation(
            session,
            request.workspace_id,
            task_id,
        )
        if task is None:
            raise ApiError(ErrorCode.NOT_FOUND, "Task not found", status_code=404)
        _require_generation_task(task)

        generation_request = await repository.find_generation_request(
            session,
            request.workspace_id,
            task.request_id,
        )
        reservation = await repository.find_generation_reservation_for_update(
            session,
            request.workspace_id,
            task.request_id,
        )
        if generation_request is None or reservation is None:
            raise RuntimeError("generation cancellation facts are incomplete")

        existing_release = await repository.find_release_cost_entry(
            session,
            request.workspace_id,
            reservation.id,
        )
        if task.status == "cancelled":
            return _replay_cancelled_task(
                task,
                generation_request,
                reservation,
                existing_release,
            )
        if task.revision != request.expected_revision:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Task revision is stale",
                status_code=409,
                next_action="reload_task",
                details={"current_revision": task.revision},
            )
        if task.status != "queued":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Generation task already started and cannot be cancelled locally",
                status_code=409,
                next_action="wait_for_provider_cancellation_support",
                details={"status": task.status},
            )
        if reservation.status != "active" or existing_release is not None:
            raise RuntimeError("queued generation reservation is inconsistent")

        now = datetime.now(UTC)
        previous_status = task.status
        task.status = "cancelled"
        task.progress_stage = "cancelled_before_dispatch"
        task.error_code = None
        task.error_retryable = None
        task.error_summary = None
        task.next_action = None
        task.cancel_status = "accepted"
        task.revision += 1
        task.updated_at = now

        reservation.status = "released"
        reservation.revision += 1
        reservation.updated_at = now
        release_entry = CostEntry(
            id=uuid7(),
            workspace_id=request.workspace_id,
            reservation_id=reservation.id,
            attempt_id=None,
            entry_type="release",
            amount=reservation.reserved_amount,
            currency=reservation.currency,
            provider_bill_ref=None,
            idempotency_key=(
                "queued-cancel:"
                + hashlib.sha256(request.idempotency_key.encode("utf-8")).hexdigest()
            ),
            created_at=now,
        )
        session.add(release_entry)
        await session.flush()
        append_audit_event(
            session,
            workspace_id=request.workspace_id,
            actor_id=actor.user_id,
            action="task.cancelled",
            target_type="task",
            target_id=task.id,
            trace_id=trace_id,
            metadata={
                "revision": task.revision,
                "task_type": task.task_type,
                "request_type": task.request_type,
                "request_id": str(task.request_id),
                "previous_status": previous_status,
                "status": task.status,
                "progress_stage": task.progress_stage,
                "next_action": task.next_action,
                "cancel_status": task.cancel_status,
                "reason": request.reason,
                "reservation_id": str(reservation.id),
                "release_cost_entry_id": str(release_entry.id),
            },
            occurred_at=now,
        )
        await session.flush()
        return _cancellation_response(
            task,
            generation_request,
            reservation,
            release_entry,
            replayed=False,
        )


def _require_generation_task(task: Task) -> None:
    if (
        task.request_type != "generation_request"
        or task.task_type not in _GENERATION_TASK_TYPES
    ):
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Only generation tasks support this cancellation command",
            status_code=409,
            next_action="use_task_specific_command",
        )


def _replay_cancelled_task(
    task: Task,
    generation_request: GenerationRequest,
    reservation: Reservation,
    release_entry: CostEntry | None,
) -> GenerationTaskCancellationResponse:
    if (
        task.cancel_status != "accepted"
        or reservation.status != "released"
        or release_entry is None
    ):
        raise RuntimeError("cancelled generation facts are incomplete")
    return _cancellation_response(
        task,
        generation_request,
        reservation,
        release_entry,
        replayed=True,
    )


def _cancellation_response(
    task: Task,
    generation_request: GenerationRequest,
    reservation: Reservation,
    release_entry: CostEntry,
    *,
    replayed: bool,
) -> GenerationTaskCancellationResponse:
    return GenerationTaskCancellationResponse(
        task=task_response(task),
        reservation=reservation_response(reservation),
        release_cost_entry=cost_entry_response(
            release_entry,
            reservation,
            generation_request,
            task,
        ),
        replayed=replayed,
    )
