from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

from lanverse.modules.project_catalog.domain.values import ProductionSpec


@dataclass(frozen=True, slots=True)
class ProjectSnapshot:
    id: UUID
    title: str
    status: str
    production_spec: ProductionSpec
    created_at: datetime
    updated_at: datetime


@dataclass(frozen=True, slots=True)
class EpisodeSnapshot:
    id: UUID
    project_id: UUID
    target_min_ticks: int
    target_max_ticks: int
    current_source_revision_id: UUID | None
    created_at: datetime
    updated_at: datetime


@dataclass(frozen=True, slots=True)
class ProjectDetail:
    project: ProjectSnapshot
    episode: EpisodeSnapshot


@dataclass(frozen=True, slots=True)
class SourceRevisionSnapshot:
    id: UUID
    episode_id: UUID
    version: int
    parent_id: UUID | None
    content: str
    normalization_version: str
    codepoint_count: int
    sha256: str
    rights_basis: str
    rights_declared_at: datetime
    status: str
    resource_version: int
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None
