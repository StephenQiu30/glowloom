import json
import os
from pathlib import Path
from typing import Any, Literal, cast
from uuid import NAMESPACE_URL, UUID, uuid5

import pytest

from app.core.config import get_settings
from app.integrations.deepseek import DeepSeekStoryboardDrafter
from app.modules.storyboards import (
    StoryboardDraftInput,
    StoryboardDraftUnit,
)
from app.modules.storyboards.drafts.schemas import DraftProviderResult

pytestmark = pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_DEEPSEEK_STORYBOARD_CONTRACT") != "1",
    reason=("set LANVERSE_RUN_DEEPSEEK_STORYBOARD_CONTRACT=1 with a configured DEEPSEEK_API_KEY"),
)


def _stable_id(value: str) -> UUID:
    return uuid5(NAMESPACE_URL, f"lanverse:mvp-a:{value}")


@pytest.mark.asyncio
async def test_real_deepseek_creates_reviewable_golden_storyboard() -> None:
    settings = get_settings()
    assert settings.deepseek_api_key is not None
    fixture_path = (
        Path(__file__).parents[1] / "fixtures" / "mvp_a" / "golden_candidate_harbor_countdown.json"
    )
    fixture = cast(dict[str, Any], json.loads(fixture_path.read_text(encoding="utf-8")))
    source_units = cast(list[dict[str, Any]], fixture["narrative_units"])
    scene_id = _stable_id("scene:pump-station")
    units = tuple(
        StoryboardDraftUnit(
            unit_version_id=_stable_id(f"unit:{unit['narrative_unit_id']}"),
            position=cast(int, unit["order"]),
            kind=cast(
                Literal["scene_heading", "action", "dialogue", "narration"],
                unit["kind"],
            ),
            exact_text=cast(str, unit["exact_text"]),
            required_for_coverage=cast(bool, unit["required_for_coverage"]),
            source_scene_id=scene_id,
            source_dialogue_id=(
                _stable_id(f"dialogue:{unit['narrative_unit_id']}")
                if unit["kind"] == "dialogue"
                else None
            ),
        )
        for unit in source_units
    )
    draft_input = StoryboardDraftInput(
        batch_id=_stable_id("batch:golden"),
        task_id=_stable_id("task:golden"),
        input_hash="a" * 64,
        script_version_id=_stable_id("script:golden"),
        target_duration_ms=92_000,
        aspect_ratio="9:16",
        visual_style="写实灾难悬疑，冷色潮湿环境，红色应急灯",
        units=units,
        assets=(),
    )

    result = DraftProviderResult.model_validate(
        await DeepSeekStoryboardDrafter(settings.deepseek_api_key).draft(draft_input)
    )

    assert 12 <= len(result.shots) <= 24
    assert 69_000 <= sum(shot.spec.duration_ms for shot in result.shots) <= 115_000
    assert all(4_000 <= shot.spec.duration_ms <= 15_000 for shot in result.shots)
    required = {unit.unit_version_id for unit in units if unit.required_for_coverage}
    covered = {unit_id for shot in result.shots for unit_id in shot.narrative_unit_version_ids}
    assert required <= covered
    allowed_units = {unit.unit_version_id for unit in units}
    assert covered <= allowed_units
    assert all(
        shot.spec.script_reference.confirmed_script_version_id == draft_input.script_version_id
        and shot.spec.script_reference.scene_id == scene_id
        and not shot.asset_references
        for shot in result.shots
    )
