from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.shared_kernel.ids import new_id


class OutputSlotConflict(Exception):
    pass


@dataclass(frozen=True, slots=True)
class TaskOutputSnapshot:
    id: UUID
    task_id: UUID
    output_type: str
    output_id: UUID
    ordinal: int


class TaskOutputStore:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database

    async def record(
        self,
        task_id: UUID,
        output_type: str,
        output_id: UUID,
        *,
        ordinal: int,
    ) -> TaskOutputSnapshot:
        async with self._database.transaction() as connection:
            await connection.execute(
                """
                INSERT INTO task_outputs(id, task_id, output_type, output_id, ordinal)
                VALUES($1,$2,$3,$4,$5)
                ON CONFLICT DO NOTHING
                """,
                new_id(),
                task_id,
                output_type,
                output_id,
                ordinal,
            )
            row = await connection.fetchrow(
                """
                SELECT id, task_id, output_type, output_id, ordinal
                FROM task_outputs
                WHERE task_id=$1 AND output_type=$2 AND ordinal=$3
                FOR UPDATE
                """,
                task_id,
                output_type,
                ordinal,
            )
            if row is None or row["output_id"] != output_id:
                raise OutputSlotConflict
            return TaskOutputSnapshot(
                id=row["id"],
                task_id=row["task_id"],
                output_type=row["output_type"],
                output_id=row["output_id"],
                ordinal=row["ordinal"],
            )
