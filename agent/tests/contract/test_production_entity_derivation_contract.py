from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any, cast
from uuid import UUID, uuid4

import pytest
from pydantic import ValidationError

from app.harness.scene_analysis_schemas import (
    ProductionEntityDerivationInput,
    SceneAnalysisCandidateRevisionIdentity,
    SceneAnalysisInvocation,
    SceneAnalysisPayload,
    SceneAnalysisStageVariant,
)
from app.modules.storygraph.scene_analysis_bundle import (
    SCENE_ANALYSIS_SKILL_BUNDLE_HASH,
    SceneAnalysisBundle,
)
from app.modules.storygraph.scene_analysis_candidates import ProductionEntityFragmentCandidate
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
SCENE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-scene-analysis-wire.json"


def _input() -> ProductionEntityDerivationInput:
    fixture = json.loads(SCENE_FIXTURE.read_text(encoding="utf-8"))
    facts = fixture["valid_scene_fact_candidate"]
    source = fixture["valid_invocation"]["payload"]["stage_input"]
    return ProductionEntityDerivationInput.model_validate(
        {
            "source_version_id": source["source_version_id"],
            "source_hash": source["source_hash"],
            "normalized_text": source["normalized_text"],
            "structure_identity_set_version_id": "11111111-1111-4111-8111-111111111111",
            "structure_identity_set_version_hash": "1" * 64,
            "structure_identity_set": {
                "schema_version": "structure-identity-set-production",
                "id": "11111111-1111-4111-8111-111111111111",
                "workspace_id": "11111111-1111-4111-8111-111111111111",
                "project_id": "22222222-2222-4222-8222-222222222222",
                "version": 1,
                "parent_version_id": None,
                "gate_input_id": "44444444-4444-4444-8444-444444444444",
                "gate_input_hash": "4" * 64,
                "review_decision_id": "55555555-5555-4555-8555-555555555555",
                "project_episode_receipt_id": "66666666-6666-4666-8666-666666666666",
                "document_revision_id": source["source_version_id"],
                "span_index_id": "77777777-7777-4777-8777-777777777777",
                "candidate_refs": [],
                "episode_refs": [
                    {
                        "temporary_episode_id": "episode_0001",
                        "episode_id": "88888888-8888-4888-8888-888888888888",
                        "episode_revision": 1,
                        "position": 1,
                        "script_version_id": "99999999-9999-4999-8999-999999999998",
                        "script_version": 1,
                        "source_start": 0,
                        "source_end": 29,
                        "content_hash": "8" * 64,
                    }
                ],
                "scene_refs": [
                    {
                        "temporary_episode_id": "episode_0001",
                        "episode_id": "88888888-8888-4888-8888-888888888888",
                        "temporary_span_id": "span_0001",
                        "temporary_scene_id": "scene_0001",
                        "scene_owner_logical_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                        "scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                        "source_start": 0,
                        "source_end": 16,
                        "evidence_hash": "a" * 64,
                    },
                    {
                        "temporary_episode_id": "episode_0001",
                        "episode_id": "88888888-8888-4888-8888-888888888888",
                        "temporary_span_id": "span_0002",
                        "temporary_scene_id": "scene_0002",
                        "scene_owner_logical_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
                        "scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
                        "source_start": 16,
                        "source_end": 29,
                        "evidence_hash": "b" * 64,
                    },
                ],
                "identities": [
                    {
                        "temporary_identity_key": key,
                        "identity_key": stable,
                        "kind": kind,
                        "resolution": "new",
                        "reuse_identity_key": None,
                        "canonical_name": name,
                        "aliases": [name],
                    }
                    for key, stable, kind, name in (
                        ("identity_character_linzhou", "character:linzhou", "character", "林舟"),
                        ("identity_location_interior", "location:interior", "location", "内"),
                        ("identity_location_exterior", "location:exterior", "location", "外"),
                        ("identity_prop_door_handle", "prop:door_handle", "prop", "门把"),
                    )
                ],
                "mention_mappings": [
                    {
                        "kind": kind,
                        "occurrence_role": "actual",
                        "temporary_scene_id": scene,
                        "source_start": start,
                        "source_end": end,
                        "text_hash": text_hash,
                        "exact_anchor": anchor,
                        "resolution": "resolved",
                        "identity_key": identity,
                    }
                    for kind, scene, start, end, text_hash, anchor, identity in (
                        (
                            "location",
                            "scene_0001",
                            6,
                            7,
                            "b2126ce9c100bff0cc963326e29fadc751106ad7fd7f34aa46cddc06d8198c6a",
                            "内",
                            "location:interior",
                        ),
                        (
                            "character",
                            "scene_0001",
                            8,
                            10,
                            "475fe7b6fbd3ec67f16c535eb23fb4630efa5a8958a041408682f88e9685d4f1",
                            "林舟",
                            "character:linzhou",
                        ),
                        (
                            "prop",
                            "scene_0001",
                            12,
                            14,
                            "3ff7bc50c9533059a629804286be3602fd8805c9454949ba69ce9b1e8c0b96d9",
                            "门把",
                            "prop:door_handle",
                        ),
                        (
                            "location",
                            "scene_0002",
                            22,
                            23,
                            "0e826095b60ccf039478a7e093220d79a4614158557924383bd3c7ef1bb06d8a",
                            "外",
                            "location:exterior",
                        ),
                        (
                            "character",
                            "scene_0002",
                            24,
                            26,
                            "475fe7b6fbd3ec67f16c535eb23fb4630efa5a8958a041408682f88e9685d4f1",
                            "林舟",
                            "character:linzhou",
                        ),
                    )
                ],
                "coverage": {
                    "scene_count": 2,
                    "identity_count": 4,
                    "mention_count": 5,
                    "resolved_count": 5,
                    "unresolved_count": 0,
                    "mention_universe_hash": "c" * 64,
                    "scope_set_hash": "d" * 64,
                },
                "content_hash": "1" * 64,
                "created_by": "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
                "created_at": "2026-09-09T08:00:00Z",
            },
            "scene_fact_candidate_revision_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            "scene_fact_candidate_revision_hash": "a" * 64,
            "scene_fact_candidate": facts,
        }
    )


def _evidence(start: int, end: int, text_hash: str, anchor: str) -> dict[str, Any]:
    return {
        "source_start": start,
        "source_end": end,
        "text_hash": text_hash,
        "exact_anchor": anchor,
    }


def _candidate() -> dict[str, Any]:
    stage_input = _input()
    evidence_by_identity = {
        "character:linzhou": _evidence(
            8, 10, "475fe7b6fbd3ec67f16c535eb23fb4630efa5a8958a041408682f88e9685d4f1", "林舟"
        ),
        "location:interior": _evidence(
            6, 7, "b2126ce9c100bff0cc963326e29fadc751106ad7fd7f34aa46cddc06d8198c6a", "内"
        ),
        "location:exterior": _evidence(
            22, 23, "0e826095b60ccf039478a7e093220d79a4614158557924383bd3c7ef1bb06d8a", "外"
        ),
        "prop:door_handle": _evidence(
            12, 14, "3ff7bc50c9533059a629804286be3602fd8805c9454949ba69ce9b1e8c0b96d9", "门把"
        ),
    }
    scenes_by_identity = {
        "character:linzhou": [
            "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
            "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
        ],
        "location:interior": ["scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"],
        "location:exterior": ["scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2"],
        "prop:door_handle": ["scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"],
    }
    names = {
        "character:linzhou": "林舟",
        "location:interior": "内",
        "location:exterior": "外",
        "prop:door_handle": "门把",
    }
    kinds = {
        "character:linzhou": ("character", "character_appearance"),
        "location:interior": ("location", "location_state"),
        "location:exterior": ("location", "location_state"),
        "prop:door_handle": ("prop", "prop_state"),
    }
    entities: list[dict[str, Any]] = []
    for identity_key in sorted(kinds):
        kind, state_kind = kinds[identity_key]
        semantic_key = identity_key.replace(":", "_")
        basis = {
            "provenance": "source_explicit",
            "evidence": [evidence_by_identity[identity_key]],
            "creator_decision_proposal": None,
        }
        entities.append(
            {
                "identity_key": identity_key,
                "kind": kind,
                "specification_key": f"specification_{semantic_key}",
                "specification_slots": [
                    {
                        "slot_key": "canonical_name",
                        "resolution": "known",
                        "value": names[identity_key],
                        "design_gap_key": None,
                    }
                ],
                "basis": basis,
                "states": [
                    {
                        "state_key": f"state_{semantic_key}_initial",
                        "state_kind": state_kind,
                        "complete_slots": [
                            {
                                "slot_key": "baseline",
                                "resolution": "known",
                                "value": names[identity_key],
                                "design_gap_key": None,
                            }
                        ],
                        "applicable_scene_scope_keys": scenes_by_identity[identity_key],
                        "entry_reason": "剧本首次出现",
                        "exit_reason": "剧本范围结束",
                        "previous_state_key": None,
                        "next_state_key": None,
                        "basis": copy.deepcopy(basis),
                    }
                ],
            }
        )
    return {
        "source_version_id": str(stage_input.source_version_id),
        "source_hash": stage_input.source_hash,
        "structure_identity_set_version_id": str(stage_input.structure_identity_set_version_id),
        "structure_identity_set_version_hash": stage_input.structure_identity_set_version_hash,
        "scene_fact_candidate_revision_id": str(stage_input.scene_fact_candidate_revision_id),
        "scene_fact_candidate_revision_hash": stage_input.scene_fact_candidate_revision_hash,
        "entities": entities,
        "world_claims": [],
        "design_gaps": [],
        "review_issues": [],
    }


def test_production_entity_candidate_covers_each_formal_identity_with_typed_state() -> None:
    stage_input = _input()
    candidate = ProductionEntityFragmentCandidate.model_validate(_candidate())

    candidate.validate_for(stage_input)
    assert {entity.kind for entity in candidate.entities} == {"character", "location", "prop"}
    assert {state.state_kind for entity in candidate.entities for state in entity.states} == {
        "character_appearance",
        "location_state",
        "prop_state",
    }


@pytest.mark.parametrize(
    "mutation", ["missing_identity", "kind_drift", "double_source", "missing_source", "preset"]
)
def test_production_entity_candidate_rejects_identity_source_and_visual_drift(
    mutation: str,
) -> None:
    stage_input = _input()
    raw = _candidate()
    entities = cast(list[dict[str, Any]], raw["entities"])
    if mutation == "missing_identity":
        entities.pop()
    elif mutation == "kind_drift":
        entities[0]["kind"] = "prop"
    elif mutation == "double_source":
        entities[0]["basis"]["creator_decision_proposal"] = {
            "decision_key": "decision_character_linzhou",
            "rationale": "补充人物设计",
        }
    elif mutation == "missing_source":
        entities[0]["basis"]["evidence"] = []
    else:
        raw["preset"] = "cinematic"

    with pytest.raises((ValidationError, ValueError)):
        candidate = ProductionEntityFragmentCandidate.model_validate(raw)
        candidate.validate_for(stage_input)


def test_production_entity_candidate_accepts_creator_decision_without_source_evidence() -> None:
    stage_input = _input()
    raw = _candidate()
    entity = cast(list[dict[str, Any]], raw["entities"])[0]
    entity["basis"] = {
        "provenance": "user_supplied",
        "evidence": [],
        "creator_decision_proposal": {
            "decision_key": "decision_character_linzhou",
            "rationale": "创作者明确补充人物制作设定",
        },
    }

    candidate = ProductionEntityFragmentCandidate.model_validate(raw)
    candidate.validate_for(stage_input)


def test_production_entity_candidate_requires_explicit_narrative_claim_facts() -> None:
    stage_input = _input()
    raw = _candidate()
    scene = stage_input.structure_identity_set.scene_refs[0]
    raw["world_claims"] = [
        {
            "claim_key": "claim_linzhou_protects_home",
            "claim_type": "relationship",
            "participants": [{"role": "subject", "identity_key": "character:linzhou"}],
            "statement": "林舟守护故乡。",
            "narrative": {
                "claim_series_key": "claim_linzhou_protects_home",
                "predicate": "protects",
                "anchors": [{"role": "scene", "target_key": scene.scope_key}],
                "valid_scope": {
                    "kind": "scene",
                    "owner_logical_id": scene.scope_key,
                },
                "story_time_range": None,
                "polarity": "positive",
                "status": "asserted",
            },
            "basis": copy.deepcopy(cast(list[dict[str, Any]], raw["entities"])[0]["basis"]),
        }
    ]

    candidate = ProductionEntityFragmentCandidate.model_validate(raw)
    candidate.validate_for(stage_input)


@pytest.mark.parametrize(
    "mutation",
    ["missing_narrative", "missing_subject", "unknown_identity", "unknown_anchor", "scope_drift"],
)
def test_production_entity_candidate_rejects_incomplete_narrative_claim_facts(
    mutation: str,
) -> None:
    stage_input = _input()
    raw = _candidate()
    scene = stage_input.structure_identity_set.scene_refs[0]
    claim: dict[str, Any] = {
        "claim_key": "claim_linzhou_protects_home",
        "claim_type": "relationship",
        "participants": [{"role": "subject", "identity_key": "character:linzhou"}],
        "statement": "林舟守护故乡。",
        "narrative": {
            "claim_series_key": "claim_linzhou_protects_home",
            "predicate": "protects",
            "anchors": [{"role": "scene", "target_key": scene.scope_key}],
            "valid_scope": {
                "kind": "scene",
                "owner_logical_id": scene.scope_key,
            },
            "story_time_range": None,
            "polarity": "positive",
            "status": "asserted",
        },
        "basis": copy.deepcopy(cast(list[dict[str, Any]], raw["entities"])[0]["basis"]),
    }
    raw["world_claims"] = [claim]
    if mutation == "missing_narrative":
        claim["narrative"] = None
    elif mutation == "missing_subject":
        claim["participants"][0]["role"] = "participant"
    elif mutation == "unknown_identity":
        claim["participants"][0]["identity_key"] = "character:unknown"
    elif mutation == "unknown_anchor":
        claim["narrative"]["anchors"][0]["target_key"] = f"scene:{uuid4()}"
    else:
        claim["narrative"]["valid_scope"]["owner_logical_id"] = str(uuid4())

    with pytest.raises((ValidationError, ValueError)):
        candidate = ProductionEntityFragmentCandidate.model_validate(raw)
        candidate.validate_for(stage_input)


def test_production_entity_stage_has_its_own_schema_and_skill_resource() -> None:
    spec = scene_analysis_stage_spec("derive_production_entities", "default")

    assert spec.candidate_type == "production_entity_fragment_candidate"
    assert spec.candidate_model is ProductionEntityFragmentCandidate
    assert SceneAnalysisBundle().loaded_paths("derive_production_entities", "default") == (
        "SKILL.md",
        "references/production-entities.md",
    )


@pytest.mark.asyncio
async def test_production_entity_stage_runs_from_formal_identity_and_scene_fact(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fixture = json.loads(SCENE_FIXTURE.read_text(encoding="utf-8"))
    base = SceneAnalysisInvocation.model_validate(fixture["valid_invocation"])
    stage_input = _input()
    candidate = ProductionEntityFragmentCandidate.model_validate(_candidate())
    release = base.stage_release.model_copy(
        update={"bundle_content_hash": SCENE_ANALYSIS_SKILL_BUNDLE_HASH}
    )
    invocation = SceneAnalysisInvocation.build(
        invocation_id=base.invocation_id,
        attempt_id=base.attempt_id,
        stage_release=release,
        control=base.control,
        budget=base.budget,
        payload=SceneAnalysisPayload(
            variant=SceneAnalysisStageVariant(
                stage_key="derive_production_entities",
                profile_key="default",
                lane_key="primary",
                output_schema_version="production-entity-fragment-candidate-production",
            ),
            scope=base.payload.scope,
            source_refs=base.payload.source_refs,
            upstream_candidates=[
                SceneAnalysisCandidateRevisionIdentity(
                    stage_key="extract_scene_facts",
                    shard_key="script:full",
                    candidate_revision_id=stage_input.scene_fact_candidate_revision_id,
                    candidate_revision_hash=stage_input.scene_fact_candidate_revision_hash,
                    source_invocation_id=UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
                    source_result_hash="b" * 64,
                )
            ],
            shard=base.payload.shard,
            stage_input=stage_input.model_dump(mode="json"),
        ),
    )

    async def return_candidate(*_: object) -> ProductionEntityFragmentCandidate:
        return candidate

    monkeypatch.setattr(SceneAnalysisHarness, "_run_codex", return_candidate)
    result = await SceneAnalysisHarness(invocation, repository_root=REPOSITORY_ROOT).execute()

    assert result == candidate
