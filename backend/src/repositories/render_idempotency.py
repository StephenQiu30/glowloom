from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Literal
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from repositories.idempotency import IdempotencyKeyReused


@dataclass(frozen=True, slots=True)
class RenderIdempotencyRecord:
    state: Literal["pending", "completed"]
    reference: dict[str, object] | None


class RenderIdempotencyRepository:
    async def reserve(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        scope: str,
        key: str,
        request_hash: str,
        request_id: UUID,
    ) -> RenderIdempotencyRecord:
        await connection.execute(
            """
            INSERT INTO idempotency_records(
                id,owner_module,operation_scope,idempotency_key,request_hash,request_id
            ) VALUES($1,'delivery',$2,$3,$4,$5)
            ON CONFLICT (operation_scope,idempotency_key) DO NOTHING
            """,
            new_id(),
            scope,
            key,
            request_hash,
            request_id,
        )
        row = await connection.fetchrow(
            """
            SELECT request_hash,state,response_ref_json FROM idempotency_records
            WHERE operation_scope=$1 AND idempotency_key=$2 FOR UPDATE
            """,
            scope,
            key,
        )
        if row is None or row["request_hash"] != request_hash:
            raise IdempotencyKeyReused
        reference = row["response_ref_json"]
        if isinstance(reference, str):
            reference = json.loads(reference)
        return RenderIdempotencyRecord(row["state"], reference)

    async def set_snapshot_reference(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        scope: str,
        key: str,
        snapshot_id: UUID,
    ) -> None:
        await connection.execute(
            """
            UPDATE idempotency_records SET response_ref_json=$3::jsonb,updated_at=now()
            WHERE operation_scope=$1 AND idempotency_key=$2 AND state='pending'
            """,
            scope,
            key,
            json.dumps(
                {
                    "resource_type": "render_snapshot",
                    "render_snapshot_id": str(snapshot_id),
                }
            ),
        )

    async def complete(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        scope: str,
        key: str,
        task_id: UUID,
        submission_snapshot_id: UUID,
        render_snapshot_id: UUID,
    ) -> None:
        await connection.execute(
            """
            UPDATE idempotency_records SET state='completed',response_status=202,
                response_ref_json=$3::jsonb,updated_at=now(),completed_at=now()
            WHERE operation_scope=$1 AND idempotency_key=$2 AND state='pending'
            """,
            scope,
            key,
            json.dumps(
                {
                    "resource_type": "production_task",
                    "task_id": str(task_id),
                    "snapshot_id": str(submission_snapshot_id),
                    "render_snapshot_id": str(render_snapshot_id),
                }
            ),
        )
