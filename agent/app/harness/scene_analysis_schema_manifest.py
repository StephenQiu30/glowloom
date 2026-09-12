from __future__ import annotations

from pydantic import BaseModel

from app.harness.reference_plan_schemas import (
    ReferencePlanAttemptResult,
    ReferencePlanInvocation,
)
from app.harness.scene_analysis_schemas import (
    IdentityResolutionInput,
    InteractionContinuityInput,
    ProductionEntityDerivationInput,
    SceneAnalysisAttemptResult,
    SceneAnalysisDispatchAuthorizationClaims,
    SceneAnalysisInvocation,
    SceneFactExtractionInput,
    SceneOccurrenceBindingInput,
    ScriptSpanProposalInput,
    StructureIdentityReviewInput,
)
from app.harness.visual_foundation_schemas import (
    VisualFoundationAttemptResult,
    VisualFoundationInvocation,
)
from app.modules.storygraph.reference_plan_contract import ReferencePlanInput
from app.modules.storygraph.visual_foundation_contract import VisualFoundationInput
from app.protocol.canonical import production_canonical_hash

WIRE_SCHEMA_ID = "storygraph-stage-wire-production"
WIRE_SCHEMA_SET_CONTRACT_ID = "storygraph-scene-analysis-wire-schema-set-production"
INPUT_SCHEMA_SET_CONTRACT_ID = "storygraph-scene-analysis-input-schema-set-production"

_WIRE_SCHEMAS: tuple[tuple[str, type[BaseModel]], ...] = (
    (
        "storygraph-dispatch-authorization-claims-production",
        SceneAnalysisDispatchAuthorizationClaims,
    ),
    (
        "storygraph-reference-plan-stage-attempt-result-production",
        ReferencePlanAttemptResult,
    ),
    (
        "storygraph-reference-plan-stage-invocation-production",
        ReferencePlanInvocation,
    ),
    ("storygraph-stage-attempt-result-production", SceneAnalysisAttemptResult),
    ("storygraph-stage-invocation-production", SceneAnalysisInvocation),
    (
        "storygraph-visual-foundation-stage-attempt-result-production",
        VisualFoundationAttemptResult,
    ),
    (
        "storygraph-visual-foundation-stage-invocation-production",
        VisualFoundationInvocation,
    ),
)

_INPUT_SCHEMAS: tuple[tuple[str, str, str, type[BaseModel]], ...] = (
    (
        "bind_scene_occurrences",
        "default",
        "scene-occurrence-binding-input-production",
        SceneOccurrenceBindingInput,
    ),
    (
        "derive_production_entities",
        "default",
        "production-entity-derivation-input-production",
        ProductionEntityDerivationInput,
    ),
    (
        "extract_scene_facts",
        "default",
        "scene-fact-extraction-input-production",
        SceneFactExtractionInput,
    ),
    (
        "plan_reference_assets",
        "default",
        "reference-plan-input-production",
        ReferencePlanInput,
    ),
    (
        "propose_script_spans",
        "default",
        "script-span-proposal-input-production",
        ScriptSpanProposalInput,
    ),
    (
        "reconcile_interaction_continuity",
        "default",
        "interaction-continuity-input-production",
        InteractionContinuityInput,
    ),
    (
        "resolve_identities",
        "default",
        "identity-resolution-input-production",
        IdentityResolutionInput,
    ),
    (
        "resolve_visual_foundation",
        "default",
        "visual-foundation-input-production",
        VisualFoundationInput,
    ),
    (
        "review_candidate",
        "structure_identity",
        "structure-identity-review-input-production",
        StructureIdentityReviewInput,
    ),
)


def scene_analysis_wire_schema_manifest() -> dict[str, object]:
    schemas = [
        {
            "contract_id": contract_id,
            "schema_hash": production_canonical_hash(model.model_json_schema()),
        }
        for contract_id, model in _WIRE_SCHEMAS
    ]
    material: dict[str, object] = {
        "contract_id": WIRE_SCHEMA_SET_CONTRACT_ID,
        "wire_schema_id": WIRE_SCHEMA_ID,
        "schemas": schemas,
    }
    return {**material, "wire_schema_hash": production_canonical_hash(material)}


def scene_analysis_input_schema_manifest() -> dict[str, object]:
    schemas = [
        {
            "stage_key": stage_key,
            "profile_key": profile_key,
            "input_contract_id": contract_id,
            "schema_hash": production_canonical_hash(model.model_json_schema()),
        }
        for stage_key, profile_key, contract_id, model in _INPUT_SCHEMAS
    ]
    material: dict[str, object] = {
        "contract_id": INPUT_SCHEMA_SET_CONTRACT_ID,
        "schemas": schemas,
    }
    return {**material, "schema_set_hash": production_canonical_hash(material)}
