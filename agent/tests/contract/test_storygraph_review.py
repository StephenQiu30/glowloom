from __future__ import annotations

import copy
from typing import Any

import pytest
from pydantic import ValidationError

from app.candidate_runtime.canonical import canonical_hash
from app.candidate_runtime.schemas import (
    StoryGraphDeterministicGateResult,
    StoryGraphRepairStageInput,
    StoryGraphReviewStageInput,
    StoryGraphStagePayload,
)
from app.modules.storygraph.candidate_schemas import (
    CandidateRepairPatch,
    StoryGraphReviewCandidate,
)


def _evidence() -> dict[str, object]:
    return {
        "source_start": 0,
        "source_end": 2,
        "text_hash": "a" * 64,
        "exact_anchor": "林一",
        "episode_number": 1,
    }


def _asset_spec() -> dict[str, object]:
    return {
        "kind": "character",
        "identity": None,
        "appearance": None,
        "age_impression": None,
        "temperament": [],
        "goals": [],
        "relationships": [],
        "arc_summary": None,
        "voice_profile": None,
        "spatial_description": None,
        "time_weather": None,
        "visual_elements": [],
        "lighting": None,
        "material": None,
        "usage_context": None,
        "visual_language": None,
        "palette": None,
        "lighting_language": None,
        "negative_constraints": [],
        "source_kind": None,
        "language": None,
        "performance_traits": [],
        "allowed_usage": [],
    }


def _candidate() -> dict[str, object]:
    return {
        "canonical_entities": [
            {
                "entity_key": "character:lin-yi",
                "kind": "character",
                "canonical_name": "林一",
                "normalized_name": "林一",
                "aliases": [],
                "stable_spec": _asset_spec(),
                "episode_numbers": [1],
                "evidence": [_evidence()],
                "states": [],
                "ambiguities": [],
            }
        ],
        "canonical_world_entries": [],
        "merged_claims": [],
        "merged_arcs": [],
        "conflicts": [],
        "review_issues": [],
    }


def _review_input() -> StoryGraphReviewStageInput:
    target_id = "10000000-0000-0000-0000-000000000001"
    target_hash = "b" * 64
    return StoryGraphReviewStageInput.model_validate(
        {
            "reviewed_stage": "reconcile_story",
            "target_candidate_revision_id": target_id,
            "target_candidate_revision_hash": target_hash,
            "candidate_item_start": 0,
            "candidate_item_end": 1,
            "target_candidate": _candidate(),
            "deterministic_gate": {
                "gate_version": "bible-deterministic-gate",
                "target_candidate_revision_id": target_id,
                "target_candidate_revision_hash": target_hash,
                "blockers": [],
            },
        }
    )


def test_review_input_freezes_gate_and_model_output_cannot_rewrite_it() -> None:
    stage_input = _review_input()
    assert isinstance(stage_input.deterministic_gate, StoryGraphDeterministicGateResult)

    result = StoryGraphReviewCandidate.model_validate(
        {
            "reviewed_stage": "reconcile_story",
            "target_candidate_revision_id": str(stage_input.target_candidate_revision_id),
            "target_candidate_revision_hash": stage_input.target_candidate_revision_hash,
            "review_issues": [
                {
                    "issue_key": "issue:identity",
                    "code": "identity_ambiguous",
                    "severity": "warning",
                    "scope": "entity",
                    "subject_key": "character:lin-yi",
                    "summary": "身份仍需确认",
                    "repair_hint": None,
                    "evidence": [_evidence()],
                }
            ],
        }
    )
    result.validate_for(stage_input)

    with pytest.raises(ValidationError):
        StoryGraphReviewCandidate.model_validate(
            {**result.model_dump(mode="json"), "deterministic_blockers": []}
        )

    impersonating = result.model_copy(deep=True)
    impersonating.review_issues[0].code = "world_unknown_entity"
    with pytest.raises(ValueError, match="deterministic Gate code"):
        impersonating.validate_for(stage_input)

    forged = result.model_copy(deep=True)
    forged.review_issues[0].evidence[0].text_hash = "f" * 64
    with pytest.raises(ValueError, match="Evidence"):
        forged.validate_for(stage_input)


def test_repair_patch_is_confined_to_the_frozen_fragment_allowlist() -> None:
    fragment = {
        "entity_key": "character:lin-yi",
        "canonical_name": "林一",
    }
    fragment_hash = canonical_hash(fragment)
    assert fragment_hash == "d4d2e657ebe16dd6ecab5d3aa2c8d5e536ffc385fba3ee9e0627e3ee24d8c17b"
    stage_input = StoryGraphRepairStageInput.model_validate(
        {
            "target_candidate_revision_id": "10000000-0000-0000-0000-000000000001",
            "target_candidate_revision_hash": "b" * 64,
            "review_candidate_revision_id": "10000000-0000-0000-0000-000000000002",
            "review_candidate_revision_hash": "c" * 64,
            "target_issue": {
                "issue_key": "issue:canonical-name",
                "code": "canonical_name",
                "severity": "blocking",
                "scope": "entity",
                "subject_key": "character:lin-yi",
                "summary": "规范名冲突",
                "repair_hint": "选择有证据的名称",
                "evidence": [_evidence()],
            },
            "allowed_targets": [
                {
                    "candidate_key": "character:lin-yi",
                    "allowed_fields": ["canonical_name"],
                    "base_fragment_hash": fragment_hash,
                    "fragment": fragment,
                }
            ],
            "read_only_adjacency": [],
            "repair_round": 1,
            "max_repair_rounds": 2,
        }
    )
    patch = CandidateRepairPatch.model_validate(
        {
            "target_candidate_revision_id": str(stage_input.target_candidate_revision_id),
            "target_candidate_revision_hash": stage_input.target_candidate_revision_hash,
            "operations": [
                {
                    "target_candidate_key": "character:lin-yi",
                    "base_fragment_hash": fragment_hash,
                    "field_name": "canonical_name",
                    "replacement": {
                        "text": "林一",
                        "integer": None,
                        "flag": None,
                        "strings": None,
                    },
                }
            ],
            "review_issues": [],
        }
    )
    patch.validate_for(stage_input)

    unsafe_boundary = stage_input.model_dump(mode="json")
    unsafe_boundary["allowed_targets"][0]["allowed_fields"] = ["graph_json"]
    with pytest.raises(ValidationError):
        StoryGraphRepairStageInput.model_validate(unsafe_boundary)

    impersonating_boundary = stage_input.model_dump(mode="json")
    impersonating_boundary["target_issue"]["code"] = "world_unknown_entity"
    with pytest.raises(ValidationError):
        StoryGraphRepairStageInput.model_validate(impersonating_boundary)

    escaped = copy.deepcopy(patch.model_dump(mode="json"))
    escaped["operations"][0]["field_name"] = "stable_spec"
    escaped_patch = CandidateRepairPatch.model_validate(escaped)
    with pytest.raises(ValueError, match="allowlist"):
        escaped_patch.validate_for(stage_input)

    drifted = copy.deepcopy(patch.model_dump(mode="json"))
    drifted["operations"][0]["base_fragment_hash"] = "d" * 64
    drifted_patch = CandidateRepairPatch.model_validate(drifted)
    with pytest.raises(ValueError, match="fragment"):
        drifted_patch.validate_for(stage_input)

    wrong_type = copy.deepcopy(patch.model_dump(mode="json"))
    wrong_type["operations"][0]["replacement"] = {
        "text": None,
        "integer": 1,
        "flag": None,
        "strings": None,
    }
    wrong_type_patch = CandidateRepairPatch.model_validate(wrong_type)
    with pytest.raises(ValueError, match="replacement type"):
        wrong_type_patch.validate_for(stage_input)


def test_review_and_repair_payloads_bind_their_exact_upstream_revisions() -> None:
    review_input = _review_input()
    review_payload = _payload_base(
        stage="review_storygraph",
        shard_key="review:0000",
        shard_kind="story_review",
        stage_input=review_input.model_dump(mode="json"),
        upstream_candidates=[
            {
                "stage": "reconcile_story",
                "shard_key": "story-reduce:root",
                "candidate_revision_id": str(review_input.target_candidate_revision_id),
                "candidate_revision_hash": review_input.target_candidate_revision_hash,
                "source_invocation_id": "20000000-0000-0000-0000-000000000001",
                "source_result_hash": "c" * 64,
            }
        ],
    )
    StoryGraphStagePayload.model_validate(review_payload)
    drifted = copy.deepcopy(review_payload)
    drifted["upstream_candidates"][0]["candidate_revision_hash"] = "d" * 64
    with pytest.raises(ValidationError):
        StoryGraphStagePayload.model_validate(drifted)

    fragment = {"entity_key": "character:lin-yi", "canonical_name": "林一"}
    fragment_hash = canonical_hash(fragment)
    repair_input = StoryGraphRepairStageInput.model_validate(
        {
            "target_candidate_revision_id": "10000000-0000-0000-0000-000000000001",
            "target_candidate_revision_hash": "b" * 64,
            "review_candidate_revision_id": "10000000-0000-0000-0000-000000000002",
            "review_candidate_revision_hash": "c" * 64,
            "target_issue": {
                "issue_key": "issue:canonical-name",
                "code": "canonical_name",
                "severity": "blocking",
                "scope": "entity",
                "subject_key": "character:lin-yi",
                "summary": "规范名冲突",
                "repair_hint": None,
                "evidence": [_evidence()],
            },
            "allowed_targets": [
                {
                    "candidate_key": "character:lin-yi",
                    "allowed_fields": ["canonical_name"],
                    "base_fragment_hash": fragment_hash,
                    "fragment": fragment,
                }
            ],
            "read_only_adjacency": [],
            "repair_round": 1,
            "max_repair_rounds": 2,
        }
    )
    repair_payload = _payload_base(
        stage="repair_candidate",
        shard_key="repair:0000",
        shard_kind="candidate_repair",
        stage_input=repair_input.model_dump(mode="json"),
        upstream_candidates=[
            {
                "stage": "reconcile_story",
                "shard_key": "story-reduce:root",
                "candidate_revision_id": str(repair_input.target_candidate_revision_id),
                "candidate_revision_hash": repair_input.target_candidate_revision_hash,
                "source_invocation_id": "20000000-0000-0000-0000-000000000002",
                "source_result_hash": "d" * 64,
            },
            {
                "stage": "review_storygraph",
                "shard_key": "review:0000",
                "candidate_revision_id": str(repair_input.review_candidate_revision_id),
                "candidate_revision_hash": repair_input.review_candidate_revision_hash,
                "source_invocation_id": "20000000-0000-0000-0000-000000000003",
                "source_result_hash": "e" * 64,
            },
        ],
    )
    StoryGraphStagePayload.model_validate(repair_payload)
    published_target = copy.deepcopy(repair_payload)
    published_target["base_storygraph_version_id"] = "30000000-0000-0000-0000-000000000005"
    published_target["base_storygraph_hash"] = "2" * 64
    with pytest.raises(ValidationError):
        StoryGraphStagePayload.model_validate(published_target)
    missing_review = copy.deepcopy(repair_payload)
    missing_review["upstream_candidates"] = missing_review["upstream_candidates"][:1]
    with pytest.raises(ValidationError):
        StoryGraphStagePayload.model_validate(missing_review)


def _payload_base(
    *,
    stage: str,
    shard_key: str,
    shard_kind: str,
    stage_input: dict[str, Any],
    upstream_candidates: list[dict[str, Any]],
) -> dict[str, Any]:
    return {
        "stage": stage,
        "shard_key": shard_key,
        "workspace_id": "30000000-0000-0000-0000-000000000001",
        "project_id": "30000000-0000-0000-0000-000000000002",
        "source_refs": [
            {
                "owner_kind": "production/script",
                "owner_logical_id": "script:1",
                "owner_version_id": "30000000-0000-0000-0000-000000000003",
                "revision": 1,
                "content_hash": "f" * 64,
            }
        ],
        "upstream_candidates": upstream_candidates,
        "shard_manifest_ref": {
            "manifest_id": "30000000-0000-0000-0000-000000000004",
            "version": 1,
            "hash": "1" * 64,
        },
        "shard": {"kind": shard_kind, "key": shard_key, "tree_path": shard_key},
        "stage_input": stage_input,
    }
