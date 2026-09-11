from __future__ import annotations

import re
from collections.abc import Sequence
from typing import Literal
from uuid import UUID

from pydantic import Field, model_validator

from app.modules.storygraph.scene_analysis_candidates import StrictSceneAnalysisModel
from app.protocol.canonical import production_canonical_hash

FIDELITY_INVARIANTS = [
    "character_identity",
    "holder_relation",
    "scene_continuity",
    "story_fact",
]

DesignDomain = Literal[
    "architecture",
    "civilization",
    "material",
    "prop",
    "technology_or_magic",
    "wardrobe",
]


class ConfirmedWorldRoot(StrictSceneAnalysisModel):
    owner_family: Literal[
        "asset_identity_state_set",
        "bible_production_world_set",
        "planning_scene_set",
    ]
    scope_key: str = Field(pattern=r"^(project|episode):[0-9a-f-]{36}$")
    collection_root_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class PresetReleaseIdentity(StrictSceneAnalysisModel):
    key: str = Field(pattern=r"^[a-z][a-z0-9-]{2,63}$")
    release: str = Field(pattern=r"^[0-9]{4}\.(0[1-9]|1[0-2])\.(0[1-9]|[12][0-9]|3[01])$")
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class PresetAdaptationRuleSnapshot(StrictSceneAnalysisModel):
    rule_key: str = Field(pattern=r"^[a-z][a-z0-9-]{2,63}$")
    source_fact_kind: DesignDomain
    design_domain: DesignDomain
    directive: str = Field(min_length=1, max_length=800)
    preserved_invariant_keys: list[str]
    impact_scope_kinds: list[
        Literal["asset", "interaction", "reference_plan", "scene", "storyboard"]
    ]

    @model_validator(mode="after")
    def validate_rule(self) -> PresetAdaptationRuleSnapshot:
        if self.preserved_invariant_keys != FIDELITY_INVARIANTS:
            raise ValueError("Preset adaptation rule must preserve every fidelity invariant")
        if not _sorted_unique(self.impact_scope_kinds):
            raise ValueError("Preset adaptation impact kinds must be non-empty, sorted, and unique")
        if self.directive != self.directive.strip():
            raise ValueError("Preset adaptation directive must be canonical text")
        return self


class VisualGrammarSnapshot(StrictSceneAnalysisModel):
    palette: str = Field(min_length=1, max_length=800)
    material_rendering: str = Field(min_length=1, max_length=800)
    lighting: str = Field(min_length=1, max_length=800)
    camera: str = Field(min_length=1, max_length=800)
    negative_constraints: list[str] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_grammar(self) -> VisualGrammarSnapshot:
        if not _sorted_unique(self.negative_constraints):
            raise ValueError("visual grammar constraints must be sorted and unique")
        return self


class TypedVisualOverride(StrictSceneAnalysisModel):
    override_key: str = Field(pattern=r"^override_[a-z0-9_]{1,120}$")
    scope_kind: Literal["project", "asset"]
    scope_key: str = Field(pattern=r"^(project|asset):[0-9a-f-]{36}$")
    design_domain: DesignDomain
    value: str = Field(min_length=1, max_length=1200)
    creator_decision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def validate_scope(self) -> TypedVisualOverride:
        if not self.scope_key.startswith(self.scope_kind + ":") or self.value != self.value.strip():
            raise ValueError("typed visual override scope or value is invalid")
        return self


class VisualReferenceAttachment(StrictSceneAnalysisModel):
    attachment_id: UUID
    object_key: str = Field(min_length=1, max_length=512)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    media_type: Literal["image/jpeg", "image/png", "image/webp"]
    rights_basis: Literal["licensed", "owned", "public_domain"]
    rights_ref_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def validate_object_key(self) -> VisualReferenceAttachment:
        if (
            self.object_key.startswith("/")
            or "\\" in self.object_key
            or any(part in {"", ".", ".."} for part in self.object_key.split("/"))
        ):
            raise ValueError("visual reference attachment object key is unsafe")
        return self


class ConfirmedWorldFact(StrictSceneAnalysisModel):
    fact_key: str = Field(pattern=r"^fact_[a-z0-9_]{1,120}$")
    fact_kind: DesignDomain
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    affected_scope_keys: list[str] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_scopes(self) -> ConfirmedWorldFact:
        _validate_scene_scopes(self.affected_scope_keys)
        return self


class VisualDesignGap(StrictSceneAnalysisModel):
    gap_key: str = Field(pattern=r"^gap_[a-z0-9_]{1,120}$")
    design_domain: DesignDomain
    source_constraint_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    affected_scope_keys: list[str] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_scopes(self) -> VisualDesignGap:
        _validate_scene_scopes(self.affected_scope_keys)
        return self


class VisualFoundationInput(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID
    production_world_owner_set_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    confirmed_world_roots: list[ConfirmedWorldRoot] = Field(min_length=3)
    preset_release: PresetReleaseIdentity
    application_mode: Literal["faithful", "world_adaptation"]
    fidelity_invariants: list[str]
    adaptation_rules: list[PresetAdaptationRuleSnapshot] = Field(min_length=1)
    visual_grammar: VisualGrammarSnapshot
    typed_overrides: list[TypedVisualOverride]
    typed_overrides_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    reference_attachments: list[VisualReferenceAttachment]
    reference_attachments_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    confirmed_world_facts: list[ConfirmedWorldFact]
    design_gaps: list[VisualDesignGap]

    @model_validator(mode="after")
    def validate_frozen_input(self) -> VisualFoundationInput:
        if self.fidelity_invariants != FIDELITY_INVARIANTS:
            raise ValueError("Visual Foundation input must preserve every fidelity invariant")
        project_scope = f"project:{self.project_id}"
        root_keys = [
            (value.owner_family, value.scope_key, value.collection_root_hash)
            for value in self.confirmed_world_roots
        ]
        if root_keys != sorted(set(root_keys)):
            raise ValueError("confirmed world roots must be sorted and unique")
        project_families = [
            value.owner_family
            for value in self.confirmed_world_roots
            if value.scope_key == project_scope
        ]
        if project_families != ["asset_identity_state_set", "bible_production_world_set"]:
            raise ValueError("Visual Foundation input lacks exact project world roots")
        planning = [
            value
            for value in self.confirmed_world_roots
            if value.owner_family == "planning_scene_set"
        ]
        if not planning or any(not value.scope_key.startswith("episode:") for value in planning):
            raise ValueError("Visual Foundation input lacks episode planning roots")
        if any(
            value.owner_family != "planning_scene_set" and value.scope_key != project_scope
            for value in self.confirmed_world_roots
        ):
            raise ValueError("Visual Foundation world root crosses its project scope")
        _validate_key_order(self.adaptation_rules, "rule_key")
        _validate_key_order(self.typed_overrides, "override_key")
        _validate_key_order(self.reference_attachments, "object_key")
        _validate_key_order(self.confirmed_world_facts, "fact_key")
        _validate_key_order(self.design_gaps, "gap_key")
        if any(
            value.scope_kind == "project" and value.scope_key != project_scope
            for value in self.typed_overrides
        ):
            raise ValueError("typed visual override crosses its project scope")
        if self.typed_overrides_hash != production_canonical_hash(
            [value.model_dump(mode="json") for value in self.typed_overrides]
        ):
            raise ValueError("typed visual override set hash drifted")
        if self.reference_attachments_hash != production_canonical_hash(
            [value.model_dump(mode="json") for value in self.reference_attachments]
        ):
            raise ValueError("visual reference attachment set hash drifted")
        return self


class VisualFoundationPolicy(StrictSceneAnalysisModel):
    palette_rules: list[str] = Field(min_length=1)
    material_rules: list[str] = Field(min_length=1)
    lighting_rules: list[str] = Field(min_length=1)
    camera_rules: list[str] = Field(min_length=1)
    forbidden_changes: list[str]

    @model_validator(mode="after")
    def validate_policy(self) -> VisualFoundationPolicy:
        for values in (
            self.palette_rules,
            self.material_rules,
            self.lighting_rules,
            self.camera_rules,
        ):
            if not _sorted_unique(values):
                raise ValueError("Visual Foundation policy rules must be sorted and unique")
        if self.forbidden_changes != FIDELITY_INVARIANTS:
            raise ValueError("Visual Foundation policy cannot relax fidelity invariants")
        return self


class WorldAdaptationProposal(StrictSceneAnalysisModel):
    mapping_key: str = Field(pattern=r"^mapping_[a-z0-9_]{1,120}$")
    source_fact_key: str = Field(pattern=r"^fact_[a-z0-9_]{1,120}$")
    preset_rule_key: str = Field(pattern=r"^[a-z][a-z0-9-]{2,63}$")
    design_value: str = Field(min_length=1, max_length=1200)
    preserved_invariant_keys: list[str]
    affected_scope_keys: list[str] = Field(min_length=1)
    decision_status: Literal["needs_creator_decision"]

    @model_validator(mode="after")
    def validate_proposal(self) -> WorldAdaptationProposal:
        if self.preserved_invariant_keys != FIDELITY_INVARIANTS:
            raise ValueError("world adaptation must preserve every fidelity invariant")
        if self.design_value != self.design_value.strip():
            raise ValueError("world adaptation design value must be canonical text")
        _validate_scene_scopes(self.affected_scope_keys)
        return self


class VisualWorldConflict(StrictSceneAnalysisModel):
    conflict_key: str = Field(pattern=r"^conflict_[a-z0-9_]{1,120}$")
    source_fact_key: str = Field(pattern=r"^fact_[a-z0-9_]{1,120}$")
    preset_rule_key: str = Field(pattern=r"^[a-z][a-z0-9-]{2,63}$")
    severity: Literal["blocking", "warning"]
    summary: str = Field(min_length=1, max_length=800)
    affected_scope_keys: list[str] = Field(min_length=1)
    resolution_status: Literal["needs_creator_decision"]

    @model_validator(mode="after")
    def validate_conflict(self) -> VisualWorldConflict:
        if self.summary != self.summary.strip():
            raise ValueError("Visual Foundation conflict summary must be canonical text")
        _validate_scene_scopes(self.affected_scope_keys)
        return self


class CreativeFillProposal(StrictSceneAnalysisModel):
    gap_key: str = Field(pattern=r"^gap_[a-z0-9_]{1,120}$")
    design_domain: DesignDomain
    proposal: str = Field(min_length=1, max_length=1200)
    affected_scope_keys: list[str] = Field(min_length=1)
    decision_status: Literal["needs_creator_decision"]

    @model_validator(mode="after")
    def validate_proposal(self) -> CreativeFillProposal:
        if self.proposal != self.proposal.strip():
            raise ValueError("creative fill proposal must be canonical text")
        _validate_scene_scopes(self.affected_scope_keys)
        return self


class VisualFoundationCandidate(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID
    production_world_owner_set_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    preset_release_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    application_mode: Literal["faithful", "world_adaptation"]
    typed_overrides_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    reference_attachments_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    fidelity_invariants: list[str]
    style_policy: VisualFoundationPolicy
    world_adaptations: list[WorldAdaptationProposal]
    world_conflicts: list[VisualWorldConflict]
    creative_fill_proposals: list[CreativeFillProposal]

    @model_validator(mode="after")
    def validate_shape(self) -> VisualFoundationCandidate:
        if self.fidelity_invariants != FIDELITY_INVARIANTS:
            raise ValueError("Visual Foundation Candidate changed fidelity invariants")
        _validate_key_order(self.world_adaptations, "mapping_key")
        _validate_key_order(self.world_conflicts, "conflict_key")
        _validate_key_order(self.creative_fill_proposals, "gap_key")
        return self

    def validate_for(self, stage_input: VisualFoundationInput) -> None:
        if (
            self.workspace_id != stage_input.workspace_id
            or self.project_id != stage_input.project_id
            or self.production_world_owner_set_hash != stage_input.production_world_owner_set_hash
            or self.preset_release_content_hash != stage_input.preset_release.content_hash
            or self.application_mode != stage_input.application_mode
            or self.typed_overrides_hash != stage_input.typed_overrides_hash
            or self.reference_attachments_hash != stage_input.reference_attachments_hash
            or self.fidelity_invariants != stage_input.fidelity_invariants
        ):
            raise ValueError("Visual Foundation Candidate input lineage drifted")
        if self.application_mode == "faithful" and self.world_adaptations:
            raise ValueError("faithful Visual Foundation cannot adapt confirmed world facts")

        facts = {value.fact_key: value for value in stage_input.confirmed_world_facts}
        rules = {value.rule_key: value for value in stage_input.adaptation_rules}
        for value in self.world_adaptations:
            fact = facts.get(value.source_fact_key)
            rule = rules.get(value.preset_rule_key)
            if (
                fact is None
                or rule is None
                or fact.fact_kind != rule.source_fact_kind
                or value.affected_scope_keys != fact.affected_scope_keys
            ):
                raise ValueError("world adaptation is outside the frozen fact and Preset rule")
        for value in self.world_conflicts:
            fact = facts.get(value.source_fact_key)
            if (
                fact is None
                or value.preset_rule_key not in rules
                or value.affected_scope_keys != fact.affected_scope_keys
            ):
                raise ValueError("world conflict is outside the frozen fact and Preset rule")

        gaps = {value.gap_key: value for value in stage_input.design_gaps}
        if [value.gap_key for value in self.creative_fill_proposals] != sorted(gaps):
            raise ValueError("Visual Foundation Candidate must cover every frozen Design Gap")
        for value in self.creative_fill_proposals:
            gap = gaps[value.gap_key]
            if (
                value.design_domain != gap.design_domain
                or value.affected_scope_keys != gap.affected_scope_keys
            ):
                raise ValueError("creative fill proposal drifted from its frozen Design Gap")


def visual_foundation_schema_manifest() -> dict[str, object]:
    schemas = [
        {
            "contract_id": "visual-foundation-candidate-production",
            "schema_hash": production_canonical_hash(VisualFoundationCandidate.model_json_schema()),
        },
        {
            "contract_id": "visual-foundation-input-production",
            "schema_hash": production_canonical_hash(VisualFoundationInput.model_json_schema()),
        },
    ]
    material: dict[str, object] = {
        "contract_id": "storygraph-visual-foundation-schema-set-production",
        "schemas": schemas,
    }
    return {**material, "schema_set_hash": production_canonical_hash(material)}


def _validate_key_order(values: Sequence[object], attribute: str) -> None:
    keys = [str(getattr(value, attribute)) for value in values]
    if keys != sorted(set(keys)):
        raise ValueError(f"{attribute} values must be sorted and unique")


def _validate_scene_scopes(values: list[str]) -> None:
    if not _sorted_unique(values) or any(
        re.fullmatch(r"scene:[0-9a-f-]{36}", value) is None for value in values
    ):
        raise ValueError("Visual Foundation scopes must be non-empty, sorted Scene scopes")


def _sorted_unique(values: Sequence[str]) -> bool:
    return bool(values) and values == sorted(set(values))
