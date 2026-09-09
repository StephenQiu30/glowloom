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
    candidate = production_candidate(stage)
    candidate_type = {
        "derive_production_entities": "production_entity_fragment_candidate",
        "bind_scene_occurrences": "scene_binding_fragment_candidate",
        "reconcile_interaction_continuity": "continuity_fragment_candidate",
    }[stage]
    target, reason = {
        "revise_production_entity": ("character:linzhou", "production_entity_incorrect"),
        "rebind_scene_occurrence": (
            "occurrence_scene_0001_character",
            "scene_occurrence_incorrect",
        ),
        "revise_interaction": ("interaction_one", "interaction_incorrect"),
        "revise_continuity": ("continuity_one", "continuity_incorrect"),
    }[operation]
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
            "target_keys": [target],
        },
        "closure": {
            "scene_scope_keys": ["scene:0001"],
            "entity_keys": ["character:linzhou"],
            "state_keys": ["state_character_linzhou_initial"],
            "occurrence_keys": ["occurrence_scene_0001_character"],
            "interaction_keys": ["interaction_one"],
            "continuity_keys": ["continuity_one"],
            "ledger_keys": ["ledger_character_scene_0001"],
        },
        "reason_code": reason,
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
    directive = ProductionWorldRepairDirective.model_validate(payload)
    with pytest.raises(ValueError, match="another stage"):
        directive.validate_for("derive_production_entities")


@pytest.mark.parametrize(
    ("stage", "operation"),
    [
        ("derive_production_entities", "revise_production_entity"),
        ("bind_scene_occurrences", "rebind_scene_occurrence"),
        ("reconcile_interaction_continuity", "revise_interaction"),
        ("reconcile_interaction_continuity", "revise_continuity"),
    ],
)
def test_production_world_repair_candidate_preserves_unauthorized_content(
    stage: str,
    operation: str,
) -> None:
    directive = ProductionWorldRepairDirective.model_validate(repair_payload(operation, stage))
    authorized = copy.deepcopy(production_candidate(stage))
    forbidden = copy.deepcopy(production_candidate(stage))
    with pytest.raises(ValueError, match="did not change"):
        directive.validate_candidate_for(stage, copy.deepcopy(authorized))  # type: ignore[arg-type]
    if operation == "revise_production_entity":
        authorized["entities"][0]["description"] = "修正后"  # type: ignore[index]
        forbidden["entities"][1]["description"] = "越界修改"  # type: ignore[index]
    elif operation == "rebind_scene_occurrence":
        authorized["scenes"][0]["occurrences"][0]["state_key"] = (  # type: ignore[index]
            "state_character_corrected"
        )
        forbidden["scenes"][0]["dialogues"] = [  # type: ignore[index]
            {"dialogue_key": "dialogue_added"}
        ]
    elif operation == "revise_interaction":
        authorized["interactions"][0]["predicate"] = "use"  # type: ignore[index]
        authorized["continuity"][0]["delta"] = "修正"  # type: ignore[index]
        authorized["continuity_ledger"][0]["state_key"] = (  # type: ignore[index]
            "state_prop_corrected"
        )
        forbidden["scene_story_times"][0]["story_time_key"] = (  # type: ignore[index]
            "storytime:changed"
        )
    else:
        authorized["continuity"][0]["delta"] = "修正"  # type: ignore[index]
        forbidden["interactions"][0]["predicate"] = "use"  # type: ignore[index]

    directive.validate_candidate_for(stage, authorized)  # type: ignore[arg-type]
    with pytest.raises(ValueError, match="outside its authorized closure"):
        directive.validate_candidate_for(stage, forbidden)  # type: ignore[arg-type]


def production_candidate(stage: str) -> dict[str, object]:
    if stage == "derive_production_entities":
        return {
            "source_hash": "frozen",
            "entities": [
                {
                    "identity_key": "character:linzhou",
                    "kind": "character",
                    "specification_key": "specification_character_linzhou",
                    "description": "原值",
                    "states": [
                        {
                            "state_key": "state_character_linzhou_initial",
                            "value": "原值",
                        }
                    ],
                },
                {
                    "identity_key": "prop:key",
                    "kind": "prop",
                    "specification_key": "specification_prop_key",
                    "description": "原值",
                    "states": [{"state_key": "state_prop_key_initial", "value": "原值"}],
                },
            ],
            "world_claims": [],
            "design_gaps": [],
            "review_issues": [],
        }
    if stage == "bind_scene_occurrences":
        return {
            "source_hash": "frozen",
            "scenes": [
                {
                    "scene_scope_key": "scene:0001",
                    "dialogues": [],
                    "beats": [],
                    "occurrences": [
                        {
                            "occurrence_key": "occurrence_scene_0001_character",
                            "state_key": "state_character_linzhou_initial",
                        }
                    ],
                }
            ],
            "review_issues": [],
        }
    return {
        "source_hash": "frozen",
        "scene_story_times": [{"scene_scope_key": "scene:0001", "story_time_key": "storytime:one"}],
        "interactions": [
            {"interaction_key": "interaction_one", "predicate": "hold"},
            {"interaction_key": "interaction_two", "predicate": "carry"},
        ],
        "continuity": [
            {"continuity_key": "continuity_one", "delta": "原值"},
            {"continuity_key": "continuity_two", "delta": "原值"},
        ],
        "continuity_ledger": [
            {
                "ledger_key": "ledger_character_scene_0001",
                "state_key": "state_character_linzhou_initial",
            },
            {"ledger_key": "ledger_other", "state_key": "state_other"},
        ],
        "review_issues": [],
    }
