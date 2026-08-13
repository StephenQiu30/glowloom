from copy import deepcopy
from uuid import UUID

import pytest

from app.modules.storyboards.conservation import (
    TransformConservationError,
    validate_merge_content,
    validate_split_content,
)
from app.modules.storyboards.schemas import (
    ShotSpec,
    TargetShotSpecRequest,
)

SCRIPT_ID = UUID("019fbe40-a000-7000-8000-000000000001")
SCENE_ID = UUID("019fbe40-a000-7000-8000-000000000002")
OTHER_SCENE_ID = UUID("019fbe40-a000-7000-8000-000000000003")
DIALOGUE_A = UUID("019fbe40-a000-7000-8000-000000000004")
DIALOGUE_B = UUID("019fbe40-a000-7000-8000-000000000005")


def _spec(
    *,
    scene_id: UUID = SCENE_ID,
    beat_prefix: str,
    descriptions: list[str],
    dialogue_ids: list[UUID],
    duration_ms: int = 3_000,
) -> ShotSpec:
    beats = [
        {
            "beat_key": f"{beat_prefix}-{index}",
            "order": index,
            "description": description,
        }
        for index, description in enumerate(descriptions, start=1)
    ]
    return ShotSpec.model_validate(
        {
            "schema_version": 1,
            "script_reference": {
                "confirmed_script_version_id": str(SCRIPT_ID),
                "scene_id": str(scene_id),
                "dialogue_ids": [str(dialogue_id) for dialogue_id in dialogue_ids],
            },
            "narrative": {"purpose": "守恒测试", "continuity_note": None},
            "visual": {
                "shot_size": "medium",
                "camera_angle": "eye_level",
                "camera_movement": "static",
                "composition": "人物位于画面中心",
                "environment": "雨夜月台",
                "subject_placements": [],
                "mood_lighting": "冷蓝侧光",
            },
            "action_beats": beats,
            "dialogue_or_narration": [
                {
                    "source_dialogue_id": str(dialogue_id),
                    "beat_key": beats[min(index, len(beats) - 1)]["beat_key"],
                    "speaker_subject_key": "hero",
                    "render_as_audio": True,
                    "performance_note": f"表演提示 {index + 1}",
                }
                for index, dialogue_id in enumerate(dialogue_ids)
            ],
            "duration_ms": duration_ms,
            "audio_intent": {"ambient": "雨声", "sound_effects": []},
            "generation_intent": {
                "mode": "text_to_video",
                "first_frame": None,
                "last_frame": None,
                "keyframe_notes": None,
            },
        }
    )


def _target(spec: ShotSpec, title: str = "目标镜头") -> TargetShotSpecRequest:
    return TargetShotSpecRequest(title=title, spec=spec, asset_references=[])


def _split_targets(source: ShotSpec) -> list[TargetShotSpecRequest]:
    first = source.model_copy(deep=True)
    first.action_beats = [first.action_beats[0].model_copy(update={"order": 1})]
    first.script_reference.dialogue_ids = [DIALOGUE_A]
    first.dialogue_or_narration = [first.dialogue_or_narration[0]]
    first.duration_ms = 1_500

    second = source.model_copy(deep=True)
    second.action_beats = [
        beat.model_copy(update={"order": index})
        for index, beat in enumerate(second.action_beats[1:], start=1)
    ]
    second.script_reference.dialogue_ids = [DIALOGUE_B]
    second.dialogue_or_narration = [second.dialogue_or_narration[1]]
    second.duration_ms = 1_500
    return [_target(first, "前段"), _target(second, "后段")]


def test_split_requires_an_ordered_exact_action_and_dialogue_partition() -> None:
    source = _spec(
        beat_prefix="source",
        descriptions=["进入月台", "观察灯箱", "听见脚步"],
        dialogue_ids=[DIALOGUE_A, DIALOGUE_B],
    )
    targets = _split_targets(source)
    validate_split_content(source, targets)

    missing_dialogue = deepcopy(targets)
    missing_dialogue[1].spec.script_reference.dialogue_ids = []
    missing_dialogue[1].spec.dialogue_or_narration = []
    with pytest.raises(TransformConservationError, match="split dialogue IDs"):
        validate_split_content(source, missing_dialogue)

    repeated_action = deepcopy(targets)
    repeated_action[1].spec.action_beats = [
        targets[0].spec.action_beats[0].model_copy(update={"order": 1}),
        *[
            beat.model_copy(update={"order": index})
            for index, beat in enumerate(targets[1].spec.action_beats, start=2)
        ],
    ]
    with pytest.raises(TransformConservationError, match="split action beats"):
        validate_split_content(source, repeated_action)


def test_merge_preserves_content_and_rekeys_dialogue_beat_links() -> None:
    first = _spec(
        beat_prefix="beat",
        descriptions=["进入月台"],
        dialogue_ids=[DIALOGUE_A],
        duration_ms=1_500,
    )
    second = _spec(
        beat_prefix="beat",
        descriptions=["观察灯箱"],
        dialogue_ids=[DIALOGUE_B],
        duration_ms=1_500,
    )
    merged = _spec(
        beat_prefix="merged",
        descriptions=["进入月台", "观察灯箱"],
        dialogue_ids=[DIALOGUE_A, DIALOGUE_B],
    )
    merged.dialogue_or_narration[1].performance_note = second.dialogue_or_narration[
        0
    ].performance_note
    validate_merge_content([first, second], _target(merged))

    wrong_link = merged.model_copy(deep=True)
    wrong_link.dialogue_or_narration[1].beat_key = "merged-1"
    with pytest.raises(TransformConservationError, match="merge dialogue content"):
        validate_merge_content([first, second], _target(wrong_link))


@pytest.mark.parametrize(
    ("sources", "target", "message"),
    [
        (
            [
                _spec(
                    beat_prefix="a",
                    descriptions=["进入月台"],
                    dialogue_ids=[],
                ),
                _spec(
                    scene_id=OTHER_SCENE_ID,
                    beat_prefix="b",
                    descriptions=["切到站外"],
                    dialogue_ids=[],
                ),
            ],
            _target(
                _spec(
                    beat_prefix="m",
                    descriptions=["进入月台", "切到站外"],
                    dialogue_ids=[],
                    duration_ms=6_000,
                )
            ),
            "same scene",
        ),
        (
            [
                _spec(
                    beat_prefix="a",
                    descriptions=[f"动作 {index}" for index in range(5)],
                    dialogue_ids=[],
                ),
                _spec(
                    beat_prefix="b",
                    descriptions=[f"动作 {index}" for index in range(4)],
                    dialogue_ids=[],
                ),
            ],
            None,
            "at most 8 action beats",
        ),
        (
            [
                _spec(
                    beat_prefix="a",
                    descriptions=["进入月台"],
                    dialogue_ids=[],
                    duration_ms=8_000,
                ),
                _spec(
                    beat_prefix="b",
                    descriptions=["观察灯箱"],
                    dialogue_ids=[],
                    duration_ms=8_000,
                ),
            ],
            None,
            "15 seconds",
        ),
    ],
)
def test_merge_rejects_unrepresentable_sources(
    sources: list[ShotSpec],
    target: TargetShotSpecRequest | None,
    message: str,
) -> None:
    with pytest.raises(TransformConservationError, match=message):
        validate_merge_content(sources, target)


def test_merge_rejects_duplicate_dialogue_identity() -> None:
    first = _spec(
        beat_prefix="a",
        descriptions=["甲说话"],
        dialogue_ids=[DIALOGUE_A],
    )
    second = _spec(
        beat_prefix="b",
        descriptions=["切反应"],
        dialogue_ids=[DIALOGUE_A],
    )
    with pytest.raises(TransformConservationError, match="duplicate dialogue"):
        validate_merge_content([first, second], None)
