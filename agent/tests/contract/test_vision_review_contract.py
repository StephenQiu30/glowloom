from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any

import pytest

from app.modules.storygraph.reference_brief_contract import REFERENCE_BRIEF_VIEW_ROLES
from app.modules.storygraph.vision_review_contract import (
    VisionReviewCandidate,
    decode_vision_review_candidate,
)
from app.protocol.canonical import production_canonical_hash


def review_document() -> dict[str, Any]:
    path = (
        Path(__file__).resolve().parents[3]
        / "backend/tests/agent/testdata/vision_review_candidate.json"
    )
    return json.loads(path.read_text(encoding="utf-8"))


def test_vision_review_shared_golden_and_frozen_subject() -> None:
    payload = review_document()
    candidate = VisionReviewCandidate.model_validate(payload)
    candidate.validate_for(candidate.subject)
    changed = candidate.subject.model_copy(update={"input_hash": "b" * 64})
    with pytest.raises(ValueError):
        candidate.validate_for(changed)
    assert production_canonical_hash(payload) == production_canonical_hash(
        candidate.model_dump(mode="json")
    )
    assert decode_vision_review_candidate(json.dumps(payload)) == candidate
    assert (
        production_canonical_hash(payload)
        == "a536d9ff3a5b82fbc3ce10ef02509bd5908710ef41c2919e14e045acfa86f340"
    )


@pytest.mark.parametrize("kind", list(REFERENCE_BRIEF_VIEW_ROLES))
def test_vision_review_all_target_purposes(kind: str) -> None:
    value = review_document()
    subject = value["subject"]
    sample = subject["slots"][0]
    subject["target_kind"] = kind
    subject["slots"] = []
    for index, role in enumerate(REFERENCE_BRIEF_VIEW_ROLES[kind]):
        slot = copy.deepcopy(sample)
        slot.update(slot_key=role, view_role=role, sha256="abcd"[index] * 64)
        slot["staged_media_ref"]["id"] = f"00000000-0000-4000-8000-{21 + index:012d}"
        subject["slots"].append(slot)
    for check in value["checks"]:
        check["evidence"] = [
            {
                "slot_key": slot["slot_key"],
                "region": {"x": 0, "y": 0, "width": 10000, "height": 10000},
            }
            for slot in subject["slots"]
        ]
    VisionReviewCandidate.model_validate(value)


@pytest.mark.parametrize(
    "field",
    [
        "project_id",
        "brief_revision_hash",
        "stage_release_hash",
        "bundle_input_ref",
        "execution_ref",
        "media_sha256",
    ],
)
def test_vision_review_rejects_subject_drift(field: str) -> None:
    candidate = VisionReviewCandidate.model_validate(review_document())
    subject = candidate.subject.model_copy(deep=True)
    if field == "project_id":
        subject.project_id = "00000000-0000-4000-8000-000000000099"
    elif field == "brief_revision_hash":
        subject.brief_revision_hash = "b" * 64
    elif field == "stage_release_hash":
        subject.stage_release_hash = "b" * 64
    elif field == "bundle_input_ref":
        subject.bundle_input_ref.content_hash = "b" * 64
    elif field == "execution_ref":
        subject.execution_ref.content_hash = "b" * 64
    else:
        subject.slots[0].sha256 = "d" * 64
    with pytest.raises(ValueError):
        candidate.validate_for(subject)


@pytest.mark.parametrize("field", ["confidence_bps", "recommendation", "x"])
def test_vision_review_requires_zero_valued_fields(field: str) -> None:
    value = review_document()
    check = value["checks"][0]
    target = check["evidence"][0]["region"] if field == "x" else check
    del target[field]
    with pytest.raises(ValueError):
        decode_vision_review_candidate(json.dumps(value))


@pytest.mark.parametrize("status", ["warn", "fail"])
def test_vision_review_preserves_issue_status(status: str) -> None:
    value = review_document()
    value["checks"][0].update(
        status=status, issue_code="identity_drift", recommendation="核对人物身份特征"
    )
    assert VisionReviewCandidate.model_validate(value).checks[0].status == status


@pytest.mark.parametrize(
    "mutation",
    [
        "missing_category",
        "duplicate_category",
        "reordered",
        "unknown_slot",
        "missing_pass_view",
        "outside_region",
        "zero_area",
        "duplicate_region",
        "negative_confidence",
        "over_confidence",
        "pass_issue",
        "fail_without_evidence",
        "warn_without_advice",
        "unknown_status",
        "mixed_media",
        "wrong_view",
        "incomplete_views",
        "current_ref",
        "empty_input_hash",
        "selected",
        "extra_nested",
    ],
)
def test_vision_review_rejects_invalid_output(mutation: str):
    value = copy.deepcopy(review_document())
    subject, checks = value["subject"], value["checks"]
    first = checks[0]
    evidence = first["evidence"]
    region = evidence[0]["region"]
    slots = subject["slots"]
    if mutation == "missing_category":
        value["checks"] = checks[1:]
    elif mutation == "duplicate_category":
        checks[1] = checks[0]
    elif mutation == "reordered":
        checks[0], checks[1] = checks[1], checks[0]
    elif mutation == "unknown_slot":
        evidence[0]["slot_key"] = "other_bundle"
    elif mutation == "missing_pass_view":
        first["evidence"] = evidence[1:]
    elif mutation == "outside_region":
        region["x"] = 5000
    elif mutation == "zero_area":
        region["width"] = 0
    elif mutation == "duplicate_region":
        first["evidence"].append(evidence[0])
    elif mutation == "negative_confidence":
        first["confidence_bps"] = -1
    elif mutation == "over_confidence":
        first["confidence_bps"] = 10001
    elif mutation == "pass_issue":
        first["issue_code"] = "identity_drift"
    elif mutation == "fail_without_evidence":
        first.update(
            status="fail", issue_code="identity_drift", recommendation="修复身份", evidence=[]
        )
    elif mutation == "warn_without_advice":
        first.update(status="warn", issue_code="identity_uncertain")
    elif mutation == "unknown_status":
        first["status"] = "eligible"
    elif mutation == "mixed_media":
        slots[1]["staged_media_ref"] = slots[0]["staged_media_ref"]
    elif mutation == "wrong_view":
        slots[0]["view_role"] = "front"
    elif mutation == "incomplete_views":
        subject["slots"] = slots[1:]
    elif mutation == "current_ref":
        subject["execution_ref"]["id"] = "current"
    elif mutation == "empty_input_hash":
        subject["input_hash"] = ""
    elif mutation == "selected":
        value["selected"] = True
    else:
        region["prompt"] = "override"
    with pytest.raises(ValueError):
        VisionReviewCandidate.model_validate(value)


def test_vision_review_preserves_not_assessable():
    value = review_document()
    check = value["checks"][1]
    check.update(
        status="not_assessable",
        confidence_bps=0,
        issue_code="contact_not_visible",
        recommendation="提供可见接触点的视图",
        evidence=[],
    )
    candidate = VisionReviewCandidate.model_validate(value)
    assert candidate.checks[1].status == "not_assessable"
    check["confidence_bps"] = 9000
    with pytest.raises(ValueError):
        VisionReviewCandidate.model_validate(value)


def test_vision_review_rejects_duplicate_json_keys():
    raw = json.dumps(review_document())
    with pytest.raises(ValueError):
        decode_vision_review_candidate(
            raw[:-1] + ',"contract_id":"vision-review-candidate-production"}'
        )
