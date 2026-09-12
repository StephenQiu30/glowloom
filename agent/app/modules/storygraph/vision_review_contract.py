from __future__ import annotations

from typing import Annotated, Literal
from uuid import UUID

from pydantic import Field, model_validator

from app.modules.storygraph.reference_brief_contract import REFERENCE_BRIEF_VIEW_ROLES
from app.modules.storygraph.reference_plan_contract import ReferenceTargetKind
from app.modules.storygraph.scene_analysis_candidates import StrictSceneAnalysisModel
from app.protocol.canonical import production_canonical_json_text

Digest = Annotated[str, Field(pattern=r"^[0-9a-f]{64}$")]
CanonicalUUID = Annotated[
    str, Field(pattern=r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
]
BasisPoints = Annotated[int, Field(strict=True, ge=0, le=10000)]
PositiveRevision = Annotated[int, Field(strict=True, ge=1, le=9007199254740991)]
REVIEW_CATEGORIES = ["identity", "interaction_geometry", "state", "style_fidelity", "view_role"]


class VisionReviewVersionRef(StrictSceneAnalysisModel):
    """Wire identity of an immutable Generation-owned version."""

    id: CanonicalUUID
    revision: PositiveRevision
    content_hash: Digest

    @model_validator(mode="after")
    def validate_identity(self) -> VisionReviewVersionRef:
        if UUID(self.id).int == 0:
            raise ValueError("Vision Review version identity is empty")
        return self


class VisionReviewBundleRef(StrictSceneAnalysisModel):
    id: CanonicalUUID
    content_hash: Digest

    @model_validator(mode="after")
    def validate_identity(self) -> VisionReviewBundleRef:
        if UUID(self.id).int == 0:
            raise ValueError("Vision Review Bundle identity is empty")
        return self


class VisionReviewSlot(StrictSceneAnalysisModel):
    slot_key: str
    view_role: str
    staged_media_ref: VisionReviewVersionRef
    sha256: Digest


class VisionReviewSubject(StrictSceneAnalysisModel):
    """Frozen identity summary, never an authorization or a complete model input."""

    workspace_id: CanonicalUUID
    project_id: CanonicalUUID
    target_kind: ReferenceTargetKind
    generation_round: PositiveRevision
    generation_target_ref: VisionReviewVersionRef
    execution_ref: VisionReviewVersionRef
    bundle_input_ref: VisionReviewBundleRef
    candidate_bundle_index: int = Field(strict=True, ge=0, le=3)
    brief_revision_id: CanonicalUUID
    brief_revision_hash: Digest
    stage_release_hash: Digest
    input_hash: Digest
    slots: list[VisionReviewSlot] = Field(min_length=1, max_length=4)

    @model_validator(mode="after")
    def validate_subject(self) -> VisionReviewSubject:
        if any(
            UUID(value).int == 0
            for value in (self.workspace_id, self.project_id, self.brief_revision_id)
        ):
            raise ValueError("Vision Review subject identity is empty")
        roles = REFERENCE_BRIEF_VIEW_ROLES[self.target_kind]
        if (
            [slot.slot_key for slot in self.slots] != roles
            or [slot.view_role for slot in self.slots] != roles
            or any(slot.staged_media_ref.revision != 2 for slot in self.slots)
            or len({slot.staged_media_ref.id for slot in self.slots}) != len(self.slots)
            or len({slot.sha256 for slot in self.slots}) != len(self.slots)
        ):
            raise ValueError("Vision Review requires complete, distinct validated views")
        return self


class VisionReviewRegion(StrictSceneAnalysisModel):
    x: BasisPoints
    y: BasisPoints
    width: Annotated[int, Field(strict=True, ge=1, le=10000)]
    height: Annotated[int, Field(strict=True, ge=1, le=10000)]

    @model_validator(mode="after")
    def validate_bounds(self) -> VisionReviewRegion:
        if self.x + self.width > 10000 or self.y + self.height > 10000:
            raise ValueError("Vision Review evidence lies outside the image")
        return self


class VisionReviewEvidence(StrictSceneAnalysisModel):
    slot_key: str
    region: VisionReviewRegion

    def sort_key(self) -> tuple[str, int, int, int, int]:
        return (self.slot_key, self.region.x, self.region.y, self.region.width, self.region.height)


class VisionReviewCheck(StrictSceneAnalysisModel):
    category: Literal["identity", "interaction_geometry", "state", "style_fidelity", "view_role"]
    status: Literal["pass", "warn", "fail", "not_assessable"]
    issue_code: str = Field(pattern=r"^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$", max_length=100)
    confidence_bps: BasisPoints
    summary: str
    recommendation: str
    evidence: list[VisionReviewEvidence] = Field(max_length=16)

    @model_validator(mode="after")
    def validate_result(self) -> VisionReviewCheck:
        if not _valid_text(self.summary):
            raise ValueError("Vision Review summary is invalid")
        keys = [evidence.sort_key() for evidence in self.evidence]
        if keys != sorted(set(keys)):
            raise ValueError("Vision Review evidence must be sorted and unique")
        if self.status == "pass":
            if self.issue_code != "none" or self.recommendation != "":
                raise ValueError("Vision pass cannot carry an unresolved issue")
        elif self.issue_code == "none" or not _valid_text(self.recommendation):
            raise ValueError("Vision issue requires a reason and advice")
        if self.status in {"warn", "fail"} and not self.evidence:
            raise ValueError("Vision issue requires evidence")
        if self.status == "not_assessable" and self.confidence_bps != 0:
            raise ValueError("Vision uncertainty cannot claim confidence")
        return self


class VisionReviewCandidate(StrictSceneAnalysisModel):
    contract_id: Literal["vision-review-candidate-production"]
    subject: VisionReviewSubject
    checks: list[VisionReviewCheck] = Field(min_length=5, max_length=5)

    @model_validator(mode="after")
    def validate_review(self) -> VisionReviewCandidate:
        if [check.category for check in self.checks] != REVIEW_CATEGORIES:
            raise ValueError("Vision Review requires all five ordered categories")
        slots = {slot.slot_key for slot in self.subject.slots}
        for check in self.checks:
            evidence_slots = {evidence.slot_key for evidence in check.evidence}
            if not evidence_slots <= slots or (check.status == "pass" and evidence_slots != slots):
                raise ValueError("Vision Review evidence does not cover the frozen views")
        return self

    def validate_for(self, subject: VisionReviewSubject) -> None:
        self.__class__.model_validate(self.model_dump(mode="json"))
        VisionReviewSubject.model_validate(subject.model_dump(mode="json"))
        if self.subject != subject:
            raise ValueError("Vision Review changed the frozen subject")


def decode_vision_review_candidate(raw: str | bytes) -> VisionReviewCandidate:
    canonical = production_canonical_json_text(raw.encode("utf-8") if isinstance(raw, str) else raw)
    return VisionReviewCandidate.model_validate_json(canonical)


def _valid_text(value: str) -> bool:
    return bool(value) and value.strip() == value and len(value.encode("utf-8")) <= 2000
