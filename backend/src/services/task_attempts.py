from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any
from uuid import UUID

from core.ids import new_id
from db.pool import DatabasePool
from domain.task_states import transition_task
from repositories.task_events import TaskEventRepository
from repositories.tasks import object_value


class AutomaticRetryExhausted(Exception):
    pass


class AutomaticRetryNotAllowed(Exception):
    pass


@dataclass(frozen=True, slots=True)
class AttemptSnapshot:
    id: UUID
    task_id: UUID
    attempt_no: int
    parent_attempt_id: UUID | None
    status: str


class AutomaticRetryHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._events = TaskEventRepository()

    async def execute(self, task_id: UUID) -> AttemptSnapshot:
        exhausted = False
        result: AttemptSnapshot | None = None
        async with self._database.transaction() as connection:
            task = await connection.fetchrow(
                """
                SELECT status,resource_version,snapshot_id FROM production_tasks
                WHERE id=$1 FOR UPDATE
                """,
                task_id,
            )
            attempt = await connection.fetchrow(
                """
                SELECT * FROM production_attempts WHERE task_id=$1
                ORDER BY attempt_no DESC LIMIT 1
                """,
                task_id,
            )
            if task is None or attempt is None:
                raise AutomaticRetryNotAllowed
            if attempt["status"] != "failed":
                return self._map(attempt)
            if attempt["attempt_no"] >= 3:
                exhausted = True
                if task["status"] in {"running", "unknown"}:
                    transition = transition_task(
                        task["status"], "failed", task["resource_version"]
                    )
                    await connection.execute(
                        """
                        UPDATE production_tasks SET status='failed',resource_version=$2,
                            error_code='AUTOMATIC_RETRIES_EXHAUSTED',error_json=$3::jsonb,
                            next_action='Use an explicit user retry',updated_at=now(),
                            finished_at=now() WHERE id=$1 AND resource_version=$4
                        """,
                        task_id,
                        transition.resource_version,
                        json.dumps(
                            {"retryable": True, "summary": "Automatic retries exhausted"}
                        ),
                        transition.previous_resource_version,
                    )
                    await self._events.record(
                        connection,
                        task_id=task_id,
                        resource_version=transition.resource_version,
                        event_type=transition.event_type,
                    )
            else:
                row = await connection.fetchrow(
                    """
                    INSERT INTO production_attempts(
                        id,task_id,snapshot_id,attempt_no,parent_attempt_id,status,
                        usage_json,safety_json,execution_metadata_json
                    ) VALUES($1,$2,$3,$4,$5,'created',$6::jsonb,$7::jsonb,$8::jsonb)
                    RETURNING *
                    """,
                    new_id(),
                    task_id,
                    task["snapshot_id"],
                    attempt["attempt_no"] + 1,
                    attempt["id"],
                    json.dumps(object_value(attempt["usage_json"])),
                    json.dumps(object_value(attempt["safety_json"])),
                    json.dumps(object_value(attempt["execution_metadata_json"])),
                )
                next_version = task["resource_version"] + 1
                await connection.execute(
                    """
                    UPDATE production_tasks SET resource_version=$2,
                        progress_json='{"phase":"retry_queued","completed":0,"total":1}',
                        updated_at=now() WHERE id=$1 AND resource_version=$3
                    """,
                    task_id,
                    next_version,
                    task["resource_version"],
                )
                await self._events.record(
                    connection,
                    task_id=task_id,
                    resource_version=next_version,
                    event_type="attempt.created",
                )
                if row is None:
                    raise RuntimeError("automatic retry attempt could not be read")
                result = self._map(row)
        if exhausted:
            raise AutomaticRetryExhausted
        if result is None:
            raise RuntimeError("automatic retry did not produce a result")
        return result

    @staticmethod
    def _map(row: Mapping[str, Any]) -> AttemptSnapshot:
        return AttemptSnapshot(
            id=row["id"],
            task_id=row["task_id"],
            attempt_no=row["attempt_no"],
            parent_attempt_id=row["parent_attempt_id"],
            status=row["status"],
        )
