from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any, cast
from uuid import UUID

import pytest

from app.candidate_runtime.canonical import production_canonical_hash
from app.candidate_runtime.scene_analysis_schemas import (
    SceneAnalysisInvocation,
    SceneAnalysisPayload,
)
from app.modules.storygraph.scene_analysis_candidates import (
    SceneFactCandidate,
    ScriptSpanCandidate,
)
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = (
    REPOSITORY_ROOT
    / "backend"
    / "tests"
    / "fixtures"
    / "agent"
    / "storygraph-scene-analysis-wire.json"
)


def nested_keys(value: Any) -> list[str]:
    if isinstance(value, dict):
        mapping = cast(dict[str, Any], value)
        return [*mapping, *(key for item in mapping.values() for key in nested_keys(item))]
    if isinstance(value, list):
        return [key for item in cast(list[Any], value) for key in nested_keys(item)]
    return []


@pytest.mark.skipif(
    os.getenv("LANVERSE_TEST_REAL_CODEX") != "1",
    reason="set LANVERSE_TEST_REAL_CODEX=1 to exercise the locally authenticated Codex CLI",
)
@pytest.mark.asyncio
async def test_real_codex_turns_a_chinese_script_into_strict_scene_facts() -> None:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    span_invocation = SceneAnalysisInvocation.model_validate(fixture["valid_invocation"])
    span_candidate = await SceneAnalysisHarness(
        span_invocation, repository_root=REPOSITORY_ROOT
    ).execute()

    assert isinstance(span_candidate, ScriptSpanCandidate)
    source_input = span_invocation.payload.stage_input
    normalized_text = cast(str, source_input["normalized_text"])
    span_candidate.validate_for_text(normalized_text)
    assert len(span_candidate.spans) == 2

    span_revision_id = UUID("99999999-9999-4999-8999-999999999999")
    span_revision_hash = production_canonical_hash(span_candidate.model_dump(mode="json"))
    payload = span_invocation.payload.model_dump(mode="json")
    payload["variant"] = {
        "stage_key": "extract_scene_facts",
        "profile_key": "default",
        "lane_key": "primary",
        "output_schema_version": "scene-fact-candidate-production",
    }
    payload["upstream_candidates"] = [
        {
            "stage_key": "propose_script_spans",
            "shard_key": span_invocation.payload.shard.shard_key,
            "candidate_revision_id": str(span_revision_id),
            "candidate_revision_hash": span_revision_hash,
            "source_invocation_id": str(span_invocation.invocation_id),
            "source_result_hash": span_revision_hash,
        }
    ]
    payload["stage_input"] = {
        "source_version_id": str(span_candidate.source_version_id),
        "source_hash": span_candidate.source_hash,
        "normalized_text": normalized_text,
        "span_candidate_revision_id": str(span_revision_id),
        "span_candidate_revision_hash": span_revision_hash,
        "span_candidate": span_candidate.model_dump(mode="json"),
    }
    fact_invocation = SceneAnalysisInvocation.build(
        invocation_id=UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
        attempt_id=UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
        stage_release=span_invocation.stage_release.model_copy(
            update={"stage_release_hash": "4" * 64}
        ),
        control=span_invocation.control.model_copy(
            update={
                "control_record_id": UUID("cccccccc-cccc-4ccc-8ccc-cccccccccccc"),
                "control_hash": "6" * 64,
            }
        ),
        budget=span_invocation.budget,
        payload=SceneAnalysisPayload.model_validate(payload),
    )
    fact_candidate = await SceneAnalysisHarness(
        fact_invocation, repository_root=REPOSITORY_ROOT
    ).execute()

    assert isinstance(fact_candidate, SceneFactCandidate)
    fact_candidate.validate_for_spans(normalized_text, span_candidate.spans)
    assert len(fact_candidate.scenes) == len(span_candidate.spans) == 2
    assert any(scene.raw_character_mentions for scene in fact_candidate.scenes)
    assert all("style" not in key for key in nested_keys(fact_candidate.model_dump(mode="json")))
