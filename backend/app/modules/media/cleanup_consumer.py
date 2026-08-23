from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.media import repository
from app.modules.media.storage import ObjectStoragePort, StorageObjectNotFound
from app.modules.messaging import (
    MessageEnvelope,
    finish_inbox_delivery,
    start_inbox_delivery,
)
from app.modules.production import (
    complete_upload_cleanup_task,
    fail_upload_cleanup_task,
    lock_task,
)

UPLOAD_CLEANUP_CONSUMER = "lanverse.media.upload-cleanup.v1"
UploadCleanupResult = Literal["completed", "duplicate", "rejected"]


async def _reject(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    task_id: UUID | None,
    error_code: str,
    now: datetime,
) -> UploadCleanupResult:
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=UPLOAD_CLEANUP_CONSUMER,
        task_id=task_id,
        status="rejected",
        error_code=error_code,
        now=now,
    )
    return "rejected"


async def consume_upload_cleanup(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    storage: ObjectStoragePort,
    batch_size: int,
) -> UploadCleanupResult:
    now = datetime.now(UTC)
    is_first = await start_inbox_delivery(
        session,
        envelope,
        consumer_name=UPLOAD_CLEANUP_CONSUMER,
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
        envelope.event_type != "upload_cleanup.requested"
        or envelope.schema_version != 1
        or envelope.payload != {"task_id": str(envelope.aggregate_id)}
        or task.task_type != "upload_cleanup"
        or task.request_type != "workspace"
        or task.request_id != task.workspace_id
    ):
        await fail_upload_cleanup_task(
            session,
            task.id,
            error_code="invalid_upload_cleanup_message",
            error_summary="Upload cleanup message is invalid",
            now=now,
            trace_id=envelope.trace_id,
        )
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="invalid_upload_cleanup_message",
            now=now,
        )

    uploads = await repository.lock_expired_pending_uploads(
        session,
        task.workspace_id,
        now=now,
        limit=batch_size,
    )
    for upload in uploads:
        try:
            await storage.delete(upload.object_key)
        except StorageObjectNotFound:
            pass
        upload.status = "expired"
        upload.error_code = "upload_expired"
        upload.updated_at = now

    await complete_upload_cleanup_task(
        session,
        task.id,
        cleaned_count=len(uploads),
        now=now,
        trace_id=envelope.trace_id,
    )
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=UPLOAD_CLEANUP_CONSUMER,
        task_id=task.id,
        status="completed",
        error_code=None,
        now=now,
    )
    await session.flush()
    return "completed"
