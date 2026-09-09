from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.modules.storygraph.scene_analysis_candidates import (
    IdentityResolutionCandidate,
    SceneFactCandidate,
)

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
