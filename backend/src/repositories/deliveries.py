from __future__ import annotations

import json
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from schemas.deliveries import DeliveryVersionSnapshot


class DeliveryRepository:
    async def lock_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> bool:
        return (
            await connection.fetchval(
                "SELECT true FROM episodes WHERE id=$1 FOR UPDATE", episode_id
            )
            is True
        )

    async def task_render_input(
        self, connection: asyncpg.Connection[asyncpg.Record], task_id: UUID
    ) -> tuple[UUID, UUID] | None:
        row = await connection.fetchrow(
            """
            SELECT task.episode_id,submission.input_refs_json
            FROM production_tasks task
            JOIN submission_snapshots submission ON submission.id=task.snapshot_id
            WHERE task.id=$1 AND task.type='render_episode'
              AND submission.type='render_episode'
            """,
            task_id,
        )
        if row is None:
            return None
        inputs = row["input_refs_json"]
        if isinstance(inputs, str):
            inputs = json.loads(inputs)
        try:
            snapshot_id = UUID(str(inputs["render_snapshot_id"]))
        except (KeyError, TypeError, ValueError) as error:
            raise RuntimeError("render task input is invalid") from error
        return row["episode_id"], snapshot_id

    async def get_by_task(
        self, connection: asyncpg.Connection[asyncpg.Record], task_id: UUID
    ) -> DeliveryVersionSnapshot | None:
        row = await connection.fetchrow(
            "SELECT * FROM delivery_versions WHERE render_task_id=$1", task_id
        )
        return self._map(row) if row else None

    async def insert_rendering(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        episode_id: UUID,
        task_id: UUID,
        render_snapshot_id: UUID,
    ) -> DeliveryVersionSnapshot:
        version = await connection.fetchval(
            "SELECT coalesce(max(version),0)+1 FROM delivery_versions WHERE episode_id=$1",
            episode_id,
        )
        row = await connection.fetchrow(
            """
            INSERT INTO delivery_versions(
                id,episode_id,version,render_task_id,render_snapshot_id
            ) VALUES($1,$2,$3,$4,$5) RETURNING *
            """,
            new_id(),
            episode_id,
            version,
            task_id,
            render_snapshot_id,
        )
        if row is None:
            raise RuntimeError("created delivery could not be read")
        return self._map(row)

    @staticmethod
    def _map(row: asyncpg.Record) -> DeliveryVersionSnapshot:
        return DeliveryVersionSnapshot(
            id=row["id"],
            episode_id=row["episode_id"],
            version=row["version"],
            render_task_id=row["render_task_id"],
            render_snapshot_id=row["render_snapshot_id"],
            status=row["status"],
            created_at=row["created_at"],
        )
