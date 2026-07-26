from __future__ import annotations

from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id


class TaskEventRepository:
    async def record(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        task_id: UUID,
        resource_version: int,
        event_type: str,
    ) -> None:
        await connection.execute(
            """
            INSERT INTO task_events(
                event_id,task_id,task_resource_version,event_type,correlation_id,data_json
            ) VALUES($1,$2,$3,$4,$5,'{}')
            """,
            new_id(),
            task_id,
            resource_version,
            event_type,
            new_id(),
        )
