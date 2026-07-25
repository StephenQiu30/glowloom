from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.infrastructure.idempotency.repository import (
    IdempotencyRepository,
    canonical_request_hash,
)
from lanverse.modules.project_catalog.application.contracts import SourceRevisionSnapshot
from lanverse.modules.project_catalog.domain.values import SourceTextV1
from lanverse.modules.project_catalog.infrastructure.sources import SourceRevisionRepository
from lanverse.shared_kernel.ids import new_id


class InvalidRightsBasis(ValueError):
    pass


class EpisodeNotFound(LookupError):
    pass


class SourceRevisionNotFound(LookupError):
    pass


class SourceParentNotFound(LookupError):
    pass


@dataclass(frozen=True, slots=True)
class CreateSourceRevisionCommand:
    episode_id: UUID
    content: str
    rights_basis: str | None
    parent_id: UUID | None
    idempotency_key: str


class CreateSourceRevisionHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._sources = SourceRevisionRepository()
        self._idempotency = IdempotencyRepository()

    async def execute(self, command: CreateSourceRevisionCommand) -> SourceRevisionSnapshot:
        if command.rights_basis not in {"original", "licensed"}:
            raise InvalidRightsBasis
        text = SourceTextV1.create(command.content)
        request_hash = canonical_request_hash(
            method="POST",
            operation_id="createSourceRevision",
            path_parameters={"episode_id": str(command.episode_id)},
            body={
                "content": command.content,
                "rights_basis": command.rights_basis,
                "parent_id": str(command.parent_id) if command.parent_id else None,
            },
        )
        scope = f"createSourceRevision/{command.episode_id}"
        async with self._database.transaction() as connection:
            stored = await self._idempotency.reserve(
                connection,
                owner_module="project_catalog",
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                request_hash=request_hash,
                request_id=new_id(),
            )
            if stored is not None:
                source = await self._sources.get(
                    connection, UUID(str(stored.reference["source_revision_id"]))
                )
                if source is None:
                    raise RuntimeError("idempotency response references a missing source")
                return source
            if not await self._sources.lock_episode(connection, command.episode_id):
                raise EpisodeNotFound
            if command.parent_id is not None:
                parent = await self._sources.get(connection, command.parent_id)
                if parent is None or parent.episode_id != command.episode_id:
                    raise SourceParentNotFound
            source = await self._sources.insert(
                connection,
                revision_id=new_id(),
                episode_id=command.episode_id,
                parent_id=command.parent_id,
                text=text,
                rights_basis=command.rights_basis,
            )
            await self._idempotency.complete(
                connection,
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                status=201,
                reference={"source_revision_id": str(source.id)},
            )
            return source


class GetSourceRevisionHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._sources = SourceRevisionRepository()

    async def execute(self, revision_id: UUID) -> SourceRevisionSnapshot:
        async with self._database.transaction() as connection:
            source = await self._sources.get(connection, revision_id)
        if source is None:
            raise SourceRevisionNotFound
        return source


class ListSourceRevisionsHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._sources = SourceRevisionRepository()

    async def execute(self, episode_id: UUID) -> tuple[SourceRevisionSnapshot, ...]:
        async with self._database.transaction() as connection:
            if not await self._sources.lock_episode(connection, episode_id):
                raise EpisodeNotFound
            return await self._sources.list_for_episode(connection, episode_id)
