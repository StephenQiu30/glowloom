from typing import Literal
from uuid import UUID

from pydantic import BaseModel

from app.modules.production.contracts import ScriptExtractionTaskCommand, TaskStatus

__all__ = [
    "PaginatedTasks",
    "ScriptExtractionTaskCommand",
    "TaskErrorResponse",
    "TaskResponse",
    "TaskScopeResponse",
    "TaskStatus",
]


class TaskScopeResponse(BaseModel):
    episode_id: UUID | None
    render_snapshot_id: UUID | None
    usage_type: str | None
    usage_id: UUID | None
    input_version_id: UUID | None
    input_hash: str | None


class TaskErrorResponse(BaseModel):
    code: str
    retryable: bool
    summary: str


class TaskResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    task_type: Literal["script_extraction"]
    request_type: Literal["extraction_batch"]
    request_id: UUID
    scope: TaskScopeResponse
    status: TaskStatus
    progress_stage: str
    error: TaskErrorResponse | None
    next_action: str | None
    cancel_status: Literal["none", "requested", "accepted", "rejected"]
    revision: int


class PaginatedTasks(BaseModel):
    items: list[TaskResponse]
    total: int
    limit: int
    offset: int
