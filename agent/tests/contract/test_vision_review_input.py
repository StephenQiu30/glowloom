from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any

import pytest

from app.modules.storygraph.reference_brief_contract import REFERENCE_BRIEF_VIEW_ROLES
from app.modules.storygraph.vision_review_input import decode_vision_review_input
from app.protocol.canonical import production_canonical_hash
from tests.contract.test_reference_brief_contract import (
    reference_brief_candidate,
    reference_brief_input,
)


def test_vision_review_input_shared_canonical_identity() -> None:
    path = (
        Path(__file__).resolve().parents[3]
        / "backend/tests/agent/testdata/vision_review_input.json"
    )
    raw = path.read_bytes()
    value = decode_vision_review_input(raw)
    assert json.loads(value.model_dump_json()) == json.loads(raw)
    assert value.subject.stage_release_hash != value.brief_input.stage_release.stage_release_hash
    assert (
        value.subject.input_hash
        == "9702a0c7bd5822c091597da38ab2c76a21b9f49ac03795dc11ba607cbcf09999"
    )


@pytest.mark.parametrize("kind", list(REFERENCE_BRIEF_VIEW_ROLES))
def test_vision_review_input_base_purposes_and_dependency_closure(kind: str) -> None:
    path = (
        Path(__file__).resolve().parents[3]
        / "backend/tests/agent/testdata/vision_review_input.json"
    )
    value = json.loads(path.read_bytes())
    candidate = reference_brief_candidate(kind)
    value["brief_candidate"], value["brief_input"] = candidate, reference_brief_input(candidate)
    value["brief_content_hash"] = production_canonical_hash(candidate)
    subject = value["subject"]
    subject.update(
        workspace_id=candidate["workspace_id"],
        project_id=candidate["project_id"],
        target_kind=kind,
        slots=[],
    )
    value["visual_context"]["purpose_profile"]["target_kind"] = kind
    sample = value["attachments"][0]
    value["attachments"] = []
    for index, role in enumerate(REFERENCE_BRIEF_VIEW_ROLES[kind]):
        attachment = copy.deepcopy(sample)
        attachment["slot"].update(slot_key=role, view_role=role, sha256="abcd"[index] * 64)
        attachment["slot"]["staged_media_ref"]["id"] = f"00000000-0000-4000-8000-{index + 21:012d}"
        attachment["provider_receipt_ref"]["id"] = attachment["slot"]["staged_media_ref"]["id"]
        subject["slots"].append(attachment["slot"])
        value["attachments"].append(attachment)
    del subject["input_hash"]
    subject["input_hash"] = production_canonical_hash(value)
    if kind == "character_appearance" or kind.endswith("_composition"):
        with pytest.raises(ValueError, match="dependency assets"):
            decode_vision_review_input(json.dumps(value))
    else:
        decode_vision_review_input(json.dumps(value))


@pytest.mark.parametrize(
    "field",
    [
        "contract_id",
        "input_hash",
        "scope",
        "round",
        "brief_hash",
        "brief_content",
        "admission",
        "selection",
        "publication",
        "rights",
        "grammar",
        "policy",
        "purpose",
        "empty_rules",
        "missing_attachment",
        "digest",
        "missing_boolean",
        "null_boolean",
        "unknown",
        "private_path",
    ],
)
def test_vision_review_input_rejects_drift_and_forged_shape(field: str) -> None:
    path = (
        Path(__file__).resolve().parents[3]
        / "backend/tests/agent/testdata/vision_review_input.json"
    )
    value: dict[str, Any] = json.loads(path.read_bytes())
    subject, visual, admission = value["subject"], value["visual_context"], value["admission"]
    if field == "contract_id":
        value["contract_id"] = "other"
    elif field == "input_hash":
        subject["input_hash"] = "invalid"
    elif field == "scope":
        subject["project_id"] = "00000000-0000-4000-8000-000000000099"
    elif field == "round":
        subject["generation_round"] = 2
    elif field == "brief_hash":
        value["brief_content_hash"] = subject["stage_release_hash"]
    elif field == "brief_content":
        value["brief_candidate"]["positive_instructions"] = ["changed"]
    elif field == "admission":
        admission["internal_review_ready"] = False
    elif field == "selection":
        admission["selection_ready"] = True
    elif field == "publication":
        admission["publication_ready"] = True
    elif field == "rights":
        admission["formal_use_blockers"] = ["vision_review_required"]
    elif field == "grammar":
        visual["visual_grammar"]["medium"] = ""
    elif field == "policy":
        visual["style_policy"]["forbidden_changes"] = []
    elif field == "purpose":
        visual["purpose_profile"]["target_kind"] = "prop_sheet"
    elif field == "empty_rules":
        visual["visual_grammar"]["negative_constraints"] = []
    elif field == "missing_attachment":
        value["attachments"] = value["attachments"][:1]
    elif field == "digest":
        value["attachments"][0]["slot"]["sha256"] = subject["stage_release_hash"]
    elif field == "missing_boolean":
        del admission["selection_ready"]
    elif field == "null_boolean":
        admission["selection_ready"] = None
    elif field == "unknown":
        value["approved"] = True
    else:
        value["attachments"][0]["path"] = "/private/image.png"
    with pytest.raises(ValueError):
        decode_vision_review_input(json.dumps(value))
    if field != "input_hash":
        del subject["input_hash"]
        subject["input_hash"] = production_canonical_hash(value)
        with pytest.raises(ValueError):
            decode_vision_review_input(json.dumps(value))
