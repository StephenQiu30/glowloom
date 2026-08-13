from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.modules.storyboards.schemas import AssetReferenceRequest, ShotSpec


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


class DraftBatchCreateRequest(CommandModel):
    input_script_version_id: UUID
    asset_state_ids: list[UUID] = Field(default=[], max_length=100)
    idempotency_key: str = Field(min_length=1, max_length=200)

    @model_validator(mode="after")
    def validate_asset_states(self) -> "DraftBatchCreateRequest":
        if len(set(self.asset_state_ids)) != len(self.asset_state_ids):
            raise ValueError("asset state IDs must be unique")
        return self


class DraftShotProposal(CommandModel):
    proposal_key: str = Field(min_length=1, max_length=120)
    position: int = Field(ge=1, le=120)
    title: str = Field(min_length=1, max_length=200)
    narrative_unit_version_ids: list[UUID] = Field(min_length=1, max_length=500)
    spec: ShotSpec
    asset_references: list[AssetReferenceRequest] = Field(default=[], max_length=100)
    risk_codes: list[str] = Field(default=[], max_length=20)

    @model_validator(mode="after")
    def validate_references(self) -> "DraftShotProposal":
        if len(set(self.narrative_unit_version_ids)) != len(self.narrative_unit_version_ids):
            raise ValueError("narrative unit version IDs must be unique")
        slots = [reference.slot_key for reference in self.asset_references]
        if len(set(slots)) != len(slots):
            raise ValueError("asset reference slot keys must be unique")
        if any(not code.strip() or len(code) > 80 for code in self.risk_codes):
            raise ValueError("risk codes must contain 1-80 characters")
        return self


class DraftProviderResult(CommandModel):
    shots: list[DraftShotProposal] = Field(min_length=1, max_length=120)

    @model_validator(mode="after")
    def validate_shots(self) -> "DraftProviderResult":
        keys = [shot.proposal_key for shot in self.shots]
        if len(set(keys)) != len(keys):
            raise ValueError("draft proposal keys must be unique")
        if [shot.position for shot in self.shots] != list(range(1, len(self.shots) + 1)):
            raise ValueError("draft shot positions must be continuous from 1")
        return self


class DraftTarget(CommandModel):
    title: str = Field(min_length=1, max_length=200)
    narrative_unit_version_ids: list[UUID] = Field(min_length=1, max_length=500)
    spec: ShotSpec
    asset_references: list[AssetReferenceRequest] = Field(default=[], max_length=100)

    @model_validator(mode="after")
    def validate_references(self) -> "DraftTarget":
        if len(set(self.narrative_unit_version_ids)) != len(self.narrative_unit_version_ids):
            raise ValueError("narrative unit version IDs must be unique")
        slots = [reference.slot_key for reference in self.asset_references]
        if len(set(slots)) != len(slots):
            raise ValueError("asset reference slot keys must be unique")
        return self


class DraftDecisionRequest(CommandModel):
    action: Literal["accepted", "modified", "ignored"]
    expected_batch_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)
    target: DraftTarget | None = None

    @model_validator(mode="after")
    def validate_target(self) -> "DraftDecisionRequest":
        if (self.action == "modified") != (self.target is not None):
            raise ValueError("modified decisions require a complete target")
        return self


class DraftApproveRequest(CommandModel):
    expected_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)


class DraftApplyPreflightRequest(CommandModel):
    expected_revision: int = Field(ge=1)


class DraftApplyRequest(CommandModel):
    expected_revision: int = Field(ge=1)
    expected_order_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    impact_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    idempotency_key: str = Field(min_length=1, max_length=200)


class DraftDecisionResponse(BaseModel):
    id: UUID
    sequence: int
    action: Literal["accepted", "modified", "ignored"]
    target: DraftTarget | None
    created_by: UUID
    created_at: datetime


class DraftAssetReferenceResponse(BaseModel):
    slot_key: str
    role: Literal["location", "character", "prop", "costume", "visual_style", "voice"]
    asset_version_id: UUID
    subject_key: str | None


class DraftShotResponse(BaseModel):
    id: UUID
    proposal_key: str
    position: int
    title: str
    narrative_unit_version_ids: list[UUID]
    spec: ShotSpec
    asset_references: list[DraftAssetReferenceResponse]
    risk_codes: list[str]
    decision_history: list[DraftDecisionResponse]


class DraftDecisionSummary(BaseModel):
    pending: int
    accepted: int
    modified: int
    ignored: int


class DraftInputSummary(BaseModel):
    script_version_id: UUID
    narrative_structure_id: UUID
    narrative_revision: int
    narrative_dependency_hash: str
    narrative_unit_version_ids: list[UUID]
    asset_state_ids: list[UUID]
    asset_version_ids: list[UUID]
    target_duration_ms: int
    aspect_ratio: Literal["9:16", "16:9", "1:1"]
    visual_style: str | None
    input_hash: str


class DraftBatchResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    project_id: UUID
    episode_id: UUID
    status: Literal[
        "queued",
        "running",
        "needs_review",
        "approved",
        "applied",
        "failed",
        "unknown",
        "cancelled",
    ]
    revision: int
    task_id: UUID | None
    input: DraftInputSummary
    drafts: list[DraftShotResponse]
    decision_summary: DraftDecisionSummary
    error_code: str | None
    created_at: datetime
    updated_at: datetime


class DraftDecisionResult(BaseModel):
    batch: DraftBatchResponse
    draft: DraftShotResponse


class DraftApplyDiff(BaseModel):
    kept: int
    created: int
    modified: Literal[0] = 0
    archived: Literal[0] = 0


class DraftApplyPreflightResponse(BaseModel):
    batch_id: UUID
    batch_revision: int
    order_hash: str
    impact_hash: str
    diff: DraftApplyDiff


class DraftApplyResponse(BaseModel):
    batch: DraftBatchResponse
    created_shot_ids: list[UUID]
