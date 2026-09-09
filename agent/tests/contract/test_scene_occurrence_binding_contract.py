from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.harness.scene_analysis_schemas import (
    SceneAnalysisCandidateRevisionIdentity,
    SceneAnalysisInvocation,
    SceneAnalysisPayload,
    SceneAnalysisStageVariant,
    SceneOccurrenceBindingInput,
)
from app.modules.storygraph.scene_analysis_bundle import (
    SCENE_ANALYSIS_SKILL_BUNDLE_HASH,
    SceneAnalysisBundle,
)
from app.modules.storygraph.scene_analysis_candidates import SceneBindingFragmentCandidate
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec
from tests.contract.test_production_entity_derivation_contract import (
    _candidate as production_entity_candidate,  # pyright: ignore[reportPrivateUsage]
)
from tests.contract.test_production_entity_derivation_contract import (
    _input as production_entity_input,  # pyright: ignore[reportPrivateUsage]
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
SCENE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-scene-analysis-wire.json"


def _input() -> SceneOccurrenceBindingInput:
    production_input = production_entity_input()
    return SceneOccurrenceBindingInput.model_validate(
        {
            "source_version_id": str(production_input.source_version_id),
            "source_hash": production_input.source_hash,
            "normalized_text": production_input.normalized_text,
            "structure_identity_set_version_id": str(
                production_input.structure_identity_set_version_id
            ),
            "structure_identity_set_version_hash": (
                production_input.structure_identity_set_version_hash
            ),
            "structure_identity_set": production_input.structure_identity_set.model_dump(
                mode="json"
            ),
            "scene_fact_candidate_revision_id": str(
                production_input.scene_fact_candidate_revision_id
            ),
            "scene_fact_candidate_revision_hash": (
                production_input.scene_fact_candidate_revision_hash
            ),
            "scene_fact_candidate": production_input.scene_fact_candidate,
            "production_entity_candidate_revision_id": ("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
            "production_entity_candidate_revision_hash": "b" * 64,
            "production_entity_candidate": production_entity_candidate(),
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
    return {
        "source_version_id": str(stage_input.source_version_id),
        "source_hash": stage_input.source_hash,
        "structure_identity_set_version_id": str(stage_input.structure_identity_set_version_id),
        "structure_identity_set_version_hash": stage_input.structure_identity_set_version_hash,
        "scene_fact_candidate_revision_id": str(stage_input.scene_fact_candidate_revision_id),
        "scene_fact_candidate_revision_hash": stage_input.scene_fact_candidate_revision_hash,
        "production_entity_candidate_revision_id": str(
            stage_input.production_entity_candidate_revision_id
        ),
        "production_entity_candidate_revision_hash": (
            stage_input.production_entity_candidate_revision_hash
        ),
        "scenes": [
            {
                "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "scene_owner_logical_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "temporary_scene_id": "scene_0001",
                "source_start": 0,
                "source_end": 16,
                "dialogues": [],
                "beats": [
                    {
                        "beat_key": "beat_scene_0001_0001",
                        "order": 1,
                        "text": "林舟握住门把。",
                        "evidence": _evidence(
                            8,
                            15,
                            "f2ffbd1d3cb0f5e0d95b211b12a0bdee451dd82d38bf673915e42420f7ed9ff1",
                            "林舟握住门把。",
                        ),
                    }
                ],
                "occurrences": [
                    {
                        "occurrence_key": "occurrence_scene_0001_0001",
                        "order": 1,
                        "subject_kind": "location",
                        "identity_key": "location:interior",
                        "state_key": "state_location_interior_initial",
                        "occurrence_role": "actual",
                        "evidence": _evidence(
                            6,
                            7,
                            "b2126ce9c100bff0cc963326e29fadc751106ad7fd7f34aa46cddc06d8198c6a",
                            "内",
                        ),
                    },
                    {
                        "occurrence_key": "occurrence_scene_0001_0002",
                        "order": 2,
                        "subject_kind": "character",
                        "identity_key": "character:linzhou",
                        "state_key": "state_character_linzhou_initial",
                        "occurrence_role": "actual",
                        "evidence": _evidence(
                            8,
                            10,
                            "475fe7b6fbd3ec67f16c535eb23fb4630efa5a8958a041408682f88e9685d4f1",
                            "林舟",
                        ),
                    },
                    {
                        "occurrence_key": "occurrence_scene_0001_0003",
                        "order": 3,
                        "subject_kind": "prop",
                        "identity_key": "prop:door_handle",
                        "state_key": "state_prop_door_handle_initial",
                        "occurrence_role": "actual",
                        "evidence": _evidence(
                            12,
                            14,
                            "3ff7bc50c9533059a629804286be3602fd8805c9454949ba69ce9b1e8c0b96d9",
                            "门把",
                        ),
                    },
                ],
            },
            {
                "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
                "scene_owner_logical_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
                "temporary_scene_id": "scene_0002",
                "source_start": 16,
                "source_end": 29,
                "dialogues": [],
                "beats": [
                    {
                        "beat_key": "beat_scene_0002_0001",
                        "order": 1,
                        "text": "林舟离开。",
                        "evidence": _evidence(
                            24,
                            29,
                            "d6e8609481cb01d70f08f80f286f4c2e752f1e471b55c96ca1cc5879036445c4",
                            "林舟离开。",
                        ),
                    }
                ],
                "occurrences": [
                    {
                        "occurrence_key": "occurrence_scene_0002_0001",
                        "order": 1,
                        "subject_kind": "location",
                        "identity_key": "location:exterior",
                        "state_key": "state_location_exterior_initial",
                        "occurrence_role": "actual",
                        "evidence": _evidence(
                            22,
                            23,
                            "0e826095b60ccf039478a7e093220d79a4614158557924383bd3c7ef1bb06d8a",
                            "外",
                        ),
                    },
                    {
                        "occurrence_key": "occurrence_scene_0002_0002",
                        "order": 2,
                        "subject_kind": "character",
                        "identity_key": "character:linzhou",
                        "state_key": "state_character_linzhou_initial",
                        "occurrence_role": "actual",
                        "evidence": _evidence(
                            24,
                            26,
                            "475fe7b6fbd3ec67f16c535eb23fb4630efa5a8958a041408682f88e9685d4f1",
                            "林舟",
                        ),
                    },
                ],
            },
        ],
        "review_issues": [],
    }


def test_scene_occurrence_candidate_binds_exact_scene_identity_and_state() -> None:
    stage_input = _input()
    candidate = SceneBindingFragmentCandidate.model_validate(_candidate())
    candidate.validate_for_input(stage_input)
    assert len(candidate.scenes) == 2
    assert sum(len(scene.occurrences) for scene in candidate.scenes) == 5


@pytest.mark.parametrize("drift", ["state", "presence", "scene", "visual"])
def test_scene_occurrence_candidate_rejects_fuzzy_or_invented_binding(drift: str) -> None:
    value = copy.deepcopy(_candidate())
    if drift == "state":
        value["scenes"][0]["occurrences"][1]["state_key"] = "state_location_interior_initial"
    elif drift == "presence":
        value["scenes"][0]["occurrences"][1]["occurrence_role"] = "mentioned_only"
    elif drift == "scene":
        value["scenes"].pop()
    else:
        value["scenes"][0]["visual_preset"] = "cinematic"
    with pytest.raises((ValidationError, ValueError)):
        candidate = SceneBindingFragmentCandidate.model_validate(value)
        candidate.validate_for_input(_input())


def test_scene_occurrence_stage_has_semantic_contract_and_resource() -> None:
    spec = scene_analysis_stage_spec("bind_scene_occurrences", "default")
    assert spec.candidate_type == "scene_binding_fragment_candidate"
    assert spec.candidate_model is SceneBindingFragmentCandidate
    assert SceneAnalysisBundle().loaded_paths("bind_scene_occurrences", "default") == (
        "SKILL.md",
        "references/scene-occurrences.md",
    )


@pytest.mark.asyncio
async def test_scene_occurrence_stage_runs_from_exact_frozen_upstreams(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fixture = json.loads(SCENE_FIXTURE.read_text(encoding="utf-8"))
    base = SceneAnalysisInvocation.model_validate(fixture["valid_invocation"])
    stage_input = _input()
    candidate = SceneBindingFragmentCandidate.model_validate(_candidate())
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
                stage_key="bind_scene_occurrences",
                profile_key="default",
                lane_key="primary",
                output_schema_version="scene-binding-fragment-candidate-production",
            ),
            scope=base.payload.scope,
            source_refs=base.payload.source_refs,
            upstream_candidates=[
                SceneAnalysisCandidateRevisionIdentity(
                    stage_key="extract_scene_facts",
                    shard_key="script:full",
                    candidate_revision_id=stage_input.scene_fact_candidate_revision_id,
                    candidate_revision_hash=stage_input.scene_fact_candidate_revision_hash,
                    source_invocation_id=UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
                    source_result_hash="a" * 64,
                ),
                SceneAnalysisCandidateRevisionIdentity(
                    stage_key="derive_production_entities",
                    shard_key="script:full",
                    candidate_revision_id=(stage_input.production_entity_candidate_revision_id),
                    candidate_revision_hash=(stage_input.production_entity_candidate_revision_hash),
                    source_invocation_id=UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
                    source_result_hash="b" * 64,
                ),
            ],
            shard=base.payload.shard,
            stage_input=stage_input.model_dump(mode="json"),
        ),
    )

    async def return_candidate(*_: object) -> SceneBindingFragmentCandidate:
        return candidate

    monkeypatch.setattr(SceneAnalysisHarness, "_run_codex", return_candidate)
    result = await SceneAnalysisHarness(invocation, repository_root=REPOSITORY_ROOT).execute()

    assert result == candidate
