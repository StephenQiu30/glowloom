from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import service as identity_service
from app.modules.identity.policy import (
    ActorContext,
    Capability,
    require_workspace_capability,
)
from app.modules.messaging.models import OutboxEvent
from app.modules.production import repository
from app.modules.production.models import Task
from app.modules.production.schemas import (
    PaginatedTasks,
    ScriptExtractionTaskCommand,
    TaskErrorResponse,
    TaskResponse,
    TaskScopeResponse,
    TaskStatus,
)


def _response(task: Task) -> TaskResponse:
    error = (
        TaskErrorResponse(
            code=task.error_code,
            retryable=bool(task.error_retryable),
            summary=task.error_summary or "",
        )
        if task.error_code is not None
        else None
    )
    return TaskResponse(
        id=task.id,
        workspace_id=task.workspace_id,
        task_type=cast(Literal["script_extraction"], task.task_type),
        request_type=cast(Literal["extraction_batch"], task.request_type),
        request_id=task.request_id,
        scope=TaskScopeResponse(
            episode_id=task.episode_id,
            render_snapshot_id=task.render_snapshot_id,
            usage_type=task.usage_type,
            usage_id=task.usage_id,
            input_version_id=task.input_version_id,
            input_hash=task.input_hash,
        ),
        status=cast(TaskStatus, task.status),
        progress_stage=task.progress_stage,
        error=error,
        next_action=task.next_action,
        cancel_status=cast(
            Literal["none", "requested", "accepted", "rejected"],
            task.cancel_status,
        ),
        revision=task.revision,
    )


def _same_command(task: Task, command: ScriptExtractionTaskCommand) -> bool:
    return (
        task.request_id == command.request_id
        and task.episode_id == command.episode_id
        and task.input_version_id == command.input_version_id
        and task.input_hash == command.input_hash
    )


async def create_script_extraction_task(
    session: AsyncSession,
    actor: ActorContext,
    command: ScriptExtractionTaskCommand,
    *,
    trace_id: str,
) -> Task:
    if actor.workspace_id != command.workspace_id:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403)
    try:
        require_workspace_capability(
            actor.role, actor.workspace_status, Capability.CONTENT_WRITE
        )
    except PermissionError as error:
        raise ApiError(
            ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403
        ) from error
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)

    task_id = uuid7()
    now = datetime.now(UTC)
    inserted_id = await session.scalar(
        insert(Task)
        .values(
            id=task_id,
            workspace_id=command.workspace_id,
            task_type="script_extraction",
            request_type="extraction_batch",
            request_id=command.request_id,
            episode_id=command.episode_id,
            input_version_id=command.input_version_id,
            input_hash=command.input_hash,
            status="queued",
            progress_stage="queued",
            next_action="poll_task",
            cancel_status="none",
            idempotency_key=command.idempotency_key,
            requested_by=actor.user_id,
            revision=1,
            created_at=now,
            updated_at=now,
        )
        .on_conflict_do_nothing(constraint="uq_prod_task_idempotency")
        .returning(Task.id)
    )
    if inserted_id is None:
        existing = await repository.find_idempotent_task(
            session,
            command.workspace_id,
            "script_extraction",
            command.idempotency_key,
        )
        if existing is None:
            raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
        if not _same_command(existing, command):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        return existing

    session.add(
        OutboxEvent(
            id=uuid7(),
            workspace_id=command.workspace_id,
            event_type="script_extraction.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=inserted_id,
            routing_key="io.script.extract",
            payload={"task_id": str(inserted_id)},
            trace_id=trace_id,
            status="pending",
            attempt_count=0,
            available_at=now,
            occurred_at=now,
            created_at=now,
        )
    )
    await session.flush()
    task = await repository.find_task(session, inserted_id)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    return task


async def get_task(
    session: AsyncSession,
    claims: AccessTokenClaims,
    task_id: UUID,
) -> TaskResponse:
    task = await repository.find_task(session, task_id)
    if task is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Task not found", status_code=404)
    try:
        await identity_service.actor_context(
            session, claims, task.workspace_id, Capability.CONTENT_READ
        )
    except ApiError as error:
        if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
            raise ApiError(ErrorCode.NOT_FOUND, "Task not found", status_code=404) from error
        raise
    return _response(task)


async def list_tasks(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    *,
    task_type: Literal["script_extraction"] | None,
    status: TaskStatus | None,
    limit: int,
    offset: int,
) -> PaginatedTasks:
    await identity_service.actor_context(
        session, claims, workspace_id, Capability.CONTENT_READ
    )
    tasks, total = await repository.list_tasks(
        session,
        workspace_id,
        task_type=task_type,
        status=status,
        limit=limit,
        offset=offset,
    )
    return PaginatedTasks(
        items=[_response(task) for task in tasks],
        total=total,
        limit=limit,
        offset=offset,
    )
