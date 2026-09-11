from __future__ import annotations

import json
from pathlib import Path
from typing import cast

from app.modules.storygraph.scene_analysis_registry import (
    scene_analysis_candidate_schema_manifest,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
FIXTURE = (
    REPOSITORY_ROOT
    / "backend/tests/fixtures/agent/storygraph-scene-analysis-candidate-schemas.json"
)


def test_scene_analysis_candidate_schema_manifest_matches_the_backend_fixture() -> None:
    manifest = scene_analysis_candidate_schema_manifest()
    assert manifest == json.loads(FIXTURE.read_text(encoding="utf-8"))
    schemas = cast(list[dict[str, str]], manifest["schemas"])
    assert next(
        schema for schema in schemas if schema["stage_key"] == "resolve_visual_foundation"
    ) == {
        "stage_key": "resolve_visual_foundation",
        "profile_key": "default",
        "candidate_type": "visual_foundation_candidate",
        "output_schema_version": "visual-foundation-candidate-production",
        "schema_hash": "2c4b612079acb4874032e0e8358a272bd71cce45e6473209dda21cef29e40c4c",
    }
