from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any, cast
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.harness.scene_analysis_schemas import (
    IdentityResolutionInput,
    SceneAnalysisCandidateRevisionIdentity,
    SceneAnalysisInvocation,
    SceneAnalysisPayload,
    SceneAnalysisStageVariant,
)
from app.modules.storygraph.scene_analysis_bundle import SceneAnalysisBundle
from app.modules.storygraph.scene_analysis_candidates import (
    IdentityResolutionCandidate,
    SceneFactCandidate,
)
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
SCENE_ANALYSIS_FIXTURE = (
    REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-scene-analysis-wire.json"
)
IDENTITY_FIXTURE = (
    REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-identity-resolution.json"
)


def _fixtures() -> tuple[dict[str, Any], dict[str, Any]]:
    scene_analysis = json.loads(SCENE_ANALYSIS_FIXTURE.read_text(encoding="utf-8"))
    identity = json.loads(IDENTITY_FIXTURE.read_text(encoding="utf-8"))
    return scene_analysis, identity


def test_identity_resolution_candidate_partitions_every_raw_mention_exactly_once() -> None:
    scene_analysis, identity = _fixtures()
    scene_facts = SceneFactCandidate.model_validate(scene_analysis["valid_scene_fact_candidate"])
    candidate = IdentityResolutionCandidate.model_validate(identity["valid_candidate"])

    candidate.validate_for_scene_facts(scene_facts, allowed_reuse_identity_keys=set())
    assert {cluster.kind for cluster in candidate.resolved_clusters} == {
        "character",
        "location",
        "prop",
    }


@pytest.mark.parametrize(
    "mutation",
    ["duplicate_mention", "missing_mention", "kind_drift", "agent_uuid"],
)
def test_identity_resolution_candidate_rejects_invalid_partitions(mutation: str) -> None:
    scene_analysis, identity = _fixtures()
    scene_facts = SceneFactCandidate.model_validate(scene_analysis["valid_scene_fact_candidate"])
    raw = cast(dict[str, Any], copy.deepcopy(identity["valid_candidate"]))
    clusters = cast(list[dict[str, Any]], raw["resolved_clusters"])

    if mutation == "duplicate_mention":
        clusters[1]["mention_refs"].append(copy.deepcopy(clusters[0]["mention_refs"][0]))
    elif mutation == "missing_mention":
        clusters[0]["mention_refs"].pop()
    elif mutation == "kind_drift":
        clusters[0]["mention_refs"][0]["kind"] = "prop"
    else:
        clusters[0]["temporary_identity_key"] = "44444444-4444-4444-8444-444444444444"

    with pytest.raises(ValueError):
        candidate = IdentityResolutionCandidate.model_validate(raw)
        candidate.validate_for_scene_facts(scene_facts, allowed_reuse_identity_keys=set())


def test_identity_resolution_candidate_is_strict_and_reuse_is_allowlisted() -> None:
    _, identity = _fixtures()
    raw = cast(dict[str, Any], copy.deepcopy(identity["valid_candidate"]))
    raw["visual_style"] = "赛博朋克"
    with pytest.raises(ValidationError):
        IdentityResolutionCandidate.model_validate(raw)

    raw = cast(dict[str, Any], copy.deepcopy(identity["valid_candidate"]))
    raw["resolved_clusters"][0]["resolution"] = "reuse"
    raw["resolved_clusters"][0]["reuse_identity_key"] = "character:existing:linzhou"
    candidate = IdentityResolutionCandidate.model_validate(raw)
    scene_analysis = json.loads(SCENE_ANALYSIS_FIXTURE.read_text(encoding="utf-8"))
    scene_facts = SceneFactCandidate.model_validate(scene_analysis["valid_scene_fact_candidate"])

    with pytest.raises(ValueError, match="allowlist"):
        candidate.validate_for_scene_facts(scene_facts, allowed_reuse_identity_keys=set())
    candidate.validate_for_scene_facts(
        scene_facts, allowed_reuse_identity_keys={"character:existing:linzhou"}
    )


def test_identity_resolution_stage_has_an_exact_variant_schema_and_resource() -> None:
    variant = SceneAnalysisStageVariant(
        stage_key="resolve_identities",
        profile_key="default",
        lane_key="primary",
        output_schema_version="identity-resolution-candidate-production",
    )
    spec = scene_analysis_stage_spec(variant.stage_key, variant.profile_key)

    assert spec.candidate_type == "identity_resolution_candidate"
    assert spec.candidate_model is IdentityResolutionCandidate
    assert SceneAnalysisBundle().loaded_paths(variant.stage_key, variant.profile_key) == (
        "SKILL.md",
        "references/entity-reconciliation.md",
    )


@pytest.mark.asyncio
async def test_identity_resolution_stage_runs_from_the_frozen_scene_fact_candidate(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    scene_analysis, identity = _fixtures()
    base = SceneAnalysisInvocation.model_validate(scene_analysis["valid_invocation"])
    scene_facts = SceneFactCandidate.model_validate(scene_analysis["valid_scene_fact_candidate"])
    candidate = IdentityResolutionCandidate.model_validate(identity["valid_candidate"])
    source_input = base.payload.stage_input
    invocation = SceneAnalysisInvocation.build(
        invocation_id=base.invocation_id,
        attempt_id=base.attempt_id,
        stage_release=base.stage_release,
        control=base.control,
        budget=base.budget,
        payload=SceneAnalysisPayload(
            variant=SceneAnalysisStageVariant(
                stage_key="resolve_identities",
                profile_key="default",
                lane_key="primary",
                output_schema_version="identity-resolution-candidate-production",
            ),
            scope=base.payload.scope,
            source_refs=base.payload.source_refs,
            upstream_candidates=[
                SceneAnalysisCandidateRevisionIdentity(
                    stage_key="extract_scene_facts",
                    shard_key="script:full",
                    candidate_revision_id=candidate.scene_fact_candidate_revision_id,
                    candidate_revision_hash=candidate.scene_fact_candidate_revision_hash,
                    source_invocation_id=UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
                    source_result_hash="b" * 64,
                )
            ],
            shard=base.payload.shard,
            stage_input=IdentityResolutionInput(
                source_version_id=candidate.source_version_id,
                source_hash=candidate.source_hash,
                normalized_text=source_input["normalized_text"],
                scene_fact_candidate_revision_id=candidate.scene_fact_candidate_revision_id,
                scene_fact_candidate_revision_hash=candidate.scene_fact_candidate_revision_hash,
                scene_fact_candidate=scene_facts.model_dump(mode="json"),
                allowed_reuse_identity_keys=[],
            ).model_dump(mode="json"),
        ),
    )

    async def return_candidate(*_: object) -> IdentityResolutionCandidate:
        return candidate

    monkeypatch.setattr(SceneAnalysisHarness, "_run_codex", return_candidate)
    result = await SceneAnalysisHarness(invocation, repository_root=REPOSITORY_ROOT).execute()

    assert result == candidate
    assert invocation.input_hash == identity["expected_input_hash"]
    assert invocation.stage_instance_key() == identity["expected_stage_instance_key"]
