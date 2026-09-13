from __future__ import annotations

from typing import Literal

from pydantic import Field, TypeAdapter, model_validator

from app.modules.storygraph.scene_analysis_candidates import StrictSceneAnalysisModel
from app.modules.storygraph.vision_review_contract import (
    VisionReviewBundleRef,
    VisionReviewSlot,
    VisionReviewSubject,
)
from app.protocol.canonical import production_canonical_json_text


class VisionReviewMediaAttachment(StrictSceneAnalysisModel):
    """Byte descriptor, not an Owner fact, dispatch grant or formal use permission."""

    slot: VisionReviewSlot
    provider_receipt_ref: VisionReviewBundleRef
    media_type: Literal["image/png"]
    byte_length: int = Field(strict=True, ge=1, le=10 << 20)
    pixel_width: int = Field(strict=True, ge=1, le=8192)
    pixel_height: int = Field(strict=True, ge=1, le=8192)
    page_count: int = Field(strict=True, ge=1, le=1)
    frame_count: int = Field(strict=True, ge=1, le=1)
    rights_observation: Literal["not_assessed"]

    @model_validator(mode="after")
    def validate_media_policy(self) -> VisionReviewMediaAttachment:
        if (
            self.pixel_width * self.pixel_height > 16777216
            or self.provider_receipt_ref.id != self.slot.staged_media_ref.id
        ):
            raise ValueError("Vision Review attachment provenance or dimensions are invalid")
        return self


def validate_vision_review_attachments(
    subject: VisionReviewSubject, attachments: list[VisionReviewMediaAttachment]
) -> None:
    VisionReviewSubject.model_validate(subject.model_dump(mode="json"))
    for attachment in attachments:
        VisionReviewMediaAttachment.model_validate(attachment.model_dump(mode="json"))
    if [attachment.slot for attachment in attachments] != subject.slots:
        raise ValueError("Vision Review attachments differ from the frozen complete views")
    if sum(attachment.byte_length for attachment in attachments) > 32 << 20:
        raise ValueError("Vision Review attachments exceed the bundle byte budget")


def decode_vision_review_attachments(
    raw: str | bytes, subject: VisionReviewSubject
) -> list[VisionReviewMediaAttachment]:
    canonical = production_canonical_json_text(raw.encode("utf-8") if isinstance(raw, str) else raw)
    attachments = TypeAdapter(list[VisionReviewMediaAttachment]).validate_json(canonical)
    validate_vision_review_attachments(subject, attachments)
    return attachments
