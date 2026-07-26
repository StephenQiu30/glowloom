from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Literal
from uuid import UUID

from schemas.delivery_quality import DeliveryProbeSummaryV1

DeliveryStatus = Literal["rendering", "ready", "failed", "cancelled"]
DeliveryArtifactType = Literal["mp4", "srt", "manifest"]


@dataclass(frozen=True, slots=True)
class DeliveryArtifactSnapshot:
    artifact_type: DeliveryArtifactType
    media_version_id: UUID
    source_kind: Literal["ffmpeg", "application"]
    mime_type: str
    byte_size: int
    sha256: str
    width: int | None
    height: int | None
    duration_ticks: int | None
    timebase: int | None
    bucket: str
    object_key: str


@dataclass(frozen=True, slots=True)
class DeliveryVersionSnapshot:
    id: UUID
    episode_id: UUID
    version: int
    render_task_id: UUID
    final_attempt_id: UUID | None
    retry_of_delivery_id: UUID | None
    render_snapshot_id: UUID
    mp4_media_version_id: UUID | None
    srt_media_version_id: UUID | None
    manifest_media_version_id: UUID | None
    ffmpeg_version: str | None
    ffprobe_summary: DeliveryProbeSummaryV1 | None
    status: DeliveryStatus
    error_code: str | None
    created_at: datetime
    updated_at: datetime
    finished_at: datetime | None


@dataclass(frozen=True, slots=True)
class DeliveryViewSnapshot:
    delivery: DeliveryVersionSnapshot
    artifacts: tuple[DeliveryArtifactSnapshot, ...]


@dataclass(frozen=True, slots=True)
class DeliveryDownloadAuthorization:
    artifact_type: DeliveryArtifactType
    media_version_id: UUID
    url: str
    expires_in_seconds: int
    expires_at: datetime
