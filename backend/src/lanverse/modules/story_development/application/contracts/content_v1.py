from __future__ import annotations

import hashlib
import unicodedata
from typing import Any, Literal
from uuid import UUID

import rfc8785
from pydantic import BaseModel, Field, model_validator

from lanverse.shared_kernel.http_contracts import StrictContract

VoiceId = Literal[
    "narrator_female",
    "narrator_male",
    "character_young_female",
    "character_young_male",
]


def _json_value(value: Any) -> Any:
    if isinstance(value, BaseModel):
        return _json_value(value.model_dump(mode="json"))
    if isinstance(value, UUID):
        return str(value)
    if isinstance(value, str):
        return unicodedata.normalize("NFC", value)
    if isinstance(value, dict):
        return {str(key): _json_value(item) for key, item in value.items()}
    if isinstance(value, (tuple, list)):
        return [_json_value(item) for item in value]
    return value


def canonical_content_hash(value: Any) -> str:
    return hashlib.sha256(rfc8785.dumps(_json_value(value))).hexdigest()


class SpeechLineV1(StrictContract):
    speech_line_id: UUID
    ordinal: int = Field(ge=1)
    kind: Literal["dialogue", "narration"]
    text: str = Field(min_length=1, max_length=500)
    voice_id: VoiceId
    speaker: str | None = Field(default=None, min_length=1, max_length=120)

    @model_validator(mode="after")
    def validate_speaker(self) -> SpeechLineV1:
        if self.kind == "dialogue" and self.speaker is None:
            raise ValueError("dialogue requires a speaker")
        if self.kind == "narration" and self.speaker is not None:
            raise ValueError("narration cannot declare a speaker")
        return self


class SceneV1(StrictContract):
    scene_id: UUID
    ordinal: int = Field(ge=1)
    location: str = Field(min_length=1, max_length=200)
    time_of_day: Literal["dawn", "day", "dusk", "night", "interior"]
    action: str = Field(min_length=1, max_length=2000)
    speech_lines: tuple[SpeechLineV1, ...]


class ScriptContentV1(StrictContract):
    schema_version: Literal["script-v1"] = "script-v1"
    title: str = Field(min_length=1, max_length=120)
    scenes: tuple[SceneV1, ...] = Field(min_length=1, max_length=20)

    @model_validator(mode="after")
    def validate_order_and_ids(self) -> ScriptContentV1:
        if [scene.ordinal for scene in self.scenes] != list(range(1, len(self.scenes) + 1)):
            raise ValueError("scene ordinals must be continuous")
        scene_ids = [scene.scene_id for scene in self.scenes]
        if len(scene_ids) != len(set(scene_ids)):
            raise ValueError("scene ids must be unique")
        lines = [line for scene in self.scenes for line in scene.speech_lines]
        if not lines:
            raise ValueError("script requires at least one speech line")
        if [line.ordinal for line in lines] != list(range(1, len(lines) + 1)):
            raise ValueError("speech ordinals must be continuous")
        line_ids = [line.speech_line_id for line in lines]
        if len(line_ids) != len(set(line_ids)):
            raise ValueError("speech line ids must be unique")
        return self


class CreativeAssetContentV1(StrictContract):
    schema_version: Literal["creative-asset-v1"] = "creative-asset-v1"
    asset_id: UUID
    asset_type: Literal["character", "scene", "visual_style"]
    name: str = Field(min_length=1, max_length=120)
    description: str = Field(min_length=1, max_length=2000)


class ShotV1(StrictContract):
    shot_id: UUID
    ordinal: int = Field(ge=1)
    narrative_purpose: str = Field(min_length=1, max_length=500)
    visual_prompt: str = Field(min_length=1, max_length=4000)
    action: str = Field(min_length=1, max_length=2000)
    duration_ticks: int = Field(ge=270000, le=720000)
    asset_version_ids: tuple[UUID, ...] = Field(min_length=1)
    speech_line_ids: tuple[UUID, ...]
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @classmethod
    def create(
        cls,
        *,
        shot_id: UUID,
        ordinal: int,
        narrative_purpose: str,
        visual_prompt: str,
        action: str,
        duration_ticks: int,
        asset_version_ids: tuple[UUID, ...],
        speech_line_ids: tuple[UUID, ...],
    ) -> ShotV1:
        values = {
            "shot_id": shot_id,
            "ordinal": ordinal,
            "narrative_purpose": narrative_purpose,
            "visual_prompt": visual_prompt,
            "action": action,
            "duration_ticks": duration_ticks,
            "asset_version_ids": asset_version_ids,
            "speech_line_ids": speech_line_ids,
        }
        return cls.model_validate(
            {**values, "content_hash": canonical_content_hash(values)}
        )

    def hash_input(self) -> dict[str, object]:
        return self.model_dump(exclude={"content_hash"})

    @model_validator(mode="after")
    def validate_duration_and_hash(self) -> ShotV1:
        if self.duration_ticks % 3750:
            raise ValueError("shot duration must align to 24fps")
        if len(self.asset_version_ids) != len(set(self.asset_version_ids)):
            raise ValueError("shot asset references must be unique")
        if len(self.speech_line_ids) != len(set(self.speech_line_ids)):
            raise ValueError("shot speech references must be unique")
        if self.content_hash != canonical_content_hash(self.hash_input()):
            raise ValueError("shot content hash does not match its content")
        return self


class ShotSpecCollectionV1(StrictContract):
    schema_version: Literal["shot-spec-v1"] = "shot-spec-v1"
    script_version_id: UUID
    asset_version_ids: tuple[UUID, ...] = Field(min_length=1)
    speech_line_ids: tuple[UUID, ...] = Field(min_length=1)
    shots: tuple[ShotV1, ...] = Field(min_length=6, max_length=10)

    @property
    def total_duration_ticks(self) -> int:
        return sum(shot.duration_ticks for shot in self.shots)

    @model_validator(mode="after")
    def validate_collection(self) -> ShotSpecCollectionV1:
        if len(self.asset_version_ids) != len(set(self.asset_version_ids)):
            raise ValueError("asset version ids must be unique")
        if len(self.speech_line_ids) != len(set(self.speech_line_ids)):
            raise ValueError("speech line ids must be unique")
        if [shot.ordinal for shot in self.shots] != list(range(1, len(self.shots) + 1)):
            raise ValueError("shot ordinals must be continuous")
        shot_ids = [shot.shot_id for shot in self.shots]
        if len(shot_ids) != len(set(shot_ids)):
            raise ValueError("shot ids must be unique")
        allowed_assets = set(self.asset_version_ids)
        if any(not set(shot.asset_version_ids) <= allowed_assets for shot in self.shots):
            raise ValueError("shot references an unknown asset version")
        assigned_speech = [item for shot in self.shots for item in shot.speech_line_ids]
        if sorted(assigned_speech) != sorted(self.speech_line_ids):
            raise ValueError("every speech line must belong to exactly one shot")
        if not 2700000 <= self.total_duration_ticks <= 5400000:
            raise ValueError("storyboard total duration is out of range")
        return self
