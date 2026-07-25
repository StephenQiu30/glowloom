from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.infrastructure.idempotency.repository import (
    IdempotencyKeyReused,
    IdempotencyRepository,
    canonical_request_hash,
)
from lanverse.modules.project_catalog.application.contracts import ProjectDetail
from lanverse.modules.project_catalog.domain.values import ProjectTitle
from lanverse.modules.project_catalog.infrastructure.projects import ProjectRepository
from lanverse.shared_kernel.ids import new_id

__all__ = ["CreateProjectCommand", "CreateProjectHandler", "IdempotencyKeyReused"]

OPERATION_SCOPE = "createProject/global"


@dataclass(frozen=True, slots=True)
class CreateProjectCommand:
    title: str
    idempotency_key: str


class CreateProjectHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._projects = ProjectRepository()
        self._idempotency = IdempotencyRepository()

    async def execute(self, command: CreateProjectCommand) -> ProjectDetail:
        title = ProjectTitle.create(command.title)
        request_hash = canonical_request_hash(
            method="POST",
            operation_id="createProject",
            path_parameters={},
            body={"title": command.title},
        )
        async with self._database.transaction() as connection:
            stored = await self._idempotency.reserve(
                connection,
                owner_module="project_catalog",
                operation_scope=OPERATION_SCOPE,
                idempotency_key=command.idempotency_key,
                request_hash=request_hash,
                request_id=new_id(),
            )
            if stored is not None:
                project = await self._projects.get_detail(
                    connection, UUID(str(stored.reference["project_id"]))
                )
                if project is None:
                    raise RuntimeError("idempotency response references a missing project")
                return project

            project_id = new_id()
            episode_id = new_id()
            await self._projects.insert_project_with_episode(
                connection, project_id=project_id, episode_id=episode_id, title=title.value
            )
            await self._idempotency.complete(
                connection,
                operation_scope=OPERATION_SCOPE,
                idempotency_key=command.idempotency_key,
                status=201,
                reference={"project_id": str(project_id), "episode_id": str(episode_id)},
            )
            project = await self._projects.get_detail(connection, project_id)
            if project is None:
                raise RuntimeError("created project could not be read")
            return project
