import hashlib
from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.governance.audit import append_audit_event
from app.modules.production import generation_repository as repository
from app.modules.production.contracts import (
    GenerationProtocolErrorCode,
    PreparedGenerationAttempt,
)
from app.modules.production.models import (
    CostEntry,
    GenerationAttempt,
    GenerationRequest,
    Reservation,
    Task,
)

GENERATION_TASK_TYPES = frozenset({"image_generation", "video_generation"})
TERMINAL_TASK_STATUSES = frozenset(
    {"succeeded", "failed", "cancelled", "unknown"}
)
PROVIDER_UNAVAILABLE_CODE = "provider_dispatch_unavailable"
GENERATION_PROTOCOL_ERROR_SUMMARIES: dict[GenerationProtocolErrorCode, str] = {
    "unsupported_message_schema": "Generation message schema is not supported",
    "invalid_message_payload": "Generation message payload does not match the task",
}


def generation_provider_request_key(
    *,
    workspace_id: UUID,
    task_id: UUID,
    sequence: int,
    input_hash: str,
) -> str:
    if sequence < 1:
        raise ValueError("attempt sequence must be positive")
    source = (
        f"generation-attempt-v1:{workspace_id}:{task_id}:{sequence}:{input_hash}"
    )
    return hashlib.sha256(source.encode("utf-8")).hexdigest()


async def prepare_generation_attempt(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    task_id: UUID,
    trace_id: str,
    now: datetime | None = None,
) -> PreparedGenerationAttempt:
    prepared_at = now or datetime.now(UTC)
    task = await repository.find_generation_task_for_execution(session, task_id)
    if task is None or task.workspace_id != workspace_id:
        raise RuntimeError("generation task is unavailable")
    if (
        task.task_type not in GENERATION_TASK_TYPES
        or task.request_type != "generation_request"
    ):
        raise RuntimeError("task is not a generation task")

    existing = await repository.find_latest_generation_attempt(
        session,
        workspace_id,
        task.id,
        for_update=True,
    )
    if task.status == "running" and existing is not None and existing.status == "prepared":
        return PreparedGenerationAttempt(task_id=task.id, attempt_id=existing.id)
    if task.status != "queued":
        raise RuntimeError("generation task is not ready for attempt preparation")
    if existing is not None:
        raise RuntimeError("queued generation task already has an attempt")

    request, reservation = await _generation_execution_facts(session, task)
    capability = await repository.find_capability(session, request.capability_id)
    if (
        capability is None
        or capability.config_version != request.capability_config_version
        or capability.kind not in {"image", "video"}
        or task.task_type != f"{capability.kind}_generation"
        or request.input_hash != task.input_hash
        or reservation.status != "active"
    ):
        raise RuntimeError("generation execution facts are inconsistent")

    attempt = GenerationAttempt(
        id=uuid7(),
        workspace_id=task.workspace_id,
        task_id=task.id,
        sequence=1,
        provider_request_key=generation_provider_request_key(
            workspace_id=task.workspace_id,
            task_id=task.id,
            sequence=1,
            input_hash=request.input_hash,
        ),
        provider_task_id=None,
        status="prepared",
        request_snapshot_hash=request.input_hash,
        error_code=None,
        reconcile_summary=None,
        prepared_at=prepared_at,
        submitted_at=None,
        completed_at=None,
        updated_at=prepared_at,
    )
    session.add(attempt)
    previous_status = task.status
    task.status = "running"
    task.progress_stage = "validating"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "poll_task"
    task.revision += 1
    task.updated_at = prepared_at
    _append_task_transition_audit(
        session,
        task,
        action="task.started",
        previous_status=previous_status,
        trace_id=trace_id,
        now=prepared_at,
    )
    append_audit_event(
        session,
        workspace_id=task.workspace_id,
        actor_id=task.requested_by,
        action="attempt.prepared",
        target_type="attempt",
        target_id=attempt.id,
        trace_id=trace_id,
        metadata={
            "task_id": str(task.id),
            "sequence": attempt.sequence,
            "status": attempt.status,
        },
        occurred_at=prepared_at,
    )
    await session.flush()
    return PreparedGenerationAttempt(task_id=task.id, attempt_id=attempt.id)


async def fail_generation_attempt_without_provider(
    session: AsyncSession,
    prepared: PreparedGenerationAttempt,
    *,
    workspace_id: UUID,
    trace_id: str,
    now: datetime | None = None,
) -> None:
    failed_at = now or datetime.now(UTC)
    task = await repository.find_generation_task_for_execution(
        session,
        prepared.task_id,
    )
    if task is None or task.workspace_id != workspace_id:
        raise RuntimeError("generation task is unavailable")
    attempt = await repository.find_generation_attempt_for_update(
        session,
        workspace_id,
        prepared.attempt_id,
    )
    if attempt is None or attempt.task_id != task.id:
        raise RuntimeError("generation attempt is unavailable")
    request, reservation = await _generation_execution_facts(session, task)

    if (
        attempt.status == "failed"
        and task.status == "failed"
        and reservation.status == "released"
    ):
        return
    if (
        attempt.status != "prepared"
        or task.status != "running"
        or reservation.status != "active"
        or request.input_hash != attempt.request_snapshot_hash
    ):
        raise RuntimeError("prepared generation facts are inconsistent")
    if await repository.find_release_cost_entry(
        session,
        workspace_id,
        reservation.id,
    ) is not None:
        raise RuntimeError("active reservation already has a release entry")

    previous_attempt_status = attempt.status
    attempt.status = "failed"
    attempt.error_code = PROVIDER_UNAVAILABLE_CODE
    attempt.completed_at = failed_at
    attempt.updated_at = failed_at

    previous_task_status = task.status
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = PROVIDER_UNAVAILABLE_CODE
    task.error_retryable = False
    task.error_summary = "Generation provider dispatch is unavailable"
    task.next_action = "wait_for_provider_activation"
    task.revision += 1
    task.updated_at = failed_at

    reservation.status = "released"
    reservation.revision += 1
    reservation.updated_at = failed_at
    session.add(
        CostEntry(
            id=uuid7(),
            workspace_id=workspace_id,
            reservation_id=reservation.id,
            attempt_id=attempt.id,
            entry_type="release",
            amount=reservation.reserved_amount,
            currency=reservation.currency,
            provider_bill_ref=None,
            idempotency_key=f"provider-unavailable:{attempt.id}",
            created_at=failed_at,
        )
    )
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_task_status,
        trace_id=trace_id,
        now=failed_at,
    )
    append_audit_event(
        session,
        workspace_id=workspace_id,
        actor_id=task.requested_by,
        action="attempt.failed",
        target_type="attempt",
        target_id=attempt.id,
        trace_id=trace_id,
        metadata={
            "task_id": str(task.id),
            "sequence": attempt.sequence,
            "previous_status": previous_attempt_status,
            "status": attempt.status,
            "error_code": PROVIDER_UNAVAILABLE_CODE,
            "external_side_effect": "none",
        },
        occurred_at=failed_at,
    )
    await session.flush()


async def fail_generation_protocol_message(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    task_id: UUID,
    event_id: UUID,
    error_code: GenerationProtocolErrorCode,
    trace_id: str,
    now: datetime | None = None,
) -> bool:
    failed_at = now or datetime.now(UTC)
    task = await repository.find_generation_task_for_execution(session, task_id)
    if task is None or task.workspace_id != workspace_id:
        raise RuntimeError("generation task is unavailable")
    if (
        task.task_type not in GENERATION_TASK_TYPES
        or task.request_type != "generation_request"
    ):
        raise RuntimeError("task is not a generation task")
    if task.status != "queued":
        return False

    _, reservation = await _generation_execution_facts(session, task)
    existing_attempt = await repository.find_latest_generation_attempt(
        session,
        workspace_id,
        task.id,
        for_update=True,
    )
    existing_release = await repository.find_release_cost_entry(
        session,
        workspace_id,
        reservation.id,
    )
    if (
        reservation.status != "active"
        or existing_attempt is not None
        or existing_release is not None
    ):
        raise RuntimeError("queued generation protocol failure facts are inconsistent")

    previous_status = task.status
    task.status = "failed"
    task.progress_stage = "manual_attention"
    task.error_code = error_code
    task.error_retryable = False
    task.error_summary = GENERATION_PROTOCOL_ERROR_SUMMARIES[error_code]
    task.next_action = "contact_support"
    task.revision += 1
    task.updated_at = failed_at

    reservation.status = "released"
    reservation.revision += 1
    reservation.updated_at = failed_at
    session.add(
        CostEntry(
            id=uuid7(),
            workspace_id=workspace_id,
            reservation_id=reservation.id,
            attempt_id=None,
            entry_type="release",
            amount=reservation.reserved_amount,
            currency=reservation.currency,
            provider_bill_ref=None,
            idempotency_key=f"generation-protocol:{event_id}",
            created_at=failed_at,
        )
    )
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_status,
        trace_id=trace_id,
        now=failed_at,
    )
    await session.flush()
    return True


async def _generation_execution_facts(
    session: AsyncSession,
    task: Task,
) -> tuple[GenerationRequest, Reservation]:
    request = await repository.find_generation_request(
        session,
        task.workspace_id,
        task.request_id,
    )
    reservation = await repository.find_generation_reservation_for_update(
        session,
        task.workspace_id,
        task.request_id,
    )
    if request is None or reservation is None:
        raise RuntimeError("generation execution facts are incomplete")
    return request, reservation


def _append_task_transition_audit(
    session: AsyncSession,
    task: Task,
    *,
    action: Literal["task.started", "task.failed"],
    previous_status: str,
    trace_id: str,
    now: datetime,
) -> None:
    metadata: dict[str, object] = {
        "revision": task.revision,
        "task_type": task.task_type,
        "request_type": task.request_type,
        "request_id": str(task.request_id),
        "previous_status": previous_status,
        "status": task.status,
        "progress_stage": task.progress_stage,
        "next_action": task.next_action,
    }
    if task.error_code is not None:
        metadata["error_code"] = task.error_code
        metadata["retryable"] = bool(task.error_retryable)
    append_audit_event(
        session,
        workspace_id=task.workspace_id,
        actor_id=task.requested_by,
        action=action,
        target_type="task",
        target_id=task.id,
        trace_id=trace_id,
        metadata=metadata,
        occurred_at=now,
    )
