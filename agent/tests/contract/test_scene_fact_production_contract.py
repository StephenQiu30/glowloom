from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.candidate_runtime.canonical import production_canonical_hash, production_canonical_json
from app.candidate_runtime.scene_analysis_schemas import SceneAnalysisInvocation
from app.modules.storygraph.scene_analysis_candidates import (
    SceneFactCandidate,
    ScriptSpanCandidate,
)

FIXTURE = (
    Path(__file__).resolve().parents[3]
    / "backend"
    / "tests"
    / "fixtures"
    / "agent"
    / "storygraph-scene-analysis-wire.json"
)


def fixture() -> dict[str, Any]:
    return json.loads(FIXTURE.read_text(encoding="utf-8"))


def test_production_wire_and_unicode_hash_match_backend_fixture() -> None:
    value = fixture()
    invocation = SceneAnalysisInvocation.model_validate(value["valid_invocation"])
    assert invocation.wire_schema_version == "storygraph-stage-wire-production"
    assert invocation.payload.variant.lane_key == "primary"
    assert invocation.payload.variant.output_schema_version == "script-span-candidate-production"
    assert invocation.compute_input_hash() == value["expected_input_hash"]
    assert invocation.stage_instance_key() == value["expected_stage_instance_key"]
    assert (
        production_canonical_json(value["canonical_unicode_root"]).decode()
        == value["canonical_unicode_json"]
    )
    assert (
        production_canonical_hash(value["canonical_unicode_root"])
        == value["canonical_unicode_hash"]
    )


def test_production_wire_rejects_unknown_style_and_cross_project_fields() -> None:
    value = cast(dict[str, Any], fixture()["valid_invocation"])
    for mutation in fixture()["reject_mutations"]:
        changed = copy.deepcopy(value)
        if mutation["operation"] == "remove":
            del changed[mutation["path"]]
        elif mutation["operation"] == "add_stage_input":
            changed["payload"]["stage_input"][mutation["path"]] = mutation["value"]
        elif mutation["operation"] == "replace_scope":
            changed["payload"]["scope"][mutation["path"]] = mutation["value"]
        else:
            changed[mutation["path"]] = mutation["value"]
        with pytest.raises(ValidationError, match=".+"):
            SceneAnalysisInvocation.model_validate(changed)


def test_script_spans_cover_every_unicode_code_point_and_scene_facts_are_style_blind() -> None:
    value = fixture()
    source = value["valid_invocation"]["payload"]["stage_input"]["normalized_text"]
    spans = ScriptSpanCandidate.model_validate(value["valid_script_span_candidate"])
    spans.validate_for_text(source)
    assert spans.coverage.covered_codepoints == len(source)

    drifted = copy.deepcopy(value["valid_script_span_candidate"])
    drifted["spans"][1]["codepoint_start"] += 1
    with pytest.raises(ValidationError):
        ScriptSpanCandidate.model_validate(drifted)

    scene_facts = SceneFactCandidate.model_validate(value["valid_scene_fact_candidate"])
    scene_facts.validate_for_spans(source, spans.spans)
    styled = copy.deepcopy(value["valid_scene_fact_candidate"])
    styled["scenes"][0]["visual_style"] = "赛博朋克"
    with pytest.raises(ValidationError):
        SceneFactCandidate.model_validate(styled)
