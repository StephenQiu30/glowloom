from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class ExportRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    expected_input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    idempotency_key: str = Field(min_length=1, max_length=200)


class ExportBlockerResponse(BaseModel):
    code: str
    summary: str
    next_action: str
    shot_id: UUID | None = None
    dependency_id: UUID | None = None


class ExportPreflightResponse(BaseModel):
    episode_id: UUID
    status: Literal["ready", "blocked", "unavailable"]
    input_hash: str | None
    script_version_id: UUID | None
    narrative_structure_id: UUID | None
    narrative_unit_version_ids: list[UUID]
    shot_spec_version_ids: list[UUID]
    asset_version_ids: list[UUID]
    coverage_basis_hash: str | None
    coverage_evaluation_hash: str | None
    readiness_evaluation_hash: str | None
    blockers: list[ExportBlockerResponse]


class ExportFileResponse(BaseModel):
    path: str
    media_type: str
    sha256: str
    size_bytes: int


class ExportManifestResponse(BaseModel):
    id: UUID
    schema_version: int
    input_hash: str
    script_version_id: UUID
    narrative_structure_id: UUID
    narrative_unit_version_ids: list[UUID]
    shot_spec_version_ids: list[UUID]
    asset_version_ids: list[UUID]
    coverage_basis_hash: str
    coverage_evaluation_hash: str
    files: list[ExportFileResponse]
    media_version_id: UUID
    package_sha256: str
    package_size_bytes: int
    created_at: datetime


class ExportResponse(BaseModel):
    id: UUID
    episode_id: UUID
    status: Literal["queued", "running", "succeeded", "failed"]
    input_hash: str
    task_id: UUID | None
    error_code: str | None
    manifest: ExportManifestResponse | None
    created_at: datetime
    updated_at: datetime


class ExportHistoryResponse(BaseModel):
    items: list[ExportResponse]
    total: int
