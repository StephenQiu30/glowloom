from decimal import Decimal
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class ProjectCreateRequest(CommandModel):
    workspace_id: UUID
    name: str = Field(min_length=1, max_length=120)
    description: str | None = Field(default=None, max_length=2000)
    aspect_ratio: Literal["9:16", "16:9", "1:1"] = "9:16"
    language: str = Field(default="zh-CN", min_length=2, max_length=35, pattern=r"^[A-Za-z0-9-]+$")
    visual_style: str | None = Field(default=None, max_length=200)
    target_duration_ms: int = Field(default=90000, ge=1000, le=7_200_000)


class ProjectUpdateRequest(CommandModel):
    name: str | None = Field(default=None, min_length=1, max_length=120)
    description: str | None = Field(default=None, max_length=2000)
    aspect_ratio: Literal["9:16", "16:9", "1:1"] | None = None
    language: str | None = Field(
        default=None,
        min_length=2,
        max_length=35,
        pattern=r"^[A-Za-z0-9-]+$",
    )
    visual_style: str | None = Field(default=None, max_length=200)
    target_duration_ms: int | None = Field(default=None, ge=1000, le=7_200_000)
    expected_revision: int = Field(ge=1)


class BudgetLimitRequest(CommandModel):
    amount: Decimal = Field(ge=0, max_digits=20, decimal_places=6)
    currency: str = Field(min_length=3, max_length=3, pattern=r"^[A-Z]{3}$")
    expected_revision: int = Field(ge=1)


class ProjectStateRequest(CommandModel):
    expected_revision: int = Field(ge=1)


class ProjectResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    name: str
    description: str | None
    aspect_ratio: Literal["9:16", "16:9", "1:1"]
    language: str
    visual_style: str | None
    target_duration_ms: int
    budget_limit: Decimal
    currency: str
    status: Literal["active", "archived"]
    revision: int


class PaginatedProjects(BaseModel):
    items: list[ProjectResponse]
    total: int
    limit: int
    offset: int


class DeleteBlocker(BaseModel):
    code: str
    resource_type: str
    resource_id: UUID
    summary: str


class DeletePreflightResponse(BaseModel):
    allowed: bool
    blockers: list[DeleteBlocker]


class DeleteResponse(BaseModel):
    deleted: Literal[True] = True
