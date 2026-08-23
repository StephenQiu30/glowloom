from dataclasses import dataclass
from typing import Any, Literal, Protocol
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
    asset_state_id: UUID
    asset_id: UUID
    binding_source: Literal["manual", "ai"]
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
class AssetShotUsageSnapshot:
    shot_id: UUID
    shot_title: str
    episode_id: UUID
    spec_version_id: UUID
    spec_version_no: int
    current_spec_version_id: UUID | None
    shot_status: str
    slot_keys: tuple[str, ...]


@dataclass(frozen=True, slots=True)
class EpisodeStoryboardSummary:
    status: Literal["not_started", "blocked", "ready", "unavailable"]
    total: int
    ready: int
    blocked: int
    unavailable: int


@dataclass(frozen=True, slots=True)
class StoryboardDraftUnit:
    unit_version_id: UUID
    position: int
    kind: Literal["scene_heading", "action", "dialogue", "narration"]
    exact_text: str
    required_for_coverage: bool
    source_scene_id: UUID | None
    source_dialogue_id: UUID | None


@dataclass(frozen=True, slots=True)
class StoryboardDraftAsset:
    asset_version_id: UUID
    position: int
    kind: str
    name: str
    state_label: str


@dataclass(frozen=True, slots=True)
class StoryboardDraftInput:
    batch_id: UUID
    task_id: UUID
    input_hash: str
    script_version_id: UUID
    target_duration_ms: int
    aspect_ratio: Literal["9:16", "16:9", "1:1"]
    visual_style: str | None
    units: tuple[StoryboardDraftUnit, ...]
    assets: tuple[StoryboardDraftAsset, ...]


class StoryboardDraftInputChanged(RuntimeError):
    pass


class StoryboardDraftProviderError(RuntimeError):
    def __init__(
        self,
        *,
        outcome: Literal["failed", "unknown"],
        code: str,
        summary: str,
        retryable: bool,
        next_action: str,
    ) -> None:
        super().__init__(summary)
        self.outcome = outcome
        self.code = code
        self.summary = summary
        self.retryable = retryable
        self.next_action = next_action


class StoryboardDraftProvider(Protocol):
    async def draft(self, value: StoryboardDraftInput) -> dict[str, object]: ...
