from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from db.pool import DatabasePool
from repositories.idempotency import IdempotencyKeyReused, canonical_request_hash
from repositories.render_snapshots import RenderSnapshotRepository
from repositories.subtitles import SubtitleRepository
from schemas.rendering import RenderRecipeV1, RenderSnapshot
from schemas.story_content import canonical_content_hash
from services.render_inputs import RenderInputBuilder, RenderInputInvalid


@dataclass(frozen=True, slots=True)
class CreateRenderSnapshotCommand:
    episode_id: UUID
    submission_scope: str
    idempotency_key: str


class CreateRenderSnapshotHandler:
    def __init__(self, database: DatabasePool, *, recipe: RenderRecipeV1) -> None:
        self._database = database
        self._recipe = recipe
        self._subtitles = SubtitleRepository()
        self._snapshots = RenderSnapshotRepository()
        self._inputs = RenderInputBuilder()

    async def execute(self, command: CreateRenderSnapshotCommand) -> RenderSnapshot:
        async with self._database.transaction() as connection:
            if not await self._subtitles.lock_episode(connection, command.episode_id):
                raise RenderInputInvalid("episode does not exist")
            return await self.create_in_transaction(connection, command)

    async def create_in_transaction(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        command: CreateRenderSnapshotCommand,
    ) -> RenderSnapshot:
        request_hash = render_request_hash(command.episode_id)
        stored = await self._snapshots.find_submission(
            connection, command.submission_scope, command.idempotency_key
        )
        if stored is not None:
            if stored.request_hash != request_hash:
                raise IdempotencyKeyReused
            return stored
        frozen = await self._inputs.build(connection, command.episode_id)
        recipe_hash = canonical_content_hash(self._recipe)
        content_hash = canonical_content_hash(
            {
                "input_refs": frozen.refs,
                "segments": frozen.segments,
                "normalization": self._recipe,
            }
        )
        return await self._snapshots.insert(
            connection,
            episode_id=command.episode_id,
            submission_scope=command.submission_scope,
            idempotency_key=command.idempotency_key,
            request_hash=request_hash,
            input_refs=frozen.refs,
            segments=frozen.segments,
            recipe=self._recipe,
            recipe_hash=recipe_hash,
            content_hash=content_hash,
        )


def render_request_hash(episode_id: UUID) -> str:
    return canonical_request_hash(
        method="POST",
        operation_id="renderEpisode",
        path_parameters={"episode_id": str(episode_id)},
        body=None,
    )
