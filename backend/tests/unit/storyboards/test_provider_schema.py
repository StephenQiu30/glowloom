from collections.abc import Sequence
from typing import Any

import pytest
from pydantic import SecretStr
from uuid6 import uuid7

from app.integrations.deepseek import DeepSeekStoryboardDrafter
from app.modules.storyboards import (
    StoryboardDraftInput,
    StoryboardDraftProviderError,
    StoryboardDraftUnit,
)
from app.modules.storyboards.drafts.provider_schema import (
    StoryboardProviderResult,
    expand_provider_result,
)


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
                    "scene_unit_position": unit_positions[0],
                    "dialogue_unit_positions": [2] if 2 in unit_positions else [],
                    "purpose": "建立危机",
                    "shot_size": "wide",
                    "camera_angle": "eye_level",
                    "camera_movement": "dolly",
                    "composition": "倒计时与水位同框",
                    "environment": "进水的旧泵站",
                    "mood_lighting": "红色应急灯",
                    "action": "沈岚冲向闸门",
                    "duration_ms": 3_000,
                    "ambient": "警报和水声",
                    "asset_bindings": [],
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
    assert shot.spec.duration_ms == 4_000


def test_compact_provider_rejects_unknown_input_position() -> None:
    with pytest.raises(ValueError, match="unknown input position"):
        expand_provider_result(provider_result([99]), draft_input())


@pytest.mark.asyncio
async def test_deepseek_storyboard_generation_uses_skill_output_validation() -> None:
    class InvalidModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            del messages
            return {"unexpected": True}

    drafter = DeepSeekStoryboardDrafter(
        SecretStr("unused-in-unit-test"),
        model=InvalidModel(),
    )

    with pytest.raises(StoryboardDraftProviderError) as error:
        await drafter.draft(draft_input())

    assert error.value.code == "skill_output_invalid"
    assert error.value.outcome == "failed"
