from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from repositories.adopted_media import AdoptedMediaRepository
from repositories.asset_versions import CreativeAssetVersionRepository
from repositories.storyboard_versions import StoryboardVersionRepository
from schemas.media_registration import UsageType
from schemas.story_content import ShotV1, canonical_content_hash
from schemas.story_snapshots import (
    CreativeAssetVersionSnapshot,
    StoryboardVersionSnapshot,
)


class MediaInputNotFound(LookupError):
    pass


class MediaInputOutdated(ValueError):
    pass


class UnsupportedMediaUsage(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class MediaInputRequest:
    episode_id: UUID
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID


@dataclass(frozen=True, slots=True)
class FrozenMediaInput:
    capability: str
    input_refs: dict[str, object]
    prompt: str


class MediaInputFreezer:
    def __init__(self) -> None:
        self._assets = CreativeAssetVersionRepository()
        self._storyboards = StoryboardVersionRepository()
        self._adoptions = AdoptedMediaRepository()

    async def freeze(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        request: MediaInputRequest,
    ) -> FrozenMediaInput:
        if request.usage_type == "asset_image":
            return await self._asset_image(connection, request)
        if request.usage_type not in {"shot_image", "shot_video"}:
            raise UnsupportedMediaUsage("this media usage is not implemented yet")
        board, shot, assets = await self._shot_context(connection, request)
        base = self._shot_image_refs(board, shot, assets)
        prompt = self._prompt(
            shot.visual_prompt,
            f"动作：{shot.action}",
            *(f"{item.name}：{item.description}" for item in assets),
        )
        if request.usage_type == "shot_image":
            return FrozenMediaInput("image", base, prompt)
        return await self._shot_video(connection, board, shot, assets, base, prompt)

    async def _asset_image(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        request: MediaInputRequest,
    ) -> FrozenMediaInput:
        asset = await self._assets.get(connection, request.input_version_id, for_update=True)
        if asset is None or asset.episode_id != request.episode_id:
            raise MediaInputNotFound("creative asset version was not found")
        if (
            asset.asset_id != request.usage_id
            or asset.status != "confirmed"
            or asset.input_outdated
        ):
            raise MediaInputOutdated("asset is not the requested confirmed input")
        if asset.asset_type == "visual_style":
            raise UnsupportedMediaUsage("visual style does not have an image slot")
        refs = self._asset_image_refs(asset)
        prompt = self._prompt(
            f"为{asset.name}生成竖屏参考图。",
            f"类型：{asset.asset_type}。",
            f"设定：{asset.description}",
        )
        return FrozenMediaInput("image", refs, prompt)

    async def _shot_context(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        request: MediaInputRequest,
    ) -> tuple[
        StoryboardVersionSnapshot,
        ShotV1,
        tuple[CreativeAssetVersionSnapshot, ...],
    ]:
        board = await self._storyboards.get(
            connection, request.input_version_id, for_update=True
        )
        if (
            board is None
            or board.episode_id != request.episode_id
            or board.status != "confirmed"
            or board.input_outdated
        ):
            raise MediaInputOutdated("shot storyboard is not the current confirmed input")
        shot = next(
            (item for item in board.content.shots if item.shot_id == request.usage_id), None
        )
        if shot is None:
            raise MediaInputNotFound("shot was not found in the storyboard")
        assets = await self._assets.get_many(
            connection, shot.asset_version_ids, for_update=True
        )
        expected = set(shot.asset_version_ids)
        if {item.id for item in assets} != expected or any(
            item.episode_id != board.episode_id or item.status != "confirmed"
            for item in assets
        ):
            raise MediaInputOutdated("shot assets are not the confirmed input")
        return board, shot, tuple(sorted(assets, key=lambda item: item.asset_id))

    async def _shot_video(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        board: StoryboardVersionSnapshot,
        shot: ShotV1,
        assets: tuple[CreativeAssetVersionSnapshot, ...],
        shot_image_refs: dict[str, object],
        prompt: str,
    ) -> FrozenMediaInput:
        required: list[tuple[UsageType, UUID, UUID, dict[str, object]]] = [
            ("asset_image", item.asset_id, item.id, self._asset_image_refs(item))
            for item in assets
            if item.asset_type != "visual_style"
        ]
        required.append(("shot_image", shot.shot_id, board.id, shot_image_refs))
        adopted = []
        for usage_type, usage_id, version_id, refs in required:
            row = await self._adoptions.find_active_media(
                connection,
                episode_id=board.episode_id,
                usage_type=usage_type,
                usage_id=usage_id,
                input_version_id=version_id,
                input_hash=canonical_content_hash(refs),
            )
            if row is None:
                raise MediaInputOutdated("required active image adoption is missing or stale")
            adopted.append(row.frozen_ref())
        refs = {
            **shot_image_refs,
            "usage_type": "shot_video",
            "duration_ticks": shot.duration_ticks,
            "image_adoptions": sorted(
                adopted,
                key=lambda item: (str(item["usage_type"]), str(item["usage_id"])),
            ),
        }
        return FrozenMediaInput("video", refs, prompt)

    @staticmethod
    def _asset_image_refs(asset: CreativeAssetVersionSnapshot) -> dict[str, object]:
        return {
            "usage_type": "asset_image",
            "asset_id": str(asset.asset_id),
            "input_version_id": str(asset.id),
            "content_hash": asset.content_hash,
        }

    @staticmethod
    def _shot_image_refs(
        board: StoryboardVersionSnapshot,
        shot: ShotV1,
        assets: tuple[CreativeAssetVersionSnapshot, ...],
    ) -> dict[str, object]:
        return {
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
                for item in assets
            ],
        }

    @staticmethod
    def _prompt(*parts: str) -> str:
        return "\n".join(parts)[:4000]
