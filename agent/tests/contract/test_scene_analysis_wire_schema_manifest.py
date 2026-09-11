from __future__ import annotations

import json
from pathlib import Path

from app.harness.scene_analysis_schema_manifest import (
    scene_analysis_wire_schema_manifest,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
FIXTURE = REPOSITORY_ROOT.joinpath(
    "backend",
    "tests",
    "fixtures",
    "agent",
    "storygraph-scene-analysis-wire-schemas.json",
)


def test_scene_analysis_wire_schema_manifest_matches_the_backend_fixture() -> None:
    expected = json.loads(FIXTURE.read_text(encoding="utf-8"))
    assert scene_analysis_wire_schema_manifest() == expected
