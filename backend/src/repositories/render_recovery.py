from __future__ import annotations

import json
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from db.pool import DatabasePool
from domain.task_states import transition_task
from repositories.render_executions import RenderExecutionPlan
from repositories.task_events import TaskEventRepository
from repositories.tasks import object_value

RENDER_RETRY_ERROR = "RENDER_RUNTIME_UNAVAILABLE"


class RenderRecoveryStore:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._events = TaskEventRepository()

    async def retry_or_fail(
        self,
        plan: RenderExecutionPlan,
        *,
        delivery_id: UUID,
        summary: str,
    ) -> bool:
        async with self._database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT task.status task_status,task.resource_version,task.snapshot_id,
                       attempt.status attempt_status,attempt.attempt_no,
                       attempt.usage_json,attempt.safety_json,
                       attempt.execution_metadata_json,delivery.status delivery_status
                FROM production_tasks task
                JOIN production_attempts attempt ON attempt.id=$2 AND attempt.task_id=task.id
                JOIN delivery_versions delivery ON delivery.id=$3
                  AND delivery.render_task_id=task.id
                WHERE task.id=$1 FOR UPDATE OF task,attempt,delivery
                """,
                plan.task_id,
                plan.attempt_id,
                delivery_id,
            )
            if row is None:
                raise RuntimeError("render retry facts do not match")
            if row["task_status"] != "running" or row["delivery_status"] != "rendering":
                return False
            if row["attempt_status"] != "postprocessing" or row["attempt_no"] != plan.attempt_no:
                raise RuntimeError("render retry attempt is invalid")
            await connection.execute(
                """
                UPDATE production_attempts SET status='failed',error_code=$2,
                    error_summary=$3,finished_at=now() WHERE id=$1
                """,
                plan.attempt_id,
                RENDER_RETRY_ERROR,
                summary,
            )
            if plan.attempt_no < 3:
                await self._create_attempt(connection, plan, row)
                return True
            await connection.execute(
                """
                UPDATE delivery_versions SET final_attempt_id=$2,status='failed',
                    error_code=$3,updated_at=now(),finished_at=now()
                WHERE id=$1 AND status='rendering'
                """,
                delivery_id,
                plan.attempt_id,
                RENDER_RETRY_ERROR,
            )
            transition = transition_task("running", "failed", row["resource_version"])
            await connection.execute(
                """
                UPDATE production_tasks SET status='failed',resource_version=$2,
                    progress_json='{"phase":"failed","completed":0,"total":1}',
                    error_code=$3,error_json=$4::jsonb,
                    next_action='Use an explicit user retry',updated_at=now(),
                    finished_at=now() WHERE id=$1 AND resource_version=$5
                """,
                plan.task_id,
                transition.resource_version,
                RENDER_RETRY_ERROR,
                json.dumps({"retryable": True, "summary": summary}),
                transition.previous_resource_version,
            )
            await self._events.record(
                connection,
                task_id=plan.task_id,
                resource_version=transition.resource_version,
                event_type=transition.event_type,
            )
            return False

    async def _create_attempt(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        plan: RenderExecutionPlan,
        row: asyncpg.Record,
    ) -> None:
        await connection.execute(
            """
            INSERT INTO production_attempts(
                id,task_id,snapshot_id,attempt_no,parent_attempt_id,status,
                usage_json,safety_json,execution_metadata_json
            ) VALUES($1,$2,$3,$4,$5,'created',$6::jsonb,$7::jsonb,$8::jsonb)
            """,
            new_id(),
            plan.task_id,
            row["snapshot_id"],
            plan.attempt_no + 1,
            plan.attempt_id,
            json.dumps(object_value(row["usage_json"])),
            json.dumps(object_value(row["safety_json"])),
            json.dumps(object_value(row["execution_metadata_json"])),
        )
        resource_version = row["resource_version"] + 1
        await connection.execute(
            """
            UPDATE production_tasks SET resource_version=$2,
                progress_json='{"phase":"retry_queued","completed":0,"total":1}',
                updated_at=now() WHERE id=$1 AND resource_version=$3
            """,
            plan.task_id,
            resource_version,
            row["resource_version"],
        )
        await self._events.record(
            connection,
            task_id=plan.task_id,
            resource_version=resource_version,
            event_type="attempt.created",
        )
