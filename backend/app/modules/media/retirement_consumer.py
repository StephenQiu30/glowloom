from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.governance.audit import append_audit_event
from app.modules.media import repository
from app.modules.media.storage import ObjectStoragePort, StorageObjectNotFound
from app.modules.messaging import (
    MessageEnvelope,
    finish_inbox_delivery,
    start_inbox_delivery,
)
from app.modules.production import (
    complete_media_location_task,
    fail_media_location_task,
    lock_task,
)

MEDIA_LOCATION_RETIREMENT_CONSUMER = "lanverse.media.location-retirement.v1"
MediaLocationRetirementResult = Literal["completed", "duplicate", "rejected"]


async def _finish(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    task_id: UUID,
    now: datetime,
    status: Literal["completed", "rejected"],
    error_code: str | None,
) -> None:
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=MEDIA_LOCATION_RETIREMENT_CONSUMER,
        task_id=task_id,
        status=status,
        error_code=error_code,
        now=now,
    )


async def consume_media_location_retirement(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    storage: ObjectStoragePort,
    storage_profile: str,
    storage_bucket: str,
) -> MediaLocationRetirementResult:
    now = datetime.now(UTC)
    is_first = await start_inbox_delivery(
        session,
        envelope,
        consumer_name=MEDIA_LOCATION_RETIREMENT_CONSUMER,
        now=now,
    )
    if not is_first:
        return "duplicate"
    task = await lock_task(session, envelope.aggregate_id)
    if task is None:
        await finish_inbox_delivery(
            session,
            envelope,
            consumer_name=MEDIA_LOCATION_RETIREMENT_CONSUMER,
            task_id=None,
            status="rejected",
            error_code="task_not_found",
            now=now,
        )
        return "rejected"
    valid_message = (
        envelope.event_type == "media_location_retirement.requested"
        and envelope.schema_version == 1
        and envelope.payload == {"task_id": str(envelope.aggregate_id)}
        and task.workspace_id == envelope.workspace_id
        and task.task_type == "media_location_retirement"
        and task.request_type == "media_location"
        and task.request_id == task.usage_id
    )
    if not valid_message:
        await fail_media_location_task(
            session,
            task.id,
            error_code="invalid_media_location_retirement_message",
            error_summary="Media location retirement message is invalid",
            now=now,
            trace_id=envelope.trace_id,
            retryable=False,
            next_action="contact_support",
        )
        await _finish(
            session,
            envelope,
            task_id=task.id,
            now=now,
            status="rejected",
            error_code="invalid_media_location_retirement_message",
        )
        return "rejected"
    location = await repository.find_media_location(session, task.request_id, for_update=True)
    if location is None or location.workspace_id != task.workspace_id:
        await fail_media_location_task(
            session,
            task.id,
            error_code="media_location_unavailable",
            error_summary="Media location is unavailable",
            now=now,
            trace_id=envelope.trace_id,
            retryable=False,
            next_action="review_media_locations",
        )
        await _finish(
            session,
            envelope,
            task_id=task.id,
            now=now,
            status="rejected",
            error_code="media_location_unavailable",
        )
        return "rejected"
    if location.status == "retired":
        await complete_media_location_task(
            session, task.id, now=now, trace_id=envelope.trace_id, next_action=None
        )
        await _finish(
            session,
            envelope,
            task_id=task.id,
            now=now,
            status="completed",
            error_code=None,
        )
        return "completed"
    if (
        location.status != "retiring"
        or location.retire_after is None
        or location.retire_after > now
    ):
        await fail_media_location_task(
            session,
            task.id,
            error_code="media_location_retirement_not_due",
            error_summary="Media location is not eligible for retirement",
            now=now,
            trace_id=envelope.trace_id,
            retryable=False,
            next_action="wait_for_location_retirement",
        )
        await _finish(
            session,
            envelope,
            task_id=task.id,
            now=now,
            status="completed",
            error_code=None,
        )
        return "completed"
    active = await repository.find_active_location(
        session, location.media_version_id, for_update=True
    )
    if (
        active is None
        or active.id == location.id
        or active.verified_at is None
        or active.storage_profile != storage_profile
        or active.bucket != storage_bucket
        or location.storage_profile != storage_profile
        or location.bucket != storage_bucket
    ):
        raise RuntimeError("media location retirement safety gate is unavailable")
    try:
        await storage.delete(location.object_key)
    except StorageObjectNotFound:
        pass
    location.status = "retired"
    location.retired_at = now
    location.retire_after = None
    await complete_media_location_task(
        session, task.id, now=now, trace_id=envelope.trace_id, next_action=None
    )
    append_audit_event(
        session,
        workspace_id=location.workspace_id,
        actor_id=task.requested_by,
        action="media.location_retired",
        target_type="media_version",
        target_id=location.media_version_id,
        trace_id=envelope.trace_id,
        metadata={
            "media_location_id": str(location.id),
            "media_version_id": str(location.media_version_id),
        },
        occurred_at=now,
    )
    await _finish(
        session,
        envelope,
        task_id=task.id,
        now=now,
        status="completed",
        error_code=None,
    )
    await session.flush()
    return "completed"
