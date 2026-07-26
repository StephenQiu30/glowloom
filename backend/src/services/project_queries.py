from __future__ import annotations

from uuid import UUID

from db.pool import DatabasePool
from repositories.projects import ProjectRepository
from schemas.projects import (
    EpisodeSnapshot,
    ProjectDetail,
)
from services.sources import EpisodeNotFound


class ProjectNotFound(LookupError):
    pass


class ListProjectsHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._projects = ProjectRepository()

    async def execute(self) -> tuple[ProjectDetail, ...]:
        async with self._database.transaction() as connection:
            return await self._projects.list_details(connection)


class GetProjectHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._projects = ProjectRepository()

    async def execute(self, project_id: UUID) -> ProjectDetail:
        async with self._database.transaction() as connection:
            project = await self._projects.get_detail(connection, project_id)
        if project is None:
            raise ProjectNotFound
        return project


class GetEpisodeHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._projects = ProjectRepository()

    async def execute(self, episode_id: UUID) -> EpisodeSnapshot:
        async with self._database.transaction() as connection:
            episode = await self._projects.get_episode(connection, episode_id)
        if episode is None:
            raise EpisodeNotFound
        return episode
