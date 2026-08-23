from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.modules.storyboards.contracts import StoryboardDraftInput
from app.modules.storyboards.drafts.schemas import DraftProviderResult
from app.modules.storyboards.schemas import CameraAngle, CameraMovement, ShotSize

STORYBOARD_DRAFT_PROMPT_VERSION = "storyboard-draft-prompt-v2-compact-coverage"


class ProviderModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


class ProviderAssetBinding(ProviderModel):
    asset_position: int = Field(ge=1, le=100)
    role: Literal["location", "character", "prop", "costume", "visual_style", "voice"]
    subject_key: str | None = Field(default=None, min_length=1, max_length=100)


class ProviderShot(ProviderModel):
    proposal_key: str = Field(min_length=1, max_length=120)
    position: int = Field(ge=1, le=120)
    title: str = Field(min_length=1, max_length=200)
    unit_positions: list[int] = Field(min_length=1, max_length=500)
    scene_unit_position: int = Field(ge=1, le=500)
    dialogue_unit_positions: list[int] = Field(default=[], max_length=8)
    purpose: str = Field(min_length=1, max_length=500)
    shot_size: ShotSize
    camera_angle: CameraAngle
    camera_movement: CameraMovement
    composition: str = Field(min_length=1, max_length=1000)
    environment: str = Field(min_length=1, max_length=1000)
    mood_lighting: str = Field(min_length=1, max_length=1000)
    action: str = Field(min_length=1, max_length=1000)
    duration_ms: int = Field(ge=1_000, le=15_000)
    ambient: str | None = Field(default=None, max_length=1000)
    asset_bindings: list[ProviderAssetBinding] = Field(default=[], max_length=100)
    risk_codes: list[str] = Field(default=[], max_length=20)

    @model_validator(mode="after")
    def validate_local_keys(self) -> "ProviderShot":
        if len(set(self.unit_positions)) != len(self.unit_positions):
            raise ValueError("unit positions must be unique")
        if len(set(self.dialogue_unit_positions)) != len(self.dialogue_unit_positions):
            raise ValueError("dialogue unit positions must be unique")
        if not set(self.dialogue_unit_positions).issubset(self.unit_positions):
            raise ValueError("dialogue unit positions must belong to the shot")
        asset_positions = [binding.asset_position for binding in self.asset_bindings]
        if len(set(asset_positions)) != len(asset_positions):
            raise ValueError("asset positions must be unique within a shot")
        if any(not code.strip() or len(code) > 80 for code in self.risk_codes):
            raise ValueError("risk codes must contain 1-80 characters")
        return self


class StoryboardProviderResult(ProviderModel):
    shots: list[ProviderShot] = Field(min_length=1, max_length=120)

    @model_validator(mode="after")
    def validate_shot_order(self) -> "StoryboardProviderResult":
        if [shot.position for shot in self.shots] != list(range(1, len(self.shots) + 1)):
            raise ValueError("shot positions must be continuous from 1")
        keys = [shot.proposal_key for shot in self.shots]
        if len(set(keys)) != len(keys):
            raise ValueError("proposal keys must be unique")
        return self


def expand_provider_result(
    result: StoryboardProviderResult,
    draft_input: StoryboardDraftInput,
) -> DraftProviderResult:
    units = {unit.position: unit for unit in draft_input.units}
    assets = {asset.position: asset for asset in draft_input.assets}
    shots: list[dict[str, object]] = []
    for shot in result.shots:
        try:
            selected_units = [units[position] for position in shot.unit_positions]
            scene_unit = units[shot.scene_unit_position]
            dialogue_units = [units[position] for position in shot.dialogue_unit_positions]
            selected_assets = [
                (binding, assets[binding.asset_position]) for binding in shot.asset_bindings
            ]
        except KeyError as error:
            raise ValueError("provider output references an unknown input position") from error
        if scene_unit.source_scene_id is None:
            raise ValueError("scene unit position has no confirmed scene")
        if any(
            unit.source_dialogue_id is None or unit.source_scene_id != scene_unit.source_scene_id
            for unit in dialogue_units
        ):
            raise ValueError("provider dialogue position is not valid for the selected scene")
        dialogue_ids = [
            unit.source_dialogue_id
            for unit in dialogue_units
            if unit.source_dialogue_id is not None
        ]
        shots.append(
            {
                "proposal_key": shot.proposal_key,
                "position": shot.position,
                "title": shot.title,
                "narrative_unit_version_ids": [unit.unit_version_id for unit in selected_units],
                "spec": {
                    "schema_version": 1,
                    "script_reference": {
                        "confirmed_script_version_id": draft_input.script_version_id,
                        "scene_id": scene_unit.source_scene_id,
                        "dialogue_ids": dialogue_ids,
                    },
                    "narrative": {"purpose": shot.purpose, "continuity_note": None},
                    "visual": {
                        "shot_size": shot.shot_size,
                        "camera_angle": shot.camera_angle,
                        "camera_movement": shot.camera_movement,
                        "composition": shot.composition,
                        "environment": shot.environment,
                        "subject_placements": [],
                        "mood_lighting": shot.mood_lighting,
                    },
                    "action_beats": [
                        {"beat_key": "primary", "order": 1, "description": shot.action}
                    ],
                    "dialogue_or_narration": [
                        {
                            "source_dialogue_id": dialogue_id,
                            "beat_key": "primary",
                            "speaker_subject_key": None,
                            "render_as_audio": True,
                            "performance_note": None,
                        }
                        for dialogue_id in dialogue_ids
                    ],
                    "duration_ms": max(4_000, shot.duration_ms),
                    "audio_intent": {"ambient": shot.ambient, "sound_effects": []},
                    "generation_intent": {
                        "mode": ("reference_to_video" if selected_assets else "text_to_video"),
                        "first_frame": None,
                        "last_frame": None,
                        "keyframe_notes": None,
                    },
                },
                "asset_references": [
                    {
                        "slot_key": f"asset-{binding.asset_position}",
                        "role": binding.role,
                        "asset_version_id": asset.asset_version_id,
                        "subject_key": binding.subject_key,
                    }
                    for binding, asset in selected_assets
                ],
                "risk_codes": shot.risk_codes,
            }
        )
    return DraftProviderResult.model_validate({"shots": shots})
