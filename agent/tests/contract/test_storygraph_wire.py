from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.candidate_runtime.schemas import (
    StoryAnalysisStageInput,
    StoryGraphExecutionGrantClaims,
    StoryGraphStageInvocation,
    StoryGraphStageResult,
    StoryReconciliationStageInput,
)

FIXTURE = (
    Path(__file__).resolve().parents[3]
    / "backend"
    / "tests"
    / "fixtures"
    / "agent"
    / "storygraph-stage-wire-v1.json"
)


def _fixture() -> dict[str, Any]:
    return json.loads(FIXTURE.read_text(encoding="utf-8"))


def test_storygraph_stage_invocation_matches_backend_canonical_fixture() -> None:
    fixture = _fixture()
    invocation = StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])
    assert invocation.compute_input_hash() == fixture["expected_input_hash"]
    assert invocation.input_hash == fixture["expected_input_hash"]
    assert invocation.stage_instance_key() == fixture["expected_stage_instance_key"]

    changed_id = invocation.model_copy(
        update={"invocation_id": "20000000-0000-0000-0000-000000000099"}
    )
    assert changed_id.compute_input_hash() == invocation.input_hash
    changed_policy = invocation.execution_policy.model_copy(
        update={"max_model_calls": invocation.execution_policy.max_model_calls + 1}
    )
    changed = invocation.model_copy(update={"execution_policy": changed_policy})
    assert changed.compute_input_hash() != invocation.input_hash


def test_source_evidence_stage_input_is_strict_and_bound_to_the_source_shard() -> None:
    fixture = _fixture()
    valid = cast(dict[str, Any], fixture["valid_invocation"])

    with_extra = copy.deepcopy(valid)
    with_extra["payload"]["stage_input"]["unexpected"] = True
    with pytest.raises(ValidationError):
        StoryGraphStageInvocation.model_validate(with_extra)

    drifted_range = copy.deepcopy(valid)
    drifted_range["payload"]["stage_input"]["logical_end"] = 10
    with pytest.raises(ValidationError):
        StoryGraphStageInvocation.model_validate(drifted_range)

    drifted_source = copy.deepcopy(valid)
    drifted_source["payload"]["stage_input"]["normalized_hash"] = "b" * 64
    with pytest.raises(ValidationError):
        StoryGraphStageInvocation.model_validate(drifted_source)


def test_story_analysis_input_binds_one_exact_evidence_revision() -> None:
    value = StoryAnalysisStageInput.model_validate(
        {
            "evidence_shard_key": "source:00000000:00000012",
            "evidence_candidate_revision_id": "10000000-0000-0000-0000-000000000001",
            "evidence_candidate_revision_hash": "a" * 64,
            "logical_start": 0,
            "logical_end": 12,
            "candidate_item_start": 0,
            "candidate_item_end": 0,
            "evidence_candidate": {"observations": [], "review_issues": []},
        }
    )
    assert value.logical_end == 12
    with pytest.raises(ValidationError):
        StoryAnalysisStageInput.model_validate(
            {**value.model_dump(mode="json"), "unexpected": True}
        )
    with pytest.raises(ValidationError):
        StoryAnalysisStageInput.model_validate(
            {**value.model_dump(mode="json"), "candidate_item_start": 1}
        )


def test_story_reconciliation_input_is_bounded_and_exact() -> None:
    candidate: dict[str, Any] = {
        "entities": [],
        "world_entries": [],
        "claims": [],
        "arcs": [],
        "review_issues": [
            {
                "issue_key": "issue:partition",
                "code": "partition_fixture",
                "severity": "warning",
                "scope": "story",
                "subject_key": None,
                "summary": "partition fixture",
                "repair_hint": None,
                "evidence": [],
            }
        ],
    }
    value = StoryReconciliationStageInput.model_validate(
        {
            "level": 1,
            "candidate_type": "story_analysis_candidate",
            "candidates": [
                {
                    "shard_key": "story-map:0000",
                    "candidate_revision_id": "10000000-0000-0000-0000-000000000001",
                    "candidate_revision_hash": "a" * 64,
                    "candidate_item_start": 0,
                    "candidate_item_end": 1,
                    "candidate": candidate,
                },
                {
                    "shard_key": "story-map:0001",
                    "candidate_revision_id": "10000000-0000-0000-0000-000000000002",
                    "candidate_revision_hash": "b" * 64,
                    "candidate": candidate,
                },
            ],
        }
    )
    assert len(value.candidates) == 2
    incomplete_range = value.model_dump(mode="json")
    incomplete_range["candidates"] = [incomplete_range["candidates"][0]]
    incomplete_range["candidates"][0]["candidate_item_end"] = None
    with pytest.raises(ValidationError):
        StoryReconciliationStageInput.model_validate(incomplete_range)
    too_many = value.model_dump(mode="json")
    too_many["candidates"] = too_many["candidates"] * 2
    with pytest.raises(ValidationError):
        StoryReconciliationStageInput.model_validate(too_many)


def test_storygraph_stage_result_is_strict_and_hashes_candidate() -> None:
    fixture = _fixture()
    invocation = StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])
    result = StoryGraphStageResult.model_validate(fixture["valid_success_result"])
    assert result.compute_result_hash() == fixture["expected_result_hash"]
    result.validate_for(invocation)

    unknown = StoryGraphStageResult.model_validate(fixture["valid_unknown_result"])
    unknown.validate_for(invocation)

    with_extra = cast(dict[str, Any], fixture["valid_invocation"]).copy()
    with_extra["unexpected"] = True
    with pytest.raises(ValidationError):
        StoryGraphStageInvocation.model_validate(with_extra)


def test_storygraph_execution_grant_claims_bind_policy_attempt_and_fencing() -> None:
    fixture = _fixture()
    invocation = StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])
    claims = StoryGraphExecutionGrantClaims.model_validate(fixture["execution_grant_claims"])
    claims.validate_for(invocation, now_unix=1_799_999_999)

    with pytest.raises(ValidationError):
        StoryGraphExecutionGrantClaims.model_validate(
            {
                **cast(dict[str, Any], fixture["execution_grant_claims"]),
                "fencing_token": 0,
            }
        )
