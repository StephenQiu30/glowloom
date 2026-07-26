from __future__ import annotations

from typing import cast
from uuid import UUID

from core.ids import new_id
from db.pool import DatabasePool
from repositories.idempotency import (
    IdempotencyRepository,
    canonical_request_hash,
)
from repositories.task_mutations import TaskMutationRepository
from repositories.tasks import TaskRepository
from schemas.tasks import (
    Capability,
    SubmitTaskCommand,
    TaskAcceptedSnapshot,
    TaskSnapshot,
    TaskType,
)
from services.task_submission import TaskSubmitter


class TaskNotFound(LookupError):
    pass


class TaskVersionConflict(Exception):
    pass


class TaskNotRetryable(Exception):
    pass


class TaskQueryService:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._tasks = TaskRepository()

    async def get(self, task_id: UUID) -> TaskSnapshot:
        async with self._database.transaction() as connection:
            task = await self._tasks.get(connection, task_id)
        if task is None:
            raise TaskNotFound
        return task

    async def list(self, episode_id: UUID) -> tuple[TaskSnapshot, ...]:
        async with self._database.transaction() as connection:
            return await self._tasks.list_for_episode(connection, episode_id)


class CancelTaskHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._tasks = TaskRepository()
        self._mutations = TaskMutationRepository()
        self._idempotency = IdempotencyRepository()

    async def execute(
        self, task_id: UUID, expected_resource_version: int, idempotency_key: str
    ) -> TaskSnapshot:
        scope = f"cancelTask/{task_id}"
        request_hash = canonical_request_hash(
            method="POST",
            operation_id="cancelTask",
            path_parameters={"task_id": str(task_id)},
            body=None,
        )
        request_id = new_id()
        async with self._database.transaction() as connection:
            stored = await self._idempotency.reserve(
                connection,
                owner_module="production_jobs",
                operation_scope=scope,
                idempotency_key=idempotency_key,
                request_hash=request_hash,
                request_id=request_id,
            )
            if stored is not None:
                task = await self._tasks.get(connection, task_id)
                if task is None:
                    raise TaskNotFound
                return task
            task = await self._tasks.get(connection, task_id, for_update=True)
            if task is None:
                raise TaskNotFound
            if task.resource_version != expected_resource_version:
                raise TaskVersionConflict
            task = await self._mutations.cancel(connection, task, request_id)
            await self._idempotency.complete(
                connection,
                operation_scope=scope,
                idempotency_key=idempotency_key,
                status=200,
                reference={"task_id": str(task_id)},
            )
            return task


class RetryTaskHandler:
    def __init__(self, database: DatabasePool, *, release_version: str) -> None:
        self._database = database
        self._release_version = release_version
        self._tasks = TaskRepository()

    async def execute(
        self, task_id: UUID, expected_resource_version: int, idempotency_key: str
    ) -> TaskAcceptedSnapshot:
        async with self._database.transaction() as connection:
            source = await self._tasks.retry_submission(connection, task_id)
        if source is None:
            raise TaskNotFound
        if source.task.resource_version != expected_resource_version:
            raise TaskVersionConflict
        if source.task.status != "failed" or not (source.task.error or {}).get("retryable"):
            raise TaskNotRetryable
        command = SubmitTaskCommand(
            episode_id=source.task.episode_id,
            task_type=cast(TaskType, source.task.task_type),
            capability=cast(Capability | None, source.capability),
            scope=source.task.scope,
            input_refs=source.task.input_refs,
            prompt=source.prompt,
            parameters=source.parameters,
            model_profile_id=source.model_profile_id,
            provider_id=source.provider_id,
            model_id=source.model_id,
            route_version=source.route_version,
            schema_version=source.schema_version,
            operation_scope=f"retryTask/{task_id}",
            idempotency_key=idempotency_key,
            handler_version=source.handler_version,
            retry_of_task_id=task_id,
        )
        return await TaskSubmitter(self._database, release_version=self._release_version).submit(
            command
        )
