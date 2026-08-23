from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.media import MediaProbeError, MediaProbePort, repository
from app.modules.media.storage import (
    ObjectStoragePort,
    StorageObjectNotFound,
    StorageUnavailable,
)
from app.modules.messaging import (
    MessageEnvelope,
    finish_inbox_delivery,
    start_inbox_delivery,
)
from app.modules.production import (
    complete_media_probe_task,
    fail_media_probe_task,
    lock_task,
)

MEDIA_PROBE_CONSUMER = "lanverse.media.probe.v1"
MediaConsumerResult = Literal["completed", "duplicate", "rejected"]


async def _reject(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    task_id: UUID | None,
    error_code: str,
    now: datetime,
) -> MediaConsumerResult:
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=MEDIA_PROBE_CONSUMER,
        task_id=task_id,
        status="rejected",
        error_code=error_code,
        now=now,
    )
    return "rejected"


async def consume_media_probe(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    storage: ObjectStoragePort,
    probe: MediaProbePort,
) -> MediaConsumerResult:
    now = datetime.now(UTC)
    is_first = await start_inbox_delivery(
        session,
        envelope,
        consumer_name=MEDIA_PROBE_CONSUMER,
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
        envelope.event_type != "media_probe.requested"
        or envelope.schema_version != 1
        or envelope.payload != {"task_id": str(envelope.aggregate_id)}
    ):
        await fail_media_probe_task(
            session,
            task.id,
            error_code="invalid_probe_message",
            error_summary="Media probe message is invalid",
            now=now,
            trace_id=envelope.trace_id,
            retryable=False,
            next_action="contact_support",
        )
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="invalid_probe_message",
            now=now,
        )
    if task.task_type != "media_probe" or task.request_type != "media_version":
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="unsupported_task_type",
            now=now,
        )

    found = await repository.find_media_version(session, task.request_id, for_update=True)
    if found is None:
        await fail_media_probe_task(
            session,
            task.id,
            error_code="media_version_not_found",
            error_summary="Media version is unavailable",
            now=now,
            trace_id=envelope.trace_id,
            retryable=False,
            next_action="contact_support",
        )
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="media_version_not_found",
            now=now,
        )
    version, media_object = found
    if version.workspace_id != task.workspace_id or version.probe_task_id not in {
        None,
        task.id,
    }:
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="media_probe_scope_mismatch",
            now=now,
        )
    if version.probe_status == "ready":
        await complete_media_probe_task(
            session,
            task.id,
            now=now,
            trace_id=envelope.trace_id,
        )
    else:
        location = await repository.find_active_location(session, version.id)
        quarantined = False
        if location is None:
            error = MediaProbeError("media_location_unavailable", "Media location is unavailable")
            quarantined = True
        else:
            try:
                result = await probe.probe(
                    storage.stream(location.object_key),
                    kind=media_object.kind,
                    mime_type=version.mime_type,
                )
            except StorageUnavailable:
                error = MediaProbeError(
                    "object_storage_unavailable", "Object storage is unavailable"
                )
            except StorageObjectNotFound:
                error = MediaProbeError(
                    "media_object_missing", "Confirmed media bytes are unavailable"
                )
                quarantined = True
            except MediaProbeError as probe_error:
                error = probe_error
            else:
                error = None
                version.probe_status = "ready"
                version.probe_error_code = None
                version.probe_error_summary = None
                version.probe_next_action = None
                version.width = result.width
                version.height = result.height
                version.duration_ms = result.duration_ms
                version.codec = result.codec
                version.container = result.container
                await complete_media_probe_task(
                    session,
                    task.id,
                    now=now,
                    trace_id=envelope.trace_id,
                )
        if error is not None:
            version.probe_status = "quarantined" if quarantined else "failed"
            version.probe_error_code = error.code
            version.probe_error_summary = error.summary
            version.probe_next_action = "contact_support" if quarantined else "retry_probe"
            if quarantined and location is not None:
                location.status = "quarantined"
            await fail_media_probe_task(
                session,
                task.id,
                error_code=error.code,
                error_summary=error.summary,
                now=now,
                trace_id=envelope.trace_id,
                retryable=not quarantined,
                next_action="contact_support" if quarantined else "retry_probe",
            )
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=MEDIA_PROBE_CONSUMER,
        task_id=task.id,
        status="completed",
        error_code=None,
        now=now,
    )
    await session.flush()
    return "completed"
