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
    EpisodeAnalysisStageInput,
    EpisodeReconciliationStageInput,
    EpisodeSegmentationStageInput,
    SourceEvidenceStageInput,
    StoryboardDraftStageInput,
    StoryGraphStageInvocation,
    StoryGraphStagePayload,
)
from app.modules.storygraph.candidate_schemas import (
    CandidateRepairPatch,
    EpisodeAnalysisCandidate,
    EpisodeReconciliationCandidate,
    EpisodeSegmentationCandidate,
    SourceEvidenceCandidate,
    StoryAnalysisCandidate,
    StoryboardRowCandidate,
    StoryGraphReviewCandidate,
    StoryReconciliationCandidate,
)
from app.modules.storygraph.harness import StoryGraphHarness

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-stage-wire.json"


def stage_invocation(
    source: StoryGraphStageInvocation,
    *,
    invocation_id: str,
    payload: dict[str, Any],
) -> StoryGraphStageInvocation:
    draft = StoryGraphStageInvocation.model_construct(
        invocation_id=UUID(invocation_id),
        kind="storygraph_stage",
        wire_schema_version="storygraph-stage-wire",
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
async def test_real_codex_drafts_reviewable_intent_without_creating_shot() -> None:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    source = StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])
    graph_id = "72000000-0000-0000-0000-000000000001"
    style_id = "72000000-0000-0000-0000-000000000002"
    episode_id = "72000000-0000-0000-0000-000000000003"
    scene_owner_id = "72000000-0000-0000-0000-000000000004"
    document_revision_id = "72000000-0000-0000-0000-000000000005"
    asset_id = "72000000-0000-0000-0000-000000000006"
    specification_id = "72000000-0000-0000-0000-000000000007"
    state_id = "72000000-0000-0000-0000-000000000008"
    scene_key = "sgn_" + "1" * 64
    beat_key = "sgn_" + "2" * 64
    occurrence_key = "sgn_" + "3" * 64
    identity_key = "sgn_" + "4" * 64
    specification_key = "sgn_" + "5" * 64
    state_key = "sgn_" + "6" * 64
    graph_hash = "a" * 64
    style_hash = "b" * 64
    evidence_hash = "c" * 64
    evidence = [
        {
            "document_revision_id": document_revision_id,
            "absolute_start": 10,
            "absolute_end": 24,
            "text_hash": evidence_hash,
        }
    ]
    stage_input = StoryboardDraftStageInput.model_validate(
        {
            "graph_version_no": 1,
            "scene": {
                "story_node_key": scene_key,
                "owner_version_id": scene_owner_id,
                "owner_revision": 1,
                "owner_hash": "d" * 64,
                "episode_id": episode_id,
                "episode_position": 1,
                "scene_position": 1,
                "heading": "内景 客厅 日",
                "evidence": evidence,
            },
            "beats": [
                {
                    "story_node_key": beat_key,
                    "summary": "阿澜推门进入客厅并停在窗边",
                    "required_for_coverage": True,
                    "evidence": evidence,
                }
            ],
            "dialogues": [],
            "occurrences": [
                {
                    "story_node_key": occurrence_key,
                    "identity_story_node_key": identity_key,
                    "specification_story_node_key": specification_key,
                    "asset_state_story_node_key": state_key,
                    "asset_id": asset_id,
                    "specification_version_id": specification_id,
                    "asset_state_id": state_id,
                    "asset_kind": "character",
                    "summary": "阿澜以日常服装进入客厅",
                    "evidence": evidence,
                }
            ],
            "effective_style_snapshot": {
                "owner_version_id": style_id,
                "revision": 1,
                "content_hash": style_hash,
                "visual_style": "cinematic noir",
                "aspect_ratio": "9:16",
            },
            "target_duration_ms": 90_000,
            "asset_versions": [],
        }
    )
    invocation = stage_invocation(
        source,
        invocation_id="22000000-0000-0000-0000-000000000001",
        payload={
            "stage": "draft_storyboard",
            "shard_key": f"scene:{scene_key}",
            "workspace_id": source.payload.workspace_id,
            "project_id": source.payload.project_id,
            "source_refs": [
                {
                    "owner_kind": "production/storygraph",
                    "owner_logical_id": str(source.payload.project_id),
                    "owner_version_id": graph_id,
                    "revision": 1,
                    "content_hash": graph_hash,
                },
                {
                    "owner_kind": "preset/effective-style",
                    "owner_logical_id": str(source.payload.project_id),
                    "owner_version_id": style_id,
                    "revision": 1,
                    "content_hash": style_hash,
                },
            ],
            "base_storygraph_version_id": graph_id,
            "base_storygraph_hash": graph_hash,
            "upstream_candidates": [],
            "shard_manifest_ref": {
                "manifest_id": "62000000-0000-0000-0000-000000000001",
                "version": 1,
                "hash": "e" * 64,
            },
            "shard": {
                "kind": "story_scene",
                "key": f"scene:{scene_key}",
                "tree_path": "scene/0001",
            },
            "stage_input": stage_input.model_dump(mode="json"),
        },
    )

    candidate = await StoryGraphHarness(invocation, repository_root=REPOSITORY_ROOT).execute()

    assert isinstance(candidate, StoryboardRowCandidate)
    assert candidate.scene_story_node_key == scene_key
    assert candidate.asset_readiness == "needs_asset"
    assert candidate.shot_intents
    assert all(
        requirement.asset_readiness == "needs_asset" and requirement.asset_version_ref is None
        for intent in candidate.shot_intents
        for requirement in intent.visual_requirements
    )
    assert "shots" not in candidate.model_dump(mode="json")


@pytest.mark.skipif(
    os.getenv("LANVERSE_TEST_REAL_CODEX") != "1",
    reason="set LANVERSE_TEST_REAL_CODEX=1 to exercise the locally authenticated Codex CLI",
)
@pytest.mark.asyncio
async def test_real_codex_segments_full_source_without_overriding_markers() -> None:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    source = StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])
    text = "第一集\n甲出发。\n第二集\n乙抵达。"
    second_start = text.index("第二集")
    markers: list[dict[str, Any]] = []
    for episode_number, start, label in (
        (1, 0, "第一集"),
        (2, second_start, "第二集"),
    ):
        markers.append(
            {
                "episode_number": episode_number,
                "label": label,
                "evidence": {
                    "source_start": start,
                    "source_end": start + len(label),
                    "text_hash": hashlib.sha256(label.encode("utf-8")).hexdigest(),
                    "exact_anchor": label,
                    "episode_number": episode_number,
                },
            }
        )
    leaf: dict[str, Any] = {
        "shard_key": "source:00000000:00000018",
        "candidate_revision_id": "70000000-0000-0000-0000-000000000010",
        "candidate_revision_hash": "a" * 64,
    }
    stage_input = EpisodeSegmentationStageInput.model_validate(
        {
            "document_revision_id": source.payload.source_refs[0].owner_version_id,
            "normalized_hash": source.payload.source_refs[0].content_hash,
            "source_code_points": len(text),
            "target_duration_ms": 90_000,
            "bible_version_id": "70000000-0000-0000-0000-000000000020",
            "bible_version": 1,
            "bible_content_hash": "b" * 64,
            "materialization_hash": "c" * 64,
            "evidence_aggregate_revision_id": "70000000-0000-0000-0000-000000000030",
            "evidence_aggregate_revision_hash": "d" * 64,
            "evidence_leaves": [leaf],
            "marker_hints": markers,
            "evidence_index": [
                {
                    "index_key": f"marker:{index:04d}",
                    "kind": "marker",
                    "label": marker["label"],
                    **leaf,
                    "evidence": marker["evidence"],
                }
                for index, marker in enumerate(markers)
            ],
        }
    )
    invocation = stage_invocation(
        source,
        invocation_id="20000000-0000-0000-0000-000000000010",
        payload={
            "stage": "segment_episodes",
            "shard_key": "episode-segmentation:global",
            "workspace_id": source.payload.workspace_id,
            "project_id": source.payload.project_id,
            "source_refs": [
                source.payload.source_refs[0].model_dump(mode="json"),
                {
                    "owner_kind": "production/bible-materialization",
                    "owner_logical_id": str(stage_input.bible_version_id),
                    "owner_version_id": str(stage_input.bible_version_id),
                    "revision": stage_input.bible_version,
                    "content_hash": stage_input.materialization_hash,
                },
            ],
            "upstream_candidates": [
                {
                    "stage": "extract_source_evidence",
                    **leaf,
                    "source_invocation_id": source.invocation_id,
                    "source_result_hash": "e" * 64,
                }
            ],
            "shard_manifest_ref": {
                "manifest_id": "60000000-0000-0000-0000-000000000010",
                "version": 1,
                "hash": "f" * 64,
            },
            "shard": {
                "kind": "episode_segmentation",
                "key": "episode-segmentation:global",
                "tree_path": "global",
                "absolute_start": 0,
                "absolute_end": len(text),
            },
            "stage_input": stage_input.model_dump(mode="json"),
        },
    )
    candidate = await StoryGraphHarness(invocation, repository_root=REPOSITORY_ROOT).execute()
    assert isinstance(candidate, EpisodeSegmentationCandidate)
    assert candidate.boundaries[0].absolute_start == 0
    assert candidate.boundaries[-1].absolute_end == len(text)
    assert [value.absolute_start for value in candidate.boundaries] == [0, second_start]
    assert all(
        marker.evidence in candidate.boundaries[index].evidence
        for index, marker in enumerate(stage_input.marker_hints)
    )


@pytest.mark.skipif(
    os.getenv("LANVERSE_TEST_REAL_CODEX") != "1",
    reason="set LANVERSE_TEST_REAL_CODEX=1 to exercise the locally authenticated Codex CLI",
)
@pytest.mark.asyncio
async def test_real_codex_preserves_episode_structure_children_and_evidence() -> None:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    source = StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])
    episode_text = "内景 客厅 日\n阿澜推门进入。\n阿澜：雨停了。"
    source_start = 10
    source_end = source_start + len(episode_text)
    scene_label = "内景 客厅 日"
    episode_id = "71000000-0000-0000-0000-000000000001"
    script_version_id = "71000000-0000-0000-0000-000000000002"
    bible_version_id = "71000000-0000-0000-0000-000000000003"
    bible_snapshot = {
        "entities": [
            {
                "entity_key": "character:alan",
                "kind": "character",
                "canonical_name": "阿澜",
            }
        ]
    }
    known_identities = [
        {
            "entity_key": "character:alan",
            "kind": "character",
            "asset_id": "71000000-0000-0000-0000-000000000004",
            "specification_version_id": "71000000-0000-0000-0000-000000000005",
            "specification_hash": "a" * 64,
            "states": [
                {
                    "state_key": "character:alan:default",
                    "asset_state_id": "71000000-0000-0000-0000-000000000006",
                    "content_hash": "b" * 64,
                }
            ],
        }
    ]
    script_content_hash = hashlib.sha256(episode_text.encode("utf-8")).hexdigest()
    bible_content_hash = "c" * 64
    materialization_hash = "d" * 64
    stage_input = EpisodeAnalysisStageInput.model_validate(
        {
            "episode_id": episode_id,
            "episode_position": 1,
            "script_version_id": script_version_id,
            "script_version_no": 1,
            "document_revision_id": "71000000-0000-0000-0000-000000000007",
            "episode_source_start": source_start,
            "episode_source_end": source_end,
            "script_content_hash": script_content_hash,
            "logical_start": source_start,
            "logical_end": source_end,
            "context_start": source_start,
            "context_end": source_end,
            "context_text": episode_text,
            "logical_text_hash": script_content_hash,
            "scene_marker_hints": [
                {
                    "label": scene_label,
                    "absolute_start": source_start,
                    "absolute_end": source_start + len(scene_label),
                }
            ],
            "adjacent_episodes": [],
            "bible_version_id": bible_version_id,
            "bible_version": 1,
            "bible_content_hash": bible_content_hash,
            "bible_snapshot_hash": canonical_hash(bible_snapshot),
            "bible_snapshot": bible_snapshot,
            "materialization_hash": materialization_hash,
            "known_identities": known_identities,
        }
    )
    source_refs = [
        {
            "owner_kind": "production/episode-script",
            "owner_logical_id": episode_id,
            "owner_version_id": script_version_id,
            "revision": 1,
            "content_hash": script_content_hash,
        },
        {
            "owner_kind": "production/bible-version",
            "owner_logical_id": bible_version_id,
            "owner_version_id": bible_version_id,
            "revision": 1,
            "content_hash": bible_content_hash,
        },
        {
            "owner_kind": "production/bible-materialization",
            "owner_logical_id": bible_version_id,
            "owner_version_id": bible_version_id,
            "revision": 1,
            "content_hash": materialization_hash,
        },
    ]
    analysis_invocation = stage_invocation(
        source,
        invocation_id="21000000-0000-0000-0000-000000000001",
        payload={
            "stage": "analyze_episode",
            "shard_key": "episode:0001:map:0000",
            "workspace_id": source.payload.workspace_id,
            "project_id": source.payload.project_id,
            "source_refs": source_refs,
            "upstream_candidates": [],
            "shard_manifest_ref": {
                "manifest_id": "61000000-0000-0000-0000-000000000001",
                "version": 1,
                "hash": "e" * 64,
            },
            "shard": {
                "kind": "episode_map",
                "key": "episode:0001:map:0000",
                "tree_path": "episode.0001.map.0000",
                "absolute_start": source_start,
                "absolute_end": source_end,
            },
            "stage_input": stage_input.model_dump(mode="json"),
        },
    )
    analysis = await StoryGraphHarness(
        analysis_invocation, repository_root=REPOSITORY_ROOT
    ).execute()

    assert isinstance(analysis, EpisodeAnalysisCandidate)
    assert analysis.episode_id == UUID(episode_id)
    assert analysis.script_version_id == UUID(script_version_id)
    assert (analysis.logical_start, analysis.logical_end) == (source_start, source_end)
    assert analysis.fragments
    for evidence in evidence_identities(analysis):
        evidence_start, evidence_end, text_hash, anchor, episode_number = evidence
        assert episode_number == 1
        assert episode_text[evidence_start - source_start : evidence_end - source_start] == anchor
        assert hashlib.sha256(anchor.encode("utf-8")).hexdigest() == text_hash

    analysis_candidate = analysis.model_dump(mode="json")
    analysis_candidate_hash = canonical_hash(analysis_candidate)
    reconciliation_input = EpisodeReconciliationStageInput.model_validate(
        {
            "episode_id": episode_id,
            "episode_position": 1,
            "script_version_id": script_version_id,
            "script_version_no": 1,
            "episode_source_start": source_start,
            "episode_source_end": source_end,
            "script_content_hash": script_content_hash,
            "bible_version_id": bible_version_id,
            "bible_version": 1,
            "bible_content_hash": bible_content_hash,
            "materialization_hash": materialization_hash,
            "known_identities": known_identities,
            "level": 1,
            "candidate_type": "episode_analysis_candidate",
            "candidates": [
                {
                    "shard_key": analysis_invocation.payload.shard_key,
                    "candidate_revision_id": "71000000-0000-0000-0000-000000000008",
                    "candidate_revision_hash": analysis_candidate_hash,
                    "candidate": analysis_candidate,
                }
            ],
        }
    )
    reconciliation_invocation = stage_invocation(
        source,
        invocation_id="21000000-0000-0000-0000-000000000002",
        payload={
            "stage": "reconcile_episode",
            "shard_key": "episode:0001:reduce:0001:0000",
            "workspace_id": source.payload.workspace_id,
            "project_id": source.payload.project_id,
            "source_refs": source_refs,
            "upstream_candidates": [
                {
                    "stage": "analyze_episode",
                    "shard_key": analysis_invocation.payload.shard_key,
                    "candidate_revision_id": "71000000-0000-0000-0000-000000000008",
                    "candidate_revision_hash": analysis_candidate_hash,
                    "source_invocation_id": analysis_invocation.invocation_id,
                    "source_result_hash": analysis_candidate_hash,
                }
            ],
            "shard_manifest_ref": {
                "manifest_id": "61000000-0000-0000-0000-000000000002",
                "version": 1,
                "hash": "f" * 64,
            },
            "shard": {
                "kind": "episode_reduce",
                "key": "episode:0001:reduce:0001:0000",
                "tree_path": "episode.0001.reduce.0001.0000",
            },
            "stage_input": reconciliation_input.model_dump(mode="json"),
        },
    )
    reconciled = await StoryGraphHarness(
        reconciliation_invocation, repository_root=REPOSITORY_ROOT
    ).execute()

    assert isinstance(reconciled, EpisodeReconciliationCandidate)
    assert (reconciled.source_start, reconciled.source_end) == (source_start, source_end)
    assert {value.temporary_key for value in reconciled.ordered_fragments} == {
        value.temporary_key for value in analysis.fragments
    }
    assert {value.claim_key for value in reconciled.claims} == {
        value.claim_key for value in analysis.claims
    }
    assert evidence_identities(reconciled) <= evidence_identities(analysis)


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
                    "gate_version": "bible-deterministic-gate",
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
