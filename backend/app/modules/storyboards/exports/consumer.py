from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.media import (
    ObjectStoragePort,
    RenderedMediaCommand,
    RenderedMediaSource,
    RenderedMediaSourceType,
    StorageIntegrityMismatch,
    register_rendered_media,
    verify_object_integrity,
)
from app.modules.messaging import (
    MessageEnvelope,
    finish_inbox_delivery,
    start_inbox_delivery,
)
from app.modules.production import (
    complete_storyboard_export_task,
    fail_storyboard_export_task,
    lock_task,
    start_storyboard_export_task,
)
from app.modules.storyboards.exports import repository
from app.modules.storyboards.exports.contracts import ExportSnapshot
from app.modules.storyboards.exports.models import StoryboardExportManifest
from app.modules.storyboards.exports.package import build_storyboard_package

EXPORT_CONSUMER = "lanverse.media.storyboard-export.v1"
ExportConsumerResult = Literal["completed", "duplicate", "rejected"]


def _lineage(
    job_id: UUID,
    input_hash: str,
    snapshot: ExportSnapshot,
) -> tuple[RenderedMediaSource, ...]:
    values: list[tuple[RenderedMediaSourceType, UUID, str]] = [
        ("script_version", snapshot.script_version_id, snapshot.script_content_hash),
    ]
    values.extend(
        ("narrative_unit_version", item.unit_version_id, item.text_hash)
        for item in snapshot.units
    )
    values.extend(
        ("asset_version", item.asset_version_id, item.content_hash)
        for item in snapshot.assets
    )
    values.extend(
        ("shot_spec_version", item.shot_spec_version_id, item.spec_content_hash)
        for item in snapshot.shots
    )
    values.extend(
        (
            (
                "storyboard_coverage",
                snapshot.episode_id,
                snapshot.coverage_evaluation_hash,
            ),
            (
                "storyboard_readiness",
                snapshot.episode_id,
                snapshot.readiness_evaluation_hash,
            ),
            ("storyboard_export_snapshot", job_id, input_hash),
        )
    )
    return tuple(
        RenderedMediaSource(
            source_type=value[0],
            source_id=value[1],
            source_hash=value[2],
            position=position,
        )
        for position, value in enumerate(values, start=1)
    )


async def _fail(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    task_id: UUID,
    job_id: UUID,
    error_code: str,
    error_summary: str,
    now: datetime,
) -> ExportConsumerResult:
    job = await repository.find_job(session, job_id, for_update=True)
    if job is not None and job.status not in {"succeeded", "failed"}:
        job.status = "failed"
        job.error_code = error_code
        job.error_summary = error_summary
        job.updated_at = now
    await fail_storyboard_export_task(
        session,
        task_id,
        error_code=error_code,
        error_summary=error_summary,
        next_action="create_new_storyboard_export",
        now=now,
        trace_id=envelope.trace_id,
    )
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=EXPORT_CONSUMER,
        task_id=task_id,
        status="completed",
        error_code=None,
        now=now,
    )
    return "completed"


async def consume_storyboard_export(
    session: AsyncSession,
    envelope: MessageEnvelope,
    *,
    storage: ObjectStoragePort,
    storage_profile: str,
    storage_bucket: str,
) -> ExportConsumerResult:
    now = datetime.now(UTC)
    is_first = await start_inbox_delivery(
        session,
        envelope,
        consumer_name=EXPORT_CONSUMER,
        now=now,
    )
    if not is_first:
        return "duplicate"
    task = await lock_task(session, envelope.aggregate_id)
    if task is None:
        await finish_inbox_delivery(
            session,
            envelope,
            consumer_name=EXPORT_CONSUMER,
            task_id=None,
            status="rejected",
            error_code="task_not_found",
            now=now,
        )
        return "rejected"
    valid_message = (
        envelope.event_type == "storyboard_export.requested"
        and envelope.schema_version == 1
        and envelope.payload == {"task_id": str(envelope.aggregate_id)}
        and task.workspace_id == envelope.workspace_id
        and task.task_type == "storyboard_export"
        and task.request_type == "storyboard_export_job"
        and task.usage_type == "storyboard_export"
        and task.usage_id == task.request_id
    )
    if not valid_message:
        await finish_inbox_delivery(
            session,
            envelope,
            consumer_name=EXPORT_CONSUMER,
            task_id=task.id,
            status="rejected",
            error_code="invalid_storyboard_export_message",
            now=now,
        )
        return "rejected"
    job = await repository.find_job(session, task.request_id, for_update=True)
    if (
        job is None
        or job.workspace_id != task.workspace_id
        or job.task_id != task.id
        or job.input_hash != task.input_hash
    ):
        return await _fail(
            session,
            envelope,
            task_id=task.id,
            job_id=task.request_id,
            error_code="storyboard_export_job_unavailable",
            error_summary="Storyboard export job does not match its task",
            now=now,
        )

    await start_storyboard_export_task(
        session,
        task.id,
        now=now,
        trace_id=envelope.trace_id,
    )
    job.status = "running"
    job.updated_at = now
    try:
        snapshot = ExportSnapshot.model_validate(job.input_snapshot)
        package = build_storyboard_package(snapshot, job.input_hash)
    except (ValueError, TypeError) as error:
        return await _fail(
            session,
            envelope,
            task_id=task.id,
            job_id=job.id,
            error_code="storyboard_export_snapshot_invalid",
            error_summary=str(error),
            now=now,
        )

    object_key = f"exports/{job.workspace_id}/{job.id}.zip"
    await storage.ensure_bucket()
    await storage.put(object_key, package.content, "application/zip")
    try:
        await verify_object_integrity(
            storage,
            object_key,
            expected_size_bytes=package.size_bytes,
            expected_sha256=package.sha256,
            expected_content_type="application/zip",
        )
    except StorageIntegrityMismatch:
        return await _fail(
            session,
            envelope,
            task_id=task.id,
            job_id=job.id,
            error_code="storyboard_export_integrity_failed",
            error_summary="Stored storyboard package does not match its declaration",
            now=now,
        )

    sources = _lineage(job.id, job.input_hash, snapshot)
    rendered = await register_rendered_media(
        session,
        RenderedMediaCommand(
            workspace_id=job.workspace_id,
            filename=f"storyboard-{job.episode_id}.zip",
            sha256=package.sha256,
            size_bytes=package.size_bytes,
            mime_type="application/zip",
            storage_profile=storage_profile,
            bucket=storage_bucket,
            object_key=object_key,
            created_by=job.created_by,
            sources=sources,
        ),
        trace_id=envelope.trace_id,
    )
    session.add(
        StoryboardExportManifest(
            id=uuid7(),
            workspace_id=job.workspace_id,
            episode_id=job.episode_id,
            job_id=job.id,
            schema_version=1,
            input_hash=job.input_hash,
            input_snapshot=job.input_snapshot,
            file_manifest={
                "schema_label": "lanverse.storyboard.export.files.1",
                "files": [value.model_dump(mode="json") for value in package.files],
            },
            media_version_id=rendered.media_version_id,
            package_sha256=package.sha256,
            package_size_bytes=package.size_bytes,
            created_by=job.created_by,
            created_at=now,
        )
    )
    job.status = "succeeded"
    job.error_code = None
    job.error_summary = None
    job.updated_at = now
    await complete_storyboard_export_task(
        session,
        task.id,
        now=now,
        trace_id=envelope.trace_id,
    )
    await finish_inbox_delivery(
        session,
        envelope,
        consumer_name=EXPORT_CONSUMER,
        task_id=task.id,
        status="completed",
        error_code=None,
        now=now,
    )
    await session.flush()
    return "completed"
