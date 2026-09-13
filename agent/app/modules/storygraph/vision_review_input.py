from __future__ import annotations

from typing import Literal

from pydantic import Field, model_validator

from app.modules.storygraph.reference_brief_contract import (
    ReferenceBriefCandidate,
    ReferenceBriefInput,
)
from app.modules.storygraph.reference_plan_contract import ReferencePlanPurposeProfile
from app.modules.storygraph.scene_analysis_candidates import StrictSceneAnalysisModel
from app.modules.storygraph.vision_review_attachments import (
    VisionReviewMediaAttachment,
    validate_vision_review_attachments,
)
from app.modules.storygraph.vision_review_contract import Digest, VisionReviewSubject
from app.modules.storygraph.visual_foundation_contract import (
    FIDELITY_INVARIANTS,
    CreativeFillProposal,
    VisualFoundationPolicy,
    VisualWorldConflict,
    WorldAdaptationProposal,
)
from app.protocol.canonical import production_canonical_hash, production_canonical_json_text


class VisionReviewVisualGrammar(StrictSceneAnalysisModel):
    medium: str
    realism: str
    shape_language: str
    proportion: str
    palette: str
    linework: str
    texture: str
    material_rendering: str
    lighting: str
    contrast: str
    composition: str
    camera: str
    negative_constraints: list[str]

    @model_validator(mode="after")
    def validate_grammar(self) -> VisionReviewVisualGrammar:
        for value in (
            self.medium,
            self.realism,
            self.shape_language,
            self.proportion,
            self.palette,
            self.linework,
            self.texture,
            self.material_rendering,
            self.lighting,
            self.contrast,
            self.composition,
            self.camera,
        ):
            _validate_text(value)
        _validate_rules(self.negative_constraints)
        return self


class VisionReviewPolicyRef(StrictSceneAnalysisModel):
    owner: str
    key: str
    content_hash: Digest

    @model_validator(mode="after")
    def validate_reference(self) -> VisionReviewPolicyRef:
        _validate_text(self.owner)
        _validate_text(self.key)
        return self


class VisionReviewVisualContext(StrictSceneAnalysisModel):
    application_mode: Literal["faithful", "world_adaptation"]
    production_world_owner_set_hash: Digest
    visual_grammar: VisionReviewVisualGrammar
    style_policy: VisualFoundationPolicy
    fidelity_invariants: list[str]
    world_adaptations: list[WorldAdaptationProposal]
    world_conflicts: list[VisualWorldConflict]
    creative_fill_proposals: list[CreativeFillProposal]
    purpose_profile: ReferencePlanPurposeProfile
    qc_policy_ref: VisionReviewPolicyRef

    @model_validator(mode="after")
    def validate_context(self) -> VisionReviewVisualContext:
        if self.fidelity_invariants != FIDELITY_INVARIANTS:
            raise ValueError("Vision Review cannot relax fidelity invariants")
        if self.application_mode == "faithful" and self.world_adaptations:
            raise ValueError("faithful Vision Review cannot adapt world facts")
        _validate_rules(self.purpose_profile.design_focus)
        _validate_rules(self.purpose_profile.forbidden_changes)
        for keys in (
            [value.mapping_key for value in self.world_adaptations],
            [value.conflict_key for value in self.world_conflicts],
            [value.gap_key for value in self.creative_fill_proposals],
        ):
            if keys != sorted(set(keys)):
                raise ValueError("Vision Review policy proposals must be ordered and unique")
        for rules in (
            self.style_policy.palette_rules,
            self.style_policy.material_rules,
            self.style_policy.lighting_rules,
            self.style_policy.camera_rules,
        ):
            for rule in rules:
                _validate_text(rule)
                if len(rule.encode("utf-8")) > 800:
                    raise ValueError("Vision Review style policy rule exceeds its byte budget")
        for value in self.world_adaptations:
            if len(value.design_value.encode("utf-8")) > 1200:
                raise ValueError("Vision Review adaptation exceeds its byte budget")
        for value in self.world_conflicts:
            if len(value.summary.encode("utf-8")) > 800:
                raise ValueError("Vision Review conflict exceeds its byte budget")
        for value in self.creative_fill_proposals:
            if len(value.proposal.encode("utf-8")) > 1200:
                raise ValueError("Vision Review proposal exceeds its byte budget")
        return self


class VisionReviewAdmissionPolicyRef(StrictSceneAnalysisModel):
    contract_id: Literal["reference-bundle-purpose-admission"]
    content_hash: Digest


class VisionReviewAdmission(StrictSceneAnalysisModel):
    policy_ref: VisionReviewAdmissionPolicyRef
    internal_review_ready: bool = Field(strict=True)
    selection_ready: bool = Field(strict=True)
    publication_ready: bool = Field(strict=True)
    internal_review_blockers: list[str]
    formal_use_blockers: list[str]

    @model_validator(mode="after")
    def validate_material_admission(self) -> VisionReviewAdmission:
        policy_hash = production_canonical_hash(
            {
                "formal_use_requires": ["rights_assessment", "vision_review"],
                "internal_review_requires": ["complete_bundle", "technical_qc"],
                "rights_not_assessed": "internal_review_only",
            }
        )
        if (
            self.policy_ref.content_hash != policy_hash
            or not self.internal_review_ready
            or self.selection_ready
            or self.publication_ready
            or self.internal_review_blockers
            or self.formal_use_blockers != ["rights_not_assessed", "vision_review_required"]
        ):
            raise ValueError("Vision Review material admission is invalid")
        return self


class VisionReviewInput(StrictSceneAnalysisModel):
    """Closed base-wave input; never a substitute for current Owner authorization."""

    contract_id: Literal["vision-review-input-production"]
    subject: VisionReviewSubject
    brief_input: ReferenceBriefInput
    brief_candidate: ReferenceBriefCandidate
    brief_content_hash: Digest
    visual_context: VisionReviewVisualContext
    admission: VisionReviewAdmission
    attachments: list[VisionReviewMediaAttachment] = Field(min_length=1, max_length=4)

    @model_validator(mode="after")
    def validate_input(self) -> VisionReviewInput:
        self.brief_candidate.validate_for(self.brief_input)
        validate_vision_review_attachments(self.subject, self.attachments)
        if (
            self.subject.target_kind
            not in {"character_identity_anchor", "location_board", "prop_sheet"}
            or self.brief_input.dependency_selections
        ):
            raise ValueError("Vision Review dependency assets are not ready")
        if (
            self.subject.workspace_id != str(self.brief_input.workspace_id)
            or self.subject.project_id != str(self.brief_input.project_id)
            or self.subject.target_kind != self.brief_input.target_kind
            or self.subject.generation_round != 1
            or self.visual_context.purpose_profile.target_kind != self.subject.target_kind
        ):
            raise ValueError("Vision Review Brief or visual context scope has drifted")
        if (
            production_canonical_hash(self.brief_candidate.model_dump(mode="json"))
            != self.brief_content_hash
        ):
            raise ValueError("Vision Review Brief content has drifted")
        material = self.model_dump(mode="json")
        del material["subject"]["input_hash"]
        if production_canonical_hash(material) != self.subject.input_hash:
            raise ValueError("Vision Review input hash has drifted")
        return self


def decode_vision_review_input(raw: str | bytes) -> VisionReviewInput:
    wire = production_canonical_json_text(raw.encode("utf-8") if isinstance(raw, str) else raw)
    value = VisionReviewInput.model_validate_json(wire)
    typed = production_canonical_json_text(value.model_dump_json().encode("utf-8"))
    if typed != wire:
        raise ValueError("Vision Review input wire shape is incomplete")
    return value


def _validate_text(value: str) -> None:
    if not value or value.strip() != value:
        raise ValueError("Vision Review requires canonical visual text")


def _validate_rules(values: list[str]) -> None:
    if not values or values != sorted(set(values)):
        raise ValueError("Vision Review visual rules must be explicit and ordered")
    for value in values:
        _validate_text(value)
