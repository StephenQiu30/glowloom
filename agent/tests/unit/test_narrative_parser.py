from types import SimpleNamespace
from uuid import uuid4

from app.modules.scripts.narratives.parser import ParsedUnit, parse_narrative_units
from app.modules.scripts.narratives.service import (
    _source_links,  # pyright: ignore[reportPrivateUsage]
)


def test_parser_excludes_markdown_section_headings_from_storyboard_units() -> None:
    body = "# 《雾港来信》\n\n## 序幕：失踪的灯塔\n\n外景·临海旧车站·暴雨夜\n林澈：哥哥失踪三年。\n"

    units = parse_narrative_units(body)

    assert [unit.exact_text for unit in units] == [
        "外景·临海旧车站·暴雨夜",
        "林澈：哥哥失踪三年。",
    ]
    assert [unit.kind for unit in units] == ["scene_heading", "dialogue"]


def test_source_links_dialogue_by_exact_content_when_ai_range_is_inexact() -> None:
    scene_id = uuid4()
    dialogue_id = uuid4()
    parsed = ParsedUnit(
        kind="dialogue",
        source_start=97,
        source_end=108,
        exact_text="林澈：哥哥失踪三年。",
    )
    dialogue = SimpleNamespace(
        id=dialogue_id,
        scene_id=scene_id,
        source_start=91,
        source_end=102,
        speaker_candidate="林澈",
        text="哥哥失踪三年。",
    )

    assert _source_links(parsed, [], [dialogue]) == (scene_id, dialogue_id)  # type: ignore[list-item]
