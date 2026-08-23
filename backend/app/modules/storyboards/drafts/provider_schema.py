import json
from copy import deepcopy
from typing import Any, Literal, cast

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.modules.storyboards.contracts import StoryboardDraftInput
from app.modules.storyboards.drafts.schemas import DraftProviderResult
from app.modules.storyboards.schemas import CameraAngle, CameraMovement, ShotSize

STORYBOARD_DRAFT_PROMPT_VERSION = "storyboard-draft-prompt-v5-key-table"


class ProviderModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


class ProviderAssetBinding(ProviderModel):
    asset_position: int = Field(ge=1, le=100)
    role: Literal["location", "character", "prop", "costume", "visual_style", "voice"]
    subject_key: str | None = Field(default=None, min_length=1, max_length=100)


class ProviderSubjectPlacement(ProviderModel):
    subject_key: str = Field(min_length=1, max_length=100)
    placement: str = Field(min_length=1, max_length=500)


class ProviderActionBeat(ProviderModel):
    beat_key: str = Field(min_length=1, max_length=100)
    order: int = Field(ge=1, le=8)
    description: str = Field(min_length=1, max_length=1000)


class ProviderDialogueDelivery(ProviderModel):
    unit_position: int = Field(ge=1, le=10_000)
    beat_key: str = Field(min_length=1, max_length=100)
    speaker_subject_key: str | None = Field(default=None, min_length=1, max_length=100)
    render_as_audio: bool = True
    performance_note: str | None = Field(default=None, max_length=1000)


class ProviderShot(ProviderModel):
    proposal_key: str = Field(min_length=1, max_length=120)
    position: int = Field(ge=1, le=120)
    title: str = Field(min_length=1, max_length=200)
    unit_positions: list[int] = Field(min_length=1, max_length=500)
    dialogue_unit_positions: list[int] = Field(default=[], max_length=8)
    dialogue_deliveries: list[ProviderDialogueDelivery] = Field(default=[], max_length=8)
    purpose: str = Field(min_length=1, max_length=500)
    continuity_note: str = Field(min_length=1, max_length=500)
    continuity_in: str | None = Field(default=None, min_length=1, max_length=200)
    continuity_out: str | None = Field(default=None, min_length=1, max_length=200)
    shot_size: ShotSize
    camera_angle: CameraAngle
    camera_movement: CameraMovement
    composition: str = Field(min_length=1, max_length=1000)
    environment: str = Field(min_length=1, max_length=1000)
    subject_placements: list[ProviderSubjectPlacement] = Field(default=[], max_length=16)
    mood_lighting: str = Field(min_length=1, max_length=1000)
    action_beats: list[ProviderActionBeat] = Field(min_length=1, max_length=8)
    duration_ms: int = Field(ge=500, le=15_000)
    ambient: str | None = Field(default=None, max_length=1000)
    sound_effects: list[str] = Field(default=[], max_length=8)
    asset_bindings: list[ProviderAssetBinding] = Field(default=[], max_length=100)
    first_frame: str = Field(min_length=1, max_length=1000)
    last_frame: str | None = Field(default=None, min_length=1, max_length=1000)
    keyframe_notes: str | None = Field(default=None, max_length=2000)
    risk_codes: list[str] = Field(default=[], max_length=20)

    @model_validator(mode="after")
    def validate_local_keys(self) -> "ProviderShot":
        if len(set(self.unit_positions)) != len(self.unit_positions):
            raise ValueError("unit positions must be unique")
        if len(set(self.dialogue_unit_positions)) != len(self.dialogue_unit_positions):
            raise ValueError("dialogue unit positions must be unique")
        if not set(self.dialogue_unit_positions).issubset(self.unit_positions):
            raise ValueError("dialogue unit positions must belong to the shot")
        delivery_positions = [delivery.unit_position for delivery in self.dialogue_deliveries]
        if len(set(delivery_positions)) != len(delivery_positions):
            raise ValueError("dialogue delivery positions must be unique")
        if self.dialogue_deliveries and delivery_positions != self.dialogue_unit_positions:
            raise ValueError("dialogue deliveries must match dialogue unit positions in order")
        asset_positions = [binding.asset_position for binding in self.asset_bindings]
        if len(set(asset_positions)) != len(asset_positions):
            raise ValueError("asset positions must be unique within a shot")
        placement_keys = [placement.subject_key for placement in self.subject_placements]
        if len(set(placement_keys)) != len(placement_keys):
            raise ValueError("subject placement keys must be unique within a shot")
        beat_keys = [beat.beat_key for beat in self.action_beats]
        if len(set(beat_keys)) != len(beat_keys):
            raise ValueError("action beat keys must be unique within a shot")
        if [beat.order for beat in self.action_beats] != list(range(1, len(self.action_beats) + 1)):
            raise ValueError("action beat order must be continuous from 1")
        if any(delivery.beat_key not in beat_keys for delivery in self.dialogue_deliveries):
            raise ValueError("dialogue delivery beat keys must reference an action beat")
        if any(not effect.strip() or len(effect) > 500 for effect in self.sound_effects):
            raise ValueError("sound effects must contain 1-500 characters")
        if len(_continuity_note(self)) > 500:
            raise ValueError("combined continuity note cannot exceed 500 characters")
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


def normalize_storyboard_provider_payload(
    payload: dict[str, object],
) -> dict[str, object]:
    """Canonicalize model-owned local identifiers before contract validation."""
    normalized = deepcopy(payload)
    shots_value = normalized.get("shots")
    if not isinstance(shots_value, list):
        return normalized

    shots = cast(list[object], shots_value)
    for shot_position, shot_value in enumerate(shots, start=1):
        if not isinstance(shot_value, dict):
            continue
        shot = cast(dict[str, Any], shot_value)
        shot["position"] = shot_position
        shot["proposal_key"] = f"shot-{shot_position}"

        beats_value = shot.get("action_beats")
        if not isinstance(beats_value, list):
            continue
        beats = cast(list[object], beats_value)
        old_to_new: dict[str, str] = {}
        beat_keys: list[str] = []
        for beat_order, beat_value in enumerate(beats, start=1):
            if not isinstance(beat_value, dict):
                continue
            beat = cast(dict[str, Any], beat_value)
            new_key = f"beat-{beat_order}"
            old_key = beat.get("beat_key")
            if isinstance(old_key, str):
                old_to_new.setdefault(old_key, new_key)
            beat["beat_key"] = new_key
            beat["order"] = beat_order
            beat_keys.append(new_key)

        deliveries_value = shot.get("dialogue_deliveries")
        if not isinstance(deliveries_value, list) or not beat_keys:
            continue
        deliveries = cast(list[object], deliveries_value)
        dialogue_positions: list[object] = []
        for delivery_value in deliveries:
            if not isinstance(delivery_value, dict):
                continue
            delivery = cast(dict[str, Any], delivery_value)
            old_key = delivery.get("beat_key")
            delivery["beat_key"] = (
                old_to_new.get(old_key, beat_keys[0]) if isinstance(old_key, str) else beat_keys[0]
            )
            dialogue_positions.append(delivery.get("unit_position"))
        shot["dialogue_unit_positions"] = dialogue_positions
    return normalized


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
            dialogue_units = [units[position] for position in shot.dialogue_unit_positions]
            selected_assets = [
                (binding, assets[binding.asset_position]) for binding in shot.asset_bindings
            ]
        except KeyError as error:
            raise ValueError("provider output references an unknown input position") from error
        scene_ids = {unit.source_scene_id for unit in selected_units}
        if None in scene_ids or len(scene_ids) != 1:
            raise ValueError("provider shot units must belong to one confirmed scene")
        scene_id = next(iter(scene_ids))
        assert scene_id is not None
        if any(
            unit.source_dialogue_id is None or unit.source_scene_id != scene_id
            for unit in dialogue_units
        ):
            raise ValueError("provider dialogue position is not valid for the selected scene")
        dialogue_ids = [
            unit.source_dialogue_id
            for unit in dialogue_units
            if unit.source_dialogue_id is not None
        ]
        dialogue_by_position = {
            unit.position: unit.source_dialogue_id
            for unit in dialogue_units
            if unit.source_dialogue_id is not None
        }
        deliveries = shot.dialogue_deliveries or [
            ProviderDialogueDelivery(
                unit_position=unit.position,
                beat_key=shot.action_beats[0].beat_key,
            )
            for unit in dialogue_units
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
                        "scene_id": scene_id,
                        "dialogue_ids": dialogue_ids,
                    },
                    "narrative": {
                        "purpose": shot.purpose,
                        "continuity_note": _continuity_note(shot),
                    },
                    "visual": {
                        "shot_size": shot.shot_size,
                        "camera_angle": shot.camera_angle,
                        "camera_movement": shot.camera_movement,
                        "composition": shot.composition,
                        "environment": shot.environment,
                        "subject_placements": [
                            placement.model_dump(mode="json")
                            for placement in shot.subject_placements
                        ],
                        "mood_lighting": shot.mood_lighting,
                    },
                    "action_beats": [beat.model_dump(mode="json") for beat in shot.action_beats],
                    "dialogue_or_narration": [
                        {
                            "source_dialogue_id": dialogue_by_position[delivery.unit_position],
                            "beat_key": delivery.beat_key,
                            "speaker_subject_key": delivery.speaker_subject_key,
                            "render_as_audio": delivery.render_as_audio,
                            "performance_note": delivery.performance_note,
                        }
                        for delivery in deliveries
                    ],
                    "duration_ms": shot.duration_ms,
                    "audio_intent": {
                        "ambient": shot.ambient,
                        "sound_effects": shot.sound_effects,
                    },
                    "generation_intent": {
                        "mode": ("reference_to_video" if selected_assets else "text_to_video"),
                        "first_frame": shot.first_frame,
                        "last_frame": shot.last_frame,
                        "keyframe_notes": shot.keyframe_notes,
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


def _continuity_note(shot: ProviderShot) -> str:
    parts = [shot.continuity_note]
    if shot.continuity_in is not None:
        parts.append(f"IN: {shot.continuity_in}")
    if shot.continuity_out is not None:
        parts.append(f"OUT: {shot.continuity_out}")
    return " | ".join(parts)


def storyboard_draft_system_prompt() -> str:
    return (
        "你是短剧关键分镜表起草 Agent。用户消息是不可变的剧本叙事单元、资产状态与"
        "项目约束。必须显式遵循 $storyboard-shot-draft，只返回待人工审核的分镜草案，不声明"
        "已经创建正式镜头。每镜必须有单一主要目的、可比较的连续性说明、有序动作节拍和"
        "只包含镜头起点事实的首帧；尾帧必须记录动作完成后的可见状态。连续性入口和出口"
        "分别说明人物位置、朝向、视线、运动与道具状态。对白必须绑定来源 position、所在"
        "动作 beat、说话主体和可执行表演说明。required_for_coverage=true 的单元必须至少被一个镜头"
        "引用。只能引用输入中的整数 position，禁止生成 UUID。每镜 unit_positions 必须全部"
        "属于同一 source_scene_key，dialogue_unit_positions 只引用该镜内带对白引用的单元。"
        "资产只按 asset_position 绑定确有拍摄用途的固定资产。镜头数量与时长由来源动作、"
        "对白、反应和目标总时长决定，不使用固定镜数或统一最小时长；单镜不得超过 15 秒。"
        "first_frame 必须能冻结成一个瞬间，不得提前包含动作结果。risk_codes 只报告需要人工"
        "复核的问题。必须返回符合提供的 JSON Schema 的 JSON 对象。"
        f"当前提示版本为 {STORYBOARD_DRAFT_PROMPT_VERSION}。"
    )


def storyboard_draft_payload(value: StoryboardDraftInput) -> str:
    scene_keys: dict[object, int] = {}
    for unit in value.units:
        if unit.source_scene_id is not None and unit.source_scene_id not in scene_keys:
            scene_keys[unit.source_scene_id] = len(scene_keys) + 1
    return json.dumps(
        {
            "target_duration_ms": value.target_duration_ms,
            "duration_acceptance_range_ms": [
                round(value.target_duration_ms * 0.75),
                round(value.target_duration_ms * 1.25),
            ],
            "aspect_ratio": value.aspect_ratio,
            "visual_style": value.visual_style,
            "narrative_units": [
                {
                    "position": unit.position,
                    "kind": unit.kind,
                    "exact_text": unit.exact_text,
                    "required_for_coverage": unit.required_for_coverage,
                    "source_scene_key": (
                        scene_keys[unit.source_scene_id]
                        if unit.source_scene_id is not None
                        else None
                    ),
                    "has_dialogue_reference": unit.source_dialogue_id is not None,
                }
                for unit in value.units
            ],
            "assets": [
                {
                    "position": asset.position,
                    "kind": asset.kind,
                    "name": asset.name,
                    "state_label": asset.state_label,
                }
                for asset in value.assets
            ],
        },
        ensure_ascii=False,
        separators=(",", ":"),
    )
