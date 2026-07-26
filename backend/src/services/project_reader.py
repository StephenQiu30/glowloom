"""Public read contracts exported to downstream modules."""

from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from db.pool import DatabasePool
from repositories.sources import SourceRevisionRepository
from schemas.projects import (
    EpisodeSnapshot,
    ProjectDetail,
    ProjectSnapshot,
    SourceRevisionSnapshot,
)

__all__ = [
    "ConfirmedSourceNotFound",
    "EpisodeSnapshot",
    "ProjectCatalogReader",
    "ProjectDetail",
    "ProjectSnapshot",
    "SourceCompatibility",
    "SourceRevisionSnapshot",
]


class ConfirmedSourceNotFound(LookupError):
    pass


@dataclass(frozen=True, slots=True)
class SourceCompatibility:
    input_source_revision_id: UUID
    current_source_revision_id: UUID | None
    input_outdated: bool


class ProjectCatalogReader:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._sources = SourceRevisionRepository()

    async def confirmed_source(self, episode_id: UUID) -> SourceRevisionSnapshot:
        async with self._database.transaction() as connection:
            source = await self._sources.get_current(connection, episode_id)
        if source is None:
            raise ConfirmedSourceNotFound
        return source

    async def source_compatibility(
        self, episode_id: UUID, input_source_revision_id: UUID
    ) -> SourceCompatibility:
        async with self._database.transaction() as connection:
            input_source = await self._sources.get(connection, input_source_revision_id)
            if input_source is None or input_source.episode_id != episode_id:
                raise ConfirmedSourceNotFound
            current = await self._sources.get_current(connection, episode_id)
        current_id = current.id if current else None
        return SourceCompatibility(
            input_source_revision_id=input_source_revision_id,
            current_source_revision_id=current_id,
            input_outdated=current_id != input_source_revision_id,
        )
