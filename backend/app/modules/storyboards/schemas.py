from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

ShotSize = Literal[
    "extreme_wide",
    "wide",
    "full",
    "medium",
    "medium_close_up",
    "close_up",
    "extreme_close_up",
]
CameraAngle = Literal["eye_level", "high", "low", "bird_eye", "dutch"]
CameraMovement = Literal[
    "static",
    "pan",
    "tilt",
    "dolly",
    "truck",
    "pedestal",
    "zoom",
    "handheld",
    "orbit",
]
GenerationMode = Literal[
    "keyframe_then_video",
    "reference_to_video",
    "text_to_video",
]
AssetRole = Literal[
    "location",
    "character",
    "prop",
    "costume",
    "visual_style",
    "voice",
]
NarrativeChannel = Literal["visual", "audio", "both"]
NarrativeRole = Literal[
    "primary",
    "dialogue",
    "reaction",
    "insert",
    "setup",
    "payoff",
    "transition",
    "supporting",
]


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class ScriptReference(CommandModel):
    confirmed_script_version_id: UUID
    scene_id: UUID
    dialogue_ids: list[UUID] = Field(default=[], max_length=8)

    @model_validator(mode="after")
    def validate_unique_dialogues(self) -> "ScriptReference":
        if len(set(self.dialogue_ids)) != len(self.dialogue_ids):
            raise ValueError("dialogue IDs must be unique")
        return self


class NarrativeSpec(CommandModel):
    purpose: str = Field(min_length=1, max_length=500)
    continuity_note: str | None = Field(default=None, max_length=500)


class SubjectPlacement(CommandModel):
    subject_key: str = Field(min_length=1, max_length=100)
    placement: str = Field(min_length=1, max_length=500)


class VisualSpec(CommandModel):
    shot_size: ShotSize
    camera_angle: CameraAngle
    camera_movement: CameraMovement
    composition: str = Field(min_length=1, max_length=1000)
    environment: str = Field(min_length=1, max_length=1000)
    subject_placements: list[SubjectPlacement] = Field(default=[], max_length=16)
    mood_lighting: str = Field(min_length=1, max_length=1000)

    @model_validator(mode="after")
    def validate_subject_keys(self) -> "VisualSpec":
        keys = [placement.subject_key for placement in self.subject_placements]
        if len(set(keys)) != len(keys):
            raise ValueError("subject placement keys must be unique")
        return self


class ActionBeat(CommandModel):
    beat_key: str = Field(min_length=1, max_length=100)
    order: int = Field(ge=1, le=8)
    description: str = Field(min_length=1, max_length=1000)


class DialogueOrNarration(CommandModel):
    source_dialogue_id: UUID
    beat_key: str | None = Field(default=None, min_length=1, max_length=100)
    speaker_subject_key: str | None = Field(default=None, min_length=1, max_length=100)
    render_as_audio: bool = False
    performance_note: str | None = Field(default=None, max_length=1000)


class AudioIntent(CommandModel):
    ambient: str | None = Field(default=None, max_length=1000)
    sound_effects: list[str] = Field(default=[], max_length=8)


class GenerationIntent(CommandModel):
    mode: GenerationMode
    first_frame: str | None = Field(default=None, max_length=1000)
    last_frame: str | None = Field(default=None, max_length=1000)
    keyframe_notes: str | None = Field(default=None, max_length=2000)


class ShotSpec(CommandModel):
    schema_version: Literal[1] = 1
    script_reference: ScriptReference
    narrative: NarrativeSpec
    visual: VisualSpec
    action_beats: list[ActionBeat] = Field(min_length=1, max_length=8)
    dialogue_or_narration: list[DialogueOrNarration] = Field(
        default=[],
        max_length=8,
    )
    duration_ms: int = Field(default=3000, ge=500, le=15_000)
    audio_intent: AudioIntent | None = None
    generation_intent: GenerationIntent

    @model_validator(mode="after")
    def validate_cross_references(self) -> "ShotSpec":
        beat_keys = [beat.beat_key for beat in self.action_beats]
        if len(set(beat_keys)) != len(beat_keys):
            raise ValueError("action beat keys must be unique")
        if [beat.order for beat in self.action_beats] != list(range(1, len(self.action_beats) + 1)):
            raise ValueError("action beat order must be continuous from 1")

        dialogue_ids = set(self.script_reference.dialogue_ids)
        for item in self.dialogue_or_narration:
            if item.source_dialogue_id not in dialogue_ids:
                raise ValueError("dialogue must belong to script_reference.dialogue_ids")
            if item.beat_key is not None and item.beat_key not in beat_keys:
                raise ValueError("dialogue beat_key must reference an action beat")
        return self


class AssetReferenceRequest(CommandModel):
    slot_key: str = Field(min_length=1, max_length=100)
    role: AssetRole
    asset_version_id: UUID
    subject_key: str | None = Field(default=None, min_length=1, max_length=100)


class NarrativeReferenceInput(CommandModel):
    unit_version_id: UUID
    channel: NarrativeChannel
    role: NarrativeRole
    coverage_mode: Literal["full", "partial"]
    segment_start: int | None = Field(default=None, ge=0)
    segment_end: int | None = Field(default=None, ge=1)
    contribution: Literal["required", "supporting"]

    @model_validator(mode="after")
    def validate_segment(self) -> "NarrativeReferenceInput":
        if self.coverage_mode == "full":
            if self.segment_start is not None or self.segment_end is not None:
                raise ValueError("full coverage cannot include a segment range")
        elif (
            self.segment_start is None
            or self.segment_end is None
            or self.segment_end <= self.segment_start
        ):
            raise ValueError("partial coverage requires a non-empty segment range")
        return self


class AssetReferenceResponse(BaseModel):
    slot_key: str
    role: AssetRole
    asset_version_id: UUID
    asset_state_id: UUID
    asset_id: UUID
    binding_source: Literal["manual", "ai"]
    subject_key: str | None


class ShotSpecVersionResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    shot_id: UUID
    version_no: int
    schema_version: Literal[1]
    spec: ShotSpec
    content_hash: str
    input_hash: str
    asset_references: list[AssetReferenceResponse]
    created_by: UUID
    created_at: datetime


class ShotCreateRequest(CommandModel):
    title: str = Field(min_length=1, max_length=200)
    source_script_version_id: UUID
    source_scene_id: UUID
    creation_key: str = Field(min_length=1, max_length=200)


class ShotUpdateRequest(CommandModel):
    expected_revision: int = Field(ge=1)
    title: str = Field(min_length=1, max_length=200)


class ShotStateRequest(CommandModel):
    expected_revision: int = Field(ge=1)
    expected_order_hash: str = Field(min_length=64, max_length=64)


class ShotResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    episode_id: UUID
    position: int
    title: str
    source_script_version_id: UUID
    source_scene_id: UUID
    source_candidate_id: UUID | None
    source_draft_shot_id: UUID | None
    status: Literal["active", "archived"]
    current_spec_version_id: UUID | None
    revision: int
    created_at: datetime
    updated_at: datetime


class ShotOrderResponse(BaseModel):
    items: list[ShotResponse]
    order_hash: str


class ShotReorderRequest(CommandModel):
    shot_ids: list[UUID] = Field(min_length=1, max_length=120)
    expected_order_hash: str = Field(min_length=64, max_length=64)

    @model_validator(mode="after")
    def validate_unique_shots(self) -> "ShotReorderRequest":
        if len(set(self.shot_ids)) != len(self.shot_ids):
            raise ValueError("shot IDs must be unique")
        return self


class ShotStateResponse(BaseModel):
    shot: ShotResponse
    order: ShotOrderResponse


class ShotSpecCreateRequest(CommandModel):
    expected_current_spec_version_id: UUID | None
    spec: ShotSpec
    asset_references: list[AssetReferenceRequest] = Field(default=[], max_length=100)

    @model_validator(mode="after")
    def validate_reference_keys(self) -> "ShotSpecCreateRequest":
        slot_keys = [reference.slot_key for reference in self.asset_references]
        if len(set(slot_keys)) != len(slot_keys):
            raise ValueError("asset reference slot keys must be unique")
        return self


class ShotSpecCreateResponse(BaseModel):
    shot: ShotResponse
    version: ShotSpecVersionResponse


class ShotCurrentSpecRequest(CommandModel):
    version_id: UUID
    expected_current_spec_version_id: UUID | None
    expected_revision: int = Field(ge=1)


class ShotDeleteBlocker(BaseModel):
    code: Literal["SOURCE_CANDIDATE_EVIDENCE", "SPEC_VERSION_EVIDENCE"]
    summary: str


class ShotDeletePreflightResponse(BaseModel):
    allowed: bool
    blockers: list[ShotDeleteBlocker]


class ShotDeleteResponse(BaseModel):
    deleted: Literal[True] = True
    order: ShotOrderResponse


class TargetShotSpecRequest(CommandModel):
    title: str = Field(min_length=1, max_length=200)
    spec: ShotSpec
    asset_references: list[AssetReferenceRequest] = Field(default=[], max_length=100)
    narrative_references: list[NarrativeReferenceInput] = Field(max_length=100)

    @model_validator(mode="after")
    def validate_reference_keys(self) -> "TargetShotSpecRequest":
        keys = [reference.slot_key for reference in self.asset_references]
        if len(set(keys)) != len(keys):
            raise ValueError("asset reference slot keys must be unique")
        narrative_keys = [
            (
                reference.unit_version_id,
                reference.channel,
                reference.segment_start,
                reference.segment_end,
            )
            for reference in self.narrative_references
        ]
        if len(set(narrative_keys)) != len(narrative_keys):
            raise ValueError("narrative references must be unique within a shot")
        return self


class CopyShotRequest(CommandModel):
    title: str = Field(min_length=1, max_length=200)
    expected_source_spec_version_id: UUID
    expected_order_hash: str = Field(min_length=64, max_length=64)
    idempotency_key: str = Field(min_length=1, max_length=200)


class SplitPreflightRequest(CommandModel):
    expected_source_spec_version_id: UUID
    expected_order_hash: str = Field(min_length=64, max_length=64)


class SplitShotRequest(SplitPreflightRequest):
    impact_hash: str = Field(min_length=64, max_length=64)
    idempotency_key: str = Field(min_length=1, max_length=200)
    targets: list[TargetShotSpecRequest] = Field(min_length=2, max_length=2)


class MergePreflightRequest(CommandModel):
    shot_ids: list[UUID] = Field(min_length=2, max_length=2)
    expected_spec_version_ids: list[UUID] = Field(min_length=2, max_length=2)
    expected_order_hash: str = Field(min_length=64, max_length=64)

    @model_validator(mode="after")
    def validate_sources(self) -> "MergePreflightRequest":
        if len(set(self.shot_ids)) != 2:
            raise ValueError("merge requires two unique shot IDs")
        if len(set(self.expected_spec_version_ids)) != 2:
            raise ValueError("merge requires two unique spec version IDs")
        return self


class MergeShotRequest(MergePreflightRequest):
    impact_hash: str = Field(min_length=64, max_length=64)
    idempotency_key: str = Field(min_length=1, max_length=200)
    target: TargetShotSpecRequest


class DownstreamEvidenceResponse(BaseModel):
    generation_request_ids: list[UUID] = []
    candidate_ids: list[UUID] = []
    review_ids: list[UUID] = []
    issue_ids: list[UUID] = []
    timeline_source_ids: list[UUID] = []


class ShotTransformPreflightResponse(BaseModel):
    operation: Literal["split", "merge"]
    source_shot_ids: list[UUID]
    source_spec_version_ids: list[UUID]
    order_hash: str
    downstream_evidence: DownstreamEvidenceResponse
    impact_hash: str


class ShotTransformEvidenceResponse(BaseModel):
    id: UUID
    operation: Literal["copy", "split", "merge"]
    source_shot_ids: list[UUID]
    source_spec_version_ids: list[UUID]
    result_shot_ids: list[UUID]
    impact_hash: str
    input_hash: str
    idempotency_key: str
    actor_id: UUID
    created_at: datetime


class ShotTransformResponse(BaseModel):
    transform: ShotTransformEvidenceResponse
    shots: list[ShotResponse]
    spec_versions: list[ShotSpecVersionResponse]
    order: ShotOrderResponse


class ShotReadinessIssue(BaseModel):
    code: Literal[
        "CURRENT_SPEC_MISSING",
        "SPEC_FIELD_MISSING",
        "DURATION_OUT_OF_RANGE",
        "SCRIPT_VERSION_UNAVAILABLE",
        "SCRIPT_REVISION_NOT_CURRENT",
        "SOURCE_SCENE_INVALID",
        "SOURCE_DIALOGUE_INVALID",
        "LOCATION_REFERENCE_MISSING",
        "CHARACTER_REFERENCE_MISSING",
        "VOICE_REFERENCE_MISSING",
        "ASSET_KIND_MISMATCH",
        "ASSET_VERSION_UNAVAILABLE",
        "ASSET_DISABLED",
        "ASSET_NOT_READY",
        "MEDIA_REFERENCE_UNAVAILABLE",
        "RIGHTS_BLOCKED",
        "DEPENDENCY_UNAVAILABLE",
        "NARRATIVE_REFERENCE_INVALID",
        "COVERAGE_UNACCOUNTED",
        "SHOT_SOURCE_ORPHAN",
        "COVERAGE_DEPENDENCY_UNAVAILABLE",
    ]
    field_path: str | None = None
    dependency_type: str | None = None
    dependency_id: UUID | None = None
    summary: str
    next_action: str


class ShotReadinessWarning(BaseModel):
    code: Literal[
        "DURATION_ABOVE_RECOMMENDED",
        "ACTION_DENSITY_HIGH",
        "STYLE_REFERENCE_MISSING",
    ]
    field_path: str | None = None
    summary: str
    next_action: str


class ShotReadinessDependencies(BaseModel):
    shot_spec_version_id: UUID | None
    confirmed_script_version_id: UUID
    current_script_version_id: UUID | None
    narrative_structure_id: UUID | None
    narrative_structure_revision: int | None
    narrative_dependency_hash: str | None
    coverage_basis_hash: str | None
    coverage_evaluation_hash: str | None
    scene_id: UUID
    dialogue_ids: list[UUID]
    asset_version_ids: list[UUID]
    media_version_ids: list[UUID]
    consent_ids: list[UUID]
    asset_evaluation_hashes: dict[UUID, str]


class ShotReadinessResponse(BaseModel):
    shot_id: UUID
    status: Literal["ready", "blocked", "unavailable"]
    ready: bool
    blocking_reasons: list[ShotReadinessIssue]
    warnings: list[ShotReadinessWarning]
    next_actions: list[str]
    evaluated_dependencies: ShotReadinessDependencies
    evaluation_hash: str


class ShotReadinessSummary(BaseModel):
    total: int
    ready: int
    blocked: int
    unavailable: int


class ShotReadinessBatchResponse(BaseModel):
    episode_id: UUID
    items: list[ShotReadinessResponse]
    summary: ShotReadinessSummary
    evaluation_hash: str


class AssetShotUsageResponse(BaseModel):
    shot_id: UUID
    shot_title: str
    episode_id: UUID
    spec_version_id: UUID
    spec_version_no: int
    slot_keys: list[str]
    is_current: bool


class PaginatedAssetShotUsages(BaseModel):
    items: list[AssetShotUsageResponse]
    total: int
    limit: int
    offset: int


class AssetUpgradePreflightRequest(CommandModel):
    new_asset_version_id: UUID
    shot_ids: list[UUID] = Field(min_length=1, max_length=120)

    @model_validator(mode="after")
    def validate_unique_shots(self) -> "AssetUpgradePreflightRequest":
        if len(set(self.shot_ids)) != len(self.shot_ids):
            raise ValueError("asset upgrade shot IDs must be unique")
        return self


class AssetUpgradeTargetRequest(CommandModel):
    shot_id: UUID
    expected_spec_version_id: UUID
    expected_shot_revision: int = Field(ge=1)
    slot_keys: list[str] = Field(min_length=1, max_length=100)
    new_input_hash: str = Field(min_length=64, max_length=64)


class AssetUpgradePreflightResponse(BaseModel):
    old_asset_version_id: UUID
    new_asset_version_id: UUID
    targets: list[AssetUpgradeTargetRequest]
    preflight_hash: str


class AssetUpgradeApplyRequest(CommandModel):
    new_asset_version_id: UUID
    targets: list[AssetUpgradeTargetRequest] = Field(min_length=1, max_length=120)
    preflight_hash: str = Field(min_length=64, max_length=64)

    @model_validator(mode="after")
    def validate_unique_targets(self) -> "AssetUpgradeApplyRequest":
        shot_ids = [target.shot_id for target in self.targets]
        if len(set(shot_ids)) != len(shot_ids):
            raise ValueError("asset upgrade target shot IDs must be unique")
        return self


class AssetUpgradeApplyResponse(BaseModel):
    shots: list[ShotResponse]
    spec_versions: list[ShotSpecVersionResponse]
