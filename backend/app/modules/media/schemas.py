from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field

from app.modules.production.contracts import TaskResponse

MediaKind = Literal["image", "video", "audio", "subtitle", "delivery"]
MediaSource = Literal["upload", "generated", "rendered"]
ProbeStatus = Literal["pending", "ready", "failed", "quarantined"]


class UploadDeclaration(BaseModel):
    model_config = ConfigDict(extra="forbid")

    workspace_id: UUID
    kind: MediaKind
    filename: str = Field(min_length=1, max_length=255)
    size_bytes: int = Field(ge=1)
    mime_type: str = Field(min_length=1, max_length=120)
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    idempotency_key: str = Field(min_length=1, max_length=200)


class AppendVersionRequest(UploadDeclaration):
    expected_current_version_id: UUID


class UploadSessionResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    media_object_id: UUID | None
    status: Literal["pending", "completed", "expired", "failed"]
    kind: MediaKind
    filename: str
    size_bytes: int
    mime_type: str
    sha256: str
    expires_at: datetime


class UploadCapabilityResponse(BaseModel):
    method: Literal["PUT"] = "PUT"
    url: str
    headers: dict[str, str]
    expires_at: datetime


class UploadInitializationResponse(BaseModel):
    upload_session: UploadSessionResponse
    upload: UploadCapabilityResponse


class MediaObjectResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    kind: MediaKind
    source_type: MediaSource
    status: Literal["active", "archived"]
    current_version_id: UUID | None
    revision: int


class MediaVersionResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    media_object_id: UUID
    version_no: int
    filename: str
    sha256: str
    size_bytes: int
    mime_type: str
    probe_status: ProbeStatus
    probe_attempt: int
    probe_error_code: str | None
    probe_error_summary: str | None
    probe_next_action: str | None
    width: int | None
    height: int | None
    duration_ms: int | None
    codec: str | None
    container: str | None
    created_at: datetime


class UploadCompletionResponse(BaseModel):
    media_object: MediaObjectResponse
    version: MediaVersionResponse
    probe_task: TaskResponse


class PaginatedMedia(BaseModel):
    items: list[MediaVersionResponse]
    total: int
    limit: int
    offset: int


class MediaAccessRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    purpose: Literal["preview", "download"]


class MediaAccessResponse(BaseModel):
    method: Literal["GET"] = "GET"
    url: str
    purpose: Literal["preview", "download"]
    expires_at: datetime


class ArchiveMediaRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    expected_revision: int = Field(ge=1)


class ProbeRetryRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    idempotency_key: str = Field(min_length=1, max_length=120)
