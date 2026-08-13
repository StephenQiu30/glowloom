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
TaskType = Literal[
    "script_extraction",
    "episode_planning",
    "script_adaptation",
    "storyboard_draft",
    "image_generation",
    "video_generation",
    "media_probe",
    "upload_expiration",
    "upload_cleanup",
    "media_location_migration",
    "media_location_retirement",
]
TaskRequestType = Literal[
    "extraction_batch",
    "episode_plan",
    "adaptation_run",
    "storyboard_draft_batch",
    "generation_request",
    "media_version",
    "upload_session",
    "workspace",
    "media_location",
]
GenerationProtocolErrorCode = Literal[
    "unsupported_message_schema",
    "invalid_message_payload",
]
AssetImpactTaskStatus = Literal["queued", "running", "waiting_provider", "unknown"]


@dataclass(frozen=True, slots=True)
class GenerationPromptSnapshot:
    generation_request_id: UUID
    episode_id: UUID
    shot_id: UUID
    shot_spec_version_id: UUID
    input_hash: str


@dataclass(frozen=True, slots=True)
class GenerationTaskSnapshot:
    task_id: UUID
    generation_request_id: UUID
    status: AssetImpactTaskStatus
    revision: int


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


class EpisodePlanningTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    plan_id: UUID
    document_revision_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    idempotency_key: str = Field(min_length=1, max_length=200)


class ScriptAdaptationTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    episode_id: UUID
    run_id: UUID
    input_version_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    idempotency_key: str = Field(min_length=1, max_length=200)


class StoryboardDraftTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    episode_id: UUID
    batch_id: UUID
    input_version_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    idempotency_key: str = Field(min_length=1, max_length=200)


class MediaProbeTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    media_version_id: UUID
    idempotency_key: str = Field(min_length=1, max_length=200)


class UploadExpirationTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    upload_session_id: UUID
    requested_by: UUID
    idempotency_key: str = Field(min_length=1, max_length=200)


class UploadCleanupTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    requested_by: UUID
    idempotency_key: str = Field(min_length=1, max_length=200)


class MediaLocationMigrationTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    media_version_id: UUID
    location_id: UUID
    operation: Literal["migrate", "rollback"]
    requested_by: UUID
    idempotency_key: str = Field(min_length=1, max_length=200)


class MediaLocationRetirementTaskCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    media_location_id: UUID
    requested_by: UUID
    idempotency_key: str = Field(min_length=1, max_length=200)


@dataclass(frozen=True, slots=True)
class UploadExpirationTaskDispatch:
    task: TaskResponse
    outbox_event_id: UUID


@dataclass(frozen=True, slots=True)
class UploadCleanupTaskDispatch:
    task: TaskResponse
    outbox_event_id: UUID


@dataclass(frozen=True, slots=True)
class MediaLocationTaskDispatch:
    task: TaskResponse
    outbox_event_id: UUID


@dataclass(frozen=True, slots=True)
class TaskContext:
    id: UUID
    workspace_id: UUID
    request_id: UUID
    task_type: str
    request_type: str
    usage_type: str | None
    usage_id: UUID | None
    requested_by: UUID
    status: TaskStatus


@dataclass(frozen=True, slots=True)
class PreparedGenerationAttempt:
    task_id: UUID
    attempt_id: UUID


@dataclass(frozen=True, slots=True)
class EpisodeTaskSummary:
    running: int = 0
    failed: int = 0
    succeeded: int = 0
    unknown: int = 0
