from __future__ import annotations

from uuid import UUID

from core.ids import new_id
from db.pool import DatabasePool
from repositories.idempotency import IdempotencyRepository, canonical_request_hash
from repositories.subtitles import SubtitleRepository
from schemas.subtitle_versions import SubtitleVersionSnapshot
from services.script_versions import VersionImmutable
from services.subtitle_commands import (
    CreateSubtitlesCommand,
    DeriveSubtitleDraftCommand,
    SubtitleVersionNotFound,
)
from services.subtitle_inputs import SubtitleInputBuilder


class CreateSubtitlesHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._subtitles = SubtitleRepository()
        self._idempotency = IdempotencyRepository()
        self._inputs = SubtitleInputBuilder()

    async def execute(self, command: CreateSubtitlesCommand) -> SubtitleVersionSnapshot:
        scope = f"createSubtitles/{command.episode_id}"
        request_hash = canonical_request_hash(
            method="POST",
            operation_id="createSubtitles",
            path_parameters={"episode_id": str(command.episode_id)},
            body=None,
        )
        async with self._database.transaction() as connection:
            if not await self._subtitles.lock_episode(connection, command.episode_id):
                raise SubtitleVersionNotFound
            stored = await self._idempotency.reserve(
                connection,
                owner_module="delivery",
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                request_hash=request_hash,
                request_id=new_id(),
            )
            if stored is not None:
                value = await self._subtitles.get(
                    connection, UUID(str(stored.reference["subtitle_version_id"]))
                )
                if value is None:
                    raise RuntimeError("idempotency record references a missing subtitle")
                return value
            frozen = await self._inputs.build(connection, command.episode_id)
            parent = await self._subtitles.get_current(connection, command.episode_id)
            value = await self._subtitles.insert(
                connection,
                episode_id=command.episode_id,
                parent_id=parent.id if parent else None,
                input_refs=frozen.refs,
                content=frozen.content,
            )
            await self._idempotency.complete(
                connection,
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                status=201,
                reference={"subtitle_version_id": str(value.id)},
            )
            return value


class DeriveSubtitleDraftHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._subtitles = SubtitleRepository()
        self._idempotency = IdempotencyRepository()

    async def execute(
        self, command: DeriveSubtitleDraftCommand
    ) -> SubtitleVersionSnapshot:
        scope = f"deriveSubtitleDraft/{command.version_id}"
        request_hash = canonical_request_hash(
            method="POST",
            operation_id="deriveSubtitleDraft",
            path_parameters={"version_id": str(command.version_id)},
            body=None,
        )
        async with self._database.transaction() as connection:
            stored = await self._idempotency.reserve(
                connection,
                owner_module="delivery",
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                request_hash=request_hash,
                request_id=new_id(),
            )
            if stored is not None:
                value = await self._subtitles.get(
                    connection, UUID(str(stored.reference["subtitle_version_id"]))
                )
                if value is None:
                    raise RuntimeError("idempotency record references a missing subtitle")
                return value
            parent = await self._subtitles.get(connection, command.version_id)
            if parent is None:
                raise SubtitleVersionNotFound
            await self._subtitles.lock_episode(connection, parent.episode_id)
            parent = await self._subtitles.get(
                connection, command.version_id, for_update=True
            )
            if parent is None:
                raise SubtitleVersionNotFound
            if parent.status != "confirmed":
                raise VersionImmutable
            value = await self._subtitles.insert(
                connection,
                episode_id=parent.episode_id,
                parent_id=parent.id,
                input_refs=parent.input_refs,
                content=parent.content,
            )
            await self._idempotency.complete(
                connection,
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                status=201,
                reference={"subtitle_version_id": str(value.id)},
            )
            return value
