from __future__ import annotations

from typing import Any
from uuid import UUID

from db.pool import DatabasePool


async def render_state(database: DatabasePool, task_id: UUID) -> tuple[str, str, int]:
    async with database.transaction() as connection:
        row = await connection.fetchrow(
            """
            SELECT task.status task_status,delivery.status delivery_status,
                   count(version.id) artifact_count
            FROM production_tasks task
            JOIN delivery_versions delivery ON delivery.render_task_id=task.id
            LEFT JOIN production_attempts attempt ON attempt.task_id=task.id
            LEFT JOIN media_versions version ON version.origin_attempt_id=attempt.id
            WHERE task.id=$1 GROUP BY task.status,delivery.status
            """,
            task_id,
        )
    assert row is not None
    return row["task_status"], row["delivery_status"], int(row["artifact_count"])


async def historical_counts(database: DatabasePool, episode_id: UUID) -> tuple[int, int, int]:
    async with database.transaction() as connection:
        row = await connection.fetchrow(
            """
            SELECT (SELECT count(*) FROM adoptions WHERE episode_id=$1) adoptions,
                   (SELECT count(*) FROM generation_candidates WHERE episode_id=$1) candidates,
                   (SELECT count(*) FROM render_snapshots WHERE episode_id=$1) snapshots
            """,
            episode_id,
        )
    assert row is not None
    return int(row[0]), int(row[1]), int(row[2])


async def delivery_facts(database: DatabasePool, task_id: UUID) -> dict[str, Any]:
    async with database.transaction() as connection:
        delivery = await connection.fetchrow(
            """
            SELECT task.status task_status,attempt.id attempt_id,
                   attempt.status attempt_status,delivery.status delivery_status,
                   delivery.*,
                   (SELECT count(*) FROM task_outputs WHERE task_id=task.id
                    AND output_type='delivery_version') task_outputs
            FROM production_tasks task
            JOIN production_attempts attempt ON attempt.task_id=task.id
            JOIN delivery_versions delivery ON delivery.render_task_id=task.id
            WHERE task.id=$1
            """,
            task_id,
        )
        rows = await connection.fetch(
            """
            SELECT version.output_slot,object.source_kind,version.object_key,
                   version.sha256,version.byte_size
            FROM media_versions version JOIN media_objects object
              ON object.id=version.media_object_id
            JOIN production_attempts attempt ON attempt.id=version.origin_attempt_id
            WHERE attempt.task_id=$1 ORDER BY version.output_slot
            """,
            task_id,
        )
    assert delivery is not None
    result = dict(delivery)
    result["slots"] = [row["output_slot"] for row in rows]
    result["source_kinds"] = [row["source_kind"] for row in rows]
    result["artifacts"] = {row["output_slot"]: dict(row) for row in rows}
    return result
