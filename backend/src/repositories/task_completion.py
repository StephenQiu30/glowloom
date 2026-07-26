from __future__ import annotations

import json

import asyncpg  # type: ignore[import-untyped]

from db.pool import DatabasePool
from domain.task_states import transition_task
from repositories.task_events import TaskEventRepository
from repositories.task_executions import ExecutionPlan


class ExecutionCompletionStore:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._events = TaskEventRepository()

    async def mark_unknown(self, plan: ExecutionPlan) -> None:
        async with self._database.transaction() as connection:
            task = await connection.fetchrow(
                "SELECT status,resource_version FROM production_tasks WHERE id=$1 FOR UPDATE",
                plan.task_id,
            )
            if task is None or task["status"] == "unknown":
                return
            transition = transition_task(task["status"], "unknown", task["resource_version"])
            await connection.execute(
                """
                UPDATE production_attempts SET status='unknown',
                    error_code='PROVIDER_ACCEPTANCE_UNKNOWN',
                    error_summary='Provider acceptance could not be proven'
                WHERE id=$1 AND status NOT IN ('succeeded','failed','cancelled')
                """,
                plan.attempt_id,
            )
            await connection.execute(
                """
                UPDATE production_tasks SET status='unknown', resource_version=$2,
                    progress_json='{"phase":"reconcile","completed":0,"total":1}',
                    error_code='PROVIDER_ACCEPTANCE_UNKNOWN',
                    error_json=$3::jsonb, next_action='Reconcile before retrying', updated_at=now()
                WHERE id=$1 AND resource_version=$4
                """,
                plan.task_id,
                transition.resource_version,
                json.dumps({"retryable": False, "summary": "Provider acceptance is unknown"}),
                transition.previous_resource_version,
            )
            await self._record(connection, plan, transition.resource_version, transition.event_type)

    async def mark_succeeded(self, plan: ExecutionPlan) -> None:
        async with self._database.transaction() as connection:
            task = await connection.fetchrow(
                "SELECT status,resource_version FROM production_tasks WHERE id=$1 FOR UPDATE",
                plan.task_id,
            )
            if task is None or task["status"] == "succeeded":
                return
            await connection.execute(
                """
                UPDATE production_attempts SET status='succeeded', finished_at=now(),
                    error_code=NULL, error_summary=NULL
                WHERE id=$1 AND status NOT IN ('succeeded','failed','cancelled')
                """,
                plan.attempt_id,
            )
            transition = transition_task(task["status"], "succeeded", task["resource_version"])
            await connection.execute(
                """
                UPDATE production_tasks SET status='succeeded', resource_version=$2,
                    progress_json='{"phase":"completed","completed":1,"total":1}',
                    error_code=NULL,error_json=NULL,next_action=NULL,
                    updated_at=now(),finished_at=now()
                WHERE id=$1 AND resource_version=$3
                """,
                plan.task_id,
                transition.resource_version,
                transition.previous_resource_version,
            )
            await self._record(connection, plan, transition.resource_version, transition.event_type)

    async def mark_failed(
        self,
        plan: ExecutionPlan,
        *,
        error_code: str,
        summary: str,
        retryable: bool,
        next_action: str,
    ) -> None:
        async with self._database.transaction() as connection:
            task = await connection.fetchrow(
                "SELECT status,resource_version FROM production_tasks WHERE id=$1 FOR UPDATE",
                plan.task_id,
            )
            if task is None or task["status"] == "failed":
                return
            transition = transition_task(task["status"], "failed", task["resource_version"])
            await connection.execute(
                """
                UPDATE production_attempts SET status='failed',finished_at=now(),
                    error_code=$2,error_summary=$3
                WHERE id=$1 AND status NOT IN ('succeeded','failed','cancelled')
                """,
                plan.attempt_id,
                error_code,
                summary,
            )
            await connection.execute(
                """
                UPDATE production_tasks SET status='failed',resource_version=$2,
                    progress_json='{"phase":"failed","completed":0,"total":1}',
                    error_code=$3,error_json=$4::jsonb,next_action=$5,
                    updated_at=now(),finished_at=now()
                WHERE id=$1 AND resource_version=$6
                """,
                plan.task_id,
                transition.resource_version,
                error_code,
                json.dumps({"retryable": retryable, "summary": summary}),
                next_action,
                transition.previous_resource_version,
            )
            await self._record(
                connection, plan, transition.resource_version, transition.event_type
            )

    async def mark_cancelled(self, plan: ExecutionPlan) -> None:
        async with self._database.transaction() as connection:
            task = await connection.fetchrow(
                "SELECT status,resource_version FROM production_tasks WHERE id=$1 FOR UPDATE",
                plan.task_id,
            )
            if task is None or task["status"] == "cancelled":
                return
            transition = transition_task(task["status"], "cancelled", task["resource_version"])
            await connection.execute(
                """
                UPDATE production_attempts SET status='cancelled',finished_at=now()
                WHERE id=$1 AND status NOT IN ('succeeded','failed','cancelled')
                """,
                plan.attempt_id,
            )
            await connection.execute(
                """
                UPDATE production_tasks SET status='cancelled',resource_version=$2,
                    progress_json='{"phase":"cancelled","completed":0,"total":1}',
                    error_code=NULL,error_json=NULL,next_action=NULL,
                    updated_at=now(),finished_at=now()
                WHERE id=$1 AND resource_version=$3
                """,
                plan.task_id,
                transition.resource_version,
                transition.previous_resource_version,
            )
            await self._record(connection, plan, transition.resource_version, transition.event_type)

    async def _record(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        plan: ExecutionPlan,
        resource_version: int,
        event_type: str,
    ) -> None:
        await self._events.record(
            connection,
            task_id=plan.task_id,
            resource_version=resource_version,
            event_type=event_type,
        )
