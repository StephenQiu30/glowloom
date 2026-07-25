from __future__ import annotations

from dataclasses import asdict
from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import Field

from lanverse.modules.project_catalog.application.contracts import (
    EpisodeSnapshot,
    ProjectDetail,
    ProjectSnapshot,
    SourceRevisionSnapshot,
)
from lanverse.shared_kernel.http_contracts import StrictContract


class ProductionSpecResponse(StrictContract):
    aspect_ratio: Literal["9:16"]
    width: Literal[720]
    height: Literal[1280]
    fps: Literal[24]
    timebase: Literal[90000]
    target_min_ticks: Literal[2700000]
    target_max_ticks: Literal[5400000]


class ProjectResponse(StrictContract):
    id: UUID
    title: str
    status: Literal["active"]
    production_spec: ProductionSpecResponse
    created_at: datetime
    updated_at: datetime


class EpisodeResponse(StrictContract):
    id: UUID
    project_id: UUID
    target_min_ticks: Literal[2700000]
    target_max_ticks: Literal[5400000]
    current_source_revision_id: UUID | None
    created_at: datetime
    updated_at: datetime


class ProjectDetailResponse(StrictContract):
    project: ProjectResponse
    episode: EpisodeResponse


class ProjectListResponse(StrictContract):
    items: tuple[ProjectDetailResponse, ...]


class CreateProjectRequest(StrictContract):
    title: str = Field(json_schema_extra={"minLength": 1, "maxLength": 120})


class CreateSourceRevisionRequest(StrictContract):
    content: str = Field(json_schema_extra={"minLength": 300, "maxLength": 3000})
    rights_basis: Literal["original", "licensed"]
    parent_id: UUID | None = None


class SourceRevisionResponse(StrictContract):
    id: UUID
    episode_id: UUID
    version: int = Field(ge=1)
    parent_id: UUID | None
    content: str
    normalization_version: Literal["text-normalization-v1"]
    codepoint_count: int = Field(ge=300, le=3000)
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    rights_basis: Literal["original", "licensed"]
    rights_declared_at: datetime
    status: Literal["draft", "confirmed", "superseded"]
    resource_version: int = Field(ge=1)
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None


class SourceRevisionListResponse(StrictContract):
    items: tuple[SourceRevisionResponse, ...]


def project_response(value: ProjectSnapshot) -> ProjectResponse:
    return ProjectResponse(
        id=value.id,
        title=value.title,
        status="active",
        production_spec=ProductionSpecResponse(
            aspect_ratio="9:16",
            width=720,
            height=1280,
            fps=24,
            timebase=90000,
            target_min_ticks=2700000,
            target_max_ticks=5400000,
        ),
        created_at=value.created_at,
        updated_at=value.updated_at,
    )


def episode_response(value: EpisodeSnapshot) -> EpisodeResponse:
    return EpisodeResponse(**asdict(value))


def project_detail_response(value: ProjectDetail) -> ProjectDetailResponse:
    return ProjectDetailResponse(
        project=project_response(value.project), episode=episode_response(value.episode)
    )


def source_response(value: SourceRevisionSnapshot) -> SourceRevisionResponse:
    return SourceRevisionResponse(**asdict(value))
