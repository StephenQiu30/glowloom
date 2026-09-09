from __future__ import annotations

from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.harness.scene_analysis_schemas import StructureIdentityRepairDirective


def repair_payload() -> dict[str, object]:
    return {
        "review_decision_id": "10000000-0000-4000-8000-000000000001",
        "decision_payload_hash": "a" * 64,
        "issue_refs": ["issue_identity_ambiguous"],
        "evidence_refs": [
            {
                "source_version_id": "20000000-0000-4000-8000-000000000001",
                "source_start": 8,
                "source_end": 10,
                "text_hash": "b" * 64,
            }
        ],
        "change_spec": {
            "operation": "resolve_mention",
            "target_keys": ["mention:8:10"],
            "affected_scope_keys": ["scene:30000000-0000-4000-8000-000000000001"],
        },
        "reason_code": "identity_resolution_incorrect",
    }


def test_repair_directive_is_bound_to_its_exact_stage() -> None:
    directive = StructureIdentityRepairDirective.model_validate(repair_payload())
    directive.validate_for("resolve_identities")
    with pytest.raises(ValueError, match="another Scene Analysis stage"):
        directive.validate_for("propose_script_spans")


def test_repair_directive_rejects_free_form_or_expanded_scope() -> None:
    payload = repair_payload()
    payload["user_note"] = "rewrite everything"
    with pytest.raises(ValidationError):
        StructureIdentityRepairDirective.model_validate(payload)

    payload = repair_payload()
    change = dict(cast(dict[str, Any], payload["change_spec"]))
    change["affected_scope_keys"] = ["project:30000000-0000-4000-8000-000000000001"]
    payload["change_spec"] = change
    with pytest.raises(ValidationError):
        StructureIdentityRepairDirective.model_validate(payload)
