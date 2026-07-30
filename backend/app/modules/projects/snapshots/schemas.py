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
    status: Literal["not_started"] = "not_started"
    running: int = 0
    failed: int = 0


class ReviewSummary(BaseModel):
    status: Literal["not_started"] = "not_started"
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
    current_stage: Literal["script_import"]
    completion: int = Field(ge=0, le=100)
    blocking_reasons: list[BlockingReason]
    next_actions: list[NextAction]
    task_summary: TaskSummary
    review_summary: ReviewSummary
    cost_summary: CostSummary
    partial_failures: list[PartialFailure]
    computed_at: datetime


class ProjectProductionSnapshot(BaseModel):
    project_id: UUID
    current_stage: Literal["project_setup", "script_import"]
    completion: int = Field(ge=0, le=100)
    blocking_reasons: list[BlockingReason]
    next_actions: list[NextAction]
    episodes: list[EpisodeProductionSnapshot]
    partial_failures: list[PartialFailure]
    computed_at: datetime
