from __future__ import annotations

from typing import Any
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class NarrativeUnit(BaseModel):
    model_config = ConfigDict(extra="forbid")

    unit_version_id: UUID
    position: int = Field(gt=0)
    kind: str
    exact_text: str = Field(min_length=1)
    required_for_coverage: bool
    source_scene_id: UUID
    source_dialogue_id: UUID | None


class StoryboardDraftInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    batch_id: UUID
    task_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    script_version_id: UUID
    target_duration_ms: int = Field(gt=0)
    aspect_ratio: str = Field(min_length=1)
    visual_style: str | None
    units: list[NarrativeUnit] = Field(min_length=1)
    assets: list[dict[str, Any]]
    production_bible_id: UUID
    production_bible_revision: int = Field(gt=0)
    production_bible_result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    world_entries: list[dict[str, Any]]
    run_token: UUID


class ShotVisual(BaseModel):
    model_config = ConfigDict(extra="forbid")

    shot_size: str | None
    camera_angle: str | None
    camera_movement: str | None
    composition: str | None
    environment: str | None
    lighting: str | None
    subject_placement: str | None
    facing: str | None
    gaze: str | None


class ShotSpec(BaseModel):
    model_config = ConfigDict(extra="forbid")

    duration_ms: int = Field(ge=500, le=15000)
    narrative_role: str
    action_beats: list[str]
    first_frame: str
    last_frame: str
    keyframe_notes: list[str]
    continuity_in: str
    continuity_out: str
    visual: ShotVisual
    dialogue: str | None
    performance: str | None
    ambience: str | None
    sound_effects: list[str]


class AssetReference(BaseModel):
    model_config = ConfigDict(extra="forbid")

    asset_id: UUID
    asset_version_id: UUID
    state_key: str | None
    usage: str


class DraftShot(BaseModel):
    model_config = ConfigDict(extra="forbid")

    proposal_key: str
    position: int = Field(gt=0)
    title: str = Field(min_length=1)
    narrative_unit_version_ids: list[UUID] = Field(min_length=1)
    spec: ShotSpec
    asset_references: list[AssetReference]
    risk_codes: list[str]


class StoryboardCandidate(BaseModel):
    model_config = ConfigDict(extra="forbid")

    shots: list[DraftShot] = Field(min_length=1, max_length=120)
