from __future__ import annotations

import json
from typing import Any
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from lanverse.modules.production_jobs.application.contracts import (
    RetrySubmissionSnapshot,
    TaskResultSnapshot,
    TaskSnapshot,
)


def object_value(value: Any) -> dict[str, object]:
    if isinstance(value, str):
        value = json.loads(value)
    if not isinstance(value, dict):
        raise RuntimeError("expected a persisted JSON object")
    return {str(key): item for key, item in value.items()}


class TaskRepository:
    SELECT = """
        SELECT t.*, s.input_refs_json,
               current_attempt.id current_attempt_id
        FROM production_tasks t
        JOIN submission_snapshots s ON s.id = t.snapshot_id
        LEFT JOIN LATERAL (
            SELECT id FROM production_attempts
            WHERE task_id = t.id ORDER BY attempt_no DESC LIMIT 1
        ) current_attempt ON true
    """

    async def get(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        task_id: UUID,
        *,
        for_update: bool = False,
    ) -> TaskSnapshot | None:
        suffix = " WHERE t.id = $1"
        if for_update:
            suffix += " FOR UPDATE OF t"
        row = await connection.fetchrow(self.SELECT + suffix, task_id)
        return await self._map(connection, row) if row else None

    async def list_for_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> tuple[TaskSnapshot, ...]:
        rows = await connection.fetch(
            self.SELECT + " WHERE t.episode_id = $1 ORDER BY t.created_at DESC, t.id",
            episode_id,
        )
        return tuple([await self._map(connection, row) for row in rows])

    async def retry_submission(
        self, connection: asyncpg.Connection[asyncpg.Record], task_id: UUID
    ) -> RetrySubmissionSnapshot | None:
        task = await self.get(connection, task_id, for_update=True)
        if task is None:
            return None
        row = await connection.fetchrow(
            """
            SELECT s.capability, s.prompt, s.parameters_json, s.model_profile_id,
                   s.provider_id, s.model_id, s.route_version, s.schema_version,
                   j.payload_json
            FROM submission_snapshots s
            JOIN task_jobs j ON j.task_id = $1
            WHERE s.id = $2
            """,
            task_id,
            task.snapshot_id,
        )
        if row is None or any(
            row[name] is None
            for name in (
                "capability",
                "prompt",
                "model_profile_id",
                "provider_id",
                "model_id",
                "route_version",
            )
        ):
            return None
        payload = object_value(row["payload_json"])
        return RetrySubmissionSnapshot(
            task=task,
            capability=row["capability"],
            prompt=row["prompt"],
            parameters=object_value(row["parameters_json"]),
            model_profile_id=row["model_profile_id"],
            provider_id=row["provider_id"],
            model_id=row["model_id"],
            route_version=row["route_version"],
            schema_version=row["schema_version"],
            handler_version=str(payload["handler_version"]),
        )

    async def _map(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        row: asyncpg.Record,
    ) -> TaskSnapshot:
        input_refs = object_value(row["input_refs_json"])
        outputs = await connection.fetch(
            """
            SELECT output_type, output_id FROM task_outputs
            WHERE task_id = $1 ORDER BY output_type, ordinal
            """,
            row["id"],
        )
        error = object_value(row["error_json"]) if row["error_json"] is not None else None
        return TaskSnapshot(
            id=row["id"],
            episode_id=row["episode_id"],
            snapshot_id=row["snapshot_id"],
            task_type=row["type"],
            scope=object_value(row["scope_json"]),
            status=row["status"],
            progress=object_value(row["progress_json"]),
            input_refs=input_refs,
            input_outdated=await self._input_outdated(
                connection, row["episode_id"], row["type"], input_refs
            ),
            current_attempt_id=row["current_attempt_id"],
            result_refs=tuple(
                TaskResultSnapshot(output_type=item["output_type"], output_id=item["output_id"])
                for item in outputs
            ),
            error_code=row["error_code"],
            error=error,
            next_action=row["next_action"],
            resource_version=row["resource_version"],
            retry_of_task_id=row["retry_of_task_id"],
            created_at=row["created_at"],
            updated_at=row["updated_at"],
            finished_at=row["finished_at"],
        )

    @staticmethod
    async def _input_outdated(
        connection: asyncpg.Connection[asyncpg.Record],
        episode_id: UUID,
        task_type: str,
        input_refs: dict[str, object],
    ) -> bool:
        field = {
            "generate_script": "source_revision_id",
            "generate_storyboard": "script_version_id",
        }.get(task_type)
        if field is None:
            return False
        try:
            input_id = UUID(str(input_refs[field]))
        except (KeyError, ValueError):
            return True
        if task_type == "generate_script":
            matches = await connection.fetchval(
                "SELECT true FROM source_revisions "
                "WHERE id=$1 AND episode_id=$2 AND status='confirmed'",
                input_id,
                episode_id,
            )
        else:
            matches = await connection.fetchval(
                """
                SELECT true FROM script_versions script
                JOIN source_revisions source
                  ON source.episode_id=script.episode_id
                 AND source.id=script.source_revision_id
                 AND source.status='confirmed'
                WHERE script.id=$1 AND script.episode_id=$2 AND script.status='confirmed'
                """,
                input_id,
                episode_id,
            )
        return matches is not True
