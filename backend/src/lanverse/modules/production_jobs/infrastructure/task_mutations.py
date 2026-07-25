from __future__ import annotations

import json
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from lanverse.modules.production_jobs.application.contracts import TaskSnapshot
from lanverse.modules.production_jobs.domain.state_machines import transition_task
from lanverse.modules.production_jobs.infrastructure.tasks import TaskRepository
from lanverse.shared_kernel.ids import new_id


class TaskMutationRepository:
    def __init__(self) -> None:
        self._tasks = TaskRepository()

    async def cancel(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        task: TaskSnapshot,
        correlation_id: UUID,
    ) -> TaskSnapshot:
        if task.status not in {"queued", "running"}:
            return task
        target = "cancelled" if task.status == "queued" else "cancelling"
        transition = transition_task(task.status, target, task.resource_version)
        finished = ", finished_at = now()" if target == "cancelled" else ""
        result = await connection.execute(
            f"""
            UPDATE production_tasks
            SET status=$2, resource_version=$3, updated_at=now(),
                progress_json=$4::jsonb {finished}
            WHERE id=$1 AND resource_version=$5
            """,
            task.id,
            target,
            transition.resource_version,
            json.dumps({"phase": target, "completed": 0, "total": 1}),
            transition.previous_resource_version,
        )
        if result != "UPDATE 1":
            raise RuntimeError("task cancellation compare-and-set failed")
        if target == "cancelled":
            await connection.execute(
                """
                UPDATE production_attempts SET status='cancelled', finished_at=now()
                WHERE task_id=$1 AND status='created'
                """,
                task.id,
            )
            await connection.execute(
                """
                UPDATE task_jobs SET state='completed', lease_owner=NULL, lease_until=NULL,
                    updated_at=now(), completed_at=now()
                WHERE task_id=$1 AND state <> 'completed'
                """,
                task.id,
            )
        await connection.execute(
            """
            INSERT INTO task_events(
                event_id, task_id, task_resource_version, event_type, correlation_id, data_json
            ) VALUES($1,$2,$3,$4,$5,'{}')
            """,
            new_id(),
            task.id,
            transition.resource_version,
            transition.event_type,
            correlation_id,
        )
        updated = await self._tasks.get(connection, task.id)
        if updated is None:
            raise RuntimeError("cancelled task could not be read")
        return updated
