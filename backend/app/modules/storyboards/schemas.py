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
        if [beat.order for beat in self.action_beats] != list(
            range(1, len(self.action_beats) + 1)
        ):
            raise ValueError("action beat order must be continuous from 1")

        dialogue_ids = set(self.script_reference.dialogue_ids)
        for item in self.dialogue_or_narration:
            if item.source_dialogue_id not in dialogue_ids:
                raise ValueError(
                    "dialogue must belong to script_reference.dialogue_ids"
                )
            if item.beat_key is not None and item.beat_key not in beat_keys:
                raise ValueError("dialogue beat_key must reference an action beat")
        return self


class AssetReferenceRequest(CommandModel):
    slot_key: str = Field(min_length=1, max_length=100)
    role: AssetRole
    asset_version_id: UUID
    subject_key: str | None = Field(default=None, min_length=1, max_length=100)


class AssetReferenceResponse(BaseModel):
    slot_key: str
    role: AssetRole
    asset_version_id: UUID
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
