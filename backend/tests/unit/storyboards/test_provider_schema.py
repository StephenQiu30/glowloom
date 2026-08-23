from collections.abc import Sequence
from types import SimpleNamespace
from typing import Any, cast

import pytest
from uuid6 import uuid7

from app.integrations.codex_local import CodexLocalStoryboardDrafter
from app.modules.storyboards import (
    StoryboardDraftInput,
    StoryboardDraftProviderError,
    StoryboardDraftUnit,
)
from app.modules.storyboards.drafts import service as draft_service
from app.modules.storyboards.drafts.provider_schema import (
    STORYBOARD_DRAFT_PROMPT_VERSION,
    StoryboardProviderResult,
    expand_provider_result,
    normalize_storyboard_provider_payload,
    storyboard_draft_system_prompt,
)
from app.modules.storyboards.drafts.schemas import DraftProviderResult


def draft_input() -> StoryboardDraftInput:
    scene_id = uuid7()
    return StoryboardDraftInput(
        batch_id=uuid7(),
        task_id=uuid7(),
        input_hash="a" * 64,
        script_version_id=uuid7(),
        target_duration_ms=5_000,
        aspect_ratio="9:16",
        visual_style=None,
        units=(
            StoryboardDraftUnit(
                unit_version_id=uuid7(),
                position=1,
                kind="scene_heading",
                exact_text="内景·旧泵站·夜",
                required_for_coverage=True,
                source_scene_id=scene_id,
                source_dialogue_id=None,
            ),
            StoryboardDraftUnit(
                unit_version_id=uuid7(),
                position=2,
                kind="dialogue",
                exact_text="沈岚：别停！",
                required_for_coverage=True,
                source_scene_id=scene_id,
                source_dialogue_id=uuid7(),
            ),
        ),
        assets=(),
    )


def provider_result(unit_positions: list[int]) -> StoryboardProviderResult:
    return StoryboardProviderResult.model_validate(
        {
            "shots": [
                {
                    "proposal_key": "alarm",
                    "position": 1,
                    "title": "警报灯闪烁",
                    "unit_positions": unit_positions,
                    "dialogue_unit_positions": [2] if 2 in unit_positions else [],
                    "dialogue_deliveries": (
                        [
                            {
                                "unit_position": 2,
                                "beat_key": "reach",
                                "speaker_subject_key": "shen-lan",
                                "render_as_audio": True,
                                "performance_note": "急促、压低声音，尾字被警报盖住",
                            }
                        ]
                        if 2 in unit_positions
                        else []
                    ),
                    "purpose": "建立危机",
                    "continuity_note": "承接上一镜的警报声，保持沈岚向右运动",
                    "continuity_in": "沈岚位于画面左侧，面向右侧，警报持续",
                    "continuity_out": "沈岚到达控制杆，右手悬在杆前，仍面向右侧",
                    "shot_size": "wide",
                    "camera_angle": "eye_level",
                    "camera_movement": "dolly",
                    "composition": "倒计时与水位同框",
                    "environment": "进水的旧泵站",
                    "mood_lighting": "红色应急灯",
                    "subject_placements": [
                        {"subject_key": "shen-lan", "placement": "画面左前景，面向右侧"}
                    ],
                    "action_beats": [
                        {
                            "beat_key": "rush",
                            "order": 1,
                            "description": "沈岚冲向闸门",
                        },
                        {
                            "beat_key": "reach",
                            "order": 2,
                            "description": "右手伸向控制杆",
                        },
                    ],
                    "duration_ms": 3_000,
                    "ambient": "警报和水声",
                    "sound_effects": ["鞋底踏水", "控制杆金属震动"],
                    "asset_bindings": [],
                    "first_frame": "红色警报灯占据上方，沈岚位于左前景准备起跑",
                    "last_frame": "沈岚停在控制杆前，右手即将握杆",
                    "keyframe_notes": "中段切入控制杆与上涨水位的同框关系",
                    "risk_codes": [],
                }
            ]
        }
    )


def test_compact_provider_positions_expand_to_fixed_storyboard_ids() -> None:
    value = draft_input()

    result = expand_provider_result(provider_result([1, 2]), value)

    shot = result.shots[0]
    assert shot.narrative_unit_version_ids == [unit.unit_version_id for unit in value.units]
    assert shot.spec.script_reference.confirmed_script_version_id == value.script_version_id
    assert shot.spec.script_reference.scene_id == value.units[0].source_scene_id
    assert shot.spec.script_reference.dialogue_ids == [value.units[1].source_dialogue_id]
    assert shot.spec.duration_ms == 3_000
    assert shot.spec.narrative.continuity_note == (
        "承接上一镜的警报声，保持沈岚向右运动 | "
        "IN: 沈岚位于画面左侧，面向右侧，警报持续 | "
        "OUT: 沈岚到达控制杆，右手悬在杆前，仍面向右侧"
    )
    assert [placement.subject_key for placement in shot.spec.visual.subject_placements] == [
        "shen-lan"
    ]
    assert [beat.beat_key for beat in shot.spec.action_beats] == ["rush", "reach"]
    assert shot.spec.dialogue_or_narration[0].beat_key == "reach"
    assert shot.spec.dialogue_or_narration[0].speaker_subject_key == "shen-lan"
    assert shot.spec.dialogue_or_narration[0].performance_note == ("急促、压低声音，尾字被警报盖住")
    assert shot.spec.audio_intent is not None
    assert shot.spec.audio_intent.sound_effects == ["鞋底踏水", "控制杆金属震动"]
    assert shot.spec.generation_intent.first_frame == ("红色警报灯占据上方，沈岚位于左前景准备起跑")
    assert shot.spec.generation_intent.last_frame == "沈岚停在控制杆前，右手即将握杆"
    assert shot.spec.generation_intent.keyframe_notes == ("中段切入控制杆与上涨水位的同框关系")


def test_compact_provider_rejects_unknown_input_position() -> None:
    with pytest.raises(ValueError, match="unknown input position"):
        expand_provider_result(provider_result([99]), draft_input())


def test_compact_provider_rejects_units_from_multiple_scenes() -> None:
    value = draft_input()
    other_scene_unit = StoryboardDraftUnit(
        unit_version_id=uuid7(),
        position=3,
        kind="action",
        exact_text="闸门在另一处落下。",
        required_for_coverage=True,
        source_scene_id=uuid7(),
        source_dialogue_id=None,
    )
    mixed_value = StoryboardDraftInput(
        batch_id=value.batch_id,
        task_id=value.task_id,
        input_hash=value.input_hash,
        script_version_id=value.script_version_id,
        target_duration_ms=value.target_duration_ms,
        aspect_ratio=value.aspect_ratio,
        visual_style=value.visual_style,
        units=(*value.units, other_scene_unit),
        assets=value.assets,
    )

    with pytest.raises(ValueError, match="one confirmed scene"):
        expand_provider_result(provider_result([1, 3]), mixed_value)


def test_provider_local_key_tool_normalizes_model_owned_order_and_beat_keys() -> None:
    payload = provider_result([1, 2]).model_dump(mode="json")
    shot = payload["shots"][0]  # type: ignore[index]
    shot["position"] = 8  # type: ignore[index]
    shot["action_beats"][1]["beat_key"] = "rush"  # type: ignore[index]
    shot["action_beats"][1]["order"] = 1  # type: ignore[index]
    shot["dialogue_deliveries"][0]["beat_key"] = "rush"  # type: ignore[index]

    normalized = StoryboardProviderResult.model_validate(
        normalize_storyboard_provider_payload(payload)
    )

    assert normalized.shots[0].position == 1
    assert [beat.order for beat in normalized.shots[0].action_beats] == [1, 2]
    assert [beat.beat_key for beat in normalized.shots[0].action_beats] == [
        "beat-1",
        "beat-2",
    ]
    assert normalized.shots[0].dialogue_deliveries[0].beat_key == "beat-1"


@pytest.mark.asyncio
async def test_codex_storyboard_generation_uses_skill_output_validation() -> None:
    class InvalidModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            del messages
            return {"unexpected": True}

    drafter = CodexLocalStoryboardDrafter(model=InvalidModel(), verify_skill=False)

    with pytest.raises(StoryboardDraftProviderError) as error:
        await drafter.draft(draft_input())

    assert error.value.code == "skill_output_invalid"
    assert error.value.outcome == "failed"


@pytest.mark.asyncio
async def test_codex_storyboard_drafter_runs_four_specialized_stages_for_valid_candidate() -> None:
    class QueueModel:
        def __init__(self) -> None:
            shot_result = provider_result([1, 2]).model_dump(mode="json")
            shot_result["shots"][0]["duration_ms"] = 5_000  # type: ignore[index]
            self.results: list[object] = [
                {
                    "scene_key": 1,
                    "beats": [
                        {
                            "beat_key": "crisis",
                            "unit_positions": [1, 2],
                            "dramatic_function": "conflict",
                            "summary": "建立泵站危机并传达撤离命令",
                        }
                    ],
                    "conflict": "警报与上涨水位迫使沈岚立即行动",
                    "reveal": None,
                    "reaction": "沈岚喊出撤离命令",
                    "continuity_facts": ["沈岚面向控制杆"],
                },
                {
                    "scene_key": 1,
                    "spatial_axis": "沈岚到控制杆",
                    "movement_direction": "由左向右",
                    "eyeline": "看向右侧控制杆",
                    "entrances_exits": [],
                    "prop_states": ["控制杆尚未拉下"],
                    "rhythm": "快速建立后完成动作",
                    "duration_budget_ms": 5_000,
                    "shot_seeds": [
                        {
                            "seed_key": "crisis",
                            "beat_keys": ["crisis"],
                            "unit_positions": [1, 2],
                            "purpose": "建立危机并交代撤离命令",
                            "suggested_duration_ms": 5_000,
                        }
                    ],
                },
                shot_result,
                {"issues": []},
            ]
            self.messages: list[Sequence[Any]] = []

        async def ainvoke(self, messages: Sequence[Any]) -> object:
            self.messages.append(messages)
            return self.results.pop(0)

    model = QueueModel()
    result = DraftProviderResult.model_validate(
        await CodexLocalStoryboardDrafter(model=model, verify_skill=False).draft(draft_input())
    )

    assert len(model.messages) == 4
    assert [cast(str, message[0].content).split(" ", 1)[0] for message in model.messages] == [
        "$storyboard-source-analysis",
        "$storyboard-scene-plan",
        "$storyboard-shot-draft",
        "$storyboard-review",
    ]
    assert result.shots[0].spec.generation_intent.last_frame == ("沈岚停在控制杆前，右手即将握杆")


def test_storyboard_prompt_uses_intent_boundaries_instead_of_fixed_shot_counts() -> None:
    assert STORYBOARD_DRAFT_PROMPT_VERSION == "storyboard-draft-prompt-v5-key-table"
    prompt = storyboard_draft_system_prompt()
    assert "单一主要目的" in prompt
    assert "首帧" in prompt
    assert "尾帧" in prompt
    assert "对白" in prompt
    assert "连续性入口" in prompt
    assert "12–18" not in prompt
    assert "每镜 4" not in prompt


def test_storyboard_validation_allows_short_shots_and_non_fixed_shot_counts() -> None:
    value = draft_input()
    base_shot = expand_provider_result(provider_result([1, 2]), value).shots[0]
    payload = base_shot.model_dump(mode="json")
    shots: list[dict[str, Any]] = []
    for position in range(1, 31):
        shot = {**payload, "proposal_key": f"shot-{position}", "position": position}
        shot["spec"] = {**payload["spec"], "duration_ms": 2_000}
        shots.append(shot)
    result = DraftProviderResult.model_validate({"shots": shots})

    draft_service._validate_provider_result(  # pyright: ignore[reportPrivateUsage]
        result,
        cast(Any, SimpleNamespace(target_duration_ms=60_000)),
        cast(
            Any,
            [
                SimpleNamespace(
                    unit_version_id=unit.unit_version_id,
                    required_for_coverage=True,
                )
                for unit in value.units
            ],
        ),
    )
