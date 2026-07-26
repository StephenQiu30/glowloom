from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from db.pool import DatabasePool
from integrations.ai.registry import AiModelRegistry, create_mvp_registry
from repositories.asset_versions import CreativeAssetVersionRepository
from repositories.storyboard_versions import StoryboardVersionRepository
from schemas.media_registration import UsageType
from schemas.story_content import canonical_content_hash
from schemas.story_snapshots import (
    CreativeAssetVersionSnapshot,
    StoryboardVersionSnapshot,
)
from schemas.tasks import SubmitTaskCommand, TaskAcceptedSnapshot
from services.task_submission import TaskSubmitter


class MediaInputNotFound(LookupError):
    pass


class MediaInputOutdated(ValueError):
    pass


class UnsupportedMediaUsage(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class GenerateMediaCommand:
    episode_id: UUID
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    idempotency_key: str
    model_profile_id: str | None = None


@dataclass(frozen=True, slots=True)
class FrozenImageInput:
    input_refs: dict[str, object]
    prompt: str


class GenerateMediaHandler:
    def __init__(
        self,
        database: DatabasePool,
        *,
        release_version: str,
        registry: AiModelRegistry | None = None,
    ) -> None:
        self._database = database
        self._registry = registry or create_mvp_registry()
        self._tasks = TaskSubmitter(database, release_version=release_version)
        self._assets = CreativeAssetVersionRepository()
        self._storyboards = StoryboardVersionRepository()

    async def execute(self, command: GenerateMediaCommand) -> TaskAcceptedSnapshot:
        if command.usage_type not in {"asset_image", "shot_image"}:
            raise UnsupportedMediaUsage("this media usage is not implemented yet")
        profile = self._registry.select(
            "image", command.model_profile_id, schema_version="image-v1"
        )
        async with self._database.transaction() as connection:
            await self._storyboards.lock_episode(connection, command.episode_id)
            if command.usage_type == "asset_image":
                frozen = await self._freeze_asset_image(connection, command)
            else:
                frozen = await self._freeze_shot_image(connection, command)
            input_hash = canonical_content_hash(frozen.input_refs)
            input_refs = {**frozen.input_refs, "input_hash": input_hash}
            return await self._tasks.submit_in_transaction(
                connection,
                SubmitTaskCommand(
                    episode_id=command.episode_id,
                    task_type="generate_media",
                    capability="image",
                    scope={
                        "episode_id": str(command.episode_id),
                        "usage_type": command.usage_type,
                        "usage_id": str(command.usage_id),
                    },
                    input_refs=input_refs,
                    prompt=frozen.prompt,
                    parameters=profile.parameters,
                    model_profile_id=profile.model_profile_id,
                    provider_id=profile.provider_id,
                    model_id=profile.model_id,
                    route_version=profile.route_version,
                    schema_version=profile.schema_version,
                    operation_scope=(
                        f"generateMedia/{command.usage_type}/{command.usage_id}/"
                        f"{command.input_version_id}/{input_hash}"
                    ),
                    idempotency_key=command.idempotency_key,
                    handler_version="1",
                ),
            )

    async def _freeze_asset_image(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        command: GenerateMediaCommand,
    ) -> FrozenImageInput:
        asset = await self._assets.get(connection, command.input_version_id, for_update=True)
        self._validate_asset(asset, command)
        assert asset is not None
        refs: dict[str, object] = {
            "usage_type": "asset_image",
            "asset_id": str(asset.asset_id),
            "input_version_id": str(asset.id),
            "content_hash": asset.content_hash,
        }
        prompt = self._prompt(
            f"为{asset.name}生成竖屏参考图。",
            f"类型：{asset.asset_type}。",
            f"设定：{asset.description}",
        )
        return FrozenImageInput(refs, prompt)

    async def _freeze_shot_image(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        command: GenerateMediaCommand,
    ) -> FrozenImageInput:
        board = await self._storyboards.get(
            connection, command.input_version_id, for_update=True
        )
        if (
            board is None
            or board.episode_id != command.episode_id
            or board.status != "confirmed"
            or board.input_outdated
        ):
            raise MediaInputOutdated("shot storyboard is not the current confirmed input")
        shot = next(
            (item for item in board.content.shots if item.shot_id == command.usage_id), None
        )
        if shot is None:
            raise MediaInputNotFound("shot was not found in the storyboard")
        assets = await self._assets.get_many(
            connection, shot.asset_version_ids, for_update=True
        )
        self._validate_shot_assets(board, assets, set(shot.asset_version_ids))
        related = tuple(sorted(assets, key=lambda item: item.asset_id))
        refs: dict[str, object] = {
            "usage_type": "shot_image",
            "shot_id": str(shot.shot_id),
            "input_version_id": str(board.id),
            "shot_content_hash": shot.content_hash,
            "asset_versions": [
                {
                    "asset_id": str(item.asset_id),
                    "version_id": str(item.id),
                    "content_hash": item.content_hash,
                }
                for item in related
            ],
        }
        prompt = self._prompt(
            shot.visual_prompt,
            f"动作：{shot.action}",
            *(f"{item.name}：{item.description}" for item in related),
        )
        return FrozenImageInput(refs, prompt)

    @staticmethod
    def _validate_asset(
        asset: CreativeAssetVersionSnapshot | None, command: GenerateMediaCommand
    ) -> None:
        if asset is None or asset.episode_id != command.episode_id:
            raise MediaInputNotFound("creative asset version was not found")
        if asset.asset_id != command.usage_id or asset.status != "confirmed":
            raise MediaInputOutdated("asset is not the requested confirmed input")
        if asset.asset_type == "visual_style":
            raise UnsupportedMediaUsage("visual style does not have an image slot")

    @staticmethod
    def _validate_shot_assets(
        board: StoryboardVersionSnapshot,
        assets: tuple[CreativeAssetVersionSnapshot, ...],
        expected_ids: set[UUID],
    ) -> None:
        if {item.id for item in assets} != expected_ids or any(
            item.episode_id != board.episode_id or item.status != "confirmed"
            for item in assets
        ):
            raise MediaInputOutdated("shot assets are not the confirmed input")

    @staticmethod
    def _prompt(*parts: str) -> str:
        return "\n".join(parts)[:4000]
