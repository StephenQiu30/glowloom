from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Literal
from uuid import UUID

TaskType = Literal["generate_script", "generate_storyboard", "generate_media", "render_episode"]
Capability = Literal["text", "image", "video", "tts"]


@dataclass(frozen=True, slots=True)
class SubmitTaskCommand:
    episode_id: UUID
    task_type: TaskType
    capability: Capability | None
    scope: dict[str, object]
    input_refs: dict[str, object]
    prompt: str | None
    parameters: dict[str, object]
    model_profile_id: str | None
    provider_id: str | None
    model_id: str | None
    route_version: str | None
    schema_version: str
    operation_scope: str
    idempotency_key: str
    handler_version: str
    retry_of_task_id: UUID | None = None


@dataclass(frozen=True, slots=True)
class TaskAcceptedSnapshot:
    task_id: UUID
    snapshot_id: UUID
    status: Literal["queued"] = "queued"
    resource_version: int = 1
    poll_after_ms: Literal[2000] = 2000

    @property
    def status_url(self) -> str:
        return f"/v1/tasks/{self.task_id}"


@dataclass(frozen=True, slots=True)
class TaskResultSnapshot:
    output_type: str
    output_id: UUID


@dataclass(frozen=True, slots=True)
class TaskSnapshot:
    id: UUID
    episode_id: UUID
    snapshot_id: UUID
    task_type: str
    scope: dict[str, object]
    status: str
    progress: dict[str, object]
    input_refs: dict[str, object]
    input_outdated: bool
    current_attempt_id: UUID | None
    result_refs: tuple[TaskResultSnapshot, ...]
    error_code: str | None
    error: dict[str, object] | None
    next_action: str | None
    resource_version: int
    retry_of_task_id: UUID | None
    created_at: datetime
    updated_at: datetime
    finished_at: datetime | None


@dataclass(frozen=True, slots=True)
class RetrySubmissionSnapshot:
    task: TaskSnapshot
    capability: str | None
    prompt: str | None
    parameters: dict[str, object]
    model_profile_id: str | None
    provider_id: str | None
    model_id: str | None
    route_version: str | None
    schema_version: str
    handler_version: str
