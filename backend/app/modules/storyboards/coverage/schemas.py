from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, Field, model_validator

from app.modules.storyboards.schemas import (
    CommandModel,
    NarrativeChannel,
    NarrativeReferenceInput,
    NarrativeRole,
)

CoverageAction = Literal[
    "approve_omission",
    "revoke_omission",
    "approve_invented",
    "revoke_invented",
]


class NarrativeReferenceReplaceRequest(CommandModel):
    expected_shot_revision: int = Field(ge=1)
    expected_current_spec_version_id: UUID
    expected_evaluation_hash: str = Field(min_length=64, max_length=64)
    references: list[NarrativeReferenceInput] = Field(max_length=100)

    @model_validator(mode="after")
    def validate_unique_edges(self) -> "NarrativeReferenceReplaceRequest":
        keys = [
            (
                item.unit_version_id,
                item.channel,
                item.segment_start,
                item.segment_end,
            )
            for item in self.references
        ]
        if len(set(keys)) != len(keys):
            raise ValueError("narrative references must be unique within a shot")
        return self


class NarrativeReferenceResponse(BaseModel):
    id: UUID
    shot_id: UUID
    shot_spec_version_id: UUID
    narrative_unit_id: UUID
    unit_version_id: UUID
    channel: NarrativeChannel
    role: NarrativeRole
    coverage_mode: Literal["full", "partial"]
    segment_start: int | None
    segment_end: int | None
    contribution: Literal["required", "supporting"]
    origin: Literal["ai", "human", "migrated"]
    created_by: UUID
    created_at: datetime


class UnitCoverageResponse(BaseModel):
    narrative_unit_id: UUID
    unit_version_id: UUID
    position: int
    kind: Literal["scene_heading", "action", "dialogue", "narration"]
    exact_text: str
    required_for_coverage: bool
    required_channel: Literal["visual", "audio"]
    status: Literal["covered", "approved_omitted", "uncovered"]
    shot_ids: list[UUID]


class ShotCoverageResponse(BaseModel):
    shot_id: UUID
    spec_version_id: UUID | None
    position: int
    title: str
    status: Literal["linked", "approved_invented", "orphan"]
    unit_version_ids: list[UUID]


class CoverageSummaryResponse(BaseModel):
    required_total: int
    covered: int
    approved_omitted: int
    uncovered: int
    shots_total: int
    linked: int
    approved_invented: int
    orphan: int
    stale: int


class CoverageReportResponse(BaseModel):
    episode_id: UUID
    status: Literal["ready", "blocked", "unavailable"]
    ready: bool
    basis_hash: str
    evaluation_hash: str
    summary: CoverageSummaryResponse
    units: list[UnitCoverageResponse]
    shots: list[ShotCoverageResponse]
    references: list[NarrativeReferenceResponse]
    stale_reference_ids: list[UUID]
    stale_decision_ids: list[UUID]
    next_actions: list[str]


class NarrativeReferenceReplaceResponse(BaseModel):
    shot_id: UUID
    previous_spec_version_id: UUID
    current_spec_version_id: UUID
    shot_revision: int
    references: list[NarrativeReferenceResponse]
    report: CoverageReportResponse


class CoverageDecisionRequest(CommandModel):
    action: CoverageAction
    unit_version_id: UUID | None
    shot_spec_version_id: UUID | None
    reason: str = Field(min_length=1, max_length=1000)
    evidence: str | None = Field(default=None, max_length=5000)
    expected_evaluation_hash: str = Field(min_length=64, max_length=64)
    idempotency_key: str = Field(min_length=1, max_length=200)

    @model_validator(mode="after")
    def validate_target(self) -> "CoverageDecisionRequest":
        omission = self.action in {"approve_omission", "revoke_omission"}
        if omission and (self.unit_version_id is None or self.shot_spec_version_id is not None):
            raise ValueError("omission decisions require one unit version target")
        if not omission and (self.shot_spec_version_id is None or self.unit_version_id is not None):
            raise ValueError("invented-shot decisions require one shot spec target")
        return self


class CoverageDecisionResponse(BaseModel):
    id: UUID
    episode_id: UUID
    sequence: int
    action: CoverageAction
    unit_version_id: UUID | None
    shot_spec_version_id: UUID | None
    basis_hash: str
    reason: str
    evidence: str | None
    idempotency_key: str
    actor_id: UUID
    created_at: datetime


class CoverageDecisionApplyResponse(BaseModel):
    decision: CoverageDecisionResponse
    report: CoverageReportResponse
