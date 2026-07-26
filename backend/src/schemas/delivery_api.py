from __future__ import annotations

from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import Field, field_validator, model_validator

from schemas.common import StrictContract
from schemas.deliveries import DeliveryArtifactType
from schemas.delivery_manifest import DeliveryMediaLineageV1
from schemas.delivery_quality import DeliveryProbeSummaryV1
from schemas.project_api import SourceRevisionResponse
from schemas.rendering import (
    RenderInputRefsV1,
    RenderRecipeV1,
    RenderSegmentV1,
)
from schemas.script_api import ScriptVersionResponse
from schemas.storyboard_api import (
    CreativeAssetVersionResponse,
    StoryboardVersionResponse,
)
from schemas.subtitle_api import SubtitleVersionResponse


class DeliveryArtifactResponse(StrictContract):
    artifact_type: DeliveryArtifactType
    media_version_id: UUID
    source_kind: Literal["ffmpeg", "application"]
    mime_type: str
    byte_size: int = Field(gt=0)
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    width: int | None
    height: int | None
    duration_ticks: int | None
    timebase: int | None


class DeliverySummaryResponse(StrictContract):
    id: UUID
    episode_id: UUID
    version: int
    render_task_id: UUID
    final_attempt_id: UUID | None
    retry_of_delivery_id: UUID | None
    render_snapshot_id: UUID
    status: Literal["rendering", "ready", "failed", "cancelled"]
    artifacts: tuple[DeliveryArtifactResponse, ...]
    ffmpeg_version: str | None
    ffprobe_summary: DeliveryProbeSummaryV1 | None
    error_code: str | None
    created_at: datetime
    updated_at: datetime
    finished_at: datetime | None


class DeliveryListResponse(StrictContract):
    items: tuple[DeliverySummaryResponse, ...]


class RenderSnapshotLineageResponse(StrictContract):
    id: UUID
    episode_id: UUID
    initial_task_id: UUID | None
    input_refs: RenderInputRefsV1
    segments: tuple[RenderSegmentV1, ...]
    recipe: RenderRecipeV1
    recipe_hash: str
    content_hash: str
    created_at: datetime


class RenderTaskLineageResponse(StrictContract):
    id: UUID
    submission_snapshot_id: UUID
    status: str
    resource_version: int
    retry_of_task_id: UUID | None
    created_at: datetime
    updated_at: datetime
    finished_at: datetime | None


class RenderAttemptResponse(StrictContract):
    id: UUID
    task_id: UUID
    submission_snapshot_id: UUID
    attempt_no: int
    parent_attempt_id: UUID | None
    status: str
    execution_metadata: dict[str, object]
    error_code: str | None
    error_summary: str | None
    created_at: datetime
    submitted_at: datetime | None
    started_at: datetime | None
    finished_at: datetime | None


class DeliveryLineageResponse(StrictContract):
    source_revision: SourceRevisionResponse
    script_version: ScriptVersionResponse
    creative_asset_versions: tuple[CreativeAssetVersionResponse, ...]
    shot_spec_version: StoryboardVersionResponse
    subtitle_version: SubtitleVersionResponse
    render_snapshot: RenderSnapshotLineageResponse
    render_task: RenderTaskLineageResponse
    render_attempts: tuple[RenderAttemptResponse, ...]
    input_media: tuple[DeliveryMediaLineageV1, ...]
    delivery_media: tuple[DeliveryArtifactResponse, ...]


class DeliveryDetailResponse(DeliverySummaryResponse):
    lineage: DeliveryLineageResponse


class DownloadAuthorizationRequest(StrictContract):
    episode_id: UUID = Field(strict=False)
    artifact_types: tuple[DeliveryArtifactType, ...] = ("mp4",)

    @field_validator("artifact_types", mode="before")
    @classmethod
    def parse_artifact_types(cls, value: object) -> object:
        return tuple(value) if isinstance(value, list) else value

    @model_validator(mode="after")
    def validate_exact_set(self) -> DownloadAuthorizationRequest:
        if not 1 <= len(self.artifact_types) <= 3:
            raise ValueError("one to three artifact types are required")
        if len(self.artifact_types) != len(set(self.artifact_types)):
            raise ValueError("artifact types must be unique")
        return self


class DownloadAuthorizationItem(StrictContract):
    artifact_type: DeliveryArtifactType
    media_version_id: UUID
    url: str = Field(pattern=r"^https?://")
    expires_in_seconds: Literal[900]
    expires_at: datetime


class DownloadAuthorizationResponse(StrictContract):
    items: tuple[DownloadAuthorizationItem, ...]
