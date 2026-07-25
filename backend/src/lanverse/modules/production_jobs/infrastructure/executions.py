from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from uuid import UUID

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.jobs.dispatch import JobPayload
from lanverse.modules.production_jobs.domain.state_machines import transition_task
from lanverse.modules.production_jobs.infrastructure.events import TaskEventRepository


@dataclass(frozen=True, slots=True)
class ExecutionPlan:
    task_id: UUID
    attempt_id: UUID
    provider_id: str
    provider_request_key: str
    prompt: str
    reconcile_first: bool
    skip: bool = False


class TaskExecutionStore:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._events = TaskEventRepository()

    async def prepare(self, payload: JobPayload) -> ExecutionPlan:
        async with self._database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT t.id task_id, t.status task_status, t.resource_version,
                       t.snapshot_id, s.provider_id, s.prompt,
                       a.id attempt_id, a.status attempt_status, a.provider_request_key
                FROM production_tasks t
                JOIN submission_snapshots s ON s.id=t.snapshot_id
                JOIN LATERAL (
                    SELECT * FROM production_attempts WHERE task_id=t.id
                    ORDER BY attempt_no DESC LIMIT 1
                ) a ON true
                WHERE t.id=$1
                FOR UPDATE OF t, a
                """,
                payload.task_id,
            )
            if row is None or row["snapshot_id"] != payload.snapshot_id:
                raise RuntimeError("TaskJob payload does not match its task snapshot")
            if row["task_status"] in {"cancelled", "succeeded", "failed"}:
                return ExecutionPlan(
                    task_id=row["task_id"],
                    attempt_id=row["attempt_id"],
                    provider_id=row["provider_id"],
                    provider_request_key=row["provider_request_key"] or "terminal",
                    prompt=row["prompt"],
                    reconcile_first=True,
                    skip=True,
                )
            if row["task_status"] == "queued":
                transition = transition_task("queued", "running", row["resource_version"])
                await connection.execute(
                    """
                    UPDATE production_tasks SET status='running', resource_version=$2,
                        progress_json='{"phase":"provider","completed":0,"total":1}',
                        updated_at=now() WHERE id=$1 AND resource_version=$3
                    """,
                    payload.task_id,
                    transition.resource_version,
                    transition.previous_resource_version,
                )
                await self._events.record(
                    connection,
                    task_id=payload.task_id,
                    resource_version=transition.resource_version,
                    event_type=transition.event_type,
                )
            request_key = row["provider_request_key"]
            reconcile_first = request_key is not None
            if request_key is None:
                request_key = hashlib.sha256(
                    f"{row['provider_id']}:{payload.task_id}:{row['attempt_id']}".encode()
                ).hexdigest()
                await connection.execute(
                    """
                    UPDATE production_attempts
                    SET provider_id=$2, provider_request_key=$3, status='submitted',
                        submitted_at=COALESCE(submitted_at,now()),
                        started_at=COALESCE(started_at,now())
                    WHERE id=$1 AND status='created'
                    """,
                    row["attempt_id"],
                    row["provider_id"],
                    request_key,
                )
            return ExecutionPlan(
                task_id=payload.task_id,
                attempt_id=row["attempt_id"],
                provider_id=row["provider_id"],
                provider_request_key=request_key,
                prompt=row["prompt"],
                reconcile_first=reconcile_first,
            )

    async def record_provider_success(
        self, plan: ExecutionPlan, provider_request_id: str
    ) -> None:
        async with self._database.transaction() as connection:
            await connection.execute(
                """
                UPDATE production_attempts
                SET status='provider_running', provider_request_id=$2
                WHERE id=$1 AND status IN ('submitted','unknown')
                """,
                plan.attempt_id,
                provider_request_id,
            )

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
            await self._events.record(
                connection,
                task_id=plan.task_id,
                resource_version=transition.resource_version,
                event_type=transition.event_type,
            )

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
            await self._events.record(
                connection,
                task_id=plan.task_id,
                resource_version=transition.resource_version,
                event_type=transition.event_type,
            )
