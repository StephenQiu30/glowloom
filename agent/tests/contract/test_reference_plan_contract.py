from __future__ import annotations

import copy
import hashlib
import json
from pathlib import Path
from typing import Any

import pytest
from pydantic import ValidationError

from app.modules.storygraph.reference_plan_contract import (
    ReferencePlanCandidate,
    ReferencePlanInput,
    reference_plan_schema_manifest,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
SCHEMA_FIXTURE = (
    REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-reference-plan-schemas.json"
)


def test_reference_plan_candidate_covers_frozen_seed_inventory() -> None:
    stage_input = ReferencePlanInput.model_validate(valid_input())
    candidate = ReferencePlanCandidate.model_validate(valid_candidate(stage_input))
    candidate.validate_for(stage_input)


def test_reference_plan_schema_manifest_matches_backend_fixture() -> None:
    assert reference_plan_schema_manifest() == json.loads(
        SCHEMA_FIXTURE.read_text(encoding="utf-8")
    )


@pytest.mark.parametrize(
    "mutation",
    ["unknown_anchor_state", "missing_target", "dependency_drift", "purpose_escape"],
)
def test_reference_plan_candidate_rejects_seed_or_specification_drift(mutation: str) -> None:
    stage_input = ReferencePlanInput.model_validate(valid_input())
    payload = valid_candidate(stage_input)
    if mutation == "unknown_anchor_state":
        payload["anchor_selections"][0]["selected_state_ref"]["owner_logical_id"] = "unknown"
    elif mutation == "missing_target":
        payload["target_specifications"].pop()
    elif mutation == "dependency_drift":
        payload["target_specifications"][0]["depends_on_target_business_keys"] = []
    else:
        payload["target_specifications"][1]["design_focus"] = ["invent_new_identity"]
    with pytest.raises((ValidationError, ValueError)):
        candidate = ReferencePlanCandidate.model_validate(payload)
        candidate.validate_for(stage_input)


def test_reference_plan_candidate_rejects_provider_field() -> None:
    stage_input = ReferencePlanInput.model_validate(valid_input())
    payload = valid_candidate(stage_input)
    payload["provider"] = "seedream"
    with pytest.raises(ValidationError):
        ReferencePlanCandidate.model_validate(payload)


def test_reference_plan_input_rejects_foundation_lineage_drift() -> None:
    payload = valid_input()
    payload["visual_foundation_candidate"]["production_world_owner_set_hash"] = digest(
        "other-owner-set"
    )
    with pytest.raises(ValidationError):
        ReferencePlanInput.model_validate(payload)


def test_reference_plan_input_rejects_pre_gate_effective_style_snapshot() -> None:
    payload = valid_input()
    payload["effective_style_snapshot_ref"] = owner_ref(
        payload["workspace_id"],
        payload["project_id"],
        "preset",
        "preset_effective_set",
        "style-1",
    )
    with pytest.raises(ValidationError):
        ReferencePlanInput.model_validate(payload)


def valid_input() -> dict[str, Any]:
    workspace_id = "00000000-0000-0000-0000-000000000010"
    project_id = "00000000-0000-0000-0000-000000000001"
    scene_scope_a = "scene:00000000-0000-0000-0000-000000000101"
    scene_scope_b = "scene:00000000-0000-0000-0000-000000000102"
    character = owner_ref(
        workspace_id, project_id, "asset", "asset_identity_state_set", "character-1"
    )
    specification = owner_ref(
        workspace_id,
        project_id,
        "production/bible",
        "bible_production_world_set",
        "character-spec-1",
    )
    state_a = owner_ref(workspace_id, project_id, "asset", "asset_identity_state_set", "state-a")
    state_b = owner_ref(workspace_id, project_id, "asset", "asset_identity_state_set", "state-b")
    scene_a = owner_ref(
        workspace_id, project_id, "production/planning", "planning_scene_set", scene_scope_a
    )
    scene_b = owner_ref(
        workspace_id, project_id, "production/planning", "planning_scene_set", scene_scope_b
    )
    occurrence_a = owner_ref(
        workspace_id, project_id, "production/planning", "planning_scene_set", "occurrence-a"
    )
    occurrence_b = owner_ref(
        workspace_id, project_id, "production/planning", "planning_scene_set", "occurrence-b"
    )
    location = owner_ref(
        workspace_id, project_id, "asset", "asset_identity_state_set", "location-1"
    )
    location_spec = owner_ref(
        workspace_id,
        project_id,
        "production/bible",
        "bible_production_world_set",
        "location-spec-1",
    )
    location_state = owner_ref(
        workspace_id, project_id, "asset", "asset_identity_state_set", "location-state-1"
    )
    location_occurrence = owner_ref(
        workspace_id,
        project_id,
        "production/planning",
        "planning_scene_set",
        "location-occurrence-1",
    )
    anchor_key = business_key("character_identity_anchor", "character-1")
    appearance_a_key = business_key(
        "character_appearance", "character-1", "character-spec-1", "state-a"
    )
    appearance_b_key = business_key(
        "character_appearance", "character-1", "character-spec-1", "state-b"
    )
    location_key = business_key(
        "location_board", "location-1", "location-spec-1", "location-state-1"
    )
    scene_a_key = business_key("scene_composition", "scene-101")
    scene_b_key = business_key("scene_composition", "scene-102")
    value: dict[str, Any] = {
        "workspace_id": workspace_id,
        "project_id": project_id,
        "production_world_owner_set_hash": digest("owner-set"),
        "p1_scope_keys": [scene_scope_a, scene_scope_b],
        "visual_foundation_candidate_revision_id": "00000000-0000-0000-0000-000000000201",
        "visual_foundation_candidate_revision_hash": digest("foundation-revision"),
        "visual_foundation_candidate": foundation_candidate(
            workspace_id, project_id, digest("owner-set")
        ),
        "character_seeds": [
            {
                "anchor_business_key": anchor_key,
                "identity_ref": character,
                "specification_ref": specification,
                "coverage_scope_keys": [scene_scope_a, scene_scope_b],
                "state_options": [
                    {
                        "state_ref": state_a,
                        "appearance_business_key": appearance_a_key,
                        "coverage_scope_keys": [scene_scope_a],
                        "occurrence_refs": [occurrence_a],
                    },
                    {
                        "state_ref": state_b,
                        "appearance_business_key": appearance_b_key,
                        "coverage_scope_keys": [scene_scope_b],
                        "occurrence_refs": [occurrence_b],
                    },
                ],
            }
        ],
        "fixed_target_seeds": [
            {
                "target_business_key": location_key,
                "target_kind": "location_board",
                "owner_refs": {
                    "identity": [location],
                    "specification": [location_spec],
                    "state": [location_state],
                    "scene": [scene_a],
                    "occurrence": [location_occurrence],
                    "interaction": [],
                },
                "coverage_scope_keys": [scene_scope_a],
                "fixed_dependency_business_keys": [],
                "character_dependencies": [],
            },
            {
                "target_business_key": scene_a_key,
                "target_kind": "scene_composition",
                "owner_refs": {
                    "identity": [character, location],
                    "specification": [specification, location_spec],
                    "state": [location_state, state_a],
                    "scene": [scene_a],
                    "occurrence": [location_occurrence, occurrence_a],
                    "interaction": [],
                },
                "coverage_scope_keys": [scene_scope_a],
                "fixed_dependency_business_keys": [location_key],
                "character_dependencies": [
                    {
                        "anchor_business_key": anchor_key,
                        "state_ref": state_a,
                        "appearance_business_key": appearance_a_key,
                    }
                ],
            },
            {
                "target_business_key": scene_b_key,
                "target_kind": "scene_composition",
                "owner_refs": {
                    "identity": [character],
                    "specification": [specification],
                    "state": [state_b],
                    "scene": [scene_b],
                    "occurrence": [occurrence_b],
                    "interaction": [],
                },
                "coverage_scope_keys": [scene_scope_b],
                "fixed_dependency_business_keys": [],
                "character_dependencies": [
                    {
                        "anchor_business_key": anchor_key,
                        "state_ref": state_b,
                        "appearance_business_key": appearance_b_key,
                    }
                ],
            },
        ],
        "purpose_profiles": purpose_profiles(),
        "reference_target_seed_root": "0" * 64,
    }
    draft = ReferencePlanInput.model_construct(**copy.deepcopy(value))
    value["reference_target_seed_root"] = draft.compute_seed_root()
    return value


def valid_candidate(stage_input: ReferencePlanInput) -> dict[str, Any]:
    anchor_key = stage_input.character_seeds[0].anchor_business_key
    appearance_b_key = stage_input.character_seeds[0].state_options[1].appearance_business_key
    location_key = stage_input.fixed_target_seeds[0].target_business_key
    scene_a_key = stage_input.fixed_target_seeds[1].target_business_key
    scene_b_key = stage_input.fixed_target_seeds[2].target_business_key
    return {
        "workspace_id": str(stage_input.workspace_id),
        "project_id": str(stage_input.project_id),
        "production_world_owner_set_hash": stage_input.production_world_owner_set_hash,
        "p1_scope_keys": stage_input.p1_scope_keys,
        "visual_foundation_candidate_revision_id": str(
            stage_input.visual_foundation_candidate_revision_id
        ),
        "visual_foundation_candidate_revision_hash": (
            stage_input.visual_foundation_candidate_revision_hash
        ),
        "reference_target_seed_root": stage_input.reference_target_seed_root,
        "anchor_selections": [
            {
                "anchor_business_key": anchor_key,
                "selected_state_ref": stage_input.character_seeds[0]
                .state_options[0]
                .state_ref.model_dump(mode="json"),
            }
        ],
        "target_specifications": [
            specification(
                appearance_b_key,
                "character_appearance",
                "optional",
                ["state_readability"],
                [anchor_key],
            ),
            specification(
                anchor_key,
                "character_identity_anchor",
                "required",
                ["identity_readability", "silhouette"],
                [],
            ),
            specification(location_key, "location_board", "required", ["spatial_readability"], []),
            specification(
                scene_a_key,
                "scene_composition",
                "required",
                ["composition"],
                [anchor_key, location_key],
            ),
            specification(
                scene_b_key, "scene_composition", "optional", ["composition"], [appearance_b_key]
            ),
        ],
    }


def specification(
    key: str, kind: str, fulfillment: str, focus: list[str], dependencies: list[str]
) -> dict[str, Any]:
    return {
        "target_business_key": key,
        "target_kind": kind,
        "fulfillment": fulfillment,
        "design_focus": focus,
        "forbidden_changes": ["story_fact"],
        "depends_on_target_business_keys": dependencies,
    }


def purpose_profiles() -> list[dict[str, Any]]:
    return [
        {
            "target_kind": "character_appearance",
            "design_focus": ["state_readability"],
            "forbidden_changes": ["story_fact"],
        },
        {
            "target_kind": "character_identity_anchor",
            "design_focus": ["identity_readability", "silhouette"],
            "forbidden_changes": ["story_fact"],
        },
        {
            "target_kind": "interaction_composition",
            "design_focus": ["contact_geometry"],
            "forbidden_changes": ["story_fact"],
        },
        {
            "target_kind": "location_board",
            "design_focus": ["spatial_readability"],
            "forbidden_changes": ["story_fact"],
        },
        {
            "target_kind": "prop_sheet",
            "design_focus": ["state_readability"],
            "forbidden_changes": ["story_fact"],
        },
        {
            "target_kind": "scene_composition",
            "design_focus": ["composition"],
            "forbidden_changes": ["story_fact"],
        },
    ]


def foundation_candidate(workspace_id: str, project_id: str, owner_set_hash: str) -> dict[str, Any]:
    return {
        "workspace_id": workspace_id,
        "project_id": project_id,
        "production_world_owner_set_hash": owner_set_hash,
        "preset_release_content_hash": digest("preset"),
        "application_mode": "faithful",
        "typed_overrides_hash": digest("typed-overrides"),
        "reference_attachments_hash": digest("reference-attachments"),
        "fidelity_invariants": [
            "character_identity",
            "holder_relation",
            "scene_continuity",
            "story_fact",
        ],
        "style_policy": {
            "palette_rules": ["muted_jade_and_ink"],
            "material_rules": ["grounded_material_identity"],
            "lighting_rules": ["motivated_soft_cinematic"],
            "camera_rules": ["grounded_cinematic"],
            "forbidden_changes": [
                "character_identity",
                "holder_relation",
                "scene_continuity",
                "story_fact",
            ],
        },
        "world_adaptations": [],
        "world_conflicts": [],
        "creative_fill_proposals": [],
    }


def owner_ref(
    workspace_id: str, project_id: str, owner_kind: str, family: str, logical_id: str
) -> dict[str, Any]:
    return {
        "workspace_id": workspace_id,
        "project_id": project_id,
        "owner_kind": owner_kind,
        "version_family": family,
        "owner_logical_id": logical_id,
        "owner_version_id": "00000000-0000-0000-0000-000000000301",
        "owner_revision": 1,
        "owner_content_hash": digest(owner_kind + family + logical_id),
        "fragment_key": None,
        "fragment_content_hash": None,
    }


def business_key(kind: str, *parts: str) -> str:
    return json.dumps(
        [kind, *[["owner", "family", part, ""] for part in parts]], separators=(",", ":")
    )


def digest(seed: str) -> str:
    return hashlib.sha256(seed.encode()).hexdigest()
