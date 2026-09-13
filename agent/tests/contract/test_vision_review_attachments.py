from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any

import pytest

from app.modules.storygraph.reference_brief_contract import REFERENCE_BRIEF_VIEW_ROLES
from app.modules.storygraph.vision_review_attachments import decode_vision_review_attachments
from app.modules.storygraph.vision_review_contract import VisionReviewSubject
from app.protocol.canonical import production_canonical_hash
from tests.contract.test_vision_review_contract import review_document


def attachments_document() -> list[dict[str, Any]]:
    path = (
        Path(__file__).resolve().parents[3]
        / "backend/tests/agent/testdata/vision_review_attachments.json"
    )
    return json.loads(path.read_text(encoding="utf-8"))


def test_vision_review_attachment_roundtrip_preserves_unassessed_rights() -> None:
    value = attachments_document()
    subject = VisionReviewSubject.model_validate(review_document()["subject"])
    attachments = decode_vision_review_attachments(json.dumps(value), subject)
    assert [attachment.model_dump(mode="json") for attachment in attachments] == value
    assert all(attachment.rights_observation == "not_assessed" for attachment in attachments)
    assert (
        production_canonical_hash(value)
        == "bfefa99207bf28ebcb6dd1df22cbdbb7cf8761d0c9df138a7caab5fe4035da7d"
    )


@pytest.mark.parametrize("kind", list(REFERENCE_BRIEF_VIEW_ROLES))
def test_vision_review_attachments_all_purposes_and_group_budget(kind: str) -> None:
    payload = review_document()["subject"]
    payload.update(target_kind=kind, slots=[])
    group: list[dict[str, Any]] = []
    for i, role in enumerate(REFERENCE_BRIEF_VIEW_ROLES[kind]):
        item = copy.deepcopy(attachments_document()[0])
        item["slot"].update(slot_key=role, view_role=role, sha256="abcd"[i] * 64)
        item["slot"]["staged_media_ref"]["id"] = f"00000000-0000-4000-8000-{21 + i:012d}"
        item["provider_receipt_ref"]["id"] = item["slot"]["staged_media_ref"]["id"]
        item["byte_length"] = 8 << 20
        payload["slots"].append(item["slot"])
        group.append(item)
    subject = VisionReviewSubject.model_validate(payload)
    assert len(decode_vision_review_attachments(json.dumps(group), subject)) == len(group)
    if kind == "prop_sheet":
        group[0]["byte_length"] += 1
        with pytest.raises(ValueError, match="byte budget"):
            decode_vision_review_attachments(json.dumps(group), subject)


@pytest.mark.parametrize(
    "fault",
    [
        "missing",
        "extra",
        "duplicate",
        "order",
        "slot",
        "receipt",
        "digest",
        "revision",
        "media_hash",
        "media_type",
        "empty",
        "oversize",
        "pixels",
        "edge",
        "pages",
        "frames",
        "rights",
        "private_path",
        "missing_field",
        "null",
        "bool_integer",
        "float_integer",
    ],
)
def test_vision_review_attachments_reject_incomplete_or_unsafe_manifest(fault: str) -> None:
    value = attachments_document()
    first = value[0]
    if fault == "missing":
        value.pop()
    elif fault == "extra":
        value.append(copy.deepcopy(first))
    elif fault == "duplicate":
        value[1] = copy.deepcopy(first)
    elif fault == "order":
        value.reverse()
    elif fault == "slot":
        first["slot"]["view_role"] = "other"
    elif fault == "receipt":
        first["provider_receipt_ref"]["id"] = "00000000-0000-4000-8000-000000000099"
    elif fault == "digest":
        first["slot"]["sha256"] = "f" * 64
    elif fault == "revision":
        first["slot"]["staged_media_ref"]["revision"] = 1
    elif fault == "media_hash":
        first["slot"]["staged_media_ref"]["content_hash"] = "f" * 64
    elif fault == "media_type":
        first["media_type"] = "image/jpeg"
    elif fault == "empty":
        first["byte_length"] = 0
    elif fault == "oversize":
        first["byte_length"] = (10 << 20) + 1
    elif fault == "pixels":
        first.update(pixel_width=8192, pixel_height=8192)
    elif fault == "edge":
        first["pixel_width"] = 8193
    elif fault == "pages":
        first["page_count"] = 2
    elif fault == "frames":
        first["frame_count"] = 0
    elif fault == "rights":
        first["rights_observation"] = "approved"
    elif fault == "private_path":
        first["path"] = "/private/image.png"
    elif fault == "missing_field":
        del first["frame_count"]
    elif fault == "null":
        first["frame_count"] = None
    elif fault == "bool_integer":
        first["frame_count"] = True
    else:
        first["frame_count"] = 1.0
    with pytest.raises(ValueError):
        decode_vision_review_attachments(
            json.dumps(value), VisionReviewSubject.model_validate(review_document()["subject"])
        )
