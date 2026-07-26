from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from core.ids import new_id
from db.pool import DatabasePool
from repositories.idempotency import (
    IdempotencyRepository,
    canonical_request_hash,
)
from schemas.story_content import (
    ShotSpecCollectionV1,
    ShotV1,
    canonical_content_hash,
)
from schemas.story_snapshots import (
    StoryboardGenerationSnapshot,
)
from services.script_versions import (
    VersionConflict,
    VersionImmutable,
)
from services.storyboard_versions import (
    StoryboardVersionNotFound,
    StoryReferenceInvalid,
    _StoryboardHandler,
)


@dataclass(frozen=True, slots=True)
class ConfirmStoryboardCommand:
    version_id: UUID
    expected_resource_version: int


@dataclass(frozen=True, slots=True)
class DeriveStoryboardDraftCommand:
    version_id: UUID
    idempotency_key: str


class ConfirmStoryboardHandler(_StoryboardHandler):
    async def execute(
        self, command: ConfirmStoryboardCommand
    ) -> StoryboardGenerationSnapshot:
        async with self._database.transaction() as connection:
            episode_id = await self._storyboards.episode_id_for(connection, command.version_id)
            if episode_id is None:
                raise StoryboardVersionNotFound
            await self._storyboards.lock_episode(connection, episode_id)
            board = await self._storyboards.get(
                connection, command.version_id, for_update=True
            )
            if board is None:
                raise StoryboardVersionNotFound
            if board.resource_version != command.expected_resource_version:
                raise VersionConflict
            if board.status != "draft":
                raise VersionImmutable
            if board.input_outdated:
                raise VersionConflict
            if board.content_hash != canonical_content_hash(board.content):
                raise StoryReferenceInvalid("storyboard content hash does not match")
            assets = await self._assets.get_many(
                connection, board.content.asset_version_ids, for_update=True
            )
            self.validate_refs(board, assets)
            if any(item.status != "draft" for item in assets):
                raise VersionImmutable
            if any(
                item.content_hash != canonical_content_hash(item.content) for item in assets
            ):
                raise StoryReferenceInvalid("creative asset content hash does not match")
            confirmed_assets = await self._assets.confirm_many(connection, assets)
            confirmed_board = await self._storyboards.confirm(connection, board)
            return StoryboardGenerationSnapshot(
                assets=confirmed_assets, storyboard=confirmed_board
            )


class DeriveStoryboardDraftHandler(_StoryboardHandler):
    def __init__(self, database: DatabasePool) -> None:
        super().__init__(database)
        self._idempotency = IdempotencyRepository()

    async def execute(
        self, command: DeriveStoryboardDraftCommand
    ) -> StoryboardGenerationSnapshot:
        scope = f"deriveStoryboardDraft/{command.version_id}"
        request_hash = canonical_request_hash(
            method="POST",
            operation_id="deriveStoryboardDraft",
            path_parameters={"version_id": str(command.version_id)},
            body=None,
        )
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
                board = await self._storyboards.get(
                    connection, UUID(str(stored.reference["shot_spec_version_id"]))
                )
                if board is None:
                    raise RuntimeError("idempotency record references a missing storyboard")
                assets = await self._assets.get_many(
                    connection, board.content.asset_version_ids
                )
                return StoryboardGenerationSnapshot(assets=assets, storyboard=board)
            parent = await self._storyboards.get(connection, command.version_id)
            if parent is None:
                raise StoryboardVersionNotFound
            await self._storyboards.lock_episode(connection, parent.episode_id)
            parent = await self._storyboards.get(
                connection, command.version_id, for_update=True
            )
            if parent is None:
                raise StoryboardVersionNotFound
            if parent.status != "confirmed":
                raise VersionImmutable
            parents = await self._assets.get_many(
                connection, parent.content.asset_version_ids, for_update=True
            )
            self.validate_refs(parent, parents)
            if any(item.status != "confirmed" for item in parents):
                raise VersionImmutable
            assets = await self._assets.derive_many(connection, parents)
            mapping = {old.id: new.id for old, new in zip(parents, assets, strict=True)}
            content = self._rewrite_asset_refs(parent.content, mapping)
            board = await self._storyboards.derive(connection, parent, content)
            await self._idempotency.complete(
                connection,
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                status=201,
                reference={"shot_spec_version_id": str(board.id)},
            )
            return StoryboardGenerationSnapshot(assets=assets, storyboard=board)

    @staticmethod
    def _rewrite_asset_refs(
        content: ShotSpecCollectionV1, mapping: dict[UUID, UUID]
    ) -> ShotSpecCollectionV1:
        shots = tuple(
            ShotV1.create(
                shot_id=shot.shot_id,
                ordinal=shot.ordinal,
                narrative_purpose=shot.narrative_purpose,
                visual_prompt=shot.visual_prompt,
                action=shot.action,
                duration_ticks=shot.duration_ticks,
                asset_version_ids=tuple(mapping[item] for item in shot.asset_version_ids),
                speech_line_ids=shot.speech_line_ids,
            )
            for shot in content.shots
        )
        return ShotSpecCollectionV1(
            script_version_id=content.script_version_id,
            asset_version_ids=tuple(mapping[item] for item in content.asset_version_ids),
            speech_line_ids=content.speech_line_ids,
            shots=shots,
        )
