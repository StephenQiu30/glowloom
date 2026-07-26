from __future__ import annotations

import json
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from schemas.tasks import SubmitTaskCommand


def json_value(value: object) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"))


class TaskSubmissionRepository:
    async def insert_bundle(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        command: SubmitTaskCommand,
        snapshot_id: UUID,
        task_id: UUID,
        attempt_id: UUID,
        event_id: UUID,
        job_id: UUID,
        correlation_id: UUID,
        parameters_hash: str,
        content_hash: str,
        release_version: str,
    ) -> None:
        await connection.execute(
            """
            INSERT INTO submission_snapshots(
                id, episode_id, type, capability, input_refs_json, prompt,
                parameters_json, parameters_hash, model_profile_id, provider_id,
                model_id, route_version, schema_version, content_hash
            ) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7::jsonb,$8,$9,$10,$11,$12,$13,$14)
            """,
            snapshot_id,
            command.episode_id,
            command.task_type,
            command.capability,
            json_value(command.input_refs),
            command.prompt,
            json_value(command.parameters),
            parameters_hash,
            command.model_profile_id,
            command.provider_id,
            command.model_id,
            command.route_version,
            command.schema_version,
            content_hash,
        )
        await connection.execute(
            """
            INSERT INTO production_tasks(
                id, episode_id, snapshot_id, type, scope_json, idempotency_scope,
                idempotency_key, progress_json, retry_of_task_id
            ) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7,$8::jsonb,$9)
            """,
            task_id,
            command.episode_id,
            snapshot_id,
            command.task_type,
            json_value(command.scope),
            command.operation_scope,
            command.idempotency_key,
            json_value({"phase": "queued", "completed": 0, "total": 1}),
            command.retry_of_task_id,
        )
        await connection.execute(
            """
            INSERT INTO production_attempts(
                id, task_id, snapshot_id, attempt_no, status,
                usage_json, safety_json, execution_metadata_json
            ) VALUES($1,$2,$3,1,'created','{}','{}',$4::jsonb)
            """,
            attempt_id,
            task_id,
            snapshot_id,
            json_value(
                {
                    "release_version": release_version,
                    "handler_version": command.handler_version,
                }
            ),
        )
        await connection.execute(
            """
            INSERT INTO task_events(
                event_id, task_id, task_resource_version, event_type, correlation_id, data_json
            ) VALUES($1,$2,1,'task.accepted',$3,'{}')
            """,
            event_id,
            task_id,
            correlation_id,
        )
        await connection.execute(
            """
            INSERT INTO task_jobs(id, task_id, payload_json)
            VALUES($1,$2,$3::jsonb)
            """,
            job_id,
            task_id,
            json_value(
                {
                    "release_version": release_version,
                    "handler_version": command.handler_version,
                    "task_id": str(task_id),
                    "snapshot_id": str(snapshot_id),
                }
            ),
        )
