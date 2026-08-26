from __future__ import annotations

import json
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.candidate_runtime.schemas import (
    StoryGraphExecutionGrantClaims,
    StoryGraphStageInvocation,
    StoryGraphStageResult,
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
