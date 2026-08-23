from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from app.modules.scripts.documents.schemas import NarrativeBlockResponse

EpisodePlanStatus = Literal[
    "draft",
    "review_ready",
    "confirmed",
    "materialized",
    "superseded",
]
EpisodePlanStrategy = Literal["explicit_markers", "target_duration_ai"]
ImportCommitStatus = Literal[
    "pending",
    "materializing",
    "materialized",
    "publishing",
    "published",
    "conflict",
    "failed",
]


class EpisodePlanCreateRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    strategy: EpisodePlanStrategy
    target_duration_ms: int = Field(ge=15_000, le=600_000)
    requested_episode_count: int | None = Field(default=None, ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)

    @field_validator("idempotency_key")
    @classmethod
    def strip_idempotency_key(cls, value: str) -> str:
        normalized = value.strip()
        if not normalized:
            raise ValueError("idempotency_key must contain text")
        return normalized

    @model_validator(mode="after")
    def validate_strategy(self) -> "EpisodePlanCreateRequest":
        if self.strategy == "explicit_markers" and self.requested_episode_count is not None:
            raise ValueError("explicit marker plans derive their episode count")
        return self


class EpisodePlanResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    project_id: UUID
    document_revision_id: UUID
    strategy: EpisodePlanStrategy
    status: EpisodePlanStatus
    target_duration_ms: int
    requested_episode_count: int | None
    total_estimated_duration_ms: int
    input_hash: str
    planning_engine_version: str
    model_name: str | None
    prompt_version: str | None
    schema_version: str
    planning_task_id: UUID | None
    planning_error_code: str | None
    revision: int
    confirmed_by: UUID | None
    confirmed_at: datetime | None
    created_by: UUID
    created_at: datetime
    updated_at: datetime


class EpisodeProposalResponse(BaseModel):
    id: UUID
    plan_id: UUID
    position: int
    title: str
    start_block_id: UUID
    end_block_id: UUID
    start_block_position: int
    end_block_position: int
    source_start: int
    source_end: int
    content_hash: str
    estimated_duration_ms: int
    reason: str
    confidence: float
    boundary_evidence: dict[str, object]
    is_locked: bool


class EpisodePlanImpactBlocker(BaseModel):
    code: str
    summary: str
    next_action: str


class EpisodePlanImpactResponse(BaseModel):
    project_revision: int
    active_episode_count: int
    active_order_hash: str
    projected_episode_count: int
    allowed: bool
    blockers: list[EpisodePlanImpactBlocker]


class EpisodePlanSourceResponse(BaseModel):
    document_revision_id: UUID
    normalized_text: str
    normalized_hash: str
    codepoint_count: int
    blocks: list[NarrativeBlockResponse]


class EpisodePlanDetailResponse(BaseModel):
    plan: EpisodePlanResponse
    proposals: list[EpisodeProposalResponse]
    impact: EpisodePlanImpactResponse
    source: EpisodePlanSourceResponse


class EpisodePlanCommandRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    expected_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)


class MoveEpisodeBoundaryRequest(EpisodePlanCommandRequest):
    left_proposal_id: UUID
    source_offset: int = Field(ge=1)


class SplitEpisodeProposalRequest(EpisodePlanCommandRequest):
    proposal_id: UUID
    source_offset: int = Field(ge=1)
    new_title: str = Field(min_length=1, max_length=120)

    @field_validator("new_title")
    @classmethod
    def strip_title(cls, value: str) -> str:
        normalized = value.strip()
        if not normalized:
            raise ValueError("new_title must contain text")
        return normalized


class MergeEpisodeProposalRequest(EpisodePlanCommandRequest):
    left_proposal_id: UUID


class RenameEpisodeProposalRequest(EpisodePlanCommandRequest):
    proposal_id: UUID
    title: str = Field(min_length=1, max_length=120)

    @field_validator("title")
    @classmethod
    def strip_title(cls, value: str) -> str:
        normalized = value.strip()
        if not normalized:
            raise ValueError("title must contain text")
        return normalized


class ConfirmEpisodePlanRequest(EpisodePlanCommandRequest):
    pass


class MaterializeEpisodePlanRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    mode: Literal["append_new"]
    expected_plan_revision: int = Field(ge=1)
    expected_project_revision: int = Field(ge=1)
    expected_active_order_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    idempotency_key: str = Field(min_length=1, max_length=200)


class PublishImportCommitRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    expected_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)


class ImportCommitResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    project_id: UUID
    plan_id: UUID
    mode: Literal["append_new"]
    status: ImportCommitStatus
    input_hash: str
    expected_project_revision: int
    expected_active_order_hash: str
    error_code: str | None
    revision: int
    created_by: UUID
    created_at: datetime
    updated_at: datetime


class EpisodeSegmentOriginResponse(BaseModel):
    id: UUID
    import_commit_id: UUID
    proposal_id: UUID
    document_revision_id: UUID
    episode_id: UUID
    source_id: UUID
    draft_version_id: UUID
    published_version_id: UUID | None
    position: int
    source_start: int
    source_end: int
    source_hash: str


class ImportCommitDetailResponse(BaseModel):
    commit: ImportCommitResponse
    segments: list[EpisodeSegmentOriginResponse]


class EpisodePlanningProviderProposal(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    title: str = Field(min_length=1, max_length=120)
    end_block_position: int = Field(
        ge=1,
        description=(
            "Position of the source block that ends this episode; this is not the proposal ordinal"
        ),
    )
    exact_end_anchor: str = Field(
        min_length=1,
        max_length=240,
        description="Verbatim unique suffix of the selected source block text",
    )
    estimated_duration_ms: int = Field(ge=1_000, le=600_000)
    reason: str = Field(min_length=1, max_length=500)
    confidence: float = Field(ge=0, le=1)


class EpisodePlanningProviderResult(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    proposals: list[EpisodePlanningProviderProposal] = Field(min_length=1)
