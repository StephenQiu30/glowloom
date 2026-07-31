from collections.abc import Callable
from typing import cast
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.modules.storyboards.hashing import storyboard_content_hashes
from app.modules.storyboards.schemas import AssetReferenceRequest, ShotSpec

SCRIPT_VERSION_ID = UUID("018f0f4f-7b28-7f7c-8b5a-2d4fd6341001")
SCENE_ID = UUID("018f0f4f-7b28-7f7c-8b5a-2d4fd6341002")
DIALOGUE_ID = UUID("018f0f4f-7b28-7f7c-8b5a-2d4fd6341003")
LOCATION_VERSION_ID = UUID("018f0f4f-7b28-7f7c-8b5a-2d4fd6341004")
CHARACTER_VERSION_ID = UUID("018f0f4f-7b28-7f7c-8b5a-2d4fd6341005")


def _valid_spec_payload() -> dict[str, object]:
    return {
        "schema_version": 1,
        "script_reference": {
            "confirmed_script_version_id": str(SCRIPT_VERSION_ID),
            "scene_id": str(SCENE_ID),
            "dialogue_ids": [str(DIALOGUE_ID)],
        },
        "narrative": {
            "purpose": "交代主角进入雨夜车站并发现异常",
            "continuity_note": "承接上一镜头的湿外套",
        },
        "visual": {
            "shot_size": "medium",
            "camera_angle": "eye_level",
            "camera_movement": "dolly",
            "composition": "主角位于画面左三分之一",
            "environment": "雨夜的旧车站月台",
            "subject_placements": [
                {"subject_key": "hero", "placement": "画面左侧，面向站台尽头"}
            ],
            "mood_lighting": "冷蓝顶光与远处暖色信号灯形成对比",
        },
        "action_beats": [
            {"beat_key": "enter", "order": 1, "description": "主角快步进入月台"},
            {"beat_key": "notice", "order": 2, "description": "主角停下并看向闪烁灯箱"},
        ],
        "dialogue_or_narration": [
            {
                "source_dialogue_id": str(DIALOGUE_ID),
                "beat_key": "notice",
                "speaker_subject_key": "hero",
                "render_as_audio": True,
                "performance_note": "压低声音，保持警觉",
            }
        ],
        "duration_ms": 3000,
        "audio_intent": {
            "ambient": "持续雨声和远处列车低鸣",
            "sound_effects": ["鞋底踩过积水"],
        },
        "generation_intent": {
            "mode": "keyframe_then_video",
            "first_frame": "主角刚进入画面",
            "last_frame": "主角视线停在灯箱上",
            "keyframe_notes": "保持角色外观与雨势连续",
        },
    }


def _references() -> list[AssetReferenceRequest]:
    return [
        AssetReferenceRequest(
            slot_key="location:platform",
            role="location",
            asset_version_id=LOCATION_VERSION_ID,
            subject_key=None,
        ),
        AssetReferenceRequest(
            slot_key="character:hero",
            role="character",
            asset_version_id=CHARACTER_VERSION_ID,
            subject_key="hero",
        ),
    ]


def _add_provider_field(payload: dict[str, object]) -> None:
    payload["provider_model"] = "forbidden"


def _set_unknown_shot_size(payload: dict[str, object]) -> None:
    cast(dict[str, object], payload["visual"])["shot_size"] = "unknown"


def _set_excessive_duration(payload: dict[str, object]) -> None:
    payload["duration_ms"] = 15_001


def _break_action_order(payload: dict[str, object]) -> None:
    cast(list[dict[str, object]], payload["action_beats"])[1] = {
        "beat_key": "notice",
        "order": 3,
        "description": "顺序不连续",
    }


def _reference_unknown_dialogue(payload: dict[str, object]) -> None:
    cast(list[dict[str, object]], payload["dialogue_or_narration"])[0][
        "source_dialogue_id"
    ] = "018f0f4f-7b28-7f7c-8b5a-2d4fd6341999"


def test_shot_spec_v1_accepts_the_fixed_complete_shape() -> None:
    spec = ShotSpec.model_validate(_valid_spec_payload())

    assert spec.schema_version == 1
    assert spec.duration_ms == 3000
    assert [beat.order for beat in spec.action_beats] == [1, 2]
    assert spec.dialogue_or_narration[0].source_dialogue_id == DIALOGUE_ID


@pytest.mark.parametrize(
    ("mutate", "message"),
    [
        (
            _add_provider_field,
            "Extra inputs are not permitted",
        ),
        (
            _set_unknown_shot_size,
            "Input should be",
        ),
        (
            _set_excessive_duration,
            "less than or equal to 15000",
        ),
        (
            _break_action_order,
            "action beat order must be continuous from 1",
        ),
        (
            _reference_unknown_dialogue,
            "dialogue must belong to script_reference.dialogue_ids",
        ),
    ],
)
def test_shot_spec_v1_rejects_unknown_or_inconsistent_input(
    mutate: Callable[[dict[str, object]], None],
    message: str,
) -> None:
    payload = _valid_spec_payload()
    mutate(payload)

    with pytest.raises(ValidationError, match=message):
        ShotSpec.model_validate(payload)


def test_storyboard_hashes_are_canonical_and_fix_script_inputs() -> None:
    spec = ShotSpec.model_validate(_valid_spec_payload())
    references = _references()

    first = storyboard_content_hashes(spec, references)
    second = storyboard_content_hashes(spec, list(reversed(references)))

    assert first == second
    assert len(first.content_hash) == 64
    assert len(first.input_hash) == 64

    changed_payload = _valid_spec_payload()
    changed_payload["narrative"] = {
        "purpose": "改成另一个叙事目的",
        "continuity_note": "承接上一镜头的湿外套",
    }
    changed = storyboard_content_hashes(
        ShotSpec.model_validate(changed_payload), references
    )
    assert changed.content_hash != first.content_hash
    assert changed.input_hash != first.input_hash
