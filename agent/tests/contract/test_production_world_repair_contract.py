from __future__ import annotations

import copy
from uuid import uuid4

import pytest
from pydantic import ValidationError

from app.harness.scene_analysis_schemas import ProductionWorldRepairDirective
from app.protocol.canonical import production_canonical_hash


def repair_payload(
    operation: str = "revise_production_entity",
    stage: str = "derive_production_entities",
) -> dict[str, object]:
    candidate = {"stage": stage, "value": "frozen"}
    candidate_type = {
        "derive_production_entities": "production_entity_fragment_candidate",
        "bind_scene_occurrences": "scene_binding_fragment_candidate",
        "reconcile_interaction_continuity": "continuity_fragment_candidate",
    }[stage]
    return {
        "review_decision_id": str(uuid4()),
        "decision_payload_hash": "a" * 64,
        "issue_refs": [],
        "evidence_refs": [
            {
                "source_version_id": str(uuid4()),
                "source_start": 0,
                "source_end": 2,
                "text_hash": "b" * 64,
            }
        ],
        "change_spec": {
            "operation": operation,
            "target_keys": ["character:linzhou"],
        },
        "closure": {
            "scene_scope_keys": ["scene:0001"],
            "entity_keys": ["character:linzhou"],
            "state_keys": ["state_character_linzhou_initial"],
            "occurrence_keys": ["occurrence_scene_0001_character"],
            "interaction_keys": [],
            "continuity_keys": [],
            "ledger_keys": ["ledger_character_scene_0001"],
        },
        "reason_code": "production_entity_incorrect",
        "base_candidate": {
            "identity": {
                "stage_key": stage,
                "shard_key": "script:full",
                "candidate_revision_id": str(uuid4()),
                "candidate_revision_hash": "c" * 64,
                "source_invocation_id": str(uuid4()),
                "source_result_hash": "d" * 64,
            },
            "candidate_type": candidate_type,
            "candidate_content_hash": production_canonical_hash(candidate),
            "candidate": candidate,
        },
    }


def test_production_world_repair_directive_binds_stage_base_and_closure() -> None:
    directive = ProductionWorldRepairDirective.model_validate(repair_payload())
    directive.validate_for("derive_production_entities")

    for path, value in (
        (("change_spec", "target_keys"), ["character:outside"]),
        (("closure", "scene_scope_keys"), None),
        (("base_candidate", "candidate_content_hash"), "f" * 64),
        (("reason_code",), "continuity_incorrect"),
        (("evidence_refs",), []),
    ):
        changed = copy.deepcopy(repair_payload())
        target = changed
        for key in path[:-1]:
            target = target[key]  # type: ignore[index,assignment]
        target[path[-1]] = value  # type: ignore[index]
        with pytest.raises(ValidationError):
            ProductionWorldRepairDirective.model_validate(changed)


def test_production_world_repair_directive_rejects_stage_before_root() -> None:
    payload = repair_payload("rebind_scene_occurrence", "bind_scene_occurrences")
    payload["change_spec"] = {
        "operation": "rebind_scene_occurrence",
        "target_keys": ["occurrence_scene_0001_character"],
    }
    payload["reason_code"] = "scene_occurrence_incorrect"
    directive = ProductionWorldRepairDirective.model_validate(payload)
    with pytest.raises(ValueError, match="another stage"):
        directive.validate_for("derive_production_entities")
