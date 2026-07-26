from __future__ import annotations

import json
from dataclasses import dataclass
from uuid import UUID

from db.pool import DatabasePool
from domain.task_states import transition_task
from repositories.task_events import TaskEventRepository
from schemas.jobs import JobPayload


@dataclass(frozen=True, slots=True)
class RenderExecutionPlan:
    episode_id: UUID
    task_id: UUID
    attempt_id: UUID
    render_snapshot_id: UUID
    cancel_requested: bool
    skip: bool = False


class RenderExecutionStore:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._events = TaskEventRepository()

    async def prepare(self, payload: JobPayload) -> RenderExecutionPlan:
        async with self._database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT task.id task_id,task.episode_id,task.status task_status,
                       task.resource_version,task.snapshot_id,submission.input_refs_json,
                       attempt.id attempt_id,attempt.status attempt_status
                FROM production_tasks task
                JOIN submission_snapshots submission ON submission.id=task.snapshot_id
                JOIN LATERAL (
                    SELECT * FROM production_attempts WHERE task_id=task.id
                    ORDER BY attempt_no DESC LIMIT 1
                ) attempt ON true
                WHERE task.id=$1 AND task.type='render_episode'
                FOR UPDATE OF task,attempt
                """,
                payload.task_id,
            )
            if row is None or row["snapshot_id"] != payload.snapshot_id:
                raise RuntimeError("render job payload does not match its task")
            inputs = row["input_refs_json"]
            if isinstance(inputs, str):
                inputs = json.loads(inputs)
            try:
                render_snapshot_id = UUID(str(inputs["render_snapshot_id"]))
            except (KeyError, TypeError, ValueError) as error:
                raise RuntimeError("render task input is invalid") from error
            plan = RenderExecutionPlan(
                episode_id=row["episode_id"],
                task_id=row["task_id"],
                attempt_id=row["attempt_id"],
                render_snapshot_id=render_snapshot_id,
                cancel_requested=row["task_status"] == "cancelling",
                skip=row["task_status"] in {"cancelled", "succeeded", "failed"},
            )
            if plan.skip:
                return plan
            if row["task_status"] == "queued":
                transition = transition_task("queued", "running", row["resource_version"])
                await connection.execute(
                    """
                    UPDATE production_tasks SET status='running',resource_version=$2,
                        progress_json='{"phase":"render","completed":0,"total":1}',
                        updated_at=now() WHERE id=$1 AND resource_version=$3
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
            if row["attempt_status"] == "created":
                await connection.execute(
                    """
                    UPDATE production_attempts SET status='postprocessing',
                        submitted_at=now(),started_at=now(),
                        execution_metadata_json=execution_metadata_json
                            || '{"executor":"ffmpeg"}'::jsonb
                    WHERE id=$1 AND status='created'
                    """,
                    plan.attempt_id,
                )
            elif row["attempt_status"] != "postprocessing":
                raise RuntimeError("render attempt is not executable")
            return plan
