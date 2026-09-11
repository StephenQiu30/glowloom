from __future__ import annotations

import hashlib
import json
from collections.abc import Callable
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.modules.storygraph.visual_foundation_contract import (
    VisualFoundationCandidate,
    VisualFoundationInput,
    visual_foundation_schema_manifest,
)
from app.protocol.canonical import production_canonical_hash

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
SCHEMA_FIXTURE = REPOSITORY_ROOT.joinpath(
    "backend", "tests", "fixtures", "agent", "storygraph-visual-foundation-schemas.json"
)


def test_visual_foundation_candidate_binds_preset_world_and_design_gaps() -> None:
    stage_input = VisualFoundationInput.model_validate(valid_input())
    candidate = VisualFoundationCandidate.model_validate(valid_candidate(stage_input))
    candidate.validate_for(stage_input)


def test_visual_foundation_schema_manifest_matches_backend_fixture() -> None:
    assert visual_foundation_schema_manifest() == json.loads(
        SCHEMA_FIXTURE.read_text(encoding="utf-8")
    )


def _add_approved(value: dict[str, Any]) -> None:
    value["approved"] = True


def _remove_forbidden_change(value: dict[str, Any]) -> None:
    policy = cast(dict[str, Any], value["style_policy"])
    changes = cast(list[Any], policy["forbidden_changes"])
    changes.pop()


def _remove_design_gap(value: dict[str, Any]) -> None:
    value["creative_fill_proposals"] = []


def _remove_conflict_collection(value: dict[str, Any]) -> None:
    del value["world_conflicts"]


def _invent_source_fact(value: dict[str, Any]) -> None:
    adaptations = cast(list[Any], value["world_adaptations"])
    adaptation = cast(dict[str, Any], adaptations[0])
    adaptation["source_fact_key"] = "fact_invented"


def _relax_adaptation_invariants(value: dict[str, Any]) -> None:
    adaptations = cast(list[Any], value["world_adaptations"])
    adaptation = cast(dict[str, Any], adaptations[0])
    adaptation["preserved_invariant_keys"] = ["story_fact"]


@pytest.mark.parametrize(
    "mutate",
    [
        _add_approved,
        _remove_forbidden_change,
        _remove_design_gap,
        _remove_conflict_collection,
        _invent_source_fact,
        _relax_adaptation_invariants,
    ],
)
def test_visual_foundation_candidate_rejects_unsafe_or_incomplete_output(
    mutate: Callable[[dict[str, Any]], None],
) -> None:
    stage_input = VisualFoundationInput.model_validate(valid_input())
    payload = valid_candidate(stage_input)
    mutate(payload)
    with pytest.raises((ValidationError, ValueError)):
        candidate = VisualFoundationCandidate.model_validate(payload)
        candidate.validate_for(stage_input)


def test_faithful_visual_foundation_rejects_world_adaptation() -> None:
    payload = valid_input()
    payload["application_mode"] = "faithful"
    stage_input = VisualFoundationInput.model_validate(payload)
    candidate = VisualFoundationCandidate.model_validate(valid_candidate(stage_input))
    with pytest.raises(ValueError, match="faithful"):
        candidate.validate_for(stage_input)


def valid_input() -> dict[str, object]:
    overrides = [
        {
            "override_key": "override_architecture_detail",
            "scope_kind": "project",
            "scope_key": "project:00000000-0000-0000-0000-000000000001",
            "design_domain": "architecture",
            "value": "保留原场景动线，以木构细节替换未指定表皮。",
            "creator_decision_hash": digest("decision"),
        }
    ]
    attachments = [
        {
            "attachment_id": "00000000-0000-0000-0000-000000000030",
            "object_key": "visual-references/project-1/courtyard.webp",
            "content_hash": digest("attachment"),
            "media_type": "image/webp",
            "rights_basis": "owned",
            "rights_ref_hash": digest("rights"),
        }
    ]
    return {
        "workspace_id": "00000000-0000-0000-0000-000000000010",
        "project_id": "00000000-0000-0000-0000-000000000001",
        "production_world_owner_set_hash": digest("owner-set"),
        "confirmed_world_roots": [
            {
                "owner_family": "asset_identity_state_set",
                "scope_key": "project:00000000-0000-0000-0000-000000000001",
                "collection_root_hash": digest("asset-root"),
            },
            {
                "owner_family": "bible_production_world_set",
                "scope_key": "project:00000000-0000-0000-0000-000000000001",
                "collection_root_hash": digest("bible-root"),
            },
            {
                "owner_family": "planning_scene_set",
                "scope_key": "episode:00000000-0000-0000-0000-000000000020",
                "collection_root_hash": digest("planning-root"),
            },
        ],
        "preset_release": {
            "key": "xianxia-animation",
            "release": "2026.09.12",
            "content_hash": digest("preset"),
        },
        "application_mode": "world_adaptation",
        "fidelity_invariants": [
            "character_identity",
            "holder_relation",
            "scene_continuity",
            "story_fact",
        ],
        "adaptation_rules": [
            {
                "rule_key": "translate-technology-language",
                "source_fact_kind": "technology_or_magic",
                "design_domain": "technology_or_magic",
                "directive": "translate_expression_without_changing_function_or_outcome",
                "preserved_invariant_keys": [
                    "character_identity",
                    "holder_relation",
                    "scene_continuity",
                    "story_fact",
                ],
                "impact_scope_kinds": ["asset", "interaction", "scene"],
            }
        ],
        "visual_grammar": {
            "palette": "muted_jade_and_ink",
            "material_rendering": "ink_wash_with_grounded_materials",
            "lighting": "motivated_soft_cinematic",
            "camera": "grounded_cinematic",
            "negative_constraints": ["protected_ip_imitation", "story_fact_rewrite"],
        },
        "typed_overrides": overrides,
        "typed_overrides_hash": production_canonical_hash(overrides),
        "reference_attachments": attachments,
        "reference_attachments_hash": production_canonical_hash(attachments),
        "confirmed_world_facts": [
            {
                "fact_key": "fact_phone_message",
                "fact_kind": "technology_or_magic",
                "content_hash": digest("fact"),
                "affected_scope_keys": ["scene:00000000-0000-0000-0000-000000000002"],
            }
        ],
        "design_gaps": [
            {
                "gap_key": "gap_uniform_material",
                "design_domain": "wardrobe",
                "source_constraint_hash": digest("gap"),
                "affected_scope_keys": ["scene:00000000-0000-0000-0000-000000000002"],
            }
        ],
    }


def valid_candidate(stage_input: VisualFoundationInput) -> dict[str, object]:
    return {
        "workspace_id": str(stage_input.workspace_id),
        "project_id": str(stage_input.project_id),
        "production_world_owner_set_hash": stage_input.production_world_owner_set_hash,
        "preset_release_content_hash": stage_input.preset_release.content_hash,
        "application_mode": stage_input.application_mode,
        "typed_overrides_hash": stage_input.typed_overrides_hash,
        "reference_attachments_hash": stage_input.reference_attachments_hash,
        "fidelity_invariants": stage_input.fidelity_invariants,
        "style_policy": {
            "palette_rules": ["muted_jade_and_ink"],
            "material_rules": ["grounded_material_identity", "ink_wash_surface_language"],
            "lighting_rules": ["motivated_soft_cinematic"],
            "camera_rules": ["grounded_cinematic"],
            "forbidden_changes": stage_input.fidelity_invariants,
        },
        "world_adaptations": [
            {
                "mapping_key": "mapping_phone_message",
                "source_fact_key": "fact_phone_message",
                "preset_rule_key": "translate-technology-language",
                "design_value": "手机传信改编为传音符，但发送者、接收者、信息内容与剧情结果不变。",
                "preserved_invariant_keys": stage_input.fidelity_invariants,
                "affected_scope_keys": ["scene:00000000-0000-0000-0000-000000000002"],
                "decision_status": "needs_creator_decision",
            }
        ],
        "world_conflicts": [],
        "creative_fill_proposals": [
            {
                "gap_key": "gap_uniform_material",
                "design_domain": "wardrobe",
                "proposal": "使用低反光织物并保留角色身份轮廓，具体纹样等待创作者确认。",
                "affected_scope_keys": ["scene:00000000-0000-0000-0000-000000000002"],
                "decision_status": "needs_creator_decision",
            }
        ],
    }


def digest(seed: str) -> str:
    return hashlib.sha256(seed.encode()).hexdigest()
