from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.media import repository
from app.modules.media.storage import (
    ObjectStoragePort,
    StorageObjectNotFound,
)
from app.modules.messaging import (
    MessageEnvelope,
    finish_inbox_delivery,
    start_inbox_delivery,
)
from app.modules.production import (
    complete_upload_expiration_task,
    fail_upload_expiration_task,
    lock_task,
)

UPLOAD_EXPIRATION_CONSUMER = "lanverse.media.upload-expiration.v1"
UploadExpirationResult = Literal["completed", "duplicate", "rejected"]


async def _reject(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    task_id: UUID | None,
    error_code: str,
    now: datetime,
) -> UploadExpirationResult:
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=UPLOAD_EXPIRATION_CONSUMER,
        task_id=task_id,
        status="rejected",
        error_code=error_code,
        now=now,
    )
    return "rejected"


async def consume_upload_expiration(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    storage: ObjectStoragePort,
) -> UploadExpirationResult:
    now = datetime.now(UTC)
    is_first = await start_inbox_delivery(
        session,
        envelope,
        consumer_name=UPLOAD_EXPIRATION_CONSUMER,
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
        envelope.event_type != "upload_expiration.requested"
        or envelope.schema_version != 1
        or envelope.payload != {"task_id": str(envelope.aggregate_id)}
        or task.task_type != "upload_expiration"
        or task.request_type != "upload_session"
    ):
        await fail_upload_expiration_task(
            session,
            task.id,
            error_code="invalid_upload_expiration_message",
            error_summary="Upload expiration message is invalid",
            now=now,
            trace_id=envelope.trace_id,
        )
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="invalid_upload_expiration_message",
            now=now,
        )

    upload = await repository.find_upload_session(session, task.request_id, for_update=True)
    if upload is None or upload.workspace_id != task.workspace_id:
        await fail_upload_expiration_task(
            session,
            task.id,
            error_code="upload_session_unavailable",
            error_summary="Upload session is unavailable",
            now=now,
            trace_id=envelope.trace_id,
        )
        return await _reject(
            session,
            envelope,
            task_id=task.id,
            error_code="upload_session_unavailable",
            now=now,
        )

    should_delete = upload.status in {"expired", "failed"} or (
        upload.status == "pending" and upload.expires_at <= now
    )
    if should_delete:
        try:
            await storage.delete(upload.object_key)
        except StorageObjectNotFound:
            pass
        if upload.status == "pending":
            upload.status = "expired"
            upload.error_code = "upload_expired"
            upload.updated_at = now

    await complete_upload_expiration_task(
        session,
        task.id,
        now=now,
        trace_id=envelope.trace_id,
    )
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=UPLOAD_EXPIRATION_CONSUMER,
        task_id=task.id,
        status="completed",
        error_code=None,
        now=now,
    )
    await session.flush()
    return "completed"
