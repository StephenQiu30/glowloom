from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from db.pool import DatabasePool
from repositories.asset_versions import (
    CreativeAssetVersionRepository,
)
from schemas.story_content import (
    CreativeAssetContentV1,
)
from schemas.story_snapshots import (
    CreativeAssetVersionSnapshot,
)
from services.script_versions import (
    VersionConflict,
    VersionImmutable,
)


class CreativeAssetVersionNotFound(LookupError):
    pass


class CreativeAssetIdentityInvalid(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class SaveCreativeAssetCommand:
    version_id: UUID
    expected_resource_version: int
    content: CreativeAssetContentV1


class _AssetHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._assets = CreativeAssetVersionRepository()


class SaveCreativeAssetHandler(_AssetHandler):
    async def execute(
        self, command: SaveCreativeAssetCommand
    ) -> CreativeAssetVersionSnapshot:
        async with self._database.transaction() as connection:
            episode_id = await self._assets.episode_id_for(connection, command.version_id)
            if episode_id is None:
                raise CreativeAssetVersionNotFound
            await self._assets.lock_episode(connection, episode_id)
            current = await self._assets.get(connection, command.version_id, for_update=True)
            if current is None:
                raise CreativeAssetVersionNotFound
            if current.resource_version != command.expected_resource_version:
                raise VersionConflict
            if current.status != "draft":
                raise VersionImmutable
            if (
                command.content.asset_id != current.asset_id
                or command.content.asset_type != current.asset_type
            ):
                raise CreativeAssetIdentityInvalid(
                    "stable asset identity and type cannot be changed"
                )
            return await self._assets.update_draft(connection, current, command.content)


class GetCreativeAssetVersionHandler(_AssetHandler):
    async def execute(self, version_id: UUID) -> CreativeAssetVersionSnapshot:
        async with self._database.transaction() as connection:
            value = await self._assets.get(connection, version_id)
        if value is None:
            raise CreativeAssetVersionNotFound
        return value


class ListCreativeAssetsHandler(_AssetHandler):
    async def execute(self, episode_id: UUID) -> tuple[CreativeAssetVersionSnapshot, ...]:
        async with self._database.transaction() as connection:
            return await self._assets.list_for_episode(connection, episode_id)
