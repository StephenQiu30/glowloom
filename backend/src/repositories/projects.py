from __future__ import annotations

from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from domain.projects import ProductionSpec
from schemas.projects import (
    EpisodeSnapshot,
    ProjectDetail,
    ProjectSnapshot,
)


class ProjectRepository:
    DETAIL_SELECT = """
        SELECT p.id project_id, p.title, p.status, p.created_at project_created_at,
               p.updated_at project_updated_at, e.id episode_id,
               e.target_min_ticks, e.target_max_ticks,
               e.created_at episode_created_at, e.updated_at episode_updated_at,
               s.id current_source_revision_id
        FROM projects p JOIN episodes e ON e.project_id = p.id
        LEFT JOIN source_revisions s
          ON s.episode_id = e.id AND s.status = 'confirmed'
    """

    async def insert_project_with_episode(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        project_id: UUID,
        episode_id: UUID,
        title: str,
    ) -> None:
        await connection.execute(
            "INSERT INTO projects(id, title) VALUES($1, $2)", project_id, title
        )
        await connection.execute(
            "INSERT INTO episodes(id, project_id) VALUES($1, $2)", episode_id, project_id
        )

    async def get_detail(
        self, connection: asyncpg.Connection[asyncpg.Record], project_id: UUID
    ) -> ProjectDetail | None:
        row = await connection.fetchrow(
            self.DETAIL_SELECT + " WHERE p.id = $1",
            project_id,
        )
        return self._map_detail(row) if row else None

    async def list_details(
        self, connection: asyncpg.Connection[asyncpg.Record]
    ) -> tuple[ProjectDetail, ...]:
        rows = await connection.fetch(self.DETAIL_SELECT + " ORDER BY p.created_at DESC, p.id")
        return tuple(self._map_detail(row) for row in rows)

    async def get_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> EpisodeSnapshot | None:
        row = await connection.fetchrow(
            self.DETAIL_SELECT + " WHERE e.id = $1",
            episode_id,
        )
        return self._map_detail(row).episode if row else None

    @staticmethod
    def _map_detail(row: asyncpg.Record) -> ProjectDetail:
        project = ProjectSnapshot(
            id=row["project_id"],
            title=row["title"],
            status=row["status"],
            production_spec=ProductionSpec.standard(),
            created_at=row["project_created_at"],
            updated_at=row["project_updated_at"],
        )
        episode = EpisodeSnapshot(
            id=row["episode_id"],
            project_id=row["project_id"],
            target_min_ticks=row["target_min_ticks"],
            target_max_ticks=row["target_max_ticks"],
            current_source_revision_id=row["current_source_revision_id"],
            created_at=row["episode_created_at"],
            updated_at=row["episode_updated_at"],
        )
        return ProjectDetail(project=project, episode=episode)
