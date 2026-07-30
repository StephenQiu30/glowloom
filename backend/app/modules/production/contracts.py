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


class ScriptExtractionTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    episode_id: UUID
    request_id: UUID
    input_version_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    idempotency_key: str = Field(min_length=1, max_length=200)


@dataclass(frozen=True, slots=True)
class TaskContext:
    id: UUID
    workspace_id: UUID
    request_id: UUID
    task_type: str
    request_type: str
    status: TaskStatus
