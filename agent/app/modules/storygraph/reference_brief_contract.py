from __future__ import annotations

import json
import unicodedata
from collections.abc import Sequence
from typing import Annotated, Literal, cast
from uuid import UUID

from pydantic import Field, model_validator

from app.modules.storygraph.reference_plan_contract import (
    REFERENCE_TARGET_KINDS,
    ReferencePlanOwnerRef,
    ReferencePlanTargetOwnerRefs,
    ReferenceTargetKind,
)
from app.modules.storygraph.scene_analysis_candidates import StrictSceneAnalysisModel
from app.protocol.canonical import production_canonical_json

IDENTITY_INVARIANT_SLOTS = [
    "body_shape",
    "facial_structure",
    "hair",
    "permanent_marks",
    "proportions",
]

REFERENCE_BRIEF_VIEW_ROLES: dict[str, list[str]] = {
    "character_appearance": ["back", "front", "profile"],
    "character_identity_anchor": ["back", "front", "profile"],
    "interaction_composition": ["interaction_master"],
    "location_board": ["empty_establishing", "material_scale_detail", "spatial_orientation"],
    "prop_sheet": ["back", "front", "side", "state_detail"],
    "scene_composition": ["composition_master"],
}


class ReferenceBriefStageRelease(StrictSceneAnalysisModel):
    stage_key: Literal["compile_reference_brief"]
    stage_release_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class ReferenceBriefSourceDesignSlot(StrictSceneAnalysisModel):
    slot_key: str = Field(min_length=1)
    source_requirement: str = Field(min_length=1)
    design_requirement: str = Field(min_length=1)


class ReferenceBriefQCRubricRef(StrictSceneAnalysisModel):
    contract_id: str = Field(min_length=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class ReferenceBriefDependencySelection(StrictSceneAnalysisModel):
    target_version_ref: ReferencePlanOwnerRef
    selected_asset_version_ref: ReferencePlanOwnerRef


class CharacterIdentityAnchorBrief(StrictSceneAnalysisModel):
    target_kind: Literal["character_identity_anchor"]
    identity_invariant_slots: list[str]

    @model_validator(mode="after")
    def validate_invariants(self) -> CharacterIdentityAnchorBrief:
        if self.identity_invariant_slots != IDENTITY_INVARIANT_SLOTS:
            raise ValueError("Character Identity Anchor invariants are incomplete")
        return self


class CharacterAppearanceBrief(StrictSceneAnalysisModel):
    target_kind: Literal["character_appearance"]
    identity_invariant_slots: list[str]
    approved_variable_slots: list[str] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_slots(self) -> CharacterAppearanceBrief:
        if self.identity_invariant_slots != IDENTITY_INVARIANT_SLOTS:
            raise ValueError("Character Appearance invariants are incomplete")
        _validate_sorted_text(self.approved_variable_slots, nonempty=True)
        return self


class LocationBoardBrief(StrictSceneAnalysisModel):
    target_kind: Literal["location_board"]
    topology_constraints: list[str] = Field(min_length=1)
    scale_anchors: list[str] = Field(min_length=1)
    material_slots: list[str] = Field(min_length=1)
    occupancy_policy: Literal["empty"]

    @model_validator(mode="after")
    def validate_slots(self) -> LocationBoardBrief:
        for values in (self.topology_constraints, self.scale_anchors, self.material_slots):
            _validate_sorted_text(values, nonempty=True)
        return self


class PropSheetBrief(StrictSceneAnalysisModel):
    target_kind: Literal["prop_sheet"]
    physical_dimensions: str = Field(min_length=1)
    structural_slots: list[str] = Field(min_length=1)
    state_slots: list[str] = Field(min_length=1)
    content_or_mechanism_slots: list[str] = Field(min_length=1)
    occupancy_policy: Literal["no_hands_no_people"]

    @model_validator(mode="after")
    def validate_slots(self) -> PropSheetBrief:
        if not _stable_text(self.physical_dimensions):
            raise ValueError("Prop Sheet dimensions are invalid")
        for values in (self.structural_slots, self.state_slots, self.content_or_mechanism_slots):
            _validate_sorted_text(values, nonempty=True)
        return self


class SceneCompositionBrief(StrictSceneAnalysisModel):
    target_kind: Literal["scene_composition"]
    composition_purpose: str = Field(min_length=1)
    spatial_constraints: list[str] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_slots(self) -> SceneCompositionBrief:
        if not _stable_text(self.composition_purpose):
            raise ValueError("Scene Composition purpose is invalid")
        _validate_sorted_text(self.spatial_constraints, nonempty=True)
        return self


class InteractionCompositionBrief(StrictSceneAnalysisModel):
    target_kind: Literal["interaction_composition"]
    hand_side: Literal["both", "left", "right"]
    grip_or_contact_point: str = Field(min_length=1)
    orientation: str = Field(min_length=1)
    body_prop_scale_constraints: list[str] = Field(min_length=1)
    transfer_or_use_state: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_slots(self) -> InteractionCompositionBrief:
        if not all(
            _stable_text(value)
            for value in (self.grip_or_contact_point, self.orientation, self.transfer_or_use_state)
        ):
            raise ValueError("Interaction Composition geometry is invalid")
        _validate_sorted_text(self.body_prop_scale_constraints, nonempty=True)
        return self


ReferenceBriefPurpose = Annotated[
    CharacterAppearanceBrief
    | CharacterIdentityAnchorBrief
    | InteractionCompositionBrief
    | LocationBoardBrief
    | PropSheetBrief
    | SceneCompositionBrief,
    Field(discriminator="target_kind"),
]


class ReferenceBriefCandidate(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID
    approved_reference_plan_version_ref: ReferencePlanOwnerRef
    reference_plan_target_ref: ReferencePlanOwnerRef
    target_business_key: str = Field(min_length=1)
    target_kind: ReferenceTargetKind
    visual_foundation_version_ref: ReferencePlanOwnerRef
    effective_style_snapshot_ref: ReferencePlanOwnerRef
    effective_policy_snapshot_ref: ReferencePlanOwnerRef
    dependency_selections: list[ReferenceBriefDependencySelection]
    stage_release: ReferenceBriefStageRelease
    typed_read_set_root: str = Field(pattern=r"^[0-9a-f]{64}$")
    source_design_slots: list[ReferenceBriefSourceDesignSlot] = Field(min_length=1)
    positive_instructions: list[str] = Field(min_length=1)
    negative_instructions: list[str] = Field(min_length=1)
    required_view_roles: list[str] = Field(min_length=1)
    layout_requirements: list[str] = Field(min_length=1)
    scale_requirements: list[str] = Field(min_length=1)
    rights_requirements: list[str] = Field(min_length=1)
    provenance_requirements: list[str] = Field(min_length=1)
    qc_rubric_refs: list[ReferenceBriefQCRubricRef] = Field(min_length=1)
    source_refs: ReferencePlanTargetOwnerRefs
    brief: ReferenceBriefPurpose

    @model_validator(mode="after")
    def validate_candidate(self) -> ReferenceBriefCandidate:
        workspace_id, project_id = str(self.workspace_id), str(self.project_id)
        refs = (
            self.approved_reference_plan_version_ref,
            self.reference_plan_target_ref,
            self.visual_foundation_version_ref,
            self.effective_style_snapshot_ref,
            self.effective_policy_snapshot_ref,
        )
        for ref in refs:
            _validate_ref(ref, workspace_id, project_id)
        for ref in refs[:2]:
            if (
                ref.owner_kind != "production/reference"
                or ref.version_family != "reference_plan_set"
                or ref.fragment_key is not None
            ):
                raise ValueError("Reference Brief Plan/Target ref is invalid")
        for ref in refs[2:]:
            if (
                ref.owner_kind != "preset"
                or ref.version_family != "preset_effective_set"
                or ref.fragment_key is not None
            ):
                raise ValueError("Reference Brief Visual Foundation ref is invalid")
        if (
            _business_key_kind(self.target_business_key) != self.target_kind
            or self.reference_plan_target_ref.owner_logical_id != self.target_business_key
            or self.brief.target_kind != self.target_kind
            or self.required_view_roles != REFERENCE_BRIEF_VIEW_ROLES[self.target_kind]
        ):
            raise ValueError("Reference Brief target purpose has drifted")
        self._validate_dependencies(workspace_id, project_id)
        self._validate_common_requirements(workspace_id, project_id)
        return self

    def _validate_dependencies(self, workspace_id: str, project_id: str) -> None:
        if self.target_kind in {"character_identity_anchor", "location_board", "prop_sheet"}:
            if self.dependency_selections:
                raise ValueError("Reference Brief base Target has dependencies")
        elif self.target_kind == "character_appearance":
            if (
                len(self.dependency_selections) != 1
                or _business_key_kind(
                    self.dependency_selections[0].target_version_ref.owner_logical_id
                )
                != "character_identity_anchor"
            ):
                raise ValueError("Reference Brief Appearance lacks its exact Identity Anchor")
        elif not self.dependency_selections:
            raise ValueError("Reference Brief Composition lacks selected base assets")
        keys: list[bytes] = []
        for selection in self.dependency_selections:
            target_ref, asset_ref = (
                selection.target_version_ref,
                selection.selected_asset_version_ref,
            )
            _validate_ref(target_ref, workspace_id, project_id)
            _validate_ref(asset_ref, workspace_id, project_id)
            if (
                target_ref.owner_kind != "production/reference"
                or target_ref.version_family != "reference_plan_set"
                or target_ref.fragment_key is not None
                or _business_key_kind(target_ref.owner_logical_id)
                not in {
                    "character_appearance",
                    "character_identity_anchor",
                    "location_board",
                    "prop_sheet",
                }
                or asset_ref.owner_kind != "asset"
                or asset_ref.version_family != "asset_base_reference_set"
                or asset_ref.fragment_key is not None
            ):
                raise ValueError("Reference Brief dependency selection is invalid")
            keys.append(target_ref.owner_logical_id.encode())
        if keys != sorted(set(keys)):
            raise ValueError("Reference Brief dependency selections are not sorted and unique")

    def _validate_common_requirements(self, workspace_id: str, project_id: str) -> None:
        slot_keys: list[bytes] = []
        for slot in self.source_design_slots:
            if not all(
                _stable_text(value)
                for value in (slot.slot_key, slot.source_requirement, slot.design_requirement)
            ):
                raise ValueError("Reference Brief source/design slot is invalid")
            slot_keys.append(slot.slot_key.encode())
        if slot_keys != sorted(set(slot_keys)):
            raise ValueError("Reference Brief source/design slots are not sorted and unique")
        for values in (
            self.positive_instructions,
            self.negative_instructions,
            self.layout_requirements,
            self.scale_requirements,
            self.rights_requirements,
            self.provenance_requirements,
        ):
            _validate_sorted_text(values, nonempty=True)
        rubric_keys = [(value.contract_id, value.content_hash) for value in self.qc_rubric_refs]
        if rubric_keys != sorted(set(rubric_keys)) or any(
            not _stable_text(value.contract_id) for value in self.qc_rubric_refs
        ):
            raise ValueError("Reference Brief QC rubric refs are invalid")
        for refs in (
            self.source_refs.identity,
            self.source_refs.specification,
            self.source_refs.state,
            self.source_refs.scene,
            self.source_refs.occurrence,
            self.source_refs.interaction,
        ):
            for ref in refs:
                _validate_ref(ref, workspace_id, project_id)
            keys = [ref.sort_key() for ref in refs]
            if keys != sorted(set(keys)):
                raise ValueError("Reference Brief source refs are not sorted and unique")
        refs = self.source_refs
        if not all((refs.identity, refs.specification, refs.state, refs.scene, refs.occurrence)):
            raise ValueError("Reference Brief source closure is incomplete")
        if self.target_kind in {
            "character_appearance",
            "character_identity_anchor",
            "location_board",
            "prop_sheet",
        }:
            if (
                len(refs.identity) != 1
                or len(refs.specification) != 1
                or len(refs.state) != 1
                or refs.interaction
            ):
                raise ValueError("Reference Brief base source closure is invalid")
        elif self.target_kind == "interaction_composition":
            if len(refs.scene) != 1 or len(refs.interaction) != 1:
                raise ValueError("Reference Brief Interaction source closure is invalid")
        elif len(refs.scene) != 1:
            raise ValueError("Reference Brief Scene source closure is invalid")


def _business_key_kind(value: str) -> str:
    try:
        parsed = json.loads(value)
        if production_canonical_json(parsed).decode() != value or not isinstance(parsed, list):
            return ""
    except (json.JSONDecodeError, UnicodeError, ValueError):
        return ""
    parts = cast(list[object], parsed)
    if len(parts) < 2 or not isinstance(parts[0], str) or parts[0] not in REFERENCE_TARGET_KINDS:
        return ""
    kind = parts[0]
    expected_refs = {
        "character_appearance": 3,
        "character_identity_anchor": 1,
        "interaction_composition": 1,
        "location_board": 3,
        "prop_sheet": 3,
        "scene_composition": 1,
    }[kind]
    if len(parts) != expected_refs + 1:
        return ""
    for raw_ref in parts[1:]:
        if not isinstance(raw_ref, list):
            return ""
        ref = cast(list[object], raw_ref)
        if len(ref) != 4 or not all(isinstance(item, str) for item in ref):
            return ""
        if not all(_stable_text(cast(str, item)) for item in ref[:3]):
            return ""
        if ref[3] != "" and not _stable_text(cast(str, ref[3])):
            return ""
    return kind


def _validate_ref(value: ReferencePlanOwnerRef, workspace_id: str, project_id: str) -> None:
    if str(value.workspace_id) != workspace_id or str(value.project_id) != project_id:
        raise ValueError("Reference Brief Owner ref crosses workspace or project")


def _validate_sorted_text(values: Sequence[str], *, nonempty: bool) -> None:
    if (nonempty and not values) or any(not _stable_text(value) for value in values):
        raise ValueError("Reference Brief text values are invalid")
    if list(values) != sorted(set(values)):
        raise ValueError("Reference Brief text values are not sorted and unique")


def _stable_text(value: str) -> bool:
    return (
        bool(value.strip())
        and unicodedata.normalize("NFC", value) == value
        and not any(ord(character) < 32 or ord(character) == 127 for character in value)
    )
