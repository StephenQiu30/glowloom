from __future__ import annotations

from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from db.pool import DatabasePool
from repositories.idempotency import (
    IdempotencyRepository,
    canonical_request_hash,
    canonical_value_hash,
)
from repositories.task_submissions import TaskSubmissionRepository
from schemas.tasks import (
    SubmitTaskCommand,
    TaskAcceptedSnapshot,
)


class UnsupportedTaskSubmission(ValueError):
    pass


class TaskSubmitter:
    def __init__(self, database: DatabasePool, *, release_version: str) -> None:
        self._database = database
        self._release_version = release_version
        self._submissions = TaskSubmissionRepository()
        self._idempotency = IdempotencyRepository()

    async def submit(self, command: SubmitTaskCommand) -> TaskAcceptedSnapshot:
        async with self._database.transaction() as connection:
            return await self.submit_in_transaction(connection, command)

    async def submit_in_transaction(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        command: SubmitTaskCommand,
    ) -> TaskAcceptedSnapshot:
        if command.task_type == "render_episode":
            raise UnsupportedTaskSubmission("render uses the delivery coordinator")
        request_body = self._request_body(command)
        request_hash = canonical_request_hash(
            method="POST",
            operation_id=(
                "retryTask"
                if command.retry_of_task_id is not None
                else self._operation_id(command.task_type)
            ),
            path_parameters={"episode_id": str(command.episode_id)},
            body=request_body,
        )
        request_id = new_id()
        stored = await self._idempotency.reserve(
            connection,
            owner_module="production_jobs",
            operation_scope=command.operation_scope,
            idempotency_key=command.idempotency_key,
            request_hash=request_hash,
            request_id=request_id,
        )
        if stored is not None:
            return TaskAcceptedSnapshot(
                task_id=UUID(str(stored.reference["task_id"])),
                snapshot_id=UUID(str(stored.reference["snapshot_id"])),
            )
        snapshot_id = new_id()
        task_id = new_id()
        await self._submissions.insert_bundle(
            connection,
            command=command,
            snapshot_id=snapshot_id,
            task_id=task_id,
            attempt_id=new_id(),
            event_id=new_id(),
            job_id=new_id(),
            correlation_id=request_id,
            parameters_hash=canonical_value_hash(command.parameters),
            content_hash=canonical_value_hash(request_body),
            release_version=self._release_version,
        )
        await self._idempotency.complete(
            connection,
            operation_scope=command.operation_scope,
            idempotency_key=command.idempotency_key,
            status=202,
            reference={"task_id": str(task_id), "snapshot_id": str(snapshot_id)},
        )
        return TaskAcceptedSnapshot(task_id=task_id, snapshot_id=snapshot_id)

    @staticmethod
    def _request_body(command: SubmitTaskCommand) -> dict[str, object]:
        return {
            "capability": command.capability,
            "scope": command.scope,
            "input_refs": command.input_refs,
            "prompt": command.prompt,
            "parameters": command.parameters,
            "model_profile_id": command.model_profile_id,
            "provider_id": command.provider_id,
            "model_id": command.model_id,
            "route_version": command.route_version,
            "schema_version": command.schema_version,
            "handler_version": command.handler_version,
            "retry_of_task_id": str(command.retry_of_task_id) if command.retry_of_task_id else None,
        }

    @staticmethod
    def _operation_id(task_type: str) -> str:
        return {
            "generate_script": "generateScript",
            "generate_storyboard": "generateStoryboard",
            "generate_media": "generateMedia",
        }[task_type]
