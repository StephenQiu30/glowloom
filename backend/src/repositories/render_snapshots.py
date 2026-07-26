from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Any
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from schemas.rendering import (
    RenderInputRefsV1,
    RenderRecipeV1,
    RenderSegmentV1,
    RenderSnapshot,
)


class RenderSnapshotRepository:
    async def get(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        snapshot_id: UUID,
        *,
        for_update: bool = False,
    ) -> RenderSnapshot | None:
        suffix = " WHERE id=$1"
        if for_update:
            suffix += " FOR UPDATE"
        row = await connection.fetchrow("SELECT * FROM render_snapshots" + suffix, snapshot_id)
        return self._map(row) if row else None

    async def find_submission(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        submission_scope: str,
        idempotency_key: str,
    ) -> RenderSnapshot | None:
        row = await connection.fetchrow(
            """
            SELECT * FROM render_snapshots
            WHERE submission_scope=$1 AND idempotency_key=$2 FOR UPDATE
            """,
            submission_scope,
            idempotency_key,
        )
        return self._map(row) if row else None

    async def insert(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        episode_id: UUID,
        submission_scope: str,
        idempotency_key: str,
        request_hash: str,
        input_refs: RenderInputRefsV1,
        segments: tuple[RenderSegmentV1, ...],
        recipe: RenderRecipeV1,
        recipe_hash: str,
        content_hash: str,
    ) -> RenderSnapshot:
        row = await connection.fetchrow(
            """
            INSERT INTO render_snapshots(
                id,episode_id,submission_scope,idempotency_key,request_hash,
                shot_spec_version_id,subtitle_version_id,input_refs_json,segments_json,
                normalization_json,recipe_hash,content_hash
            ) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10::jsonb,$11,$12)
            RETURNING *
            """,
            new_id(),
            episode_id,
            submission_scope,
            idempotency_key,
            request_hash,
            input_refs.shot_spec_version_id,
            input_refs.subtitle_version_id,
            input_refs.model_dump_json(),
            json.dumps([item.model_dump(mode="json") for item in segments]),
            recipe.model_dump_json(),
            recipe_hash,
            content_hash,
        )
        if row is None:
            raise RuntimeError("created render snapshot could not be read")
        return self._map(row)

    async def bind_initial_task(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        snapshot_id: UUID,
        task_id: UUID,
    ) -> RenderSnapshot:
        row = await connection.fetchrow(
            """
            UPDATE render_snapshots SET initial_task_id=$2
            WHERE id=$1 AND (initial_task_id IS NULL OR initial_task_id=$2)
            RETURNING *
            """,
            snapshot_id,
            task_id,
        )
        if row is None:
            raise RuntimeError("render snapshot is bound to another task")
        return self._map(row)

    @staticmethod
    def _map(row: Mapping[str, Any]) -> RenderSnapshot:
        return RenderSnapshot(
            id=row["id"],
            episode_id=row["episode_id"],
            submission_scope=row["submission_scope"],
            idempotency_key=row["idempotency_key"],
            request_hash=row["request_hash"],
            initial_task_id=row["initial_task_id"],
            shot_spec_version_id=row["shot_spec_version_id"],
            subtitle_version_id=row["subtitle_version_id"],
            input_refs=RenderInputRefsV1.model_validate_json(_json_text(row["input_refs_json"])),
            segments=tuple(
                RenderSegmentV1.model_validate_json(json.dumps(item))
                for item in _json_value(row["segments_json"])
            ),
            recipe=RenderRecipeV1.model_validate_json(_json_text(row["normalization_json"])),
            recipe_hash=row["recipe_hash"],
            content_hash=row["content_hash"],
            created_at=row["created_at"],
        )


def _json_value(value: object) -> Any:
    return json.loads(value) if isinstance(value, str) else value


def _json_text(value: object) -> str:
    return value if isinstance(value, str) else json.dumps(value)
