from __future__ import annotations

import json
from pathlib import Path
from typing import cast

from app.harness.scene_analysis_schema_manifest import (
    scene_analysis_input_schema_manifest,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
FIXTURE = REPOSITORY_ROOT.joinpath(
    "backend",
    "tests",
    "fixtures",
    "agent",
    "storygraph-scene-analysis-input-schemas.json",
)


def test_scene_analysis_input_schema_manifest_matches_the_backend_fixture() -> None:
    manifest = scene_analysis_input_schema_manifest()
    assert manifest == json.loads(FIXTURE.read_text(encoding="utf-8"))
    schemas = cast(list[dict[str, str]], manifest["schemas"])
    assert next(
        schema for schema in schemas if schema["stage_key"] == "resolve_visual_foundation"
    ) == {
        "stage_key": "resolve_visual_foundation",
        "profile_key": "default",
        "input_contract_id": "visual-foundation-input-production",
        "schema_hash": "4fe72c85b00f4f3348c6bb16e908d74365c873bddd4b0d5c3e919a67b00793df",
    }
    assert next(schema for schema in schemas if schema["stage_key"] == "plan_reference_assets") == {
        "stage_key": "plan_reference_assets",
        "profile_key": "default",
        "input_contract_id": "reference-plan-input-production",
        "schema_hash": "cb02abe281b1fdcd08bf01ae2ed4dd681c9a09d07676c391c5b7f1f45883c8cd",
    }
