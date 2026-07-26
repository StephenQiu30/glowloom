from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

from schemas.deliveries import DeliveryArtifactSnapshot, DeliveryVersionSnapshot
from schemas.delivery_manifest import DeliveryMediaLineageV1
from schemas.projects import SourceRevisionSnapshot
from schemas.rendering import RenderSnapshot
from schemas.story_snapshots import (
    CreativeAssetVersionSnapshot,
    ScriptVersionSnapshot,
    StoryboardVersionSnapshot,
)
from schemas.subtitle_versions import SubtitleVersionSnapshot
from schemas.tasks import TaskSnapshot


@dataclass(frozen=True, slots=True)
class RenderAttemptSnapshot:
    id: UUID
    task_id: UUID
    snapshot_id: UUID
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


@dataclass(frozen=True, slots=True)
class DeliveryLineageSnapshot:
    source_revision: SourceRevisionSnapshot
    script_version: ScriptVersionSnapshot
    creative_asset_versions: tuple[CreativeAssetVersionSnapshot, ...]
    shot_spec_version: StoryboardVersionSnapshot
    subtitle_version: SubtitleVersionSnapshot
    render_snapshot: RenderSnapshot
    render_task: TaskSnapshot
    render_attempts: tuple[RenderAttemptSnapshot, ...]
    input_media: tuple[DeliveryMediaLineageV1, ...]
    delivery_media: tuple[DeliveryArtifactSnapshot, ...]


@dataclass(frozen=True, slots=True)
class DeliveryDetailSnapshot:
    delivery: DeliveryVersionSnapshot
    lineage: DeliveryLineageSnapshot
