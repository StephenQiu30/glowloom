from __future__ import annotations

import json
from pathlib import Path
from typing import cast

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
    manifest = scene_analysis_wire_schema_manifest()
    assert manifest == json.loads(FIXTURE.read_text(encoding="utf-8"))
    schemas = cast(list[dict[str, str]], manifest["schemas"])
    assert [schema["contract_id"] for schema in schemas] == [
        "storygraph-dispatch-authorization-claims-production",
        "storygraph-reference-brief-stage-attempt-result-production",
        "storygraph-reference-brief-stage-invocation-production",
        "storygraph-reference-plan-stage-attempt-result-production",
        "storygraph-reference-plan-stage-invocation-production",
        "storygraph-stage-attempt-result-production",
        "storygraph-stage-invocation-production",
        "storygraph-vision-review-stage-attempt-result-production",
        "storygraph-vision-review-stage-invocation-production",
        "storygraph-visual-foundation-stage-attempt-result-production",
        "storygraph-visual-foundation-stage-invocation-production",
    ]
