from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
from typing import Any, cast
from uuid import UUID

import pytest
from pydantic import BaseModel

from app.candidate_runtime.canonical import canonical_hash
from app.candidate_runtime.schemas import (
    SourceEvidenceStageInput,
    StoryGraphStageInvocation,
    StoryGraphStagePayload,
)
from app.modules.storygraph.candidate_schemas import (
    CandidateRepairPatch,
    SourceEvidenceCandidate,
    StoryAnalysisCandidate,
    StoryGraphReviewCandidate,
    StoryReconciliationCandidate,
)
from app.modules.storygraph.harness import StoryGraphHarness

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-stage-wire-v1.json"


def stage_invocation(
    source: StoryGraphStageInvocation,
    *,
    invocation_id: str,
    payload: dict[str, Any],
) -> StoryGraphStageInvocation:
    draft = StoryGraphStageInvocation.model_construct(
        invocation_id=UUID(invocation_id),
        kind="storygraph_stage",
        wire_schema_version="storygraph-stage-wire-v1",
        input_hash="0" * 64,
        execution_policy=source.execution_policy,
        payload=StoryGraphStagePayload.model_validate(payload),
    )
    return StoryGraphStageInvocation.model_validate(
        draft.model_copy(update={"input_hash": draft.compute_input_hash()}).model_dump(mode="json")
    )


def evidence_identities(candidate: BaseModel) -> set[tuple[int, int, str, str, int | None]]:
    identities: set[tuple[int, int, str, str, int | None]] = set()

    def visit(value: Any) -> None:
        if isinstance(value, dict):
            mapping = cast(dict[str, Any], value)
            if {
                "source_start",
                "source_end",
                "text_hash",
                "exact_anchor",
                "episode_number",
            }.issubset(mapping):
                identities.add(
                    (
                        int(mapping["source_start"]),
                        int(mapping["source_end"]),
                        str(mapping["text_hash"]),
                        str(mapping["exact_anchor"]),
                        cast(int | None, mapping["episode_number"]),
                    )
                )
                return
            for item in mapping.values():
                visit(item)
        elif isinstance(value, list):
            for item in cast(list[Any], value):
                visit(item)

    visit(candidate.model_dump(mode="json"))
    return identities


@pytest.mark.skipif(
    os.getenv("LANVERSE_TEST_REAL_CODEX") != "1",
    reason="set LANVERSE_TEST_REAL_CODEX=1 to exercise the locally authenticated Codex CLI",
)
@pytest.mark.asyncio
async def test_real_codex_preserves_evidence_through_story_analysis_and_reconciliation() -> None:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    invocation = StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])
    source_input = SourceEvidenceStageInput.model_validate(invocation.payload.stage_input)

    harness = StoryGraphHarness(invocation, repository_root=REPOSITORY_ROOT)
    result = await harness.execute()

    assert isinstance(result, SourceEvidenceCandidate)
    assert result.observations
    for observation in result.observations:
        for evidence in observation.evidence:
            local_start = evidence.source_start - source_input.context_start
            local_end = evidence.source_end - source_input.context_start
            assert source_input.normalized_text[local_start:local_end] == evidence.exact_anchor
            assert (
                hashlib.sha256(evidence.exact_anchor.encode("utf-8")).hexdigest()
                == evidence.text_hash
            )

    source_candidate = result.model_dump(mode="json")
    source_candidate_hash = canonical_hash(source_candidate)
    analysis_invocation = stage_invocation(
        invocation,
        invocation_id="20000000-0000-0000-0000-000000000002",
        payload={
            "stage": "analyze_story",
            "shard_key": "story-map:0000",
            "workspace_id": invocation.payload.workspace_id,
            "project_id": invocation.payload.project_id,
            "source_refs": [
                value.model_dump(mode="json") for value in invocation.payload.source_refs
            ],
            "upstream_candidates": [
                {
                    "stage": "extract_source_evidence",
                    "shard_key": invocation.payload.shard_key,
                    "candidate_revision_id": "70000000-0000-0000-0000-000000000001",
                    "candidate_revision_hash": source_candidate_hash,
                    "source_invocation_id": invocation.invocation_id,
                    "source_result_hash": source_candidate_hash,
                }
            ],
            "shard_manifest_ref": {
                "manifest_id": "60000000-0000-0000-0000-000000000002",
                "version": 1,
                "hash": "d" * 64,
            },
            "shard": {
                "kind": "story_map",
                "key": "story-map:0000",
                "tree_path": "map.0000",
                "absolute_start": source_input.logical_start,
                "absolute_end": source_input.logical_end,
            },
            "stage_input": {
                "evidence_shard_key": invocation.payload.shard_key,
                "evidence_candidate_revision_id": "70000000-0000-0000-0000-000000000001",
                "evidence_candidate_revision_hash": source_candidate_hash,
                "logical_start": source_input.logical_start,
                "logical_end": source_input.logical_end,
                "candidate_item_start": 0,
                "candidate_item_end": len(source_candidate["observations"])
                + len(source_candidate["review_issues"]),
                "evidence_candidate": source_candidate,
            },
        },
    )
    analysis = await StoryGraphHarness(
        analysis_invocation, repository_root=REPOSITORY_ROOT
    ).execute()

    assert isinstance(analysis, StoryAnalysisCandidate)
    assert analysis.entities
    source_evidence = evidence_identities(result)
    analysis_evidence = evidence_identities(analysis)
    assert analysis_evidence
    assert analysis_evidence <= source_evidence

    analysis_candidate = analysis.model_dump(mode="json")
    analysis_candidate_hash = canonical_hash(analysis_candidate)
    reconciliation_invocation = stage_invocation(
        invocation,
        invocation_id="20000000-0000-0000-0000-000000000003",
        payload={
            "stage": "reconcile_story",
            "shard_key": "story-reduce:0000:0000",
            "workspace_id": invocation.payload.workspace_id,
            "project_id": invocation.payload.project_id,
            "source_refs": [
                value.model_dump(mode="json") for value in invocation.payload.source_refs
            ],
            "upstream_candidates": [
                {
                    "stage": "analyze_story",
                    "shard_key": analysis_invocation.payload.shard_key,
                    "candidate_revision_id": "70000000-0000-0000-0000-000000000002",
                    "candidate_revision_hash": analysis_candidate_hash,
                    "source_invocation_id": analysis_invocation.invocation_id,
                    "source_result_hash": analysis_candidate_hash,
                }
            ],
            "shard_manifest_ref": {
                "manifest_id": "60000000-0000-0000-0000-000000000003",
                "version": 1,
                "hash": "e" * 64,
            },
            "shard": {
                "kind": "story_reduce",
                "key": "story-reduce:0000:0000",
                "tree_path": "reduce.0000.0000",
            },
            "stage_input": {
                "level": 0,
                "candidate_type": "story_analysis_candidate",
                "candidates": [
                    {
                        "shard_key": analysis_invocation.payload.shard_key,
                        "candidate_revision_id": "70000000-0000-0000-0000-000000000002",
                        "candidate_revision_hash": analysis_candidate_hash,
                        "candidate": analysis_candidate,
                    }
                ],
            },
        },
    )
    reconciled = await StoryGraphHarness(
        reconciliation_invocation, repository_root=REPOSITORY_ROOT
    ).execute()

    assert isinstance(reconciled, StoryReconciliationCandidate)
    assert reconciled.canonical_entities
    reconciled_evidence = evidence_identities(reconciled)
    assert reconciled_evidence
    assert reconciled_evidence <= analysis_evidence

    reconciled_candidate = reconciled.model_dump(mode="json")
    reconciled_candidate_hash = canonical_hash(reconciled_candidate)
    reconciled_revision_id = "70000000-0000-0000-0000-000000000003"
    reconciled_item_count = (
        len(reconciled.canonical_entities)
        + len(reconciled.canonical_world_entries)
        + len(reconciled.merged_claims)
        + len(reconciled.merged_arcs)
        + len(reconciled.conflicts)
        + len(reconciled.review_issues)
    )
    review_invocation = stage_invocation(
        invocation,
        invocation_id="20000000-0000-0000-0000-000000000004",
        payload={
            "stage": "review_storygraph",
            "shard_key": "review:bible:0000",
            "workspace_id": invocation.payload.workspace_id,
            "project_id": invocation.payload.project_id,
            "source_refs": [
                value.model_dump(mode="json") for value in invocation.payload.source_refs
            ],
            "upstream_candidates": [
                {
                    "stage": "reconcile_story",
                    "shard_key": reconciliation_invocation.payload.shard_key,
                    "candidate_revision_id": reconciled_revision_id,
                    "candidate_revision_hash": reconciled_candidate_hash,
                    "source_invocation_id": reconciliation_invocation.invocation_id,
                    "source_result_hash": reconciled_candidate_hash,
                }
            ],
            "shard_manifest_ref": {
                "manifest_id": "60000000-0000-0000-0000-000000000004",
                "version": 1,
                "hash": "f" * 64,
            },
            "shard": {
                "kind": "story_review",
                "key": "review:bible:0000",
                "tree_path": "review.bible.0000",
            },
            "stage_input": {
                "reviewed_stage": "reconcile_story",
                "target_candidate_revision_id": reconciled_revision_id,
                "target_candidate_revision_hash": reconciled_candidate_hash,
                "candidate_item_start": 0,
                "candidate_item_end": reconciled_item_count,
                "target_candidate": reconciled_candidate,
                "deterministic_gate": {
                    "gate_version": "bible-deterministic-gate-v1",
                    "target_candidate_revision_id": reconciled_revision_id,
                    "target_candidate_revision_hash": reconciled_candidate_hash,
                    "blockers": [],
                },
            },
        },
    )
    review = await StoryGraphHarness(review_invocation, repository_root=REPOSITORY_ROOT).execute()
    assert isinstance(review, StoryGraphReviewCandidate)
    assert review.target_candidate_revision_id == UUID(reconciled_revision_id)
    assert evidence_identities(review) <= reconciled_evidence

    repair_evidence = reconciled.canonical_entities[0].evidence[0].model_dump(mode="json")
    target_issue = {
        "issue_key": "issue:canonical-name",
        "code": "canonical_name_ambiguous",
        "severity": "blocking",
        "scope": "entity",
        "subject_key": reconciled.canonical_entities[0].entity_key,
        "summary": "Use the evidence-backed canonical name already present in the candidate.",
        "repair_hint": "Keep the canonical name aligned with the supplied Evidence.",
        "evidence": [repair_evidence],
    }
    review_candidate = {
        "reviewed_stage": "reconcile_story",
        "target_candidate_revision_id": reconciled_revision_id,
        "target_candidate_revision_hash": reconciled_candidate_hash,
        "review_issues": [target_issue],
    }
    review_candidate_hash = canonical_hash(review_candidate)
    fragment = reconciled.canonical_entities[0].model_dump(mode="json")
    fragment_hash = canonical_hash(fragment)
    repair_invocation = stage_invocation(
        invocation,
        invocation_id="20000000-0000-0000-0000-000000000005",
        payload={
            "stage": "repair_candidate",
            "shard_key": "repair:bible:0000",
            "workspace_id": invocation.payload.workspace_id,
            "project_id": invocation.payload.project_id,
            "source_refs": [
                value.model_dump(mode="json") for value in invocation.payload.source_refs
            ],
            "upstream_candidates": [
                {
                    "stage": "reconcile_story",
                    "shard_key": reconciliation_invocation.payload.shard_key,
                    "candidate_revision_id": reconciled_revision_id,
                    "candidate_revision_hash": reconciled_candidate_hash,
                    "source_invocation_id": reconciliation_invocation.invocation_id,
                    "source_result_hash": reconciled_candidate_hash,
                },
                {
                    "stage": "review_storygraph",
                    "shard_key": review_invocation.payload.shard_key,
                    "candidate_revision_id": "70000000-0000-0000-0000-000000000004",
                    "candidate_revision_hash": review_candidate_hash,
                    "source_invocation_id": review_invocation.invocation_id,
                    "source_result_hash": review_candidate_hash,
                },
            ],
            "shard_manifest_ref": {
                "manifest_id": "60000000-0000-0000-0000-000000000005",
                "version": 1,
                "hash": "9" * 64,
            },
            "shard": {
                "kind": "candidate_repair",
                "key": "repair:bible:0000",
                "tree_path": "repair.bible.0000",
            },
            "stage_input": {
                "target_candidate_revision_id": reconciled_revision_id,
                "target_candidate_revision_hash": reconciled_candidate_hash,
                "review_candidate_revision_id": "70000000-0000-0000-0000-000000000004",
                "review_candidate_revision_hash": review_candidate_hash,
                "target_issue": target_issue,
                "allowed_targets": [
                    {
                        "candidate_key": reconciled.canonical_entities[0].entity_key,
                        "allowed_fields": ["canonical_name"],
                        "base_fragment_hash": fragment_hash,
                        "fragment": fragment,
                    }
                ],
                "read_only_adjacency": [],
                "repair_round": 1,
                "max_repair_rounds": 2,
            },
        },
    )
    repair = await StoryGraphHarness(repair_invocation, repository_root=REPOSITORY_ROOT).execute()
    assert isinstance(repair, CandidateRepairPatch)
    assert repair.target_candidate_revision_id == UUID(reconciled_revision_id)
    assert all(
        operation.target_candidate_key == reconciled.canonical_entities[0].entity_key
        and operation.field_name == "canonical_name"
        and operation.base_fragment_hash == fragment_hash
        for operation in repair.operations
    )
