from __future__ import annotations

import copy
import hashlib
import json
from typing import Any

import pytest
from pydantic import ValidationError

from app.modules.storygraph.reference_brief_contract import ReferenceBriefCandidate


@pytest.mark.parametrize(
    "target_kind",
    [
        "character_appearance",
        "character_identity_anchor",
        "interaction_composition",
        "location_board",
        "prop_sheet",
        "scene_composition",
    ],
)
def test_reference_brief_candidate_accepts_six_strict_purposes(target_kind: str) -> None:
    candidate = ReferenceBriefCandidate.model_validate(reference_brief_candidate(target_kind))
    assert candidate.target_kind == target_kind
    assert candidate.brief.target_kind == target_kind


@pytest.mark.parametrize(
    "mutation",
    ["provider", "wrong_view", "wrong_branch", "latest_target", "missing_dependency"],
)
def test_reference_brief_candidate_rejects_execution_or_purpose_drift(mutation: str) -> None:
    payload = reference_brief_candidate("character_appearance")
    if mutation == "provider":
        payload["provider"] = {"model": "current"}
    elif mutation == "wrong_view":
        payload["required_view_roles"] = ["front"]
    elif mutation == "wrong_branch":
        payload["brief"] = purpose_brief("prop_sheet")
    elif mutation == "latest_target":
        payload["reference_plan_target_ref"]["owner_version_id"] = "latest"
    else:
        payload["dependency_selections"] = []
    with pytest.raises(ValidationError):
        ReferenceBriefCandidate.model_validate(payload)


def test_reference_brief_candidate_rejects_unknown_nested_purpose_field() -> None:
    payload = reference_brief_candidate("scene_composition")
    payload["brief"]["prompt"] = "free-form provider prompt"
    with pytest.raises(ValidationError):
        ReferenceBriefCandidate.model_validate(payload)


def reference_brief_candidate(target_kind: str) -> dict[str, Any]:
    workspace_id = "00000000-0000-0000-0000-000000000010"
    project_id = "00000000-0000-0000-0000-000000000001"
    target_key = target_business_key(target_kind)
    dependencies: list[dict[str, Any]] = []
    if target_kind == "character_appearance" or target_kind.endswith("_composition"):
        dependencies = [
            {
                "target_version_ref": owner_ref(
                    workspace_id,
                    project_id,
                    "production/reference",
                    "reference_plan_set",
                    target_business_key("character_identity_anchor"),
                    "dependency-target",
                ),
                "selected_asset_version_ref": owner_ref(
                    workspace_id,
                    project_id,
                    "asset",
                    "asset_base_reference_set",
                    "asset:one",
                    "dependency-asset",
                ),
            }
        ]
    source_refs = {
        "identity": [
            owner_ref(
                workspace_id,
                project_id,
                "asset",
                "asset_identity_state_set",
                "identity:one",
                "identity",
            )
        ],
        "specification": [
            owner_ref(
                workspace_id,
                project_id,
                "production/bible",
                "bible_production_world_set",
                "specification:one",
                "specification",
            )
        ],
        "state": [
            owner_ref(
                workspace_id, project_id, "asset", "asset_identity_state_set", "state:one", "state"
            )
        ],
        "scene": [
            owner_ref(
                workspace_id,
                project_id,
                "production/planning",
                "planning_scene_set",
                "scene:one",
                "scene",
            )
        ],
        "occurrence": [
            owner_ref(
                workspace_id,
                project_id,
                "production/planning",
                "planning_scene_set",
                "occurrence:one",
                "occurrence",
            )
        ],
        "interaction": [],
    }
    if target_kind == "interaction_composition":
        source_refs["interaction"] = [
            owner_ref(
                workspace_id,
                project_id,
                "production/planning",
                "planning_scene_set",
                "interaction:one",
                "interaction",
            )
        ]
    return {
        "workspace_id": workspace_id,
        "project_id": project_id,
        "approved_reference_plan_version_ref": owner_ref(
            workspace_id,
            project_id,
            "production/reference",
            "reference_plan_set",
            "reference-plan:one",
            "plan",
        ),
        "reference_plan_target_ref": owner_ref(
            workspace_id,
            project_id,
            "production/reference",
            "reference_plan_set",
            target_key,
            "target",
        ),
        "target_business_key": target_key,
        "target_kind": target_kind,
        "visual_foundation_version_ref": owner_ref(
            workspace_id,
            project_id,
            "preset",
            "preset_effective_set",
            project_id,
            "foundation",
        ),
        "effective_style_snapshot_ref": owner_ref(
            workspace_id,
            project_id,
            "preset",
            "preset_effective_set",
            project_id + ":style",
            "style",
        ),
        "effective_policy_snapshot_ref": owner_ref(
            workspace_id,
            project_id,
            "preset",
            "preset_effective_set",
            project_id + ":policy",
            "policy",
        ),
        "dependency_selections": dependencies,
        "stage_release": {
            "stage_key": "compile_reference_brief",
            "stage_release_hash": digest("stage-release"),
        },
        "typed_read_set_root": digest("read-set"),
        "source_design_slots": [
            {
                "slot_key": "primary_form",
                "source_requirement": "preserve confirmed production fact",
                "design_requirement": "apply the approved visual policy only",
            }
        ],
        "positive_instructions": ["preserve exact approved identity and state"],
        "negative_instructions": ["do not add unapproved identities or props"],
        "required_view_roles": view_roles(target_kind),
        "layout_requirements": ["keep every required view independently assessable"],
        "scale_requirements": ["preserve approved relative scale"],
        "rights_requirements": ["use only authorized source and dependency material"],
        "provenance_requirements": ["preserve exact source and dependency lineage"],
        "qc_rubric_refs": [
            {
                "contract_id": "reference-visual-qc-production",
                "content_hash": digest("qc-rubric"),
            }
        ],
        "source_refs": source_refs,
        "brief": purpose_brief(target_kind),
    }


def purpose_brief(target_kind: str) -> dict[str, Any]:
    if target_kind == "character_identity_anchor":
        return {
            "target_kind": target_kind,
            "identity_invariant_slots": identity_invariants(),
        }
    if target_kind == "character_appearance":
        return {
            "target_kind": target_kind,
            "identity_invariant_slots": identity_invariants(),
            "approved_variable_slots": ["wardrobe"],
        }
    if target_kind == "location_board":
        return {
            "target_kind": target_kind,
            "topology_constraints": ["preserve entrance and exit topology"],
            "scale_anchors": ["preserve confirmed human scale"],
            "material_slots": ["approved wall material"],
            "occupancy_policy": "empty",
        }
    if target_kind == "prop_sheet":
        return {
            "target_kind": target_kind,
            "physical_dimensions": "preserve confirmed dimensions",
            "structural_slots": ["outer structure"],
            "state_slots": ["approved state"],
            "content_or_mechanism_slots": ["approved mechanism"],
            "occupancy_policy": "no_hands_no_people",
        }
    if target_kind == "scene_composition":
        return {
            "target_kind": target_kind,
            "composition_purpose": "show the approved scene relationship",
            "spatial_constraints": ["preserve approved occurrence positions"],
        }
    return {
        "target_kind": target_kind,
        "hand_side": "right",
        "grip_or_contact_point": "approved handle",
        "orientation": "toward the approved counterparty",
        "body_prop_scale_constraints": ["preserve approved body-to-prop scale"],
        "transfer_or_use_state": "held",
    }


def view_roles(target_kind: str) -> list[str]:
    return {
        "character_appearance": ["back", "front", "profile"],
        "character_identity_anchor": ["back", "front", "profile"],
        "interaction_composition": ["interaction_master"],
        "location_board": ["empty_establishing", "material_scale_detail", "spatial_orientation"],
        "prop_sheet": ["back", "front", "side", "state_detail"],
        "scene_composition": ["composition_master"],
    }[target_kind]


def identity_invariants() -> list[str]:
    return ["body_shape", "facial_structure", "hair", "permanent_marks", "proportions"]


def target_business_key(target_kind: str) -> str:
    reference = ["asset", "asset_identity_state_set", "subject:one", ""]
    count = {
        "character_appearance": 3,
        "character_identity_anchor": 1,
        "interaction_composition": 1,
        "location_board": 3,
        "prop_sheet": 3,
        "scene_composition": 1,
    }[target_kind]
    return json.dumps(
        [target_kind] + [copy.deepcopy(reference) for _ in range(count)], separators=(",", ":")
    )


def owner_ref(
    workspace_id: str,
    project_id: str,
    owner_kind: str,
    version_family: str,
    logical_id: str,
    seed: str,
) -> dict[str, Any]:
    tail = hashlib.sha256(seed.encode()).hexdigest()[:12]
    return {
        "workspace_id": workspace_id,
        "project_id": project_id,
        "owner_kind": owner_kind,
        "version_family": version_family,
        "owner_logical_id": logical_id,
        "owner_version_id": f"00000000-0000-4000-8000-{tail}",
        "owner_revision": 1,
        "owner_content_hash": digest(seed + "-content"),
        "fragment_key": None,
        "fragment_content_hash": None,
    }


def digest(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()
