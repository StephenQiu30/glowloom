import hashlib
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import ActorContext, Capability, actor_context
from app.modules.production import (
    MediaLocationRetirementTaskCommand,
    UploadCleanupTaskCommand,
    UploadExpirationTaskCommand,
    create_media_location_retirement_task,
    create_upload_cleanup_task,
    create_upload_expiration_task,
    get_internal_task,
)
from app.modules.scheduling import repository
from app.modules.scheduling.contracts import (
    InvalidSchedulePayload,
    UnregisteredScheduleHandler,
    UploadCleanupPayload,
    UploadExpirationPayload,
    parse_schedule_payload,
    public_handler_name,
)
from app.modules.scheduling.metrics import (
    SCHEDULE_DISPATCH_RESULTS,
)
from app.modules.scheduling.models import Schedule, ScheduleFire
from app.modules.scheduling.rules import (
    InvalidScheduleRule,
    next_cron_occurrence,
    plan_due_occurrences,
)
from app.modules.scheduling.schemas import (
    PaginatedSchedules,
    ScheduleConfigurationRequest,
    ScheduleCronRuleResponse,
    ScheduleFireResponse,
    ScheduleIntervalRuleResponse,
    ScheduleOneOffRuleResponse,
    ScheduleResponse,
    ScheduleResumeRequest,
    ScheduleScopeResponse,
    ScheduleStateRequest,
    ScheduleTriggerRequest,
    UnknownScheduleRuleResponse,
)


@dataclass(frozen=True, slots=True)
class ScheduleDispatchOutcome:
    dispatched_count: int
    handler: str | None = None
    result: Literal["dispatched", "skipped"] | None = None
    metric_count: int = 0
    lag_seconds: float = 0
    misfire_count: int = 0
    misfire_policy: Literal["skip", "run_once", "catch_up"] | None = None


@dataclass(frozen=True, slots=True)
class ScheduleFailureOutcome:
    manual_attention: bool
    handler: str | None = None
    reason: Literal["configuration", "retry_exhausted"] | None = None


def _rule_response(
    schedule: Schedule,
) -> (
    ScheduleOneOffRuleResponse
    | ScheduleIntervalRuleResponse
    | ScheduleCronRuleResponse
    | UnknownScheduleRuleResponse
):
    try:
        grace = int(schedule.rule.get("misfire_grace_seconds", 30))
        if grace < 0 or grace > 3600:
            raise ValueError
        if schedule.kind == "one_off":
            raw_at = schedule.rule.get("at")
            if not isinstance(raw_at, str):
                raise ValueError
            at = datetime.fromisoformat(raw_at.replace("Z", "+00:00"))
            if at.tzinfo is None:
                raise ValueError
            return ScheduleOneOffRuleResponse(
                at=at,
                misfire_grace_seconds=grace,
            )
        if schedule.kind == "interval":
            seconds = int(schedule.rule.get("seconds", 0))
            if seconds <= 0:
                raise ValueError
            return ScheduleIntervalRuleResponse(
                seconds=seconds,
                misfire_grace_seconds=grace,
            )
        if schedule.kind == "cron":
            expression = schedule.rule.get("expression")
            if not isinstance(expression, str):
                raise ValueError
            return ScheduleCronRuleResponse(
                expression=expression,
                misfire_grace_seconds=grace,
            )
    except (TypeError, ValueError):
        pass
    return UnknownScheduleRuleResponse()


def _response(schedule: Schedule) -> ScheduleResponse:
    return ScheduleResponse(
        id=schedule.id,
        workspace_id=schedule.workspace_id,
        schedule_key=schedule.schedule_key,
        handler_name=public_handler_name(schedule.handler_name),
        scope=ScheduleScopeResponse.model_validate(schedule.scope),
        kind=cast(Literal["one_off", "interval", "cron"], schedule.kind),
        rule=_rule_response(schedule),
        timezone=schedule.timezone,
        status=cast(
            Literal["active", "paused", "completed", "manual_attention"],
            schedule.status,
        ),
        next_fire_at=schedule.next_fire_at,
        next_attempt_at=schedule.next_attempt_at,
        misfire_policy=cast(
            Literal["skip", "run_once", "catch_up"], schedule.misfire_policy
        ),
        max_catch_up=schedule.max_catch_up,
        failure_count=schedule.failure_count,
        last_error=schedule.last_error,
        revision=schedule.revision,
    )


async def ensure_upload_expiration_schedule(
    session: AsyncSession,
    actor: ActorContext,
    *,
    upload_session_id: UUID,
    expires_at: datetime,
    now: datetime,
) -> Schedule:
    schedule_key = f"media.upload.expire:{upload_session_id}"
    existing = await repository.find_schedule_by_key(
        session, actor.workspace_id, schedule_key, for_update=True
    )
    if existing is not None:
        if existing.payload != {"upload_session_id": str(upload_session_id)}:
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Upload expiration schedule conflicts with existing input",
                status_code=409,
            )
        return existing
    schedule = Schedule(
        id=uuid7(),
        workspace_id=actor.workspace_id,
        schedule_key=schedule_key,
        handler_name="expire_upload_session",
        scope={"usage_type": "upload_session", "usage_id": str(upload_session_id)},
        payload={"upload_session_id": str(upload_session_id)},
        kind="one_off",
        rule={"at": expires_at.isoformat(), "misfire_grace_seconds": 30},
        timezone="UTC",
        status="active",
        next_fire_at=expires_at,
        next_attempt_at=None,
        misfire_policy="run_once",
        max_catch_up=0,
        failure_count=0,
        revision=1,
        created_by=actor.user_id,
        created_at=now,
        updated_at=now,
    )
    session.add(schedule)
    await session.flush()
    return schedule


async def ensure_upload_cleanup_schedule(
    session: AsyncSession,
    actor: ActorContext,
    *,
    interval_seconds: int,
    now: datetime,
) -> Schedule:
    schedule_key = f"media.upload.cleanup:{actor.workspace_id}"
    existing = await repository.find_schedule_by_key(
        session, actor.workspace_id, schedule_key, for_update=True
    )
    expected_payload = {"workspace_id": str(actor.workspace_id)}
    expected_rule = {"seconds": interval_seconds, "misfire_grace_seconds": 30}
    if existing is not None:
        if (
            existing.handler_name != "cleanup_expired_uploads"
            or existing.payload != expected_payload
            or existing.kind != "interval"
        ):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Upload cleanup schedule conflicts with existing configuration",
                status_code=409,
            )
        return existing
    schedule_id = uuid7()
    await session.scalar(
        insert(Schedule)
        .values(
            id=schedule_id,
            workspace_id=actor.workspace_id,
            schedule_key=schedule_key,
            handler_name="cleanup_expired_uploads",
            scope={"usage_type": "workspace", "usage_id": str(actor.workspace_id)},
            payload=expected_payload,
            kind="interval",
            rule=expected_rule,
            timezone="UTC",
            status="active",
            next_fire_at=now + timedelta(seconds=interval_seconds),
            next_attempt_at=None,
            misfire_policy="run_once",
            max_catch_up=0,
            failure_count=0,
            revision=1,
            created_by=actor.user_id,
            created_at=now,
            updated_at=now,
        )
        .on_conflict_do_nothing(constraint="uq_sys_schedule_workspace_key")
        .returning(Schedule.id)
    )
    schedule = await repository.find_schedule_by_key(
        session, actor.workspace_id, schedule_key, for_update=True
    )
    if schedule is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Upload cleanup schedule is unavailable",
            status_code=500,
        )
    if (
        schedule.handler_name != "cleanup_expired_uploads"
        or schedule.payload != expected_payload
        or schedule.kind != "interval"
    ):
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Upload cleanup schedule conflicts with existing configuration",
            status_code=409,
        )
    return schedule


async def complete_upload_expiration_schedule(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    upload_session_id: UUID,
    now: datetime,
) -> None:
    schedule = await repository.find_schedule_by_key(
        session,
        workspace_id,
        f"media.upload.expire:{upload_session_id}",
        for_update=True,
    )
    if schedule is None or schedule.status == "completed":
        return
    schedule.status = "completed"
    schedule.next_fire_at = None
    schedule.next_attempt_at = None
    schedule.lease_until = None
    schedule.leased_by = None
    schedule.revision += 1
    schedule.updated_at = now
    await session.flush()


async def ensure_media_location_retirement_schedule(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    media_location_id: UUID,
    created_by: UUID,
    retire_after: datetime,
    now: datetime,
) -> Schedule:
    schedule_key = f"media.location.retire:{media_location_id}"
    expected_payload = {"media_location_id": str(media_location_id)}
    schedule = await repository.find_schedule_by_key(
        session, workspace_id, schedule_key, for_update=True
    )
    if schedule is None:
        schedule_id = uuid7()
        await session.scalar(
            insert(Schedule)
            .values(
                id=schedule_id,
                workspace_id=workspace_id,
                schedule_key=schedule_key,
                handler_name="retire_media_location",
                scope={
                    "usage_type": "media_location",
                    "usage_id": str(media_location_id),
                },
                payload=expected_payload,
                kind="one_off",
                rule={"at": retire_after.isoformat(), "misfire_grace_seconds": 30},
                timezone="UTC",
                status="active",
                next_fire_at=retire_after,
                next_attempt_at=None,
                misfire_policy="run_once",
                max_catch_up=0,
                failure_count=0,
                revision=1,
                created_by=created_by,
                created_at=now,
                updated_at=now,
            )
            .on_conflict_do_nothing(constraint="uq_sys_schedule_workspace_key")
            .returning(Schedule.id)
        )
        schedule = await repository.find_schedule_by_key(
            session, workspace_id, schedule_key, for_update=True
        )
    if schedule is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Location retirement schedule is unavailable",
            status_code=500,
        )
    if (
        schedule.handler_name != "retire_media_location"
        or schedule.payload != expected_payload
        or schedule.kind != "one_off"
    ):
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Location retirement schedule conflicts with existing configuration",
            status_code=409,
        )
    if schedule.next_fire_at != retire_after or schedule.status != "active":
        schedule.rule = {
            "at": retire_after.isoformat(),
            "misfire_grace_seconds": 30,
        }
        schedule.status = "active"
        schedule.next_fire_at = retire_after
        schedule.next_attempt_at = None
        schedule.failure_count = 0
        schedule.last_error = None
        schedule.lease_until = None
        schedule.leased_by = None
        schedule.revision += 1
        schedule.updated_at = now
        await session.flush()
    return schedule


async def complete_media_location_retirement_schedule(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    media_location_id: UUID,
    now: datetime,
) -> None:
    schedule = await repository.find_schedule_by_key(
        session,
        workspace_id,
        f"media.location.retire:{media_location_id}",
        for_update=True,
    )
    if schedule is None or schedule.status == "completed":
        return
    schedule.status = "completed"
    schedule.next_fire_at = None
    schedule.next_attempt_at = None
    schedule.lease_until = None
    schedule.leased_by = None
    schedule.revision += 1
    schedule.updated_at = now
    await session.flush()


async def _owned_schedule(
    session: AsyncSession,
    claims: AccessTokenClaims,
    schedule_id: UUID,
    *,
    for_update: bool = False,
) -> tuple[Schedule, ActorContext]:
    schedule = await repository.find_schedule(session, schedule_id, for_update=for_update)
    if schedule is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Schedule not found", status_code=404)
    try:
        actor = await actor_context(
            session, claims, schedule.workspace_id, Capability.WORKSPACE_MANAGE
        )
    except ApiError as error:
        if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
            raise ApiError(ErrorCode.NOT_FOUND, "Schedule not found", status_code=404) from error
        raise
    return schedule, actor


def _require_revision(schedule: Schedule, expected_revision: int) -> None:
    if schedule.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Schedule revision has changed",
            status_code=409,
            details={"current_revision": schedule.revision},
        )


async def list_schedules(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    *,
    status: str | None,
    limit: int,
    offset: int,
) -> PaginatedSchedules:
    await actor_context(session, claims, workspace_id, Capability.WORKSPACE_MANAGE)
    schedules, total = await repository.list_schedules(
        session, workspace_id, status=status, limit=limit, offset=offset
    )
    return PaginatedSchedules(
        items=[_response(schedule) for schedule in schedules],
        total=total,
        limit=limit,
        offset=offset,
    )


async def configure_schedule(
    session: AsyncSession,
    claims: AccessTokenClaims,
    schedule_id: UUID,
    request: ScheduleConfigurationRequest,
    *,
    trace_id: str,
) -> ScheduleResponse:
    if request.effective_from.tzinfo is None:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "effective_from must include a timezone",
            status_code=422,
        )
    effective_from = request.effective_from.astimezone(UTC)
    try:
        if request.kind == "interval":
            if request.interval_seconds is None:
                raise InvalidScheduleRule("interval_seconds is required")
            rule = {
                "seconds": request.interval_seconds,
                "misfire_grace_seconds": request.misfire_grace_seconds,
            }
            next_fire_at = effective_from + timedelta(
                seconds=request.interval_seconds
            )
        else:
            if request.cron_expression is None:
                raise InvalidScheduleRule("cron_expression is required")
            rule = {
                "expression": request.cron_expression,
                "misfire_grace_seconds": request.misfire_grace_seconds,
            }
            next_fire_at = next_cron_occurrence(
                request.cron_expression,
                request.timezone,
                after=effective_from,
            )
    except InvalidScheduleRule as error:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Schedule configuration is invalid",
            status_code=422,
        ) from error

    now = datetime.now(UTC)
    async with session.begin():
        schedule, actor = await _owned_schedule(
            session, claims, schedule_id, for_update=True
        )
        _require_revision(schedule, request.expected_revision)
        if schedule.handler_name != "cleanup_expired_uploads":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Only the registered upload cleanup schedule can be configured",
                status_code=409,
            )
        if schedule.status == "completed":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Completed schedule cannot be configured",
                status_code=409,
            )
        schedule.kind = request.kind
        schedule.rule = rule
        schedule.timezone = request.timezone
        schedule.next_fire_at = next_fire_at
        schedule.next_attempt_at = None
        schedule.misfire_policy = request.misfire_policy
        schedule.max_catch_up = request.max_catch_up
        schedule.failure_count = 0
        schedule.last_error = None
        schedule.lease_until = None
        schedule.leased_by = None
        if schedule.status == "manual_attention":
            schedule.status = "paused"
        schedule.revision += 1
        schedule.updated_at = now
        append_audit_event(
            session,
            workspace_id=schedule.workspace_id,
            actor_id=actor.user_id,
            action="schedule.configured",
            target_type="schedule",
            target_id=schedule.id,
            trace_id=trace_id,
            metadata={
                "revision": schedule.revision,
                "handler_name": schedule.handler_name,
                "kind": schedule.kind,
                "timezone": schedule.timezone,
                "misfire_policy": schedule.misfire_policy,
                "max_catch_up": schedule.max_catch_up,
            },
            occurred_at=now,
        )
        await session.flush()
    return _response(schedule)


async def pause_schedule(
    session: AsyncSession,
    claims: AccessTokenClaims,
    schedule_id: UUID,
    request: ScheduleStateRequest,
    *,
    trace_id: str,
) -> ScheduleResponse:
    now = datetime.now(UTC)
    async with session.begin():
        schedule, actor = await _owned_schedule(
            session, claims, schedule_id, for_update=True
        )
        _require_revision(schedule, request.expected_revision)
        if schedule.status == "completed":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Completed schedule cannot be paused",
                status_code=409,
            )
        schedule.status = "paused"
        schedule.lease_until = None
        schedule.leased_by = None
        schedule.revision += 1
        schedule.updated_at = now
        append_audit_event(
            session,
            workspace_id=schedule.workspace_id,
            actor_id=actor.user_id,
            action="schedule.paused",
            target_type="schedule",
            target_id=schedule.id,
            trace_id=trace_id,
            metadata={"revision": schedule.revision, "handler_name": schedule.handler_name},
            occurred_at=now,
        )
        await session.flush()
    return _response(schedule)


async def resume_schedule(
    session: AsyncSession,
    claims: AccessTokenClaims,
    schedule_id: UUID,
    request: ScheduleResumeRequest,
    *,
    trace_id: str,
) -> ScheduleResponse:
    now = datetime.now(UTC)
    if request.resume_from.tzinfo is None:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "resume_from must include a timezone",
            status_code=422,
        )
    async with session.begin():
        schedule, actor = await _owned_schedule(
            session, claims, schedule_id, for_update=True
        )
        _require_revision(schedule, request.expected_revision)
        if schedule.status != "paused":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Only a paused schedule can be resumed",
                status_code=409,
            )
        resume_from = request.resume_from.astimezone(UTC)
        schedule.status = "active"
        if request.misfire_policy == "skip":
            try:
                if schedule.kind == "one_off":
                    if schedule.next_fire_at is None or schedule.next_fire_at < resume_from:
                        schedule.status = "completed"
                        schedule.next_fire_at = None
                elif schedule.kind == "interval":
                    seconds = int(schedule.rule.get("seconds", 0))
                    if seconds <= 0:
                        raise InvalidScheduleRule("invalid interval schedule rule")
                    current = schedule.next_fire_at or resume_from
                    if current <= resume_from:
                        elapsed = (resume_from - current).total_seconds()
                        periods = int(elapsed // seconds) + 1
                        schedule.next_fire_at = current + timedelta(
                            seconds=periods * seconds
                        )
                elif schedule.kind == "cron":
                    expression = schedule.rule.get("expression")
                    if not isinstance(expression, str):
                        raise InvalidScheduleRule("invalid cron schedule rule")
                    schedule.next_fire_at = next_cron_occurrence(
                        expression,
                        schedule.timezone,
                        after=resume_from,
                    )
                else:
                    raise InvalidScheduleRule("unsupported schedule kind")
            except InvalidScheduleRule as error:
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Schedule rule requires configuration before resume",
                    status_code=409,
                ) from error
        elif schedule.next_fire_at is None or resume_from < schedule.next_fire_at:
            schedule.next_fire_at = resume_from
        schedule.next_attempt_at = None
        schedule.misfire_policy = request.misfire_policy
        schedule.max_catch_up = request.max_catch_up
        schedule.failure_count = 0
        schedule.last_error = None
        schedule.revision += 1
        schedule.updated_at = now
        append_audit_event(
            session,
            workspace_id=schedule.workspace_id,
            actor_id=actor.user_id,
            action="schedule.resumed",
            target_type="schedule",
            target_id=schedule.id,
            trace_id=trace_id,
            metadata={
                "revision": schedule.revision,
                "handler_name": schedule.handler_name,
                "misfire_policy": schedule.misfire_policy,
                "max_catch_up": schedule.max_catch_up,
            },
            occurred_at=now,
        )
        await session.flush()
    return _response(schedule)


async def _dispatch_fire(
    session: AsyncSession,
    schedule: Schedule,
    *,
    fire_key: str,
    scheduled_for: datetime,
    trigger_kind: Literal["scheduled", "manual"],
    trace_id: str,
    dispatched_at: datetime,
) -> tuple[ScheduleFireResponse, bool]:
    existing = await repository.find_fire(session, schedule.id, fire_key)
    if existing is not None:
        task = await get_internal_task(session, existing.task_id)
        return (
            ScheduleFireResponse(
                id=existing.id,
                schedule_id=existing.schedule_id,
                scheduled_for=existing.scheduled_for,
                trigger_kind=cast(
                    Literal["scheduled", "manual"], existing.trigger_kind
                ),
                task=task,
            ),
            False,
        )
    payload = parse_schedule_payload(schedule.handler_name, schedule.payload)
    if isinstance(payload, UploadExpirationPayload):
        dispatch = await create_upload_expiration_task(
            session,
            UploadExpirationTaskCommand(
                workspace_id=schedule.workspace_id,
                upload_session_id=payload.upload_session_id,
                requested_by=schedule.created_by,
                idempotency_key=f"schedule-fire:{fire_key}",
            ),
            trace_id=trace_id,
        )
    elif isinstance(payload, UploadCleanupPayload):
        if payload.workspace_id != schedule.workspace_id:
            raise InvalidSchedulePayload("cleanup schedule workspace mismatch")
        dispatch = await create_upload_cleanup_task(
            session,
            UploadCleanupTaskCommand(
                workspace_id=schedule.workspace_id,
                requested_by=schedule.created_by,
                idempotency_key=f"schedule-fire:{fire_key}",
            ),
            trace_id=trace_id,
        )
    else:
        dispatch = await create_media_location_retirement_task(
            session,
            MediaLocationRetirementTaskCommand(
                workspace_id=schedule.workspace_id,
                media_location_id=payload.media_location_id,
                requested_by=schedule.created_by,
                idempotency_key=f"schedule-fire:{fire_key}",
            ),
            trace_id=trace_id,
        )
    fire = ScheduleFire(
        id=uuid7(),
        workspace_id=schedule.workspace_id,
        schedule_id=schedule.id,
        fire_key=fire_key,
        scheduled_for=scheduled_for,
        trigger_kind=trigger_kind,
        status="dispatched",
        task_id=dispatch.task.id,
        outbox_event_id=dispatch.outbox_event_id,
        trace_id=trace_id,
        created_at=dispatched_at,
    )
    session.add(fire)
    await session.flush()
    return (
        ScheduleFireResponse(
            id=fire.id,
            schedule_id=fire.schedule_id,
            scheduled_for=fire.scheduled_for,
            trigger_kind=trigger_kind,
            task=dispatch.task,
        ),
        True,
    )


async def trigger_schedule(
    session: AsyncSession,
    claims: AccessTokenClaims,
    schedule_id: UUID,
    request: ScheduleTriggerRequest,
    *,
    trace_id: str,
) -> ScheduleFireResponse:
    now = datetime.now(UTC)
    fire_key = f"manual:{hashlib.sha256(request.idempotency_key.encode()).hexdigest()}"
    handler_label: str | None = None
    async with session.begin():
        schedule, actor = await _owned_schedule(
            session, claims, schedule_id, for_update=True
        )
        _require_revision(schedule, request.expected_revision)
        if schedule.status == "completed":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Completed schedule cannot be triggered",
                status_code=409,
            )
        result, created = await _dispatch_fire(
            session,
            schedule,
            fire_key=fire_key,
            scheduled_for=now,
            trigger_kind="manual",
            trace_id=trace_id,
            dispatched_at=now,
        )
        if created:
            handler_label = public_handler_name(schedule.handler_name)
            append_audit_event(
                session,
                workspace_id=schedule.workspace_id,
                actor_id=actor.user_id,
                action="schedule.triggered",
                target_type="schedule",
                target_id=schedule.id,
                trace_id=trace_id,
                metadata={"revision": schedule.revision, "fire_id": str(result.id)},
                occurred_at=now,
            )
        await session.flush()
    if handler_label is not None:
        SCHEDULE_DISPATCH_RESULTS.labels(
            handler=handler_label,
            result="manual",
        ).inc()
    return result


async def dispatch_claimed_schedule(
    session: AsyncSession,
    schedule_id: UUID,
    *,
    dispatcher_id: str,
    now: datetime,
) -> ScheduleDispatchOutcome:
    schedule = await repository.find_claimed_schedule(
        session, schedule_id, dispatcher_id=dispatcher_id
    )
    if (
        schedule is None
        or schedule.status != "active"
        or schedule.next_fire_at is None
        or schedule.next_fire_at > now
    ):
        return ScheduleDispatchOutcome(dispatched_count=0)
    plan = plan_due_occurrences(
        kind=cast(Literal["one_off", "interval", "cron"], schedule.kind),
        rule=schedule.rule,
        timezone=schedule.timezone,
        next_fire_at=schedule.next_fire_at,
        misfire_policy=cast(
            Literal["skip", "run_once", "catch_up"], schedule.misfire_policy
        ),
        max_catch_up=schedule.max_catch_up,
        now=now,
    )
    handler_label = public_handler_name(schedule.handler_name)
    lag_seconds = max(0.0, (now - schedule.next_fire_at).total_seconds())
    dispatched = 0
    for scheduled_for in plan.fire_times:
        normalized = scheduled_for.astimezone(UTC).isoformat().replace(
            "+00:00", "Z"
        )
        _, created = await _dispatch_fire(
            session,
            schedule,
            fire_key=f"scheduled:{normalized}",
            scheduled_for=scheduled_for,
            trigger_kind="scheduled",
            trace_id=f"schedule:{uuid7()}",
            dispatched_at=now,
        )
        dispatched += int(created)
    schedule.status = "completed" if plan.completed else "active"
    schedule.next_fire_at = plan.next_fire_at
    schedule.next_attempt_at = None
    schedule.failure_count = 0
    schedule.last_error = None
    schedule.lease_until = None
    schedule.leased_by = None
    schedule.revision += 1
    schedule.updated_at = now
    await session.flush()
    result_label = "dispatched" if dispatched else "skipped"
    return ScheduleDispatchOutcome(
        dispatched_count=dispatched,
        handler=handler_label,
        result=result_label,
        metric_count=dispatched or 1,
        lag_seconds=lag_seconds,
        misfire_count=plan.misfire_count,
        misfire_policy=cast(
            Literal["skip", "run_once", "catch_up"], schedule.misfire_policy
        ),
    )


async def record_dispatch_failure(
    session: AsyncSession,
    schedule_id: UUID,
    *,
    dispatcher_id: str,
    now: datetime,
    error: Exception,
) -> ScheduleFailureOutcome:
    schedule = await repository.find_claimed_schedule(
        session, schedule_id, dispatcher_id=dispatcher_id
    )
    if schedule is None:
        return ScheduleFailureOutcome(manual_attention=False)
    schedule.failure_count += 1
    schedule.last_error = type(error).__name__
    unrecoverable = isinstance(
        error,
        (InvalidSchedulePayload, InvalidScheduleRule, UnregisteredScheduleHandler),
    )
    schedule.lease_until = None
    schedule.leased_by = None
    if unrecoverable or schedule.failure_count >= 5:
        schedule.status = "manual_attention"
        schedule.next_attempt_at = None
        reason = "configuration" if unrecoverable else "retry_exhausted"
        metric_reason: Literal["configuration", "retry_exhausted"] | None = reason
    else:
        metric_reason = None
        schedule.next_attempt_at = now + timedelta(
            seconds=min(2**schedule.failure_count, 300)
        )
    schedule.revision += 1
    schedule.updated_at = now
    await session.flush()
    return ScheduleFailureOutcome(
        manual_attention=schedule.status == "manual_attention",
        handler=public_handler_name(schedule.handler_name),
        reason=metric_reason,
    )
