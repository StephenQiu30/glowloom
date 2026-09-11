from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

from pydantic import BaseModel

from app.modules.storygraph.scene_analysis_candidates import (
    IdentityResolutionCandidate,
    InteractionContinuityCandidate,
    ProductionEntityFragmentCandidate,
    SceneBindingFragmentCandidate,
    SceneFactCandidate,
    ScriptSpanCandidate,
    StructureIdentityReviewCandidate,
)
from app.modules.storygraph.visual_foundation_contract import VisualFoundationCandidate
from app.protocol.canonical import production_canonical_hash

SceneAnalysisCandidateType = Literal[
    "script_span_candidate",
    "scene_fact_candidate",
    "identity_resolution_candidate",
    "structure_identity_review_candidate",
    "production_entity_fragment_candidate",
    "scene_binding_fragment_candidate",
    "continuity_fragment_candidate",
    "visual_foundation_candidate",
]


@dataclass(frozen=True)
class SceneAnalysisStageSpec:
    candidate_type: SceneAnalysisCandidateType
    candidate_model: type[BaseModel]
    output_schema_version: str
    references: tuple[str, ...]


SCENE_ANALYSIS_REGISTRY: dict[tuple[str, str], SceneAnalysisStageSpec] = {
    ("propose_script_spans", "default"): SceneAnalysisStageSpec(
        candidate_type="script_span_candidate",
        candidate_model=ScriptSpanCandidate,
        output_schema_version="script-span-candidate-production",
        references=("script-spans.md",),
    ),
    ("extract_scene_facts", "default"): SceneAnalysisStageSpec(
        candidate_type="scene_fact_candidate",
        candidate_model=SceneFactCandidate,
        output_schema_version="scene-fact-candidate-production",
        references=("scene-facts.md",),
    ),
    ("resolve_identities", "default"): SceneAnalysisStageSpec(
        candidate_type="identity_resolution_candidate",
        candidate_model=IdentityResolutionCandidate,
        output_schema_version="identity-resolution-candidate-production",
        references=("entity-reconciliation.md",),
    ),
    ("resolve_visual_foundation", "default"): SceneAnalysisStageSpec(
        candidate_type="visual_foundation_candidate",
        candidate_model=VisualFoundationCandidate,
        output_schema_version="visual-foundation-candidate-production",
        references=("visual-identity.md",),
    ),
    ("review_candidate", "structure_identity"): SceneAnalysisStageSpec(
        candidate_type="structure_identity_review_candidate",
        candidate_model=StructureIdentityReviewCandidate,
        output_schema_version="structure-identity-review-candidate-production",
        references=("structure-identity-review.md",),
    ),
    ("derive_production_entities", "default"): SceneAnalysisStageSpec(
        candidate_type="production_entity_fragment_candidate",
        candidate_model=ProductionEntityFragmentCandidate,
        output_schema_version="production-entity-fragment-candidate-production",
        references=("production-entities.md",),
    ),
    ("bind_scene_occurrences", "default"): SceneAnalysisStageSpec(
        candidate_type="scene_binding_fragment_candidate",
        candidate_model=SceneBindingFragmentCandidate,
        output_schema_version="scene-binding-fragment-candidate-production",
        references=("scene-occurrences.md",),
    ),
    ("reconcile_interaction_continuity", "default"): SceneAnalysisStageSpec(
        candidate_type="continuity_fragment_candidate",
        candidate_model=InteractionContinuityCandidate,
        output_schema_version="continuity-fragment-candidate-production",
        references=("interaction-continuity.md",),
    ),
}


def scene_analysis_stage_spec(stage: str, profile: str) -> SceneAnalysisStageSpec:
    try:
        return SCENE_ANALYSIS_REGISTRY[(stage, profile)]
    except KeyError as error:
        raise ValueError("unknown Scene Analysis stage variant") from error


def scene_analysis_candidate_schema_manifest() -> dict[str, object]:
    schemas: list[dict[str, str]] = []
    for (stage_key, profile_key), spec in sorted(SCENE_ANALYSIS_REGISTRY.items()):
        schemas.append(
            {
                "stage_key": stage_key,
                "profile_key": profile_key,
                "candidate_type": spec.candidate_type,
                "output_schema_version": spec.output_schema_version,
                "schema_hash": production_canonical_hash(spec.candidate_model.model_json_schema()),
            }
        )
    material: dict[str, object] = {
        "contract_id": "storygraph-scene-analysis-candidate-schema-set-production",
        "schemas": schemas,
    }
    return {**material, "schema_set_hash": production_canonical_hash(material)}
