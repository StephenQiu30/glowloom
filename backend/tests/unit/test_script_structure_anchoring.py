from app.modules.scripts.extractions.anchoring import anchor_script_structure_ranges
from app.modules.scripts.extractions.ports import SCRIPT_STRUCTURE_EXTRACTOR_VERSION
from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.skills.script_structure_prompt import script_structure_system_prompt


def test_script_structure_extractor_snapshot_is_complete_and_bounded() -> None:
    assert SCRIPT_STRUCTURE_EXTRACTOR_VERSION == "langgraph-map-reduce-v1:prompt-v5:schema-v3"
    assert len(SCRIPT_STRUCTURE_EXTRACTOR_VERSION) <= 80


def test_script_structure_prompt_keeps_storyboards_in_their_own_skill() -> None:
    prompt = script_structure_system_prompt()

    assert "禁止生成 shot 候选" in prompt
    assert "storyboard.plan" in prompt
    assert "Markdown 标题" in prompt
    assert "逐字复制原文中的场景标题行" in prompt


def test_script_structure_ranges_are_anchored_to_source() -> None:
    script = "内景·屋内·日\n林澈：开始。\n外景·路口·夜\n周岑：停下。"
    raw_result = ScriptExtractionResult.model_validate(
        {
            "candidates": [
                {
                    "candidate_key": "scene-1",
                    "source_range": {"start": 1, "end": len(script)},
                    "proposal": {
                        "kind": "scene",
                        "heading": "内景·屋内·日",
                        "location": "屋内",
                        "time_of_day": "日",
                        "summary": "林澈开始行动。",
                    },
                },
                {
                    "candidate_key": "dialogue-1",
                    "source_range": {"start": 1, "end": len(script)},
                    "proposal": {
                        "kind": "dialogue",
                        "scene_candidate_key": "scene-1",
                        "speaker_candidate": "林澈",
                        "dialogue_kind": "spoken",
                        "text": "开始。",
                    },
                },
                {
                    "candidate_key": "asset-1",
                    "source_range": {"start": 1, "end": len(script)},
                    "proposal": {
                        "kind": "asset",
                        "asset_kind": "character",
                        "name": "周岑",
                        "description": "路口出现的角色",
                    },
                },
                {
                    "candidate_key": "scene-2",
                    "source_range": {"start": 1, "end": len(script)},
                    "proposal": {
                        "kind": "scene",
                        "heading": "外景·路口·夜",
                        "location": "路口",
                        "time_of_day": "夜",
                        "summary": "周岑阻止行动。",
                    },
                },
                {
                    "candidate_key": "dialogue-2",
                    "source_range": {"start": 1, "end": len(script)},
                    "proposal": {
                        "kind": "dialogue",
                        "scene_candidate_key": "scene-2",
                        "speaker_candidate": "周岑",
                        "dialogue_kind": "spoken",
                        "text": "停下。",
                    },
                },
            ]
        }
    )

    result = anchor_script_structure_ranges(raw_result, script)

    second_scene_start = script.index("外景·路口·夜")
    ranges = {
        item.candidate_key: (item.source_range.start, item.source_range.end)
        for item in result.candidates
    }
    assert ranges == {
        "scene-1": (0, second_scene_start),
        "dialogue-1": (
            script.index("林澈：开始。"),
            script.index("林澈：开始。") + len("林澈：开始。"),
        ),
        "asset-1": (script.index("周岑"), script.index("周岑") + len("周岑")),
        "scene-2": (second_scene_start, len(script)),
        "dialogue-2": (
            script.index("周岑：停下。"),
            script.index("周岑：停下。") + len("周岑：停下。"),
        ),
    }
