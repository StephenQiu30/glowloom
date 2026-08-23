import hashlib
import json
from pathlib import Path
from typing import cast

import pytest

from app.modules.scripts.documents.analysis import analyze_document

FIXTURE_FILE = Path(__file__).parents[3] / "fixtures/mvp_a/script_format_cases.json"


def _format_cases() -> list[dict[str, object]]:
    loaded: object = json.loads(FIXTURE_FILE.read_text(encoding="utf-8"))
    assert isinstance(loaded, list)
    return [
        cast(dict[str, object], item)
        for item in cast(list[object], loaded)
        if isinstance(item, dict)
    ]


@pytest.mark.parametrize(
    "case",
    _format_cases(),
    ids=lambda case: str(case["fixture_id"]),
)
def test_format_fixture_markers_issues_and_status_are_exact(
    case: dict[str, object],
) -> None:
    text = str(case["input_text"])

    analysis = analyze_document(text)

    assert analysis.status == case["expected_analysis"]
    assert [
        {
            "episode_number": marker.episode_number,
            "marker_text": marker.marker_text,
            "line_number": marker.line_number,
            "start_codepoint": marker.start_codepoint,
            "end_codepoint": marker.end_codepoint,
        }
        for marker in analysis.markers
    ] == case["expected_markers"]
    assert [issue.code for issue in analysis.issues] == case["expected_problem_kinds"]


@pytest.mark.parametrize(
    "case",
    _format_cases(),
    ids=lambda case: str(case["fixture_id"]),
)
def test_every_block_is_a_contiguous_exact_codepoint_slice(
    case: dict[str, object],
) -> None:
    text = str(case["input_text"])

    analysis = analyze_document(text)

    assert analysis.content_hash == hashlib.sha256(text.encode("utf-8")).hexdigest()
    assert analysis.codepoint_count == len(text)
    assert analysis.blocks
    assert analysis.blocks[0].start_codepoint == 0
    assert analysis.blocks[-1].end_codepoint == len(text)
    assert (
        "".join(text[block.start_codepoint : block.end_codepoint] for block in analysis.blocks)
        == text
    )
    for position, block in enumerate(analysis.blocks, start=1):
        exact = text[block.start_codepoint : block.end_codepoint]
        assert block.position == position
        assert block.start_codepoint < block.end_codepoint
        assert block.text_hash == hashlib.sha256(exact.encode("utf-8")).hexdigest()
        if position > 1:
            assert analysis.blocks[position - 2].end_codepoint == block.start_codepoint


def test_line_taxonomy_preserves_separators_and_does_not_use_order_as_identity() -> None:
    text = "人物表：甲。\n\n第一集\n内景·控制室·夜\n甲：开始。\n旁白：警报响起。\n门外传来脚步声。"

    analysis = analyze_document(text)

    assert [block.kind for block in analysis.blocks] == [
        "preamble",
        "separator",
        "episode_marker",
        "scene_heading",
        "dialogue",
        "narration",
        "action",
    ]
    assert [issue.code for issue in analysis.issues] == ["preamble_requires_decision"]


def test_i_e_scene_heading_is_recognized_for_script_chunk_boundaries() -> None:
    analysis = analyze_document("EPISODE 1\nI/E. CHILDREN'S CABIN / SHAFT – DAY\nA door opens.")

    assert [block.kind for block in analysis.blocks] == [
        "episode_marker",
        "scene_heading",
        "action",
    ]


def test_bom_is_precisely_reported_without_losing_the_first_marker() -> None:
    text = "\ufeff第一集\n正文。"

    analysis = analyze_document(text)

    assert analysis.status == "rejected"
    assert [issue.code for issue in analysis.issues] == ["utf8_bom_not_allowed"]
    assert analysis.issues[0].start_codepoint == 0
    assert analysis.issues[0].end_codepoint == 1
    assert analysis.issues[0].line_number == 1
    assert analysis.issues[0].column_number == 1
    assert analysis.markers[0].marker_text == "第一集"
    assert analysis.markers[0].start_codepoint == 1


def test_explicit_episode_count_is_not_limited_by_an_import_cap() -> None:
    text = "\n".join(
        part for number in range(1, 102) for part in (f"第{number}集", f"第{number}集的正文。")
    )

    analysis = analyze_document(text)

    assert analysis.status == "deterministic"
    assert [marker.episode_number for marker in analysis.markers] == list(range(1, 102))
    assert analysis.issues == ()
