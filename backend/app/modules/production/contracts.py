from dataclasses import dataclass
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field

TaskStatus = Literal[
    "queued",
    "running",
    "waiting_provider",
    "succeeded",
    "failed",
    "cancelled",
    "unknown",
]
TaskType = Literal["script_extraction", "media_probe"]
TaskRequestType = Literal["extraction_batch", "media_version"]


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
    task_type: TaskType
    request_type: TaskRequestType
    request_id: UUID
    scope: TaskScopeResponse
    status: TaskStatus
    progress_stage: str
    error: TaskErrorResponse | None
    next_action: str | None
    cancel_status: Literal["none", "requested", "accepted", "rejected"]
    revision: int


class ScriptExtractionTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    episode_id: UUID
    request_id: UUID
    input_version_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    idempotency_key: str = Field(min_length=1, max_length=200)


class MediaProbeTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    media_version_id: UUID
    idempotency_key: str = Field(min_length=1, max_length=200)


@dataclass(frozen=True, slots=True)
class TaskContext:
    id: UUID
    workspace_id: UUID
    request_id: UUID
    task_type: str
    request_type: str
    status: TaskStatus


@dataclass(frozen=True, slots=True)
class EpisodeTaskSummary:
    running: int = 0
    failed: int = 0
    succeeded: int = 0
    unknown: int = 0
