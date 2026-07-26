from __future__ import annotations

from uuid import UUID

from core.ids import new_id
from db.pool import DatabasePool
from repositories.idempotency import canonical_value_hash
from repositories.task_submissions import TaskSubmissionRepository
from schemas.tasks import SubmitTaskCommand, TaskAcceptedSnapshot
from services.render_inputs import RenderInputInvalid


class RenderTaskSubmitter:
    def __init__(self, database: DatabasePool, *, release_version: str) -> None:
        self._database = database
        self._release_version = release_version
        self._tasks = TaskSubmissionRepository()

    async def submit(self, episode_id: UUID, render_snapshot_id: UUID) -> TaskAcceptedSnapshot:
        key = str(render_snapshot_id)
        async with self._database.transaction() as connection:
            locked = await connection.fetchval(
                "SELECT true FROM episodes WHERE id=$1 FOR UPDATE", episode_id
            )
            if locked is not True:
                raise RenderInputInvalid("episode does not exist")
            existing = await self._tasks.find_by_idempotency(
                connection, scope="render-task", key=key
            )
            if existing is not None:
                if (
                    existing.episode_id != episode_id
                    or existing.task_type != "render_episode"
                    or existing.input_refs != {"render_snapshot_id": key}
                ):
                    raise RuntimeError("render task idempotency facts conflict")
                return TaskAcceptedSnapshot(existing.task_id, existing.snapshot_id)
            command = _render_task_command(episode_id, render_snapshot_id)
            task = TaskAcceptedSnapshot(new_id(), new_id())
            content = {
                "type": command.task_type,
                "capability": None,
                "input_refs": command.input_refs,
                "prompt": None,
                "parameters": {},
                "model_profile_id": None,
                "provider_id": None,
                "model_id": None,
                "route_version": None,
                "schema_version": command.schema_version,
            }
            await self._tasks.insert_bundle(
                connection,
                command=command,
                snapshot_id=task.snapshot_id,
                task_id=task.task_id,
                attempt_id=new_id(),
                event_id=new_id(),
                job_id=new_id(),
                correlation_id=new_id(),
                parameters_hash=canonical_value_hash({}),
                content_hash=canonical_value_hash(content),
                release_version=self._release_version,
            )
            return task


def _render_task_command(episode_id: UUID, snapshot_id: UUID) -> SubmitTaskCommand:
    return SubmitTaskCommand(
        episode_id=episode_id,
        task_type="render_episode",
        capability=None,
        scope={"episode_id": str(episode_id), "render_snapshot_id": str(snapshot_id)},
        input_refs={"render_snapshot_id": str(snapshot_id)},
        prompt=None,
        parameters={},
        model_profile_id=None,
        provider_id=None,
        model_id=None,
        route_version=None,
        schema_version="render-submission-v1",
        operation_scope="render-task",
        idempotency_key=str(snapshot_id),
        handler_version="render-v1",
    )
