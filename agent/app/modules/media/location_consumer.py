from datetime import UTC, datetime, timedelta
from typing import Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.governance.audit import append_audit_event
from app.modules.media import repository
from app.modules.media.models import MediaLocation, MediaVersion
from app.modules.media.storage import (
    ObjectStoragePort,
    StorageIntegrityMismatch,
    StorageObjectNotFound,
    verify_object_integrity,
)
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
from app.modules.scheduling import (
    complete_media_location_retirement_schedule,
    ensure_media_location_retirement_schedule,
)

MEDIA_LOCATION_CONSUMER = "lanverse.media.location-migration.v1"
MediaLocationConsumerResult = Literal["completed", "duplicate", "rejected"]


async def _reject(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    task_id: UUID | None,
    error_code: str,
    now: datetime,
) -> MediaLocationConsumerResult:
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=MEDIA_LOCATION_CONSUMER,
        task_id=task_id,
        status="rejected",
        error_code=error_code,
        now=now,
    )
    return "rejected"


async def _verified_hash(
    storage: ObjectStoragePort,
    object_key: str,
    version: MediaVersion,
) -> bool:
    try:
        await verify_object_integrity(
            storage,
            object_key,
            expected_size_bytes=version.size_bytes,
            expected_sha256=version.sha256,
            expected_content_type=version.mime_type,
        )
    except StorageIntegrityMismatch:
        return False
    return True


async def _integrity_failure(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    task_id: UUID,
    now: datetime,
) -> MediaLocationConsumerResult:
    await fail_media_location_task(
        session,
        task_id,
        error_code="media_location_integrity_failed",
        error_summary="Copied media bytes do not match the immutable media version",
        now=now,
        trace_id=envelope.trace_id,
        retryable=False,
        next_action="retry_location_migration",
    )
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=MEDIA_LOCATION_CONSUMER,
        task_id=task_id,
        status="completed",
        error_code=None,
        now=now,
    )
    return "completed"


async def consume_media_location_migration(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    storage: ObjectStoragePort,
    storage_profile: str,
    storage_bucket: str,
    rollback_seconds: int,
) -> MediaLocationConsumerResult:
    now = datetime.now(UTC)
    is_first = await start_inbox_delivery(
        session,
        envelope,
        consumer_name=MEDIA_LOCATION_CONSUMER,
        now=now,
    )
    if not is_first:
        return "duplicate"
    task = await lock_task(session, envelope.aggregate_id)
    if task is None:
        return await _reject(
            session,
            envelope,
            task_id=None,
            error_code="task_not_found",
            now=now,
        )
    if task.workspace_id != envelope.workspace_id:
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="workspace_mismatch",
            now=now,
        )
    if (
        envelope.event_type != "media_location_migration.requested"
        or envelope.schema_version != 1
        or envelope.payload != {"task_id": str(envelope.aggregate_id)}
        or task.task_type != "media_location_migration"
        or task.request_type != "media_version"
        or task.usage_type not in {"media_location_migration", "media_location_rollback"}
        or task.usage_id is None
    ):
        await fail_media_location_task(
            session,
            task.id,
            error_code="invalid_media_location_message",
            error_summary="Media location migration message is invalid",
            now=now,
            trace_id=envelope.trace_id,
            retryable=False,
            next_action="contact_support",
        )
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="invalid_media_location_message",
            now=now,
        )

    found = await repository.find_media_version(session, task.request_id, for_update=True)
    location = await repository.find_media_location(session, task.usage_id, for_update=True)
    if (
        found is None
        or found[0].workspace_id != task.workspace_id
        or location is None
        or location.workspace_id != task.workspace_id
        or location.media_version_id != task.request_id
    ):
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
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="media_location_unavailable",
            now=now,
        )
    version = found[0]
    if location.storage_profile != storage_profile or location.bucket != storage_bucket:
        await fail_media_location_task(
            session,
            task.id,
            error_code="storage_profile_unavailable",
            error_summary="Media location is not managed by the configured storage profile",
            now=now,
            trace_id=envelope.trace_id,
            retryable=False,
            next_action="contact_support",
        )
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="storage_profile_unavailable",
            now=now,
        )

    if task.usage_type == "media_location_migration":
        active = await repository.find_active_location(session, version.id, for_update=True)
        if active is None or active.id != location.id or location.status != "active":
            await fail_media_location_task(
                session,
                task.id,
                error_code="media_location_changed",
                error_summary="Active media location changed before migration",
                now=now,
                trace_id=envelope.trace_id,
                retryable=False,
                next_action="request_location_migration",
            )
            return await _reject(
                session,
                envelope,
                task_id=task.id,
                error_code="media_location_changed",
                now=now,
            )
        target_key = f"workspaces/{version.workspace_id}/media/{version.id}/locations/{task.id}"
        try:
            await storage.copy(location.object_key, target_key)
            valid = await _verified_hash(storage, target_key, version)
        except StorageObjectNotFound:
            location.status = "quarantined"
            await fail_media_location_task(
                session,
                task.id,
                error_code="media_location_missing",
                error_summary="Confirmed media bytes are unavailable",
                now=now,
                trace_id=envelope.trace_id,
                retryable=False,
                next_action="contact_support",
            )
            return await _reject(
                session,
                envelope,
                task_id=task.id,
                error_code="media_location_missing",
                now=now,
            )
        if not valid:
            try:
                await storage.delete(target_key)
            except StorageObjectNotFound:
                pass
            return await _integrity_failure(session, envelope, task_id=task.id, now=now)
        target = MediaLocation(
            id=uuid7(),
            workspace_id=version.workspace_id,
            media_version_id=version.id,
            storage_profile=storage_profile,
            bucket=storage_bucket,
            object_key=target_key,
            status="verified",
            verified_at=now,
            migration_task_id=task.id,
            created_at=now,
        )
        session.add(target)
        await session.flush()
        retire_after = now + timedelta(seconds=rollback_seconds)
        location.status = "retiring"
        location.retire_after = retire_after
        await session.flush()
        target.status = "active"
        await ensure_media_location_retirement_schedule(
            session,
            workspace_id=version.workspace_id,
            media_location_id=location.id,
            created_by=task.requested_by,
            retire_after=retire_after,
            now=now,
        )
        action = "media.location_migrated"
        previous_location_id = location.id
        active_location_id = target.id
    else:
        if (
            location.status != "retiring"
            or location.retire_after is None
            or location.retire_after <= now
        ):
            await fail_media_location_task(
                session,
                task.id,
                error_code="media_location_rollback_unavailable",
                error_summary="Media location is outside its rollback window",
                now=now,
                trace_id=envelope.trace_id,
                retryable=False,
                next_action="review_media_locations",
            )
            return await _reject(
                session,
                envelope,
                task_id=task.id,
                error_code="media_location_rollback_unavailable",
                now=now,
            )
        try:
            valid = await _verified_hash(storage, location.object_key, version)
        except StorageObjectNotFound:
            location.status = "quarantined"
            valid = False
        if not valid:
            return await _integrity_failure(session, envelope, task_id=task.id, now=now)
        active = await repository.find_active_location(session, version.id, for_update=True)
        if active is None or active.id == location.id:
            raise RuntimeError("rollback requires a different active media location")
        if active.storage_profile != storage_profile or active.bucket != storage_bucket:
            raise RuntimeError("active media location uses an unavailable profile")
        retire_after = now + timedelta(seconds=rollback_seconds)
        active.status = "retiring"
        active.retire_after = retire_after
        await session.flush()
        location.status = "active"
        location.retire_after = None
        location.retired_at = None
        await complete_media_location_retirement_schedule(
            session,
            workspace_id=version.workspace_id,
            media_location_id=location.id,
            now=now,
        )
        await ensure_media_location_retirement_schedule(
            session,
            workspace_id=version.workspace_id,
            media_location_id=active.id,
            created_by=task.requested_by,
            retire_after=retire_after,
            now=now,
        )
        action = "media.location_rolled_back"
        previous_location_id = active.id
        active_location_id = location.id

    await complete_media_location_task(
        session,
        task.id,
        now=now,
        trace_id=envelope.trace_id,
    )
    append_audit_event(
        session,
        workspace_id=version.workspace_id,
        actor_id=task.requested_by,
        action=action,
        target_type="media_version",
        target_id=version.id,
        trace_id=envelope.trace_id,
        metadata={
            "previous_location_id": str(previous_location_id),
            "active_location_id": str(active_location_id),
            "media_version_id": str(version.id),
        },
        occurred_at=now,
    )
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=MEDIA_LOCATION_CONSUMER,
        task_id=task.id,
        status="completed",
        error_code=None,
        now=now,
    )
    await session.flush()
    return "completed"
