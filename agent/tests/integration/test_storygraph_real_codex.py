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
    SourceEvidenceCandidate,
    StoryAnalysisCandidate,
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
