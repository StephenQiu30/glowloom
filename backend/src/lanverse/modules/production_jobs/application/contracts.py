from __future__ import annotations

from dataclasses import dataclass
from typing import Literal
from uuid import UUID

TaskType = Literal["generate_script", "generate_storyboard", "generate_media", "render_episode"]
Capability = Literal["text", "image", "video", "tts"]


@dataclass(frozen=True, slots=True)
class SubmitTaskCommand:
    episode_id: UUID
    task_type: TaskType
    capability: Capability
    scope: dict[str, object]
    input_refs: dict[str, object]
    prompt: str
    parameters: dict[str, object]
    model_profile_id: str
    provider_id: str
    model_id: str
    route_version: str
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
