from __future__ import annotations

import json
import unicodedata
from collections.abc import Sequence
from typing import Any, Literal, cast
from uuid import UUID

from pydantic import Field, model_validator

from app.modules.storygraph.scene_analysis_candidates import StrictSceneAnalysisModel
from app.modules.storygraph.visual_foundation_contract import VisualFoundationCandidate
from app.protocol.canonical import production_canonical_hash, production_canonical_json

ReferenceTargetKind = Literal[
    "character_appearance",
    "character_identity_anchor",
    "interaction_composition",
    "location_board",
    "prop_sheet",
    "scene_composition",
]

REFERENCE_TARGET_KINDS = [
    "character_appearance",
    "character_identity_anchor",
    "interaction_composition",
    "location_board",
    "prop_sheet",
    "scene_composition",
]


class ReferencePlanOwnerRef(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID
    owner_kind: str = Field(min_length=1)
    version_family: str = Field(min_length=1)
    owner_logical_id: str = Field(min_length=1)
    owner_version_id: UUID
    owner_revision: int = Field(ge=1, le=9_007_199_254_740_991)
    owner_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    fragment_key: str | None
    fragment_content_hash: str | None = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def validate_ref(self) -> ReferencePlanOwnerRef:
        if (
            not _stable_text(self.owner_kind)
            or not _stable_text(self.version_family)
            or not _stable_text(self.owner_logical_id)
            or (self.fragment_key is None) != (self.fragment_content_hash is None)
            or (self.fragment_key is not None and not _stable_text(self.fragment_key))
        ):
            raise ValueError("invalid Reference Plan Owner ref")
        return self

    def sort_key(self) -> tuple[bytes, bytes, bytes, bytes, bytes]:
        return (
            self.owner_kind.encode("utf-8"),
            self.version_family.encode("utf-8"),
            self.owner_logical_id.encode("utf-8"),
            (self.fragment_key or "").encode("utf-8"),
            str(self.owner_version_id).encode("utf-8"),
        )


class ReferencePlanCharacterStateSeed(StrictSceneAnalysisModel):
    state_ref: ReferencePlanOwnerRef
    appearance_business_key: str = Field(min_length=1)
    coverage_scope_keys: list[str] = Field(min_length=1)
    occurrence_refs: list[ReferencePlanOwnerRef] = Field(min_length=1)


class ReferencePlanCharacterSeed(StrictSceneAnalysisModel):
    anchor_business_key: str = Field(min_length=1)
    identity_ref: ReferencePlanOwnerRef
    specification_ref: ReferencePlanOwnerRef
    coverage_scope_keys: list[str] = Field(min_length=1)
    state_options: list[ReferencePlanCharacterStateSeed] = Field(min_length=1)


class ReferencePlanTargetOwnerRefs(StrictSceneAnalysisModel):
    identity: list[ReferencePlanOwnerRef]
    specification: list[ReferencePlanOwnerRef]
    state: list[ReferencePlanOwnerRef]
    scene: list[ReferencePlanOwnerRef]
    occurrence: list[ReferencePlanOwnerRef]
    interaction: list[ReferencePlanOwnerRef]


class ReferencePlanCharacterDependencySeed(StrictSceneAnalysisModel):
    anchor_business_key: str = Field(min_length=1)
    state_ref: ReferencePlanOwnerRef
    appearance_business_key: str = Field(min_length=1)


class ReferencePlanFixedTargetSeed(StrictSceneAnalysisModel):
    target_business_key: str = Field(min_length=1)
    target_kind: Literal[
        "interaction_composition", "location_board", "prop_sheet", "scene_composition"
    ]
    owner_refs: ReferencePlanTargetOwnerRefs
    coverage_scope_keys: list[str] = Field(min_length=1)
    fixed_dependency_business_keys: list[str]
    character_dependencies: list[ReferencePlanCharacterDependencySeed]


class ReferencePlanPurposeProfile(StrictSceneAnalysisModel):
    target_kind: ReferenceTargetKind
    design_focus: list[str] = Field(min_length=1)
    forbidden_changes: list[str] = Field(min_length=1)


class ReferencePlanInput(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID
    production_world_owner_set_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    p1_scope_keys: list[str] = Field(min_length=1)
    visual_foundation_candidate_revision_id: UUID
    visual_foundation_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    visual_foundation_candidate: VisualFoundationCandidate
    character_seeds: list[ReferencePlanCharacterSeed]
    fixed_target_seeds: list[ReferencePlanFixedTargetSeed] = Field(min_length=1)
    purpose_profiles: list[ReferencePlanPurposeProfile] = Field(min_length=6, max_length=6)
    reference_target_seed_root: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def validate_frozen_seeds(self) -> ReferencePlanInput:
        workspace_id, project_id = str(self.workspace_id), str(self.project_id)
        _validate_scopes(self.p1_scope_keys)
        if (
            str(self.visual_foundation_candidate.workspace_id) != workspace_id
            or str(self.visual_foundation_candidate.project_id) != project_id
            or self.visual_foundation_candidate.production_world_owner_set_hash
            != self.production_world_owner_set_hash
            or (
                self.visual_foundation_candidate.application_mode == "faithful"
                and self.visual_foundation_candidate.world_adaptations
            )
        ):
            raise ValueError("Reference Plan Visual Foundation Candidate lineage drifted")
        if [profile.target_kind for profile in self.purpose_profiles] != REFERENCE_TARGET_KINDS:
            raise ValueError("Reference Plan PurposeProfiles are incomplete or unordered")
        for profile in self.purpose_profiles:
            _validate_sorted_text(profile.design_focus, nonempty=True)
            _validate_sorted_text(profile.forbidden_changes, nonempty=True)

        anchors: dict[str, ReferencePlanCharacterSeed] = {}
        appearances: dict[str, ReferencePlanCharacterStateSeed] = {}
        anchor_keys = [seed.anchor_business_key for seed in self.character_seeds]
        if anchor_keys != sorted(set(anchor_keys)):
            raise ValueError("Reference Plan Character seeds must be sorted and unique")
        for seed in self.character_seeds:
            if _business_key_kind(seed.anchor_business_key) != "character_identity_anchor":
                raise ValueError("Reference Plan Character Anchor key is invalid")
            _validate_ref(seed.identity_ref, workspace_id, project_id)
            _validate_ref(seed.specification_ref, workspace_id, project_id)
            _validate_scopes(seed.coverage_scope_keys)
            if not set(seed.coverage_scope_keys).issubset(self.p1_scope_keys):
                raise ValueError("Reference Plan Character scope escapes P1")
            state_keys = [option.state_ref.sort_key() for option in seed.state_options]
            if state_keys != sorted(set(state_keys)):
                raise ValueError("Reference Plan Character State options are not sorted and unique")
            coverage: set[str] = set()
            occurrences: set[tuple[bytes, ...]] = set()
            for option in seed.state_options:
                _validate_ref(option.state_ref, workspace_id, project_id)
                if _business_key_kind(option.appearance_business_key) != "character_appearance":
                    raise ValueError("Reference Plan Appearance seed key is invalid")
                _validate_scopes(option.coverage_scope_keys)
                _validate_refs(option.occurrence_refs, workspace_id, project_id)
                if not set(option.coverage_scope_keys).issubset(seed.coverage_scope_keys):
                    raise ValueError("Reference Plan Character State scope escapes its Character")
                coverage.update(option.coverage_scope_keys)
                for occurrence in option.occurrence_refs:
                    if occurrence.sort_key() in occurrences:
                        raise ValueError("Reference Plan Character occurrence seed is duplicated")
                    occurrences.add(occurrence.sort_key())
                if option.appearance_business_key in appearances:
                    raise ValueError("Reference Plan Appearance seed is duplicated")
                appearances[option.appearance_business_key] = option
            if sorted(coverage) != seed.coverage_scope_keys:
                raise ValueError("Reference Plan Character seed coverage is incomplete")
            anchors[seed.anchor_business_key] = seed

        fixed: dict[str, ReferencePlanFixedTargetSeed] = {}
        fixed_keys = [seed.target_business_key for seed in self.fixed_target_seeds]
        if fixed_keys != sorted(set(fixed_keys)):
            raise ValueError("Reference Plan fixed target seeds must be sorted and unique")
        scene_scopes: set[str] = set()
        for seed in self.fixed_target_seeds:
            if _business_key_kind(seed.target_business_key) != seed.target_kind:
                raise ValueError("Reference Plan fixed target key does not match its kind")
            _validate_scopes(seed.coverage_scope_keys)
            if not set(seed.coverage_scope_keys).issubset(self.p1_scope_keys):
                raise ValueError("Reference Plan fixed target scope escapes P1")
            for refs in (
                seed.owner_refs.identity,
                seed.owner_refs.specification,
                seed.owner_refs.state,
                seed.owner_refs.scene,
                seed.owner_refs.occurrence,
                seed.owner_refs.interaction,
            ):
                _validate_refs(refs, workspace_id, project_id)
            if (
                sorted({ref.owner_logical_id for ref in seed.owner_refs.scene})
                != seed.coverage_scope_keys
            ):
                raise ValueError("Reference Plan fixed target coverage differs from Scene refs")
            _validate_sorted_text(seed.fixed_dependency_business_keys, nonempty=False)
            if seed.target_kind in {"location_board", "prop_sheet"}:
                if (
                    len(seed.owner_refs.identity) != 1
                    or len(seed.owner_refs.specification) != 1
                    or len(seed.owner_refs.state) != 1
                    or not seed.owner_refs.occurrence
                    or seed.fixed_dependency_business_keys
                    or seed.character_dependencies
                ):
                    raise ValueError("Reference Plan base target seed is invalid")
            elif seed.target_kind == "scene_composition":
                if (
                    len(seed.owner_refs.scene) != 1
                    or not seed.owner_refs.identity
                    or not seed.owner_refs.specification
                    or not seed.owner_refs.state
                    or not seed.owner_refs.occurrence
                ):
                    raise ValueError("Reference Plan Scene seed is incomplete")
                scene_scopes.add(seed.coverage_scope_keys[0])
            elif (
                len(seed.owner_refs.scene) != 1
                or len(seed.owner_refs.interaction) != 1
                or len(seed.owner_refs.occurrence) < 2
            ):
                raise ValueError("Reference Plan Interaction seed is incomplete")
            _validate_character_dependencies(seed.character_dependencies, anchors, appearances)
            fixed[seed.target_business_key] = seed
        if sorted(scene_scopes) != self.p1_scope_keys:
            raise ValueError("Reference Plan Scene seed coverage differs from P1")
        if set(anchors) & set(fixed) or set(appearances) & set(fixed):
            raise ValueError("Reference Plan target seed keys collide")
        for seed in self.fixed_target_seeds:
            for dependency_key in seed.fixed_dependency_business_keys:
                dependency = fixed.get(dependency_key)
                if dependency is None or dependency.target_kind not in {
                    "location_board",
                    "prop_sheet",
                }:
                    raise ValueError("Reference Plan fixed dependency is not a base target")
        if self.reference_target_seed_root != self.compute_seed_root():
            raise ValueError("Reference Target seed root has drifted")
        return self

    def compute_seed_root(self) -> str:
        return production_canonical_hash(
            {
                "contract_id": "reference-target-seed-inventory-production",
                "character_seeds": _dump_models(self.character_seeds),
                "fixed_target_seeds": _dump_models(self.fixed_target_seeds),
            }
        )


class ReferencePlanAnchorSelection(StrictSceneAnalysisModel):
    anchor_business_key: str = Field(min_length=1)
    selected_state_ref: ReferencePlanOwnerRef


class ReferencePlanTargetSpecification(StrictSceneAnalysisModel):
    target_business_key: str = Field(min_length=1)
    target_kind: ReferenceTargetKind
    fulfillment: Literal["not_generated", "optional", "required"]
    design_focus: list[str] = Field(min_length=1)
    forbidden_changes: list[str] = Field(min_length=1)
    depends_on_target_business_keys: list[str]

    @model_validator(mode="after")
    def validate_specification(self) -> ReferencePlanTargetSpecification:
        if _business_key_kind(self.target_business_key) != self.target_kind:
            raise ValueError("Reference Plan Target key does not match its kind")
        _validate_sorted_text(self.design_focus, nonempty=True)
        _validate_sorted_text(self.forbidden_changes, nonempty=True)
        _validate_sorted_text(self.depends_on_target_business_keys, nonempty=False)
        return self


class ReferencePlanCandidate(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID
    production_world_owner_set_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    p1_scope_keys: list[str] = Field(min_length=1)
    visual_foundation_candidate_revision_id: UUID
    visual_foundation_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    reference_target_seed_root: str = Field(pattern=r"^[0-9a-f]{64}$")
    anchor_selections: list[ReferencePlanAnchorSelection]
    target_specifications: list[ReferencePlanTargetSpecification] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_shape(self) -> ReferencePlanCandidate:
        _validate_scopes(self.p1_scope_keys)
        anchor_keys = [selection.anchor_business_key for selection in self.anchor_selections]
        if anchor_keys != sorted(set(anchor_keys)) or any(
            _business_key_kind(key) != "character_identity_anchor" for key in anchor_keys
        ):
            raise ValueError("Reference Plan Anchor selections are not sorted and unique")
        target_keys = [value.target_business_key for value in self.target_specifications]
        if target_keys != sorted(set(target_keys)):
            raise ValueError("Reference Plan Target specifications are not sorted and unique")
        return self

    def validate_for(self, stage_input: ReferencePlanInput) -> None:
        if (
            self.workspace_id != stage_input.workspace_id
            or self.project_id != stage_input.project_id
            or self.production_world_owner_set_hash != stage_input.production_world_owner_set_hash
            or self.p1_scope_keys != stage_input.p1_scope_keys
            or self.visual_foundation_candidate_revision_id
            != stage_input.visual_foundation_candidate_revision_id
            or self.visual_foundation_candidate_revision_hash
            != stage_input.visual_foundation_candidate_revision_hash
            or self.reference_target_seed_root != stage_input.reference_target_seed_root
        ):
            raise ValueError("Reference Plan Candidate input lineage drifted")

        if len(self.anchor_selections) != len(stage_input.character_seeds):
            raise ValueError("Reference Plan Anchor selections are incomplete")
        selected: dict[str, ReferencePlanOwnerRef] = {}
        expected: dict[str, str] = {}
        for seed, selection in zip(
            stage_input.character_seeds, self.anchor_selections, strict=True
        ):
            if selection.anchor_business_key != seed.anchor_business_key:
                raise ValueError("Reference Plan Anchor selections differ from frozen seeds")
            option = next(
                (
                    value
                    for value in seed.state_options
                    if value.state_ref == selection.selected_state_ref
                ),
                None,
            )
            if option is None:
                raise ValueError("Reference Plan Anchor selected a non-occurring State")
            selected[seed.anchor_business_key] = selection.selected_state_ref
            expected[seed.anchor_business_key] = "character_identity_anchor"
            for state_option in seed.state_options:
                if state_option.state_ref != selection.selected_state_ref:
                    expected[state_option.appearance_business_key] = "character_appearance"
        for seed in stage_input.fixed_target_seeds:
            expected[seed.target_business_key] = seed.target_kind
        specifications = {value.target_business_key: value for value in self.target_specifications}
        if list(specifications) != sorted(expected) or set(specifications) != set(expected):
            raise ValueError("Reference Plan Candidate target set differs from frozen seeds")

        profiles = {profile.target_kind: profile for profile in stage_input.purpose_profiles}
        for key, specification in specifications.items():
            profile = profiles[specification.target_kind]
            if (
                expected[key] != specification.target_kind
                or not set(specification.design_focus).issubset(profile.design_focus)
                or specification.forbidden_changes != profile.forbidden_changes
            ):
                raise ValueError(
                    "Reference Plan Candidate target specification escapes its PurposeProfile"
                )
        for specification in self.target_specifications:
            dependencies = _expected_dependencies(stage_input, selected, specification)
            if specification.depends_on_target_business_keys != dependencies:
                raise ValueError(
                    "Reference Plan Candidate dependency set differs from frozen seeds"
                )
            for dependency_key in dependencies:
                if _fulfillment_rank(
                    specifications[dependency_key].fulfillment
                ) < _fulfillment_rank(specification.fulfillment):
                    raise ValueError(
                        "Reference Plan Candidate dependency fulfillment rank is invalid"
                    )


def reference_plan_schema_manifest() -> dict[str, object]:
    schemas = [
        {
            "contract_id": "reference-plan-candidate-production",
            "schema_hash": production_canonical_hash(ReferencePlanCandidate.model_json_schema()),
        },
        {
            "contract_id": "reference-plan-input-production",
            "schema_hash": production_canonical_hash(ReferencePlanInput.model_json_schema()),
        },
    ]
    material: dict[str, object] = {
        "contract_id": "storygraph-reference-plan-schema-set-production",
        "schemas": schemas,
    }
    return {**material, "schema_set_hash": production_canonical_hash(material)}


def _expected_dependencies(
    stage_input: ReferencePlanInput,
    selected: dict[str, ReferencePlanOwnerRef],
    specification: ReferencePlanTargetSpecification,
) -> list[str]:
    for seed in stage_input.character_seeds:
        if any(
            option.appearance_business_key == specification.target_business_key
            for option in seed.state_options
        ):
            return [seed.anchor_business_key]
    fixed = next(
        (
            seed
            for seed in stage_input.fixed_target_seeds
            if seed.target_business_key == specification.target_business_key
        ),
        None,
    )
    if (
        fixed is None
        or fixed.target_kind not in {"scene_composition", "interaction_composition"}
        or specification.fulfillment == "not_generated"
    ):
        return []
    dependencies = list(fixed.fixed_dependency_business_keys)
    for dependency in fixed.character_dependencies:
        dependencies.append(
            dependency.anchor_business_key
            if selected[dependency.anchor_business_key] == dependency.state_ref
            else dependency.appearance_business_key
        )
    return sorted(set(dependencies))


def _validate_character_dependencies(
    dependencies: list[ReferencePlanCharacterDependencySeed],
    anchors: dict[str, ReferencePlanCharacterSeed],
    appearances: dict[str, ReferencePlanCharacterStateSeed],
) -> None:
    keys = [
        (dependency.anchor_business_key.encode(), dependency.state_ref.sort_key())
        for dependency in dependencies
    ]
    if keys != sorted(set(keys)):
        raise ValueError("Reference Plan Character dependency seeds are not sorted and unique")
    for dependency in dependencies:
        seed = anchors.get(dependency.anchor_business_key)
        appearance = appearances.get(dependency.appearance_business_key)
        if (
            seed is None
            or appearance is None
            or appearance.state_ref != dependency.state_ref
            or not any(
                option.appearance_business_key == dependency.appearance_business_key
                and option.state_ref == dependency.state_ref
                for option in seed.state_options
            )
        ):
            raise ValueError("Reference Plan Character dependency seed is invalid")


def _business_key_kind(value: str) -> str:
    try:
        parsed = json.loads(value)
        if production_canonical_json(parsed).decode() != value:
            return ""
    except (json.JSONDecodeError, UnicodeError, ValueError):
        return ""
    if not isinstance(parsed, list):
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
        if (
            len(ref) != 4
            or not all(isinstance(value, str) for value in ref)
            or not all(_stable_text(cast(str, value)) for value in ref[:3])
            or (ref[3] != "" and not _stable_text(cast(str, ref[3])))
        ):
            return ""
    return kind


def _validate_ref(value: ReferencePlanOwnerRef, workspace_id: str, project_id: str) -> None:
    if str(value.workspace_id) != workspace_id or str(value.project_id) != project_id:
        raise ValueError("Reference Plan Owner ref crosses workspace or project")


def _validate_refs(values: list[ReferencePlanOwnerRef], workspace_id: str, project_id: str) -> None:
    for value in values:
        _validate_ref(value, workspace_id, project_id)
    keys = [value.sort_key() for value in values]
    if keys != sorted(set(keys)):
        raise ValueError("Reference Plan Owner refs are not sorted and unique")


def _validate_scopes(values: list[str]) -> None:
    _validate_sorted_text(values, nonempty=True)
    if any(not value.startswith("scene:") for value in values):
        raise ValueError("Reference Plan scope must name a Scene")


def _validate_sorted_text(values: Sequence[str], *, nonempty: bool) -> None:
    if (nonempty and not values) or any(not _stable_text(value) for value in values):
        raise ValueError("Reference Plan text values are invalid")
    if list(values) != sorted(set(values)):
        raise ValueError("Reference Plan text values are not sorted and unique")


def _stable_text(value: str) -> bool:
    return (
        bool(value.strip())
        and unicodedata.normalize("NFC", value) == value
        and not any(ord(character) < 32 or ord(character) == 127 for character in value)
    )


def _fulfillment_rank(value: str) -> int:
    return ["not_generated", "optional", "required"].index(value)


def _dump_models(values: Sequence[Any]) -> list[Any]:
    return [
        value.model_dump(mode="json") if hasattr(value, "model_dump") else value for value in values
    ]
