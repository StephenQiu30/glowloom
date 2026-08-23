from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import (
    ActorContext,
    Capability,
    actor_context,
    require_workspace_capability,
)
from app.modules.messaging import (
    OutboxEventCommand,
    enqueue_outbox_event,
    find_outbox_event_id,
)
from app.modules.production import repository
from app.modules.production.contracts import (
    EpisodePlanningTaskCommand,
    EpisodeTaskSummary,
    MediaLocationMigrationTaskCommand,
    MediaLocationRetirementTaskCommand,
    MediaLocationTaskDispatch,
    MediaProbeTaskCommand,
    ScriptAdaptationTaskCommand,
    ScriptExtractionTaskCommand,
    StoryboardDraftTaskCommand,
    StoryboardExportTaskCommand,
    TaskContext,
    TaskErrorResponse,
    TaskRequestType,
    TaskResponse,
    TaskScopeResponse,
    TaskStatus,
    TaskType,
    UploadCleanupTaskCommand,
    UploadCleanupTaskDispatch,
    UploadExpirationTaskCommand,
    UploadExpirationTaskDispatch,
)
from app.modules.production.models import Task
from app.modules.production.schemas import PaginatedTasks


def task_response(task: Task) -> TaskResponse:
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
        task_type=cast(TaskType, task.task_type),
        request_type=cast(TaskRequestType, task.request_type),
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


async def summarize_episode_tasks(
    session: AsyncSession,
    workspace_id: UUID,
    episode_ids: list[UUID],
) -> dict[UUID, EpisodeTaskSummary]:
    counts: dict[UUID, dict[str, int]] = {episode_id: {} for episode_id in episode_ids}
    for episode_id, status, count in await repository.count_task_statuses_by_episode(
        session, workspace_id, episode_ids
    ):
        counts[episode_id][status] = count
    return {
        episode_id: EpisodeTaskSummary(
            running=sum(
                statuses.get(status, 0) for status in ("queued", "running", "waiting_provider")
            ),
            failed=statuses.get("failed", 0) + statuses.get("cancelled", 0),
            succeeded=statuses.get("succeeded", 0),
            unknown=statuses.get("unknown", 0),
        )
        for episode_id, statuses in counts.items()
    }


async def count_episode_task_references(
    session: AsyncSession,
    workspace_id: UUID,
    episode_ids: list[UUID],
) -> dict[UUID, int]:
    counts = {episode_id: 0 for episode_id in episode_ids}
    for episode_id, _, count in await repository.count_task_statuses_by_episode(
        session,
        workspace_id,
        episode_ids,
    ):
        counts[episode_id] += count
    return counts


def _task_context(task: Task) -> TaskContext:
    return TaskContext(
        id=task.id,
        workspace_id=task.workspace_id,
        request_id=task.request_id,
        task_type=task.task_type,
        request_type=task.request_type,
        usage_type=task.usage_type,
        usage_id=task.usage_id,
        input_hash=task.input_hash,
        requested_by=task.requested_by,
        status=cast(TaskStatus, task.status),
    )


def _same_command(task: Task, command: ScriptExtractionTaskCommand) -> bool:
    return (
        task.request_id == command.request_id
        and task.episode_id == command.episode_id
        and task.input_version_id == command.input_version_id
        and task.input_hash == command.input_hash
    )


def _same_episode_planning_command(
    task: Task,
    command: EpisodePlanningTaskCommand,
) -> bool:
    return (
        task.request_id == command.plan_id
        and task.input_version_id == command.document_revision_id
        and task.input_hash == command.input_hash
        and task.usage_type == "document_revision"
        and task.usage_id == command.document_revision_id
    )


def _same_script_adaptation_command(
    task: Task,
    command: ScriptAdaptationTaskCommand,
) -> bool:
    return (
        task.request_id == command.run_id
        and task.episode_id == command.episode_id
        and task.input_version_id == command.input_version_id
        and task.input_hash == command.input_hash
        and task.usage_type == "script_version"
        and task.usage_id == command.input_version_id
    )


def _same_storyboard_draft_command(
    task: Task,
    command: StoryboardDraftTaskCommand,
) -> bool:
    return (
        task.request_id == command.batch_id
        and task.episode_id == command.episode_id
        and task.input_version_id == command.input_version_id
        and task.input_hash == command.input_hash
        and task.usage_type == "script_version"
        and task.usage_id == command.input_version_id
    )


def _same_storyboard_export_command(
    task: Task,
    command: StoryboardExportTaskCommand,
) -> bool:
    return (
        task.request_id == command.job_id
        and task.episode_id == command.episode_id
        and task.input_version_id == command.input_version_id
        and task.input_hash == command.input_hash
        and task.usage_type == "storyboard_export"
        and task.usage_id == command.job_id
    )


def _same_media_probe_command(task: Task, command: MediaProbeTaskCommand) -> bool:
    return (
        task.request_id == command.media_version_id
        and task.input_version_id == command.media_version_id
        and task.usage_type == "media_version"
        and task.usage_id == command.media_version_id
    )


def _append_task_transition_audit(
    session: AsyncSession,
    task: Task,
    *,
    action: Literal["task.started", "task.succeeded", "task.failed", "task.unknown"],
    previous_status: str,
    trace_id: str,
    now: datetime,
    additional_metadata: dict[str, object] | None = None,
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
    if additional_metadata is not None:
        metadata.update(additional_metadata)
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


def _fail_script_extraction_task(
    task: Task,
    *,
    error_code: str,
    error_summary: str,
    next_action: str,
    retryable: bool,
    now: datetime,
) -> bool:
    if task.task_type != "script_extraction":
        raise ValueError("task is not a script extraction task")
    if task.status in {"succeeded", "failed", "cancelled"}:
        return False
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = error_code
    task.error_retryable = retryable
    task.error_summary = error_summary
    task.next_action = next_action
    task.revision += 1
    task.updated_at = now
    return True


def _start_script_extraction_task(task: Task, *, now: datetime) -> bool:
    if task.task_type != "script_extraction":
        raise ValueError("task is not a script extraction task")
    if task.status != "queued":
        return False
    task.status = "running"
    task.progress_stage = "calling_provider"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "poll_task"
    task.revision += 1
    task.updated_at = now
    return True


def _mark_script_extraction_task_unknown(task: Task, *, now: datetime) -> bool:
    if task.task_type != "script_extraction":
        raise ValueError("task is not a script extraction task")
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        return False
    task.status = "unknown"
    task.progress_stage = "reconciliation_required"
    task.error_code = "ai_result_unknown"
    task.error_retryable = False
    task.error_summary = "DeepSeek response outcome is unknown"
    task.next_action = "start_new_extraction"
    task.revision += 1
    task.updated_at = now
    return True


def _complete_script_extraction_task(task: Task, *, now: datetime) -> bool:
    if task.task_type != "script_extraction":
        raise ValueError("task is not a script extraction task")
    if task.status == "succeeded":
        return False
    if task.status in {"failed", "cancelled"}:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Task cannot be completed from its current state",
            status_code=409,
        )
    task.status = "succeeded"
    task.progress_stage = "completed"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "review_candidates"
    task.revision += 1
    task.updated_at = now
    return True


async def lock_task(
    session: AsyncSession,
    task_id: UUID,
) -> TaskContext | None:
    task = await repository.find_task(session, task_id, for_update=True)
    return None if task is None else _task_context(task)


async def fail_script_extraction_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    error_code: str,
    error_summary: str,
    next_action: str,
    now: datetime,
    trace_id: str,
    retryable: bool = False,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Task state is unavailable",
            status_code=500,
        )
    previous_status = task.status
    changed = _fail_script_extraction_task(
        task,
        error_code=error_code,
        error_summary=error_summary,
        next_action=next_action,
        retryable=retryable,
        now=now,
    )
    if changed:
        _append_task_transition_audit(
            session,
            task,
            action="task.failed",
            previous_status=previous_status,
            trace_id=trace_id,
            now=now,
        )
        await session.flush()
    return changed


async def start_script_extraction_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Task state is unavailable",
            status_code=500,
        )
    previous_status = task.status
    changed = _start_script_extraction_task(task, now=now)
    if changed:
        _append_task_transition_audit(
            session,
            task,
            action="task.started",
            previous_status=previous_status,
            trace_id=trace_id,
            now=now,
        )
        await session.flush()
    return changed


async def mark_script_extraction_task_unknown(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Task state is unavailable",
            status_code=500,
        )
    previous_status = task.status
    changed = _mark_script_extraction_task_unknown(task, now=now)
    if changed:
        _append_task_transition_audit(
            session,
            task,
            action="task.unknown",
            previous_status=previous_status,
            trace_id=trace_id,
            now=now,
        )
        await session.flush()
    return changed


async def complete_script_extraction_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Task state is unavailable",
            status_code=500,
        )
    previous_status = task.status
    changed = _complete_script_extraction_task(task, now=now)
    if changed:
        _append_task_transition_audit(
            session,
            task,
            action="task.succeeded",
            previous_status=previous_status,
            trace_id=trace_id,
            now=now,
        )
        await session.flush()
    return changed


async def start_episode_planning_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "episode_planning":
        raise ValueError("task is not an episode planning task")
    if task.status != "queued":
        return False
    previous_status = task.status
    task.status = "running"
    task.progress_stage = "calling_provider"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "poll_task"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.started",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def fail_episode_planning_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    error_code: str,
    error_summary: str,
    next_action: str,
    now: datetime,
    trace_id: str,
    retryable: bool = False,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "episode_planning":
        raise ValueError("task is not an episode planning task")
    if task.status in {"succeeded", "failed", "cancelled"}:
        return False
    previous_status = task.status
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = error_code
    task.error_retryable = retryable
    task.error_summary = error_summary
    task.next_action = next_action
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def mark_episode_planning_task_unknown(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "episode_planning":
        raise ValueError("task is not an episode planning task")
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        return False
    previous_status = task.status
    task.status = "unknown"
    task.progress_stage = "reconciliation_required"
    task.error_code = "ai_result_unknown"
    task.error_retryable = False
    task.error_summary = "DeepSeek response outcome is unknown"
    task.next_action = "start_new_episode_plan"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.unknown",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def complete_episode_planning_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "episode_planning":
        raise ValueError("task is not an episode planning task")
    if task.status == "succeeded":
        return False
    if task.status in {"failed", "cancelled"}:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Task cannot be completed from its current state",
            status_code=409,
        )
    previous_status = task.status
    task.status = "succeeded"
    task.progress_stage = "completed"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "review_episode_plan"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.succeeded",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def start_script_adaptation_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "script_adaptation":
        raise ValueError("task is not a script adaptation task")
    if task.status != "queued":
        return False
    previous_status = task.status
    task.status = "running"
    task.progress_stage = "calling_provider"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "poll_task"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.started",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def start_storyboard_draft_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "storyboard_draft":
        raise ValueError("task is not a storyboard draft task")
    if task.status != "queued":
        return False
    previous_status = task.status
    task.status = "running"
    task.progress_stage = "calling_provider"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "poll_task"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.started",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def fail_storyboard_draft_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    error_code: str,
    error_summary: str,
    next_action: str,
    now: datetime,
    trace_id: str,
    retryable: bool = False,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "storyboard_draft":
        raise ValueError("task is not a storyboard draft task")
    if task.status in {"succeeded", "failed", "cancelled"}:
        return False
    previous_status = task.status
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = error_code
    task.error_retryable = retryable
    task.error_summary = error_summary
    task.next_action = next_action
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def mark_storyboard_draft_task_unknown(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "storyboard_draft":
        raise ValueError("task is not a storyboard draft task")
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        return False
    previous_status = task.status
    task.status = "unknown"
    task.progress_stage = "reconciliation_required"
    task.error_code = "ai_result_unknown"
    task.error_retryable = False
    task.error_summary = "AI storyboard draft result is unknown"
    task.next_action = "create_new_storyboard_draft_batch"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.unknown",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def fail_script_adaptation_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    error_code: str,
    error_summary: str,
    next_action: str,
    now: datetime,
    trace_id: str,
    retryable: bool = False,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "script_adaptation":
        raise ValueError("task is not a script adaptation task")
    if task.status in {"succeeded", "failed", "cancelled"}:
        return False
    previous_status = task.status
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = error_code
    task.error_retryable = retryable
    task.error_summary = error_summary
    task.next_action = next_action
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def mark_script_adaptation_task_unknown(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "script_adaptation":
        raise ValueError("task is not a script adaptation task")
    if task.status in {"succeeded", "failed", "cancelled", "unknown"}:
        return False
    previous_status = task.status
    task.status = "unknown"
    task.progress_stage = "reconciliation_required"
    task.error_code = "ai_result_unknown"
    task.error_retryable = False
    task.error_summary = "AI adaptation result is unknown"
    task.next_action = "start_new_adaptation"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.unknown",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def complete_script_adaptation_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "script_adaptation":
        raise ValueError("task is not a script adaptation task")
    if task.status == "succeeded":
        return False
    if task.status in {"failed", "cancelled"}:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Task cannot be completed from its current state",
            status_code=409,
        )
    previous_status = task.status
    task.status = "succeeded"
    task.progress_stage = "completed"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "review_adaptation"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.succeeded",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def complete_storyboard_draft_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "storyboard_draft":
        raise ValueError("task is not a storyboard draft task")
    if task.status == "succeeded":
        return False
    if task.status != "running":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Task cannot be completed from its current state",
            status_code=409,
        )
    previous_status = task.status
    task.status = "succeeded"
    task.progress_stage = "completed"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "review_storyboard_drafts"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.succeeded",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def start_storyboard_export_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "storyboard_export":
        raise ValueError("task is not a storyboard export task")
    if task.status != "queued":
        return False
    previous_status = task.status
    task.status = "running"
    task.progress_stage = "building_package"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "poll_storyboard_export"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.started",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def fail_storyboard_export_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    error_code: str,
    error_summary: str,
    next_action: str,
    now: datetime,
    trace_id: str,
    retryable: bool = False,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "storyboard_export":
        raise ValueError("task is not a storyboard export task")
    if task.status in {"succeeded", "failed", "cancelled"}:
        return False
    previous_status = task.status
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = error_code
    task.error_retryable = retryable
    task.error_summary = error_summary
    task.next_action = next_action
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def complete_storyboard_export_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "storyboard_export":
        raise ValueError("task is not a storyboard export task")
    if task.status == "succeeded":
        return False
    if task.status != "running":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Task cannot be completed from its current state",
            status_code=409,
        )
    previous_status = task.status
    task.status = "succeeded"
    task.progress_stage = "completed"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "download_storyboard_export"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.succeeded",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def cancel_script_adaptation_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "script_adaptation":
        raise ValueError("task is not a script adaptation task")
    if task.status == "cancelled":
        return False
    if task.status != "queued":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Only a queued adaptation task can be cancelled",
            status_code=409,
        )
    previous_status = task.status
    task.status = "cancelled"
    task.progress_stage = "cancelled"
    task.next_action = None
    task.cancel_status = "accepted"
    task.revision += 1
    task.updated_at = now
    append_audit_event(
        session,
        workspace_id=task.workspace_id,
        actor_id=task.requested_by,
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
        },
        occurred_at=now,
    )
    await session.flush()
    return True


async def fail_media_probe_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    error_code: str,
    error_summary: str,
    now: datetime,
    trace_id: str,
    retryable: bool = True,
    next_action: str = "retry_probe",
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "media_probe":
        raise ValueError("task is not a media probe task")
    if task.status in {"succeeded", "failed", "cancelled"}:
        return False
    previous_status = task.status
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = error_code
    task.error_retryable = retryable
    task.error_summary = error_summary
    task.next_action = next_action
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def complete_media_probe_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "media_probe":
        raise ValueError("task is not a media probe task")
    if task.status == "succeeded":
        return False
    if task.status in {"failed", "cancelled"}:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Task cannot be completed from its current state",
            status_code=409,
        )
    previous_status = task.status
    task.status = "succeeded"
    task.progress_stage = "completed"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = "review_media"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.succeeded",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def fail_upload_expiration_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    error_code: str,
    error_summary: str,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "upload_expiration":
        raise ValueError("task is not an upload expiration task")
    if task.status in {"succeeded", "failed", "cancelled"}:
        return False
    previous_status = task.status
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = error_code
    task.error_retryable = False
    task.error_summary = error_summary
    task.next_action = "contact_support"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def complete_upload_expiration_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "upload_expiration":
        raise ValueError("task is not an upload expiration task")
    if task.status == "succeeded":
        return False
    if task.status in {"failed", "cancelled"}:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Task cannot be completed from its current state",
            status_code=409,
        )
    previous_status = task.status
    task.status = "succeeded"
    task.progress_stage = "completed"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = None
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.succeeded",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def fail_upload_cleanup_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    error_code: str,
    error_summary: str,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "upload_cleanup":
        raise ValueError("task is not an upload cleanup task")
    if task.status in {"succeeded", "failed", "cancelled"}:
        return False
    previous_status = task.status
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = error_code
    task.error_retryable = False
    task.error_summary = error_summary
    task.next_action = "contact_support"
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def complete_upload_cleanup_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    cleaned_count: int,
    now: datetime,
    trace_id: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type != "upload_cleanup":
        raise ValueError("task is not an upload cleanup task")
    if task.status == "succeeded":
        return False
    if task.status in {"failed", "cancelled"}:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Task cannot be completed from its current state",
            status_code=409,
        )
    previous_status = task.status
    task.status = "succeeded"
    task.progress_stage = "completed"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = None
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.succeeded",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
        additional_metadata={"cleaned_count": cleaned_count},
    )
    await session.flush()
    return True


async def fail_media_location_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    error_code: str,
    error_summary: str,
    now: datetime,
    trace_id: str,
    retryable: bool,
    next_action: str,
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type not in {
        "media_location_migration",
        "media_location_retirement",
    }:
        raise ValueError("task is not a media location task")
    if task.status in {"succeeded", "failed", "cancelled"}:
        return False
    previous_status = task.status
    task.status = "failed"
    task.progress_stage = "blocked"
    task.error_code = error_code
    task.error_retryable = retryable
    task.error_summary = error_summary
    task.next_action = next_action
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.failed",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def complete_media_location_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    now: datetime,
    trace_id: str,
    next_action: str | None = "review_media_locations",
) -> bool:
    task = await repository.find_task(session, task_id, for_update=True)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    if task.task_type not in {
        "media_location_migration",
        "media_location_retirement",
    }:
        raise ValueError("task is not a media location task")
    if task.status == "succeeded":
        return False
    if task.status in {"failed", "cancelled"}:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Task cannot be completed from its current state",
            status_code=409,
        )
    previous_status = task.status
    task.status = "succeeded"
    task.progress_stage = "completed"
    task.error_code = None
    task.error_retryable = None
    task.error_summary = None
    task.next_action = next_action
    task.revision += 1
    task.updated_at = now
    _append_task_transition_audit(
        session,
        task,
        action="task.succeeded",
        previous_status=previous_status,
        trace_id=trace_id,
        now=now,
    )
    await session.flush()
    return True


async def create_script_extraction_task(
    session: AsyncSession,
    actor: ActorContext,
    command: ScriptExtractionTaskCommand,
    *,
    trace_id: str,
) -> TaskResponse:
    if actor.workspace_id != command.workspace_id:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403)
    try:
        require_workspace_capability(actor.role, actor.workspace_status, Capability.CONTENT_WRITE)
    except PermissionError as error:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403) from error
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
        return task_response(existing)

    await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="script_extraction.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=inserted_id,
            routing_key="io.script.extract",
            payload={"task_id": str(inserted_id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=actor.user_id,
        action="task.created",
        target_type="task",
        target_id=inserted_id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": "script_extraction",
            "request_type": "extraction_batch",
            "request_id": str(command.request_id),
        },
        occurred_at=now,
    )
    await session.flush()
    task = await repository.find_task(session, inserted_id)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    return task_response(task)


async def create_media_probe_task(
    session: AsyncSession,
    actor: ActorContext,
    command: MediaProbeTaskCommand,
    *,
    trace_id: str,
) -> TaskResponse:
    if actor.workspace_id != command.workspace_id:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403)
    try:
        require_workspace_capability(actor.role, actor.workspace_status, Capability.CONTENT_WRITE)
    except PermissionError as error:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403) from error
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)

    task_id = uuid7()
    now = datetime.now(UTC)
    inserted_id = await session.scalar(
        insert(Task)
        .values(
            id=task_id,
            workspace_id=command.workspace_id,
            task_type="media_probe",
            request_type="media_version",
            request_id=command.media_version_id,
            usage_type="media_version",
            usage_id=command.media_version_id,
            input_version_id=command.media_version_id,
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
            "media_probe",
            command.idempotency_key,
        )
        if existing is None:
            raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
        if not _same_media_probe_command(existing, command):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        return task_response(existing)

    await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="media_probe.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=inserted_id,
            routing_key="media.probe",
            payload={"task_id": str(inserted_id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=actor.user_id,
        action="task.created",
        target_type="task",
        target_id=inserted_id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": "media_probe",
            "request_type": "media_version",
            "request_id": str(command.media_version_id),
        },
        occurred_at=now,
    )
    await session.flush()
    task = await repository.find_task(session, inserted_id)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    return task_response(task)


async def create_episode_planning_task(
    session: AsyncSession,
    actor: ActorContext,
    command: EpisodePlanningTaskCommand,
    *,
    trace_id: str,
) -> TaskResponse:
    if actor.workspace_id != command.workspace_id:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403)
    try:
        require_workspace_capability(actor.role, actor.workspace_status, Capability.CONTENT_WRITE)
    except PermissionError as error:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403) from error
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)

    task_id = uuid7()
    now = datetime.now(UTC)
    inserted_id = await session.scalar(
        insert(Task)
        .values(
            id=task_id,
            workspace_id=command.workspace_id,
            task_type="episode_planning",
            request_type="episode_plan",
            request_id=command.plan_id,
            usage_type="document_revision",
            usage_id=command.document_revision_id,
            input_version_id=command.document_revision_id,
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
            "episode_planning",
            command.idempotency_key,
        )
        if existing is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Task state is unavailable",
                status_code=500,
            )
        if not _same_episode_planning_command(existing, command):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        return task_response(existing)

    await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="episode_planning.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=inserted_id,
            routing_key="io.script.plan",
            payload={"task_id": str(inserted_id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=actor.user_id,
        action="task.created",
        target_type="task",
        target_id=inserted_id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": "episode_planning",
            "request_type": "episode_plan",
            "request_id": str(command.plan_id),
        },
        occurred_at=now,
    )
    await session.flush()
    task = await repository.find_task(session, inserted_id)
    if task is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Task state is unavailable",
            status_code=500,
        )
    return task_response(task)


async def create_script_adaptation_task(
    session: AsyncSession,
    actor: ActorContext,
    command: ScriptAdaptationTaskCommand,
    *,
    trace_id: str,
) -> TaskResponse:
    if actor.workspace_id != command.workspace_id:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403)
    try:
        require_workspace_capability(actor.role, actor.workspace_status, Capability.CONTENT_WRITE)
    except PermissionError as error:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403) from error
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)

    task_id = uuid7()
    now = datetime.now(UTC)
    inserted_id = await session.scalar(
        insert(Task)
        .values(
            id=task_id,
            workspace_id=command.workspace_id,
            task_type="script_adaptation",
            request_type="adaptation_run",
            request_id=command.run_id,
            episode_id=command.episode_id,
            usage_type="script_version",
            usage_id=command.input_version_id,
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
            "script_adaptation",
            command.idempotency_key,
        )
        if existing is None:
            raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
        if not _same_script_adaptation_command(existing, command):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        return task_response(existing)

    await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="script_adaptation.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=inserted_id,
            routing_key="io.script.adapt",
            payload={"task_id": str(inserted_id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=actor.user_id,
        action="task.created",
        target_type="task",
        target_id=inserted_id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": "script_adaptation",
            "request_type": "adaptation_run",
            "request_id": str(command.run_id),
        },
        occurred_at=now,
    )
    await session.flush()
    task = await repository.find_task(session, inserted_id)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    return task_response(task)


async def create_storyboard_draft_task(
    session: AsyncSession,
    actor: ActorContext,
    command: StoryboardDraftTaskCommand,
    *,
    trace_id: str,
) -> TaskResponse:
    if actor.workspace_id != command.workspace_id:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403)
    try:
        require_workspace_capability(actor.role, actor.workspace_status, Capability.CONTENT_WRITE)
    except PermissionError as error:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403) from error
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)

    now = datetime.now(UTC)
    inserted_id = await session.scalar(
        insert(Task)
        .values(
            id=uuid7(),
            workspace_id=command.workspace_id,
            task_type="storyboard_draft",
            request_type="storyboard_draft_batch",
            request_id=command.batch_id,
            episode_id=command.episode_id,
            usage_type="script_version",
            usage_id=command.input_version_id,
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
            "storyboard_draft",
            command.idempotency_key,
        )
        if existing is None:
            raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
        if not _same_storyboard_draft_command(existing, command):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        return task_response(existing)

    await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="storyboard_draft.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=inserted_id,
            routing_key="io.storyboard.draft",
            payload={"task_id": str(inserted_id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=actor.user_id,
        action="task.created",
        target_type="task",
        target_id=inserted_id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": "storyboard_draft",
            "request_type": "storyboard_draft_batch",
            "request_id": str(command.batch_id),
        },
        occurred_at=now,
    )
    await session.flush()
    task = await repository.find_task(session, inserted_id)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    return task_response(task)


async def create_storyboard_export_task(
    session: AsyncSession,
    actor: ActorContext,
    command: StoryboardExportTaskCommand,
    *,
    trace_id: str,
) -> TaskResponse:
    if actor.workspace_id != command.workspace_id:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403)
    try:
        require_workspace_capability(
            actor.role,
            actor.workspace_status,
            Capability.CONTENT_WRITE,
        )
    except PermissionError as error:
        raise ApiError(
            ErrorCode.FORBIDDEN,
            "Action is not allowed",
            status_code=403,
        ) from error
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)

    now = datetime.now(UTC)
    inserted_id = await session.scalar(
        insert(Task)
        .values(
            id=uuid7(),
            workspace_id=command.workspace_id,
            task_type="storyboard_export",
            request_type="storyboard_export_job",
            request_id=command.job_id,
            episode_id=command.episode_id,
            usage_type="storyboard_export",
            usage_id=command.job_id,
            input_version_id=command.input_version_id,
            input_hash=command.input_hash,
            status="queued",
            progress_stage="queued",
            next_action="poll_storyboard_export",
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
            "storyboard_export",
            command.idempotency_key,
        )
        if existing is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Task state is unavailable",
                status_code=500,
            )
        if not _same_storyboard_export_command(existing, command):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        return task_response(existing)

    await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="storyboard_export.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=inserted_id,
            routing_key="media.storyboard.export",
            payload={"task_id": str(inserted_id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=actor.user_id,
        action="task.created",
        target_type="task",
        target_id=inserted_id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": "storyboard_export",
            "request_type": "storyboard_export_job",
            "request_id": str(command.job_id),
        },
        occurred_at=now,
    )
    await session.flush()
    task = await repository.find_task(session, inserted_id)
    if task is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Task state is unavailable",
            status_code=500,
        )
    return task_response(task)


async def create_upload_expiration_task(
    session: AsyncSession,
    command: UploadExpirationTaskCommand,
    *,
    trace_id: str,
) -> UploadExpirationTaskDispatch:
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)
    existing = await repository.find_idempotent_task(
        session,
        command.workspace_id,
        "upload_expiration",
        command.idempotency_key,
    )
    if existing is not None:
        if (
            existing.request_id != command.upload_session_id
            or existing.requested_by != command.requested_by
        ):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Upload expiration task already exists without a fire record",
            status_code=409,
        )

    now = datetime.now(UTC)
    task = Task(
        id=uuid7(),
        workspace_id=command.workspace_id,
        task_type="upload_expiration",
        request_type="upload_session",
        request_id=command.upload_session_id,
        usage_type="upload_session",
        usage_id=command.upload_session_id,
        status="queued",
        progress_stage="queued",
        next_action="wait_for_cleanup",
        cancel_status="none",
        idempotency_key=command.idempotency_key,
        requested_by=command.requested_by,
        revision=1,
        created_at=now,
        updated_at=now,
    )
    session.add(task)
    await session.flush()
    outbox_event_id = await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="upload_expiration.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=task.id,
            routing_key="media.upload.expire",
            payload={"task_id": str(task.id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=command.requested_by,
        action="task.created",
        target_type="task",
        target_id=task.id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": "upload_expiration",
            "request_type": "upload_session",
            "request_id": str(command.upload_session_id),
        },
        occurred_at=now,
    )
    await session.flush()
    return UploadExpirationTaskDispatch(task=task_response(task), outbox_event_id=outbox_event_id)


async def create_upload_cleanup_task(
    session: AsyncSession,
    command: UploadCleanupTaskCommand,
    *,
    trace_id: str,
) -> UploadCleanupTaskDispatch:
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)
    existing = await repository.find_idempotent_task(
        session,
        command.workspace_id,
        "upload_cleanup",
        command.idempotency_key,
    )
    if existing is not None:
        if (
            existing.request_id != command.workspace_id
            or existing.requested_by != command.requested_by
        ):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Upload cleanup task already exists without a fire record",
            status_code=409,
        )

    now = datetime.now(UTC)
    task = Task(
        id=uuid7(),
        workspace_id=command.workspace_id,
        task_type="upload_cleanup",
        request_type="workspace",
        request_id=command.workspace_id,
        usage_type="workspace",
        usage_id=command.workspace_id,
        status="queued",
        progress_stage="queued",
        next_action="wait_for_cleanup",
        cancel_status="none",
        idempotency_key=command.idempotency_key,
        requested_by=command.requested_by,
        revision=1,
        created_at=now,
        updated_at=now,
    )
    session.add(task)
    await session.flush()
    outbox_event_id = await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="upload_cleanup.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=task.id,
            routing_key="media.upload.cleanup",
            payload={"task_id": str(task.id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=command.requested_by,
        action="task.created",
        target_type="task",
        target_id=task.id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": "upload_cleanup",
            "request_type": "workspace",
            "request_id": str(command.workspace_id),
        },
        occurred_at=now,
    )
    await session.flush()
    return UploadCleanupTaskDispatch(task=task_response(task), outbox_event_id=outbox_event_id)


async def create_media_location_migration_task(
    session: AsyncSession,
    command: MediaLocationMigrationTaskCommand,
    *,
    trace_id: str,
) -> MediaLocationTaskDispatch:
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)
    usage_type = (
        "media_location_migration" if command.operation == "migrate" else "media_location_rollback"
    )
    existing = await repository.find_idempotent_task(
        session,
        command.workspace_id,
        "media_location_migration",
        command.idempotency_key,
    )
    if existing is not None:
        if (
            existing.request_id != command.media_version_id
            or existing.requested_by != command.requested_by
            or existing.usage_type != usage_type
            or (command.operation == "rollback" and existing.usage_id != command.location_id)
        ):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        event_id = await find_outbox_event_id(
            session,
            aggregate_id=existing.id,
            event_type="media_location_migration.requested",
        )
        if event_id is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Location migration event is unavailable",
                status_code=500,
            )
        return MediaLocationTaskDispatch(task=task_response(existing), outbox_event_id=event_id)

    now = datetime.now(UTC)
    task = Task(
        id=uuid7(),
        workspace_id=command.workspace_id,
        task_type="media_location_migration",
        request_type="media_version",
        request_id=command.media_version_id,
        usage_type=usage_type,
        usage_id=command.location_id,
        input_version_id=command.media_version_id,
        status="queued",
        progress_stage="queued",
        next_action="wait_for_location_migration",
        cancel_status="none",
        idempotency_key=command.idempotency_key,
        requested_by=command.requested_by,
        revision=1,
        created_at=now,
        updated_at=now,
    )
    session.add(task)
    await session.flush()
    event_id = await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="media_location_migration.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=task.id,
            routing_key="media.location.migrate",
            payload={"task_id": str(task.id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=command.requested_by,
        action="task.created",
        target_type="task",
        target_id=task.id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": task.task_type,
            "request_type": task.request_type,
            "request_id": str(task.request_id),
        },
        occurred_at=now,
    )
    await session.flush()
    return MediaLocationTaskDispatch(task=task_response(task), outbox_event_id=event_id)


async def create_media_location_retirement_task(
    session: AsyncSession,
    command: MediaLocationRetirementTaskCommand,
    *,
    trace_id: str,
) -> MediaLocationTaskDispatch:
    if not trace_id or len(trace_id) > 64:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Invalid trace identifier", status_code=422)
    existing = await repository.find_idempotent_task(
        session,
        command.workspace_id,
        "media_location_retirement",
        command.idempotency_key,
    )
    if existing is not None:
        if (
            existing.request_id != command.media_location_id
            or existing.requested_by != command.requested_by
        ):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Idempotency key was used with different input",
                status_code=409,
            )
        event_id = await find_outbox_event_id(
            session,
            aggregate_id=existing.id,
            event_type="media_location_retirement.requested",
        )
        if event_id is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Location retirement event is unavailable",
                status_code=500,
            )
        return MediaLocationTaskDispatch(task=task_response(existing), outbox_event_id=event_id)

    now = datetime.now(UTC)
    task = Task(
        id=uuid7(),
        workspace_id=command.workspace_id,
        task_type="media_location_retirement",
        request_type="media_location",
        request_id=command.media_location_id,
        usage_type="media_location",
        usage_id=command.media_location_id,
        status="queued",
        progress_stage="queued",
        next_action="wait_for_location_retirement",
        cancel_status="none",
        idempotency_key=command.idempotency_key,
        requested_by=command.requested_by,
        revision=1,
        created_at=now,
        updated_at=now,
    )
    session.add(task)
    await session.flush()
    event_id = await enqueue_outbox_event(
        session,
        OutboxEventCommand(
            workspace_id=command.workspace_id,
            event_type="media_location_retirement.requested",
            schema_version=1,
            aggregate_type="task",
            aggregate_id=task.id,
            routing_key="media.location.retire",
            payload={"task_id": str(task.id)},
            trace_id=trace_id,
            available_at=now,
            occurred_at=now,
        ),
    )
    append_audit_event(
        session,
        workspace_id=command.workspace_id,
        actor_id=command.requested_by,
        action="task.created",
        target_type="task",
        target_id=task.id,
        trace_id=trace_id,
        metadata={
            "revision": 1,
            "task_type": task.task_type,
            "request_type": task.request_type,
            "request_id": str(task.request_id),
        },
        occurred_at=now,
    )
    await session.flush()
    return MediaLocationTaskDispatch(task=task_response(task), outbox_event_id=event_id)


async def get_internal_task(session: AsyncSession, task_id: UUID) -> TaskResponse:
    task = await repository.find_task(session, task_id)
    if task is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Task state is unavailable", status_code=500)
    return task_response(task)


async def get_task(
    session: AsyncSession,
    claims: AccessTokenClaims,
    task_id: UUID,
) -> TaskResponse:
    task = await repository.find_task(session, task_id)
    if task is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Task not found", status_code=404)
    try:
        await actor_context(session, claims, task.workspace_id, Capability.CONTENT_READ)
    except ApiError as error:
        if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
            raise ApiError(ErrorCode.NOT_FOUND, "Task not found", status_code=404) from error
        raise
    return task_response(task)


async def list_tasks(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    *,
    task_type: Literal[
        "script_extraction",
        "episode_planning",
        "script_adaptation",
        "storyboard_draft",
        "image_generation",
        "video_generation",
        "media_probe",
        "upload_expiration",
        "upload_cleanup",
        "media_location_migration",
        "media_location_retirement",
    ]
    | None,
    status: TaskStatus | None,
    limit: int,
    offset: int,
) -> PaginatedTasks:
    await actor_context(session, claims, workspace_id, Capability.CONTENT_READ)
    tasks, total = await repository.list_tasks(
        session,
        workspace_id,
        task_type=task_type,
        status=status,
        limit=limit,
        offset=offset,
    )
    return PaginatedTasks(
        items=[task_response(task) for task in tasks],
        total=total,
        limit=limit,
        offset=offset,
    )
