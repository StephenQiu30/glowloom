from app.modules.scripts.extractions.anchoring import anchor_script_structure_ranges
from app.modules.scripts.extractions.ports import SCRIPT_STRUCTURE_EXTRACTOR_VERSION
from app.modules.scripts.extractions.schemas import (
    DialogueCandidateProposal,
    ScriptExtractionResult,
)
from app.modules.skills.script_structure_prompt import script_structure_system_prompt


def _production_tasks() -> list[dict[str, str]]:
    return [
        {
            "task_type": "shot_breakdown",
            "title": "拆解场景分镜",
            "objective": "将场景拆解为可审核镜头。",
            "priority": "normal",
        }
    ]


def test_script_structure_extractor_snapshot_is_complete_and_bounded() -> None:
    assert (
        SCRIPT_STRUCTURE_EXTRACTOR_VERSION
        == "langgraph-map-reduce-v1:prompt-v7:schema-v5:anchor-v2"
    )
    assert len(SCRIPT_STRUCTURE_EXTRACTOR_VERSION) <= 80


def test_script_structure_prompt_keeps_storyboards_in_their_own_skill() -> None:
    prompt = script_structure_system_prompt()

    assert "禁止生成 shot 候选" in prompt
    assert "storyboard.plan" in prompt
    assert "Markdown 标题" in prompt
    assert "逐字复制原文中的场景标题行" in prompt
    assert "每个 scene 必须给出 1 到 4 个" in prompt
    assert "不允许空数组" in prompt
    assert "至少包含一个 shot_breakdown" in prompt


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
                        "production_tasks": _production_tasks(),
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
                        "production_tasks": _production_tasks(),
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


def test_script_structure_anchoring_ignores_colon_metadata_before_first_scene() -> None:
    script = (
        "Continuity Revision: 08/03/2026\n"
        "Vertical-Drama Dialogue Polish: 08/03/2026\n"
        "INT. HOUSE - DAY\n"
        "MARA: Check the door."
    )
    raw_result = ScriptExtractionResult.model_validate(
        {
            "candidates": [
                {
                    "candidate_key": "scene-house",
                    "source_range": {"start": 0, "end": len(script)},
                    "proposal": {
                        "kind": "scene",
                        "heading": "INT. HOUSE - DAY",
                        "location": "HOUSE",
                        "time_of_day": "DAY",
                        "summary": "Mara checks the door.",
                        "production_tasks": _production_tasks(),
                    },
                }
            ]
        }
    )

    result = anchor_script_structure_ranges(raw_result, script)

    assert [candidate.proposal.kind for candidate in result.candidates] == [
        "scene",
        "dialogue",
    ]
    dialogue = result.candidates[1]
    assert dialogue.source_range.start == script.index("MARA: Check the door.")


def test_script_structure_anchoring_extracts_screenplay_cues_and_drops_technical_cards() -> None:
    script = (
        "INT. HOUSE - DAY\n"
        "AURELIA (V.O.)\n\n"
        "I erased\n"
        "my name.\n\n"
        "CARD: 24 HOURS AGO\n\n"
        "VFX SHOT: The sky flashes.\n\n"
        "TRISTAN\n\n"
        "(annoyed) Leave now.\n\n"
        "TRISTAN\n\n"
        "My little star.\n\n"
        "(to Jace, softly) Protect your sister."
    )
    raw_result = ScriptExtractionResult.model_validate(
        {
            "candidates": [
                {
                    "candidate_key": "scene-house",
                    "source_range": {"start": 0, "end": len(script)},
                    "proposal": {
                        "kind": "scene",
                        "heading": "INT. HOUSE - DAY",
                        "location": "HOUSE",
                        "time_of_day": "DAY",
                        "summary": "A family argues before the sky flashes.",
                        "production_tasks": _production_tasks(),
                    },
                },
                {
                    "candidate_key": "wrong-card-dialogue",
                    "source_range": {
                        "start": script.index("CARD:"),
                        "end": script.index("CARD:") + len("CARD: 24 HOURS AGO"),
                    },
                    "proposal": {
                        "kind": "dialogue",
                        "scene_candidate_key": "scene-house",
                        "speaker_candidate": "CARD",
                        "dialogue_kind": "spoken",
                        "text": "24 HOURS AGO",
                    },
                },
            ]
        }
    )

    result = anchor_script_structure_ranges(raw_result, script)
    dialogues = [
        candidate.proposal
        for candidate in result.candidates
        if isinstance(candidate.proposal, DialogueCandidateProposal)
    ]

    assert [(dialogue.speaker_candidate, dialogue.text) for dialogue in dialogues] == [
        ("AURELIA", "I erased my name."),
        ("TRISTAN", "Leave now."),
        ("TRISTAN", "My little star."),
        ("TRISTAN", "Protect your sister."),
    ]
    assert dialogues[0].dialogue_kind == "voice_over"
    assert dialogues[1].performance_note == "annoyed"
    assert dialogues[3].performance_note == "to Jace, softly"
