from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

from pydantic import BaseModel

from app.modules.storygraph.scene_analysis_candidates import (
    IdentityResolutionCandidate,
    ProductionEntityFragmentCandidate,
    SceneFactCandidate,
    ScriptSpanCandidate,
    StructureIdentityReviewCandidate,
)

SceneAnalysisCandidateType = Literal[
    "script_span_candidate",
    "scene_fact_candidate",
    "identity_resolution_candidate",
    "structure_identity_review_candidate",
    "production_entity_fragment_candidate",
]


@dataclass(frozen=True)
class SceneAnalysisStageSpec:
    candidate_type: SceneAnalysisCandidateType
    candidate_model: type[BaseModel]
    references: tuple[str, ...]


SCENE_ANALYSIS_REGISTRY: dict[tuple[str, str], SceneAnalysisStageSpec] = {
    ("propose_script_spans", "default"): SceneAnalysisStageSpec(
        candidate_type="script_span_candidate",
        candidate_model=ScriptSpanCandidate,
        references=("script-spans.md",),
    ),
    ("extract_scene_facts", "default"): SceneAnalysisStageSpec(
        candidate_type="scene_fact_candidate",
        candidate_model=SceneFactCandidate,
        references=("scene-facts.md",),
    ),
    ("resolve_identities", "default"): SceneAnalysisStageSpec(
        candidate_type="identity_resolution_candidate",
        candidate_model=IdentityResolutionCandidate,
        references=("entity-reconciliation.md",),
    ),
    ("review_candidate", "structure_identity"): SceneAnalysisStageSpec(
        candidate_type="structure_identity_review_candidate",
        candidate_model=StructureIdentityReviewCandidate,
        references=("structure-identity-review.md",),
    ),
    ("derive_production_entities", "default"): SceneAnalysisStageSpec(
        candidate_type="production_entity_fragment_candidate",
        candidate_model=ProductionEntityFragmentCandidate,
        references=("production-entities.md",),
    ),
}


def scene_analysis_stage_spec(stage: str, profile: str) -> SceneAnalysisStageSpec:
    try:
        return SCENE_ANALYSIS_REGISTRY[(stage, profile)]
    except KeyError as error:
        raise ValueError("unknown Scene Analysis stage variant") from error
