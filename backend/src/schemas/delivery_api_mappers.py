from __future__ import annotations

from typing import Any

from schemas.deliveries import (
    DeliveryArtifactSnapshot,
    DeliveryDownloadAuthorization,
    DeliveryVersionSnapshot,
    DeliveryViewSnapshot,
)
from schemas.delivery_api import (
    DeliveryArtifactResponse,
    DeliveryDetailResponse,
    DeliveryLineageResponse,
    DeliverySummaryResponse,
    DownloadAuthorizationItem,
    DownloadAuthorizationResponse,
    RenderAttemptResponse,
    RenderSnapshotLineageResponse,
    RenderTaskLineageResponse,
)
from schemas.delivery_lineage import DeliveryDetailSnapshot, RenderAttemptSnapshot
from schemas.project_api import source_response
from schemas.rendering import RenderSnapshot
from schemas.script_api import script_response
from schemas.storyboard_api import asset_response, storyboard_response
from schemas.subtitle_api import subtitle_response


def artifact_response(value: DeliveryArtifactSnapshot) -> DeliveryArtifactResponse:
    return DeliveryArtifactResponse(
        artifact_type=value.artifact_type,
        media_version_id=value.media_version_id,
        source_kind=value.source_kind,
        mime_type=value.mime_type,
        byte_size=value.byte_size,
        sha256=value.sha256,
        width=value.width,
        height=value.height,
        duration_ticks=value.duration_ticks,
        timebase=value.timebase,
    )


def summary_response(value: DeliveryViewSnapshot) -> DeliverySummaryResponse:
    return DeliverySummaryResponse(
        **_delivery_fields(value.delivery),
        artifacts=tuple(artifact_response(item) for item in value.artifacts),
    )


def detail_response(value: DeliveryDetailSnapshot) -> DeliveryDetailResponse:
    lineage = value.lineage
    return DeliveryDetailResponse(
        **_delivery_fields(value.delivery),
        artifacts=tuple(artifact_response(item) for item in lineage.delivery_media),
        lineage=DeliveryLineageResponse(
            source_revision=source_response(lineage.source_revision),
            script_version=script_response(lineage.script_version),
            creative_asset_versions=tuple(
                asset_response(item) for item in lineage.creative_asset_versions
            ),
            shot_spec_version=storyboard_response(lineage.shot_spec_version),
            subtitle_version=subtitle_response(lineage.subtitle_version),
            render_snapshot=_render_snapshot(lineage.render_snapshot),
            render_task=RenderTaskLineageResponse(
                id=lineage.render_task.id,
                submission_snapshot_id=lineage.render_task.snapshot_id,
                status=lineage.render_task.status,
                resource_version=lineage.render_task.resource_version,
                retry_of_task_id=lineage.render_task.retry_of_task_id,
                created_at=lineage.render_task.created_at,
                updated_at=lineage.render_task.updated_at,
                finished_at=lineage.render_task.finished_at,
            ),
            render_attempts=tuple(_attempt(item) for item in lineage.render_attempts),
            input_media=lineage.input_media,
            delivery_media=tuple(artifact_response(item) for item in lineage.delivery_media),
        ),
    )


def authorization_response(
    values: tuple[DeliveryDownloadAuthorization, ...],
) -> DownloadAuthorizationResponse:
    return DownloadAuthorizationResponse(
        items=tuple(
            DownloadAuthorizationItem(
                artifact_type=item.artifact_type,
                media_version_id=item.media_version_id,
                url=item.url,
                expires_in_seconds=900,
                expires_at=item.expires_at,
            )
            for item in values
        )
    )


def _delivery_fields(value: DeliveryVersionSnapshot) -> dict[str, Any]:
    return {
        "id": value.id,
        "episode_id": value.episode_id,
        "version": value.version,
        "render_task_id": value.render_task_id,
        "final_attempt_id": value.final_attempt_id,
        "retry_of_delivery_id": value.retry_of_delivery_id,
        "render_snapshot_id": value.render_snapshot_id,
        "status": value.status,
        "ffmpeg_version": value.ffmpeg_version,
        "ffprobe_summary": value.ffprobe_summary,
        "error_code": value.error_code,
        "created_at": value.created_at,
        "updated_at": value.updated_at,
        "finished_at": value.finished_at,
    }


def _render_snapshot(value: RenderSnapshot) -> RenderSnapshotLineageResponse:
    return RenderSnapshotLineageResponse(
        id=value.id,
        episode_id=value.episode_id,
        initial_task_id=value.initial_task_id,
        input_refs=value.input_refs,
        segments=value.segments,
        recipe=value.recipe,
        recipe_hash=value.recipe_hash,
        content_hash=value.content_hash,
        created_at=value.created_at,
    )


def _attempt(value: RenderAttemptSnapshot) -> RenderAttemptResponse:
    return RenderAttemptResponse(
        id=value.id,
        task_id=value.task_id,
        submission_snapshot_id=value.snapshot_id,
        attempt_no=value.attempt_no,
        parent_attempt_id=value.parent_attempt_id,
        status=value.status,
        execution_metadata=value.execution_metadata,
        error_code=value.error_code,
        error_summary=value.error_summary,
        created_at=value.created_at,
        submitted_at=value.submitted_at,
        started_at=value.started_at,
        finished_at=value.finished_at,
    )
