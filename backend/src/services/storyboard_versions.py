from __future__ import annotations

from dataclasses import dataclass, replace
from uuid import UUID

from db.pool import DatabasePool
from repositories.asset_versions import (
    CreativeAssetVersionRepository,
)
from repositories.storyboard_versions import (
    StoryboardVersionRepository,
)
from schemas.story_content import (
    ShotSpecCollectionV1,
)
from schemas.story_snapshots import (
    CreativeAssetVersionSnapshot,
    StoryboardVersionSnapshot,
)
from services.script_versions import (
    VersionConflict,
    VersionImmutable,
)


class StoryboardVersionNotFound(LookupError):
    pass


class StoryReferenceInvalid(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class SaveStoryboardCommand:
    version_id: UUID
    expected_resource_version: int
    content: ShotSpecCollectionV1


class _StoryboardHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._storyboards = StoryboardVersionRepository()
        self._assets = CreativeAssetVersionRepository()

    @staticmethod
    def validate_refs(
        board: StoryboardVersionSnapshot,
        assets: tuple[CreativeAssetVersionSnapshot, ...],
    ) -> None:
        version_ids = {item.id for item in assets}
        if version_ids != set(board.content.asset_version_ids):
            raise StoryReferenceInvalid("one or more creative asset versions are missing")
        if any(
            item.episode_id != board.episode_id
            or item.source_script_version_id != board.content.script_version_id
            for item in assets
        ):
            raise StoryReferenceInvalid("creative assets belong to another story input")


class SaveStoryboardHandler(_StoryboardHandler):
    async def execute(self, command: SaveStoryboardCommand) -> StoryboardVersionSnapshot:
        async with self._database.transaction() as connection:
            episode_id = await self._storyboards.episode_id_for(connection, command.version_id)
            if episode_id is None:
                raise StoryboardVersionNotFound
            await self._storyboards.lock_episode(connection, episode_id)
            current = await self._storyboards.get(connection, command.version_id, for_update=True)
            if current is None:
                raise StoryboardVersionNotFound
            if current.resource_version != command.expected_resource_version:
                raise VersionConflict
            if current.status != "draft":
                raise VersionImmutable
            if command.content.script_version_id != current.content.script_version_id:
                raise StoryReferenceInvalid("storyboard script input cannot be changed")
            assets = await self._assets.get_many(
                connection, command.content.asset_version_ids
            )
            candidate = replace(current, content=command.content)
            self.validate_refs(candidate, assets)
            return await self._storyboards.update_draft(
                connection, current, command.content
            )


class GetStoryboardVersionHandler(_StoryboardHandler):
    async def execute(self, version_id: UUID) -> StoryboardVersionSnapshot:
        async with self._database.transaction() as connection:
            value = await self._storyboards.get(connection, version_id)
        if value is None:
            raise StoryboardVersionNotFound
        return value


class GetStoryboardHandler(_StoryboardHandler):
    async def execute(self, episode_id: UUID) -> StoryboardVersionSnapshot:
        async with self._database.transaction() as connection:
            value = await self._storyboards.get_current(connection, episode_id)
        if value is None:
            raise StoryboardVersionNotFound
        return value


class ListStoryboardVersionsHandler(_StoryboardHandler):
    async def execute(self, episode_id: UUID) -> tuple[StoryboardVersionSnapshot, ...]:
        async with self._database.transaction() as connection:
            return await self._storyboards.list_for_episode(connection, episode_id)
