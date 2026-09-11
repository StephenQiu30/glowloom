from __future__ import annotations

import json
from pathlib import Path

from app.modules.storygraph.scene_analysis_registry import (
    scene_analysis_candidate_schema_manifest,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
FIXTURE = (
    REPOSITORY_ROOT
    / "backend/tests/fixtures/agent/storygraph-scene-analysis-candidate-schemas.json"
)


def test_scene_analysis_candidate_schema_manifest_matches_the_backend_fixture() -> None:
    assert scene_analysis_candidate_schema_manifest() == json.loads(
        FIXTURE.read_text(encoding="utf-8")
    )
