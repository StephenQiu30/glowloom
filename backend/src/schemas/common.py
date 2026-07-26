from __future__ import annotations

from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, RootModel


class StrictContract(BaseModel):
    model_config = ConfigDict(strict=True, extra="forbid", frozen=True)


class ProblemFieldError(StrictContract):
    field: str
    code: str
    message: str


class Problem(StrictContract):
    type: str
    title: str
    status: int = Field(ge=400, le=599)
    code: str
    retryable: bool
    request_id: UUID
    detail: str | None = None
    errors: tuple[ProblemFieldError, ...] | None = None
    metadata: dict[str, str | int | bool | None] | None = None


class TaskAccepted(StrictContract):
    task_id: UUID
    status: Literal["queued"]
    resource_version: int = Field(ge=1)
    status_url: str = Field(pattern=r"^/v1/tasks/[0-9a-f-]{36}$")


class TaskScope(RootModel[dict[str, str]]):
    model_config = ConfigDict(strict=True, frozen=True)


class TaskProgress(StrictContract):
    phase: str
    completed: int = Field(ge=0)
    total: int = Field(ge=1)
    message: str | None = None


class TaskResultRef(StrictContract):
    output_type: Literal[
        "script_version",
        "creative_asset_version",
        "shot_spec_version",
        "generation_candidate",
        "delivery_version",
    ]
    output_id: UUID


class TaskError(StrictContract):
    code: str
    retryable: bool
    summary: str


class TaskResponse(StrictContract):
    id: UUID
    type: Literal["generate_script", "generate_storyboard", "generate_media", "render_episode"]
    scope: TaskScope
    status: Literal[
        "queued", "running", "cancelling", "cancelled", "succeeded", "failed", "unknown"
    ]
    progress: TaskProgress
    input_outdated: bool
    current_attempt_id: UUID | None = None
    result_refs: tuple[TaskResultRef, ...]
    error: TaskError | None = None
    resource_version: int = Field(ge=1)
    poll_after_ms: Literal[2000] = 2000
    created_at: datetime
    updated_at: datetime
    finished_at: datetime | None = None
