from datetime import UTC, datetime

from uuid6 import uuid7

from app.modules.scripts.models import Dialogue, Scene
from app.modules.scripts.narratives.parser import parse_narrative_units
from app.modules.scripts.narratives.service import build_confirmed_narrative_units


def test_confirmed_units_drop_preamble_and_merge_anchored_dialogue() -> None:
    body = (
        "Revision Note: 2026-08-03\n"
        "INT. CABIN - DAY\n"
        "CARD: HOURS AGO\n"
        "MARA (V.O.)\n\n"
        "We leave now.\n"
        "VFX SHOT: The dome flashes.\n"
        "I/E. WATER SHAFT - DAY\n"
        "Water surges.\n"
    )
    version_id = uuid7()
    workspace_id = uuid7()
    now = datetime.now(UTC)
    first_scene_start = body.index("INT. CABIN")
    second_scene_start = body.index("I/E. WATER")
    first_scene = Scene(
        id=uuid7(),
        workspace_id=workspace_id,
        script_version_id=version_id,
        position=1,
        heading="INT. CABIN - DAY",
        location="CABIN",
        time_of_day="DAY",
        summary="Mara speaks.",
        semantic_context={},
        source_start=first_scene_start,
        source_end=second_scene_start,
        created_at=now,
    )
    second_scene = Scene(
        id=uuid7(),
        workspace_id=workspace_id,
        script_version_id=version_id,
        position=2,
        heading="I/E. WATER SHAFT - DAY",
        location="WATER SHAFT",
        time_of_day="DAY",
        summary="Water surges.",
        semantic_context={},
        source_start=second_scene_start,
        source_end=len(body),
        created_at=now,
    )
    dialogue_start = body.index("MARA (V.O.)")
    dialogue_end = body.index("We leave now.") + len("We leave now.")
    dialogue = Dialogue(
        id=uuid7(),
        workspace_id=workspace_id,
        scene_id=first_scene.id,
        position=1,
        speaker_candidate="MARA",
        dialogue_kind="voice_over",
        text="We leave now.",
        performance_note="V.O.",
        source_start=dialogue_start,
        source_end=dialogue_end,
        created_at=now,
    )

    units = build_confirmed_narrative_units(
        body,
        parse_narrative_units(body),
        [first_scene, second_scene],
        [dialogue],
    )

    assert units[0].exact_text == "INT. CABIN - DAY"
    assert all("Revision Note" not in unit.exact_text for unit in units)
    assert [unit.kind for unit in units].count("dialogue") == 1
    dialogue_unit = next(unit for unit in units if unit.kind == "dialogue")
    assert dialogue_unit.exact_text == "MARA (V.O.)\n\nWe leave now."
    assert next(unit for unit in units if unit.exact_text.startswith("CARD:")).kind == "action"
    assert next(unit for unit in units if unit.exact_text.startswith("VFX SHOT:")).kind == "action"
    assert next(unit for unit in units if unit.exact_text.startswith("I/E.")).kind == (
        "scene_heading"
    )
