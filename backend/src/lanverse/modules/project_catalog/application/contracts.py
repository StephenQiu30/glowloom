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
