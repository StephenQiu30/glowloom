import os
from uuid import NAMESPACE_URL, UUID, uuid5

import pytest

from app.core.config import get_settings
from app.integrations.codex_local import CodexLocalStoryboardDrafter
from app.modules.storyboards import (
    StoryboardDraftAsset,
    StoryboardDraftInput,
    StoryboardDraftUnit,
)
from app.modules.storyboards.drafts.schemas import DraftProviderResult

pytestmark = pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_CODEX_LOCAL_CONTRACT") != "1",
    reason="set LANVERSE_RUN_CODEX_LOCAL_CONTRACT=1 with a logged-in local Codex",
)


def _stable_id(value: str) -> UUID:
    return uuid5(NAMESPACE_URL, f"lanverse:mvp-a:{value}")


@pytest.mark.asyncio
async def test_real_codex_creates_reviewable_multi_scene_storyboard() -> None:
    settings = get_settings()
    control_room_scene_id = _stable_id("scene:pump-station-control-room")
    floodgate_scene_id = _stable_id("scene:pump-station-floodgate")
    warning_dialogue_id = _stable_id("dialogue:warning")
    response_dialogue_id = _stable_id("dialogue:response")
    units = (
        StoryboardDraftUnit(
            unit_version_id=_stable_id("unit:scene"),
            position=1,
            kind="scene_heading",
            exact_text="内景·旧泵站·夜",
            required_for_coverage=True,
            source_scene_id=control_room_scene_id,
            source_dialogue_id=None,
        ),
        StoryboardDraftUnit(
            unit_version_id=_stable_id("unit:alarm"),
            position=2,
            kind="action",
            exact_text="红色警报灯闪烁，水位快速上涨。",
            required_for_coverage=True,
            source_scene_id=control_room_scene_id,
            source_dialogue_id=None,
        ),
        StoryboardDraftUnit(
            unit_version_id=_stable_id("unit:warning"),
            position=3,
            kind="dialogue",
            exact_text="沈岚：快关闸！",
            required_for_coverage=True,
            source_scene_id=control_room_scene_id,
            source_dialogue_id=warning_dialogue_id,
        ),
        StoryboardDraftUnit(
            unit_version_id=_stable_id("unit:lever"),
            position=4,
            kind="action",
            exact_text="沈岚冲向控制杆。",
            required_for_coverage=True,
            source_scene_id=control_room_scene_id,
            source_dialogue_id=None,
        ),
        StoryboardDraftUnit(
            unit_version_id=_stable_id("unit:floodgate-scene"),
            position=5,
            kind="scene_heading",
            exact_text="内景·旧泵站泄洪门·连续",
            required_for_coverage=True,
            source_scene_id=floodgate_scene_id,
            source_dialogue_id=None,
        ),
        StoryboardDraftUnit(
            unit_version_id=_stable_id("unit:wrench-ready"),
            position=6,
            kind="action",
            exact_text="周野握紧黄色工业扳手，守在泄洪门另一侧。",
            required_for_coverage=True,
            source_scene_id=floodgate_scene_id,
            source_dialogue_id=None,
        ),
        StoryboardDraftUnit(
            unit_version_id=_stable_id("unit:response"),
            position=7,
            kind="dialogue",
            exact_text="周野：我来卡住齿轮！",
            required_for_coverage=True,
            source_scene_id=floodgate_scene_id,
            source_dialogue_id=response_dialogue_id,
        ),
        StoryboardDraftUnit(
            unit_version_id=_stable_id("unit:wrench-action"),
            position=8,
            kind="action",
            exact_text="闸门剧烈震动，周野把黄色工业扳手插入齿轮，沈岚隔窗向他点头。",
            required_for_coverage=True,
            source_scene_id=floodgate_scene_id,
            source_dialogue_id=None,
        ),
    )
    draft_input = StoryboardDraftInput(
        batch_id=_stable_id("batch:golden"),
        task_id=_stable_id("task:golden"),
        input_hash="a" * 64,
        script_version_id=_stable_id("script:golden"),
        target_duration_ms=30_000,
        aspect_ratio="9:16",
        visual_style="写实灾难悬疑，冷色潮湿环境，红色应急灯",
        units=units,
        assets=(
            StoryboardDraftAsset(
                asset_version_id=_stable_id("asset:yellow-industrial-wrench:v1"),
                position=1,
                kind="prop",
                name="黄色工业扳手",
                state_label="完好，表面有黄色磨损漆",
            ),
        ),
    )

    drafter = CodexLocalStoryboardDrafter(
        codex_cli_path=settings.codex_cli_path,
        codex_model=settings.codex_model,
        max_concurrency=2,
    )
    try:
        result = DraftProviderResult.model_validate(await drafter.draft(draft_input))
    finally:
        await drafter.aclose()

    assert result.shots
    assert len(result.shots) >= 2
    assert 22_500 <= sum(shot.spec.duration_ms for shot in result.shots) <= 37_500
    assert all(500 <= shot.spec.duration_ms <= 15_000 for shot in result.shots)
    incomplete_rows = [
        {
            "proposal_key": shot.proposal_key,
            "continuity_note": shot.spec.narrative.continuity_note,
            "action_beat_count": len(shot.spec.action_beats),
            "first_frame": shot.spec.generation_intent.first_frame,
            "last_frame": shot.spec.generation_intent.last_frame,
        }
        for shot in result.shots
        if not (
            shot.spec.narrative.continuity_note
            and shot.spec.action_beats
            and shot.spec.generation_intent.first_frame
        )
    ]
    assert not incomplete_rows, incomplete_rows
    assert any(shot.spec.generation_intent.last_frame is not None for shot in result.shots)
    required = {unit.unit_version_id for unit in units if unit.required_for_coverage}
    covered = {unit_id for shot in result.shots for unit_id in shot.narrative_unit_version_ids}
    assert required <= covered
    allowed_units = {unit.unit_version_id for unit in units}
    assert covered <= allowed_units
    scene_ids = {shot.spec.script_reference.scene_id for shot in result.shots}
    assert scene_ids == {control_room_scene_id, floodgate_scene_id}
    assert all(
        shot.spec.script_reference.confirmed_script_version_id == draft_input.script_version_id
        for shot in result.shots
    )
    assert any(shot.spec.dialogue_or_narration for shot in result.shots)
    referenced_assets = {
        reference.asset_version_id for shot in result.shots for reference in shot.asset_references
    }
    assert referenced_assets == {_stable_id("asset:yellow-industrial-wrench:v1")}
