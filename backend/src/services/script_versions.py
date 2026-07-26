from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from core.ids import new_id
from db.pool import DatabasePool
from repositories.idempotency import (
    IdempotencyRepository,
    canonical_request_hash,
)
from repositories.script_versions import (
    ScriptVersioningRepository,
)
from schemas.story_content import ScriptContentV1
from schemas.story_snapshots import (
    ScriptVersionSnapshot,
)


class ScriptVersionNotFound(LookupError):
    pass


class VersionConflict(Exception):
    pass


class VersionImmutable(Exception):
    pass


@dataclass(frozen=True, slots=True)
class SaveScriptCommand:
    version_id: UUID
    expected_resource_version: int
    content: ScriptContentV1


@dataclass(frozen=True, slots=True)
class ConfirmScriptCommand:
    version_id: UUID
    expected_resource_version: int


@dataclass(frozen=True, slots=True)
class DeriveScriptDraftCommand:
    version_id: UUID
    idempotency_key: str


class _ScriptHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._scripts = ScriptVersioningRepository()


class SaveScriptHandler(_ScriptHandler):
    async def execute(self, command: SaveScriptCommand) -> ScriptVersionSnapshot:
        async with self._database.transaction() as connection:
            episode_id = await self._scripts.episode_id_for(connection, command.version_id)
            if episode_id is None:
                raise ScriptVersionNotFound
            await self._scripts.lock_episode(connection, episode_id)
            current = await self._scripts.get(connection, command.version_id, for_update=True)
            if current is None:
                raise ScriptVersionNotFound
            if current.resource_version != command.expected_resource_version:
                raise VersionConflict
            if current.status != "draft":
                raise VersionImmutable
            return await self._scripts.update_draft(connection, current, command.content)


class ConfirmScriptHandler(_ScriptHandler):
    async def execute(self, command: ConfirmScriptCommand) -> ScriptVersionSnapshot:
        async with self._database.transaction() as connection:
            episode_id = await self._scripts.episode_id_for(connection, command.version_id)
            if episode_id is None:
                raise ScriptVersionNotFound
            await self._scripts.lock_episode(connection, episode_id)
            current = await self._scripts.get(connection, command.version_id, for_update=True)
            if current is None:
                raise ScriptVersionNotFound
            if current.resource_version != command.expected_resource_version:
                raise VersionConflict
            if current.status != "draft":
                raise VersionImmutable
            if current.input_outdated:
                raise VersionConflict
            return await self._scripts.confirm(connection, current)


class DeriveScriptDraftHandler(_ScriptHandler):
    def __init__(self, database: DatabasePool) -> None:
        super().__init__(database)
        self._idempotency = IdempotencyRepository()

    async def execute(self, command: DeriveScriptDraftCommand) -> ScriptVersionSnapshot:
        request_hash = canonical_request_hash(
            method="POST",
            operation_id="deriveScriptDraft",
            path_parameters={"version_id": str(command.version_id)},
            body=None,
        )
        scope = f"deriveScriptDraft/{command.version_id}"
        async with self._database.transaction() as connection:
            stored = await self._idempotency.reserve(
                connection,
                owner_module="story_development",
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                request_hash=request_hash,
                request_id=new_id(),
            )
            if stored is not None:
                value = await self._scripts.get(
                    connection, UUID(str(stored.reference["script_version_id"]))
                )
                if value is None:
                    raise RuntimeError("idempotency record references a missing script")
                return value
            parent = await self._scripts.get(connection, command.version_id)
            if parent is None:
                raise ScriptVersionNotFound
            await self._scripts.lock_episode(connection, parent.episode_id)
            parent = await self._scripts.get(connection, command.version_id, for_update=True)
            if parent is None:
                raise ScriptVersionNotFound
            if parent.status != "confirmed":
                raise VersionImmutable
            draft = await self._scripts.derive(connection, parent)
            await self._idempotency.complete(
                connection,
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                status=201,
                reference={"script_version_id": str(draft.id)},
            )
            return draft


class GetCurrentScriptHandler(_ScriptHandler):
    async def execute(self, episode_id: UUID) -> ScriptVersionSnapshot:
        async with self._database.transaction() as connection:
            value = await self._scripts.get_current(connection, episode_id)
        if value is None:
            raise ScriptVersionNotFound
        return value


class GetScriptVersionHandler(_ScriptHandler):
    async def execute(self, version_id: UUID) -> ScriptVersionSnapshot:
        async with self._database.transaction() as connection:
            value = await self._scripts.get(connection, version_id)
        if value is None:
            raise ScriptVersionNotFound
        return value


class ListScriptVersionsHandler(_ScriptHandler):
    async def execute(self, episode_id: UUID) -> tuple[ScriptVersionSnapshot, ...]:
        async with self._database.transaction() as connection:
            return await self._scripts.list_for_episode(connection, episode_id)
