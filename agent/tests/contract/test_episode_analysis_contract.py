from __future__ import annotations

import hashlib
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.candidate_runtime.canonical import canonical_hash
from app.candidate_runtime.schemas import (
    EpisodeAnalysisStageInput,
    EpisodeReconciliationStageInput,
    StoryGraphExecutionPolicy,
    StoryGraphStageInvocation,
    StoryGraphStagePayload,
)
from app.modules.storygraph.candidate_schemas import (
    EpisodeAnalysisCandidate,
    EpisodeReconciliationCandidate,
)
from app.modules.storygraph.harness import (
    CodexSchemaInvalid,
    normalize_episode_candidate_evidence,
)


def _stage_invocation(stage: str, payload: dict[str, object]) -> StoryGraphStageInvocation:
    draft = StoryGraphStageInvocation.model_construct(
        invocation_id=UUID("71000000-0000-0000-0000-000000000001"),
        kind="storygraph_stage",
        wire_schema_version="storygraph-stage-wire",
        input_hash="0" * 64,
        execution_policy=StoryGraphExecutionPolicy.model_validate(
            {
                "definition_key": "storygraph_stage",
                "definition_version": "storygraph-stage-harness",
                "prompt_version": "build-storygraph-prompt",
                "skill_bundle_version": "build-storygraph",
                "skill_bundle_hash": (
                    "352d46c51661e7d989b42ddeb0a0ff0a4b48165e8e3f7700f3e60d170e4c58cb"
                ),
                "output_schema_version": "storygraph-candidate-schema",
                "model_capability": "structured_text",
                "codex_runtime_contract": "codex-cli-ephemeral-read-only",
                "allowed_tools": [],
                "max_model_calls": 2,
                "max_execution_seconds": 600,
            }
        ),
        payload=StoryGraphStagePayload.model_validate({"stage": stage, **payload}),
    )
    return StoryGraphStageInvocation.model_validate(
        draft.model_copy(update={"input_hash": draft.compute_input_hash()}).model_dump(mode="json")
    )


def _analysis_input() -> dict[str, object]:
    text = "内景 客厅 日\n阿澜：我回来了。"
    source_start = 100
    source_end = source_start + len(text)
    snapshot: dict[str, object] = {
        "canonical_entities": [],
        "canonical_world_entries": [],
        "merged_claims": [],
        "merged_arcs": [],
        "conflicts": [],
        "review_issues": [],
    }
    return {
        "episode_id": "71000000-0000-0000-0000-000000000010",
        "episode_position": 2,
        "script_version_id": "71000000-0000-0000-0000-000000000011",
        "script_version_no": 1,
        "document_revision_id": "71000000-0000-0000-0000-000000000012",
        "episode_source_start": source_start,
        "episode_source_end": source_end,
        "script_content_hash": hashlib.sha256(text.encode()).hexdigest(),
        "logical_start": source_start,
        "logical_end": source_end,
        "context_start": source_start,
        "context_end": source_end,
        "context_text": text,
        "logical_text_hash": hashlib.sha256(text.encode()).hexdigest(),
        "scene_marker_hints": [
            {
                "label": "内景 客厅 日",
                "absolute_start": source_start,
                "absolute_end": source_start + len("内景 客厅 日"),
            }
        ],
        "adjacent_episodes": [
            {
                "side": "previous",
                "episode_id": "71000000-0000-0000-0000-000000000020",
                "episode_position": 1,
                "script_version_id": "71000000-0000-0000-0000-000000000021",
                "script_version_no": 1,
                "source_start": 0,
                "source_end": 100,
                "content_hash": "1" * 64,
                "excerpt_start": 96,
                "excerpt_end": 100,
                "excerpt": "雨停了。",
                "excerpt_hash": hashlib.sha256("雨停了。".encode()).hexdigest(),
            }
        ],
        "bible_version_id": "71000000-0000-0000-0000-000000000030",
        "bible_version": 1,
        "bible_content_hash": "2" * 64,
        "bible_snapshot_hash": canonical_hash(snapshot),
        "bible_snapshot": snapshot,
        "materialization_hash": "3" * 64,
        "known_identities": [
            {
                "entity_key": "character:alan",
                "kind": "character",
                "asset_id": "71000000-0000-0000-0000-000000000040",
                "specification_version_id": "71000000-0000-0000-0000-000000000041",
                "specification_hash": "4" * 64,
                "states": [
                    {
                        "state_key": "base",
                        "asset_state_id": "71000000-0000-0000-0000-000000000042",
                        "content_hash": "5" * 64,
                    }
                ],
            }
        ],
    }


def test_analyze_episode_input_is_bound_to_published_slice_bible_and_neighbors() -> None:
    stage_input = EpisodeAnalysisStageInput.model_validate(_analysis_input())
    payload: dict[str, object] = {
        "shard_key": "episode:0002:map:0000",
        "workspace_id": "71000000-0000-0000-0000-000000000002",
        "project_id": "71000000-0000-0000-0000-000000000003",
        "source_refs": [
            {
                "owner_kind": "production/episode-script",
                "owner_logical_id": str(stage_input.episode_id),
                "owner_version_id": str(stage_input.script_version_id),
                "revision": stage_input.script_version_no,
                "content_hash": stage_input.script_content_hash,
            },
            {
                "owner_kind": "production/episode-script",
                "owner_logical_id": str(stage_input.adjacent_episodes[0].episode_id),
                "owner_version_id": str(stage_input.adjacent_episodes[0].script_version_id),
                "revision": stage_input.adjacent_episodes[0].script_version_no,
                "content_hash": stage_input.adjacent_episodes[0].content_hash,
            },
            {
                "owner_kind": "production/bible-version",
                "owner_logical_id": str(stage_input.bible_version_id),
                "owner_version_id": str(stage_input.bible_version_id),
                "revision": stage_input.bible_version,
                "content_hash": stage_input.bible_content_hash,
            },
            {
                "owner_kind": "production/bible-materialization",
                "owner_logical_id": str(stage_input.bible_version_id),
                "owner_version_id": str(stage_input.bible_version_id),
                "revision": stage_input.bible_version,
                "content_hash": stage_input.materialization_hash,
            },
        ],
        "upstream_candidates": [],
        "shard_manifest_ref": {
            "manifest_id": "71000000-0000-0000-0000-000000000004",
            "version": 1,
            "hash": "6" * 64,
        },
        "shard": {
            "kind": "episode_map",
            "key": "episode:0002:map:0000",
            "tree_path": "episode/0002/map/0000",
            "absolute_start": stage_input.logical_start,
            "absolute_end": stage_input.logical_end,
        },
        "stage_input": stage_input.model_dump(mode="json"),
    }
    invocation = _stage_invocation("analyze_episode", payload)
    assert invocation.payload.stage == "analyze_episode"

    drifted = invocation.model_dump(mode="json")
    drifted["payload"]["stage_input"]["logical_end"] -= 1
    with pytest.raises(ValidationError):
        StoryGraphStageInvocation.model_validate(drifted)


def test_episode_candidates_preserve_evidence_known_identity_and_child_set() -> None:
    stage_input = EpisodeAnalysisStageInput.model_validate(_analysis_input())
    evidence = {
        "source_start": stage_input.logical_start,
        "source_end": stage_input.logical_end,
        "text_hash": stage_input.logical_text_hash,
        "exact_anchor": stage_input.context_text,
        "episode_number": stage_input.episode_position,
    }
    analysis = EpisodeAnalysisCandidate.model_validate(
        {
            "episode_id": str(stage_input.episode_id),
            "script_version_id": str(stage_input.script_version_id),
            "logical_start": stage_input.logical_start,
            "logical_end": stage_input.logical_end,
            "fragments": [
                {
                    "temporary_key": "scene:0001",
                    "kind": "scene",
                    "source_keys": [f"episode:{stage_input.episode_id}"],
                    "source_start": stage_input.logical_start,
                    "source_end": stage_input.logical_end,
                    "summary": "阿澜回到客厅",
                    "evidence": [evidence],
                    "attributes": {
                        "scene_key": None,
                        "speaker_key": None,
                        "participant_keys": ["character:alan"],
                        "location_key": None,
                        "time_hint": "日",
                        "dialogue_text": None,
                        "action": "阿澜进入客厅",
                        "occurrence_entity_key": None,
                        "state_key": None,
                        "continuity_notes": [],
                    },
                }
            ],
            "claims": [],
            "review_issues": [],
        }
    )
    analysis.validate_for(stage_input)

    untrusted_analysis = analysis.model_copy(deep=True)
    untrusted_analysis.fragments[0].evidence[0].text_hash = "0" * 64
    normalized_analysis = normalize_episode_candidate_evidence(untrusted_analysis, stage_input)
    assert isinstance(normalized_analysis, EpisodeAnalysisCandidate)
    assert normalized_analysis.fragments[0].evidence[0].text_hash == stage_input.logical_text_hash

    drifted_analysis = analysis.model_copy(deep=True)
    drifted_analysis.fragments[0].evidence[0].exact_anchor = "虚构的文本"
    with pytest.raises(CodexSchemaInvalid, match="immutable text slice"):
        normalize_episode_candidate_evidence(drifted_analysis, stage_input)

    reconciliation_input = EpisodeReconciliationStageInput.model_validate(
        {
            "episode_id": str(stage_input.episode_id),
            "episode_position": stage_input.episode_position,
            "script_version_id": str(stage_input.script_version_id),
            "script_version_no": stage_input.script_version_no,
            "episode_source_start": stage_input.episode_source_start,
            "episode_source_end": stage_input.episode_source_end,
            "script_content_hash": stage_input.script_content_hash,
            "bible_version_id": str(stage_input.bible_version_id),
            "bible_version": stage_input.bible_version,
            "bible_content_hash": stage_input.bible_content_hash,
            "materialization_hash": stage_input.materialization_hash,
            "known_identities": stage_input.model_dump(mode="json")["known_identities"],
            "level": 1,
            "candidate_type": "episode_analysis_candidate",
            "candidates": [
                {
                    "shard_key": "episode:0002:map:0000",
                    "candidate_revision_id": "71000000-0000-0000-0000-000000000050",
                    "candidate_revision_hash": "7" * 64,
                    "candidate": analysis.model_dump(mode="json"),
                }
            ],
        }
    )
    reconciled = EpisodeReconciliationCandidate.model_validate(
        {
            "episode_id": str(stage_input.episode_id),
            "script_version_id": str(stage_input.script_version_id),
            "source_start": stage_input.episode_source_start,
            "source_end": stage_input.episode_source_end,
            "ordered_fragments": analysis.model_dump(mode="json")["fragments"],
            "claims": [],
            "conflicts": [],
            "review_issues": [],
        }
    )
    reconciled.validate_for(reconciliation_input)

    untrusted_reconciliation = reconciled.model_copy(deep=True)
    untrusted_reconciliation.ordered_fragments[0].evidence[0].text_hash = "0" * 64
    normalized_reconciliation = normalize_episode_candidate_evidence(
        untrusted_reconciliation, reconciliation_input
    )
    assert isinstance(normalized_reconciliation, EpisodeReconciliationCandidate)
    assert (
        normalized_reconciliation.ordered_fragments[0].evidence[0].text_hash
        == stage_input.logical_text_hash
    )

    drifted_reconciliation = reconciled.model_copy(deep=True)
    drifted_reconciliation.ordered_fragments[0].evidence[0].exact_anchor = "虚构的文本"
    with pytest.raises(CodexSchemaInvalid, match="exact children"):
        normalize_episode_candidate_evidence(drifted_reconciliation, reconciliation_input)

    unknown = analysis.model_copy(deep=True)
    unknown.fragments[0].attributes.participant_keys = ["character:invented"]
    with pytest.raises(ValueError, match="known identity"):
        unknown.validate_for(stage_input)
