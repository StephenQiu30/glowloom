from dataclasses import dataclass
from typing import Any, Literal
from uuid import UUID


@dataclass(frozen=True, slots=True)
class ShotSpecRef:
    workspace_id: UUID
    episode_id: UUID
    shot_id: UUID
    shot_spec_version_id: UUID
    schema_version: int
    content_hash: str
    input_hash: str


@dataclass(frozen=True, slots=True)
class ShotAssetReferenceSnapshot:
    slot_key: str
    role: Literal[
        "location",
        "character",
        "prop",
        "costume",
        "visual_style",
        "voice",
    ]
    asset_version_id: UUID
    subject_key: str | None


@dataclass(frozen=True, slots=True)
class ShotProductionSnapshot:
    spec_ref: ShotSpecRef
    shot_status: Literal["active", "archived"]
    current_spec_version_id: UUID | None
    shot_revision: int
    spec: dict[str, Any]
    asset_references: tuple[ShotAssetReferenceSnapshot, ...]
    readiness_status: Literal["ready", "blocked", "unavailable"]
    ready: bool
    blocking_codes: tuple[str, ...]
    warning_codes: tuple[str, ...]
    evaluation_hash: str


@dataclass(frozen=True, slots=True)
class StoryboardReferenceSummary:
    shot_count: int
    spec_version_count: int


@dataclass(frozen=True, slots=True)
class EpisodeStoryboardSummary:
    status: Literal["not_started", "blocked", "ready", "unavailable"]
    total: int
    ready: int
    blocked: int
    unavailable: int
