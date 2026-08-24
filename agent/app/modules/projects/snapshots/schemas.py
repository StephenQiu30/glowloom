from datetime import datetime
from decimal import Decimal
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, Field


class BlockingReason(BaseModel):
    code: str
    summary: str
    resource_type: Literal["project", "episode"]
    resource_id: UUID


class NextAction(BaseModel):
    code: str
    label: str
    href: str


class TaskSummary(BaseModel):
    status: Literal["not_started", "running", "failed", "succeeded", "unavailable"]
    running: int = 0
    failed: int = 0
    succeeded: int = 0
    unknown: int = 0


class ScriptSummary(BaseModel):
    status: Literal[
        "not_started",
        "published",
        "extracting",
        "extraction_blocked",
        "review_required",
        "confirmation_required",
        "set_current_required",
        "confirmed",
        "unavailable",
    ]
    current_version_id: UUID | None
    extraction_batch_id: UUID | None
    pending_required_candidates: int = 0


class AssetSummary(BaseModel):
    status: Literal["not_started", "draft", "blocked", "ready", "unavailable"]
    total: int = 0
    versioned: int = 0
    ready: int = 0
    draft: int = 0
    blocked: int = 0
    ready_kinds: list[str]
    required_kinds: list[str]


class StoryboardSummary(BaseModel):
    status: Literal["not_started", "blocked", "ready", "unavailable"]
    total: int = 0
    ready: int = 0
    blocked: int = 0
    unavailable: int = 0


class ReviewSummary(BaseModel):
    status: Literal["not_started", "pending", "completed", "unavailable"]
    pending: int = 0


class CostSummary(BaseModel):
    status: Literal["not_started"] = "not_started"
    currency: str
    reserved: Decimal = Decimal("0.000000")
    used: Decimal = Decimal("0.000000")


class PartialFailure(BaseModel):
    module: str
    code: str
    summary: str


class EpisodeProductionSnapshot(BaseModel):
    episode_id: UUID
    current_stage: Literal[
        "script_import",
        "structure_review",
        "asset_preparation",
        "storyboard_preparation",
    ]
    completion: int = Field(ge=0, le=100)
    blocking_reasons: list[BlockingReason]
    next_actions: list[NextAction]
    script_summary: ScriptSummary
    asset_summary: AssetSummary
    storyboard_summary: StoryboardSummary
    task_summary: TaskSummary
    review_summary: ReviewSummary
    cost_summary: CostSummary
    partial_failures: list[PartialFailure]
    computed_at: datetime


class ProjectProductionSnapshot(BaseModel):
    project_id: UUID
    current_stage: Literal[
        "project_setup",
        "script_import",
        "structure_review",
        "asset_preparation",
        "storyboard_preparation",
    ]
    completion: int = Field(ge=0, le=100)
    blocking_reasons: list[BlockingReason]
    next_actions: list[NextAction]
    episodes: list[EpisodeProductionSnapshot]
    partial_failures: list[PartialFailure]
    computed_at: datetime
