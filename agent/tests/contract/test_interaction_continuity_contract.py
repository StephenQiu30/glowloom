from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any, cast
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
from app.modules.storygraph.scene_analysis_candidates import (
    InteractionContinuityCandidate,
    ProductionEntityFragmentCandidate,
)
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec
from tests.contract.interaction_continuity_journey_fixture import (
    build_interaction_journey,
)
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
        "scene_story_times": [
            {
                "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "story_time_key": "storytime:00000001",
            },
            {
                "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
                "story_time_key": "storytime:00000002",
            },
        ],
        "interactions": [
            {
                "interaction_key": "interaction_scene_0001_0001",
                "claim_series_key": "interaction_series_door_handle_hold",
                "claim_revision": 1,
                "supersedes_interaction_key": None,
                "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "beat_key": "beat_scene_0001_0001",
                "story_time_key": "storytime:00000001",
                "predicate": "hold",
                "actor_occurrence_key": "occurrence_scene_0001_0002",
                "prop_occurrence_key": "occurrence_scene_0001_0003",
                "counterparty_occurrence_key": None,
                "holder_before_identity_key": None,
                "holder_after_identity_key": "character:linzhou",
                "prop_state_before_key": "state_prop_door_handle_initial",
                "prop_state_after_key": "state_prop_door_handle_initial",
                "state_delta": None,
                "hand": "unspecified",
                "grip_type": None,
                "contact_point": None,
                "direction": None,
                "relative_scale": None,
                "geometry_evidence": {
                    "hand": None,
                    "grip_type": None,
                    "contact_point": None,
                    "direction": None,
                    "relative_scale": None,
                },
                "evidence": _evidence(
                    8,
                    15,
                    "f2ffbd1d3cb0f5e0d95b211b12a0bdee451dd82d38bf673915e42420f7ed9ff1",
                    "林舟握住门把。",
                ),
            }
        ],
        "continuity_ledger": [
            {
                "ledger_key": "ledger_character_linzhou_scene_0001",
                "subject_kind": "character",
                "identity_key": "character:linzhou",
                "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "story_time_key": "storytime:00000001",
                "state_key": "state_character_linzhou_initial",
                "holder_identity_key": None,
                "location_identity_key": "location:interior",
                "transition_interaction_key": None,
                "evidence": [
                    _evidence(
                        6,
                        7,
                        "b2126ce9c100bff0cc963326e29fadc751106ad7fd7f34aa46cddc06d8198c6a",
                        "内",
                    ),
                    _evidence(
                        8,
                        10,
                        "475fe7b6fbd3ec67f16c535eb23fb4630efa5a8958a041408682f88e9685d4f1",
                        "林舟",
                    ),
                ],
            },
            {
                "ledger_key": "ledger_prop_door_handle_scene_0001",
                "subject_kind": "prop",
                "identity_key": "prop:door_handle",
                "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "story_time_key": "storytime:00000001",
                "state_key": "state_prop_door_handle_initial",
                "holder_identity_key": "character:linzhou",
                "location_identity_key": "location:interior",
                "transition_interaction_key": "interaction_scene_0001_0001",
                "evidence": [
                    _evidence(
                        6,
                        7,
                        "b2126ce9c100bff0cc963326e29fadc751106ad7fd7f34aa46cddc06d8198c6a",
                        "内",
                    ),
                    _evidence(
                        12,
                        14,
                        "3ff7bc50c9533059a629804286be3602fd8805c9454949ba69ce9b1e8c0b96d9",
                        "门把",
                    ),
                ],
            },
            {
                "ledger_key": "ledger_character_linzhou_scene_0002",
                "subject_kind": "character",
                "identity_key": "character:linzhou",
                "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
                "story_time_key": "storytime:00000002",
                "state_key": "state_character_linzhou_initial",
                "holder_identity_key": None,
                "location_identity_key": "location:exterior",
                "transition_interaction_key": None,
                "evidence": [
                    _evidence(
                        22,
                        23,
                        "0e826095b60ccf039478a7e093220d79a4614158557924383bd3c7ef1bb06d8a",
                        "外",
                    ),
                    _evidence(
                        24,
                        26,
                        "475fe7b6fbd3ec67f16c535eb23fb4630efa5a8958a041408682f88e9685d4f1",
                        "林舟",
                    ),
                ],
            },
        ],
        "continuity": [
            {
                "continuity_key": "continuity_character_linzhou_0001",
                "claim_series_key": "continuity_series_character_linzhou",
                "claim_revision": 1,
                "supersedes_continuity_key": None,
                "subject_kind": "character",
                "identity_key": "character:linzhou",
                "from_scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
                "to_scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
                "story_time_start": "storytime:00000001",
                "story_time_end": "storytime:00000002",
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


def test_interaction_continuity_covers_transfer_geometry_and_cross_scene_ledger() -> None:
    stage_input, value = build_interaction_journey()

    candidate = InteractionContinuityCandidate.model_validate(value)
    candidate.validate_for_input(stage_input)
    production = ProductionEntityFragmentCandidate.model_validate(
        stage_input.production_entity_candidate
    )
    states_by_identity = {
        entity.identity_key: [state.state_key for state in entity.states]
        for entity in production.entities
    }

    assert [item.predicate for item in candidate.interactions] == ["hold", "give", "carry"]
    assert len(candidate.continuity_ledger) == 7
    assert len(states_by_identity["character:linzhou"]) == 2
    assert len(states_by_identity["prop:box"]) == 2


def test_interaction_continuity_accepts_receive_as_the_single_transfer_view() -> None:
    stage_input, value = build_interaction_journey()
    transfer = value["interactions"][1]
    transfer["predicate"] = "receive"
    transfer["actor_occurrence_key"], transfer["counterparty_occurrence_key"] = (
        transfer["counterparty_occurrence_key"],
        transfer["actor_occurrence_key"],
    )

    candidate = InteractionContinuityCandidate.model_validate(value)
    candidate.validate_for_input(stage_input)


def test_interaction_continuity_rejects_transfer_double_application() -> None:
    stage_input, value = build_interaction_journey()
    duplicate = copy.deepcopy(value["interactions"][1])
    duplicate["interaction_key"] = "interaction_box_transfer_receive"
    duplicate["claim_series_key"] = "interaction_series_box_transfer_receive"
    duplicate["predicate"] = "receive"
    duplicate["actor_occurrence_key"], duplicate["counterparty_occurrence_key"] = (
        duplicate["counterparty_occurrence_key"],
        duplicate["actor_occurrence_key"],
    )
    value["interactions"].insert(2, duplicate)

    with pytest.raises((ValidationError, ValueError)):
        candidate = InteractionContinuityCandidate.model_validate(value)
        candidate.validate_for_input(stage_input)


def test_interaction_continuity_rejects_prop_teleport_without_carry_transition() -> None:
    stage_input, value = build_interaction_journey()
    value["interactions"].pop()
    value["continuity_ledger"][-1]["transition_interaction_key"] = None

    with pytest.raises((ValidationError, ValueError)):
        candidate = InteractionContinuityCandidate.model_validate(value)
        candidate.validate_for_input(stage_input)


def test_interaction_continuity_rejects_unproved_geometry() -> None:
    stage_input, value = build_interaction_journey()
    value["interactions"][0]["geometry_evidence"]["hand"] = None

    with pytest.raises((ValidationError, ValueError)):
        candidate = InteractionContinuityCandidate.model_validate(value)
        candidate.validate_for_input(stage_input)


@pytest.mark.parametrize(
    ("predicate", "holder_before", "holder_after"),
    [
        ("carry", None, "character:linzhou"),
        ("use", None, "character:linzhou"),
        ("place", None, None),
        ("drop", None, None),
    ],
)
def test_interaction_continuity_rejects_invalid_predicate_state_machine(
    predicate: str,
    holder_before: str | None,
    holder_after: str | None,
) -> None:
    value = _candidate()
    interaction = value["interactions"][0]
    interaction["predicate"] = predicate
    interaction["holder_before_identity_key"] = holder_before
    interaction["holder_after_identity_key"] = holder_after
    with pytest.raises((ValidationError, ValueError)):
        candidate = InteractionContinuityCandidate.model_validate(value)
        candidate.validate_for_input(_input())


def test_interaction_continuity_uses_explicit_story_time_not_scene_array_order() -> None:
    value = _candidate()
    value["scene_story_times"] = [
        {
            "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
            "story_time_key": "storytime:00000001",
        },
        {
            "scene_scope_key": "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
            "story_time_key": "storytime:00000002",
        },
    ]
    value["interactions"][0]["story_time_key"] = "storytime:00000002"
    ledger_entries = cast(list[dict[str, Any]], value["continuity_ledger"])
    for entry in ledger_entries:
        entry["story_time_key"] = (
            "storytime:00000001"
            if entry["scene_scope_key"] == "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2"
            else "storytime:00000002"
        )
    ledger_entries.sort(
        key=lambda item: (item["story_time_key"], item["subject_kind"], item["identity_key"])
    )
    continuity = value["continuity"][0]
    continuity["from_scene_scope_key"] = "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2"
    continuity["to_scene_scope_key"] = "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
    continuity["story_time_start"] = "storytime:00000001"
    continuity["story_time_end"] = "storytime:00000002"
    candidate = InteractionContinuityCandidate.model_validate(value)
    candidate.validate_for_input(_input())


def test_interaction_continuity_rejects_duplicate_prop_transition_at_same_story_time() -> None:
    value = _candidate()
    duplicate = copy.deepcopy(value["interactions"][0])
    duplicate["interaction_key"] = "interaction_scene_0001_0002"
    duplicate["claim_series_key"] = "interaction_series_door_handle_hold_duplicate"
    value["interactions"].append(duplicate)
    with pytest.raises((ValidationError, ValueError)):
        candidate = InteractionContinuityCandidate.model_validate(value)
        candidate.validate_for_input(_input())


@pytest.mark.parametrize(
    "drift",
    ["double_holder", "mentioned_only", "state_jump", "claim_range", "scale", "visual"],
)
def test_interaction_continuity_rejects_invalid_or_invented_state(drift: str) -> None:
    value = copy.deepcopy(_candidate())
    if drift == "double_holder":
        value["interactions"][0]["counterparty_occurrence_key"] = "occurrence_scene_0002_0002"
    elif drift == "mentioned_only":
        value["interactions"][0]["actor_occurrence_key"] = "occurrence_scene_0001_0001"
    elif drift == "state_jump":
        value["continuity"][0]["after_state_key"] = "state_location_exterior_initial"
    elif drift == "claim_range":
        value["interactions"][0]["claim_revision"] = 9_007_199_254_740_992
    elif drift == "scale":
        value["interactions"][0]["relative_scale"] = {"numerator": 2, "denominator": 4}
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
                        f"{marker * 8}-{marker * 4}-4{marker * 3}-8{marker * 3}-{marker * 12}"
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
    result = await SceneAnalysisHarness(invocation, repository_root=REPOSITORY_ROOT).execute()
    assert result == candidate
