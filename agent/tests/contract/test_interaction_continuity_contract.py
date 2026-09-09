from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.harness.scene_analysis_schemas import (
    InteractionContinuityInput,
    SceneAnalysisCandidateRevisionIdentity,
    SceneAnalysisInvocation,
    SceneAnalysisPayload,
    SceneAnalysisStageKey,
    SceneAnalysisStageVariant,
)
from app.modules.storygraph.scene_analysis_bundle import (
    SCENE_ANALYSIS_SKILL_BUNDLE_HASH,
    SceneAnalysisBundle,
)
from app.modules.storygraph.scene_analysis_candidates import InteractionContinuityCandidate
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec
from tests.contract.test_scene_occurrence_binding_contract import (
    _candidate as scene_binding_candidate,  # pyright: ignore[reportPrivateUsage]
)
from tests.contract.test_scene_occurrence_binding_contract import (
    _input as scene_binding_input,  # pyright: ignore[reportPrivateUsage]
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
SCENE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-scene-analysis-wire.json"


def _input() -> InteractionContinuityInput:
    source = scene_binding_input()
    return InteractionContinuityInput.model_validate(
        {
            **source.model_dump(mode="json"),
            "scene_binding_candidate_revision_id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
            "scene_binding_candidate_revision_hash": "c" * 64,
            "scene_binding_candidate": scene_binding_candidate(),
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
    source = _input()
    return {
        "source_version_id": str(source.source_version_id),
        "source_hash": source.source_hash,
        "structure_identity_set_version_id": str(source.structure_identity_set_version_id),
        "structure_identity_set_version_hash": source.structure_identity_set_version_hash,
        "scene_fact_candidate_revision_id": str(source.scene_fact_candidate_revision_id),
        "scene_fact_candidate_revision_hash": source.scene_fact_candidate_revision_hash,
        "production_entity_candidate_revision_id": str(
            source.production_entity_candidate_revision_id
        ),
        "production_entity_candidate_revision_hash": (
            source.production_entity_candidate_revision_hash
        ),
        "scene_binding_candidate_revision_id": str(source.scene_binding_candidate_revision_id),
        "scene_binding_candidate_revision_hash": source.scene_binding_candidate_revision_hash,
        "interactions": [
            {
                "interaction_key": "interaction_scene_0001_0001",
                "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "beat_key": "beat_scene_0001_0001",
                "predicate": "hold",
                "actor_occurrence_key": "occurrence_scene_0001_0002",
                "prop_occurrence_key": "occurrence_scene_0001_0003",
                "counterparty_occurrence_key": None,
                "holder_before_identity_key": None,
                "holder_after_identity_key": "character:linzhou",
                "prop_state_before_key": "state_prop_door_handle_initial",
                "prop_state_after_key": "state_prop_door_handle_initial",
                "hand": None,
                "contact_point": None,
                "direction": None,
                "relative_scale": None,
                "evidence": _evidence(
                    8,
                    15,
                    "f2ffbd1d3cb0f5e0d95b211b12a0bdee451dd82d38bf673915e42420f7ed9ff1",
                    "林舟握住门把。",
                ),
            }
        ],
        "continuity": [
            {
                "continuity_key": "continuity_character_linzhou_0001",
                "subject_kind": "character",
                "identity_key": "character:linzhou",
                "from_scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "to_scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
                "before_state_key": "state_character_linzhou_initial",
                "after_state_key": "state_character_linzhou_initial",
                "transition": "state_persists",
                "delta": None,
                "evidence": [
                    _evidence(
                        8,
                        10,
                        "475fe7b6fbd3ec67f16c535eb23fb4630efa5a8958a041408682f88e9685d4f1",
                        "林舟",
                    ),
                    _evidence(
                        24,
                        26,
                        "475fe7b6fbd3ec67f16c535eb23fb4630efa5a8958a041408682f88e9685d4f1",
                        "林舟",
                    ),
                ],
            }
        ],
        "review_issues": [],
    }


def test_interaction_continuity_binds_actual_occurrences_and_state_timeline() -> None:
    candidate = InteractionContinuityCandidate.model_validate(_candidate())
    candidate.validate_for_input(_input())
    assert candidate.interactions[0].predicate == "hold"
    assert candidate.continuity[0].transition == "state_persists"


@pytest.mark.parametrize("drift", ["double_holder", "mentioned_only", "state_jump", "visual"])
def test_interaction_continuity_rejects_invalid_or_invented_state(drift: str) -> None:
    value = copy.deepcopy(_candidate())
    if drift == "double_holder":
        value["interactions"][0]["counterparty_occurrence_key"] = (
            "occurrence_scene_0002_0002"
        )
    elif drift == "mentioned_only":
        value["interactions"][0]["actor_occurrence_key"] = "occurrence_scene_0001_0001"
    elif drift == "state_jump":
        value["continuity"][0]["after_state_key"] = "state_location_exterior_initial"
    else:
        value["visual_preset"] = "cinematic"
    with pytest.raises((ValidationError, ValueError)):
        candidate = InteractionContinuityCandidate.model_validate(value)
        candidate.validate_for_input(_input())


def test_interaction_continuity_stage_has_semantic_contract_and_resource() -> None:
    spec = scene_analysis_stage_spec("reconcile_interaction_continuity", "default")
    assert spec.candidate_type == "continuity_fragment_candidate"
    assert spec.candidate_model is InteractionContinuityCandidate
    assert SceneAnalysisBundle().loaded_paths("reconcile_interaction_continuity", "default") == (
        "SKILL.md",
        "references/interaction-continuity.md",
    )


@pytest.mark.asyncio
async def test_interaction_continuity_runs_from_three_exact_upstreams(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fixture = json.loads(SCENE_FIXTURE.read_text(encoding="utf-8"))
    base = SceneAnalysisInvocation.model_validate(fixture["valid_invocation"])
    stage_input = _input()
    candidate = InteractionContinuityCandidate.model_validate(_candidate())
    release = base.stage_release.model_copy(
        update={"bundle_content_hash": SCENE_ANALYSIS_SKILL_BUNDLE_HASH}
    )
    refs: list[tuple[SceneAnalysisStageKey, UUID, str, str]] = [
        (
            "extract_scene_facts",
            stage_input.scene_fact_candidate_revision_id,
            stage_input.scene_fact_candidate_revision_hash,
            "a",
        ),
        (
            "derive_production_entities",
            stage_input.production_entity_candidate_revision_id,
            stage_input.production_entity_candidate_revision_hash,
            "b",
        ),
        (
            "bind_scene_occurrences",
            stage_input.scene_binding_candidate_revision_id,
            stage_input.scene_binding_candidate_revision_hash,
            "c",
        ),
    ]
    invocation = SceneAnalysisInvocation.build(
        invocation_id=base.invocation_id,
        attempt_id=base.attempt_id,
        stage_release=release,
        control=base.control,
        budget=base.budget,
        payload=SceneAnalysisPayload(
            variant=SceneAnalysisStageVariant(
                stage_key="reconcile_interaction_continuity",
                profile_key="default",
                lane_key="primary",
                output_schema_version="continuity-fragment-candidate-production",
            ),
            scope=base.payload.scope,
            source_refs=base.payload.source_refs,
            upstream_candidates=[
                SceneAnalysisCandidateRevisionIdentity(
                    stage_key=stage,
                    shard_key="script:full",
                    candidate_revision_id=revision_id,
                    candidate_revision_hash=revision_hash,
                    source_invocation_id=UUID(
                        f"{marker * 8}-{marker * 4}-4{marker * 3}"
                        f"-8{marker * 3}-{marker * 12}"
                    ),
                    source_result_hash=marker * 64,
                )
                for stage, revision_id, revision_hash, marker in refs
            ],
            shard=base.payload.shard,
            stage_input=stage_input.model_dump(mode="json"),
        ),
    )

    async def return_candidate(*_: object) -> InteractionContinuityCandidate:
        return candidate

    monkeypatch.setattr(SceneAnalysisHarness, "_run_codex", return_candidate)
    result = await SceneAnalysisHarness(
        invocation, repository_root=REPOSITORY_ROOT
    ).execute()
    assert result == candidate
