from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.harness.scene_analysis_schemas import (
    SceneAnalysisCandidateRevisionIdentity,
    SceneAnalysisInvocation,
    SceneAnalysisPayload,
    SceneAnalysisStageVariant,
    StructureIdentityReviewInput,
)
from app.modules.storygraph.scene_analysis_bundle import SceneAnalysisBundle
from app.modules.storygraph.scene_analysis_candidates import (
    StructureIdentityReviewCandidate,
)
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
SCENE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-scene-analysis-wire.json"
IDENTITY_FIXTURE = (
    REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-identity-resolution.json"
)
REVIEW_FIXTURE = (
    REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-structure-identity-review.json"
)


def _fixtures() -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
    return (
        json.loads(SCENE_FIXTURE.read_text(encoding="utf-8")),
        json.loads(IDENTITY_FIXTURE.read_text(encoding="utf-8")),
        json.loads(REVIEW_FIXTURE.read_text(encoding="utf-8")),
    )


def _input() -> StructureIdentityReviewInput:
    scene, identity, review = _fixtures()
    return StructureIdentityReviewInput.model_validate(
        {
            **review["input_identity"],
            "normalized_text": scene["valid_invocation"]["payload"]["stage_input"][
                "normalized_text"
            ],
            "span_candidate": scene["valid_script_span_candidate"],
            "scene_fact_candidate": scene["valid_scene_fact_candidate"],
            "identity_candidate": identity["valid_candidate"],
            "deterministic_issues": review["deterministic_issues"],
        }
    )


def test_structure_identity_review_is_profile_bound_and_has_no_gate_decision() -> None:
    _, _, review = _fixtures()
    stage_input = _input()
    candidate = StructureIdentityReviewCandidate.model_validate(review["valid_candidate"])

    candidate.validate_for(stage_input)
    assert scene_analysis_stage_spec("review_candidate", "structure_identity").candidate_model is (
        StructureIdentityReviewCandidate
    )
    assert SceneAnalysisBundle().loaded_paths("review_candidate", "structure_identity") == (
        "SKILL.md",
        "references/structure-identity-review.md",
    )

    with pytest.raises(ValidationError):
        SceneAnalysisStageVariant(
            stage_key="review_candidate",
            profile_key="default",
            lane_key="primary",
            output_schema_version="structure-identity-review-candidate-production",
        )
    with pytest.raises(ValueError, match="unknown Scene Analysis stage variant"):
        scene_analysis_stage_spec("review_candidate", "storyboard")

    forbidden = {**review["valid_candidate"], "gate_status": "passed"}
    with pytest.raises(ValidationError):
        StructureIdentityReviewCandidate.model_validate(forbidden)


def test_structure_identity_review_preserves_deterministic_blockers_and_sorted_issues() -> None:
    _, _, review = _fixtures()
    raw_input = _input().model_dump(mode="json")
    blocker: dict[str, Any] = {
        "issue_key": "issue_identity_partition_0001",
        "code": "identity_partition_incomplete",
        "severity": "blocking",
        "scope": "project:demo",
        "summary": "身份提及分区不完整",
        "evidence": [],
    }
    raw_input["deterministic_issues"] = [blocker]
    stage_input = StructureIdentityReviewInput.model_validate(raw_input)
    raw_candidate = copy.deepcopy(review["valid_candidate"])
    raw_candidate["review_issues"] = [
        blocker,
        *raw_candidate["review_issues"],
    ]
    review_issues = cast(list[dict[str, Any]], raw_candidate["review_issues"])
    review_issues.sort(key=lambda value: str(value["issue_key"]))
    candidate = StructureIdentityReviewCandidate.model_validate(raw_candidate)
    candidate.validate_for(stage_input)

    omitted = candidate.model_copy(deep=True)
    omitted.review_issues = [
        issue for issue in omitted.review_issues if issue.issue_key != blocker["issue_key"]
    ]
    with pytest.raises(ValueError, match="deterministic"):
        omitted.validate_for(stage_input)

    downgraded = candidate.model_copy(deep=True)
    blocker_issue = next(
        issue for issue in downgraded.review_issues if issue.issue_key == blocker["issue_key"]
    )
    blocker_issue.severity = "warning"
    with pytest.raises(ValueError, match="deterministic"):
        downgraded.validate_for(stage_input)

    unsorted = copy.deepcopy(raw_candidate)
    unsorted["review_issues"].reverse()
    with pytest.raises(ValidationError, match="sorted"):
        StructureIdentityReviewCandidate.model_validate(unsorted)


@pytest.mark.asyncio
async def test_structure_identity_review_runs_from_three_frozen_candidates(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    scene, _, review = _fixtures()
    base = SceneAnalysisInvocation.model_validate(scene["valid_invocation"])
    stage_input = _input()
    candidate = StructureIdentityReviewCandidate.model_validate(review["valid_candidate"])
    refs = [
        SceneAnalysisCandidateRevisionIdentity.model_validate(value)
        for value in review["upstream_candidates"]
    ]
    invocation = SceneAnalysisInvocation.build(
        invocation_id=base.invocation_id,
        attempt_id=base.attempt_id,
        stage_release=base.stage_release,
        control=base.control,
        budget=base.budget,
        payload=SceneAnalysisPayload(
            variant=SceneAnalysisStageVariant(
                stage_key="review_candidate",
                profile_key="structure_identity",
                lane_key="primary",
                output_schema_version="structure-identity-review-candidate-production",
            ),
            scope=base.payload.scope,
            source_refs=base.payload.source_refs,
            upstream_candidates=refs,
            shard=base.payload.shard,
            stage_input=stage_input.model_dump(mode="json"),
        ),
    )

    async def return_candidate(*_: object) -> StructureIdentityReviewCandidate:
        return candidate

    monkeypatch.setattr(SceneAnalysisHarness, "_run_codex", return_candidate)
    result = await SceneAnalysisHarness(invocation, repository_root=REPOSITORY_ROOT).execute()
    assert result == candidate

    drifted = invocation.model_dump(mode="json")
    drifted["payload"]["upstream_candidates"][0]["candidate_revision_hash"] = "f" * 64
    with pytest.raises(ValidationError):
        SceneAnalysisPayload.model_validate(drifted["payload"])
