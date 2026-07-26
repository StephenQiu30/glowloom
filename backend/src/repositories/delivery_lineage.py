from __future__ import annotations

import json
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from schemas.delivery_lineage import RenderAttemptSnapshot
from schemas.delivery_manifest import DeliveryMediaLineageV1
from schemas.delivery_media_lineage import media_lineage
from schemas.rendering import RenderSnapshot


class DeliveryLineageRepository:
    async def attempts(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        task_id: UUID,
    ) -> tuple[RenderAttemptSnapshot, ...]:
        rows = await connection.fetch(
            """
            SELECT * FROM production_attempts WHERE task_id=$1
            ORDER BY attempt_no,id
            """,
            task_id,
        )
        return tuple(
            RenderAttemptSnapshot(
                id=row["id"],
                task_id=row["task_id"],
                snapshot_id=row["snapshot_id"],
                attempt_no=row["attempt_no"],
                parent_attempt_id=row["parent_attempt_id"],
                status=row["status"],
                execution_metadata=_object(row["execution_metadata_json"]),
                error_code=row["error_code"],
                error_summary=row["error_summary"],
                created_at=row["created_at"],
                submitted_at=row["submitted_at"],
                started_at=row["started_at"],
                finished_at=row["finished_at"],
            )
            for row in rows
        )

    async def input_media(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        snapshot: RenderSnapshot,
    ) -> tuple[DeliveryMediaLineageV1, ...]:
        references = (
            *snapshot.input_refs.video_adoptions,
            *snapshot.input_refs.tts_adoptions,
        )
        rows = await connection.fetch(
            """
            SELECT version.id media_version_id,version.sha256 media_sha256,
                   version.mime_type,version.byte_size,version.duration_ticks,
                   version.timebase,version.probe_summary_json,
                   object.media_kind,object.source_kind,
                   candidate.usage_type,candidate.usage_id,
                   candidate.input_version_id,candidate.input_hash,
                   adoption.id adoption_id,candidate.id candidate_id,
                   attempt.id origin_attempt_id,task.id origin_task_id,
                   submission.id origin_submission_snapshot_id,
                   submission.capability,submission.model_profile_id,
                   submission.provider_id,submission.model_id,
                   submission.route_version,submission.schema_version
            FROM media_versions version
            JOIN media_objects object ON object.id=version.media_object_id
            JOIN generation_candidates candidate ON candidate.media_version_id=version.id
            JOIN adoptions adoption ON adoption.candidate_id=candidate.id
              AND adoption.id=ANY($2::uuid[])
            JOIN production_attempts attempt ON attempt.id=version.origin_attempt_id
            JOIN production_tasks task ON task.id=attempt.task_id
            JOIN submission_snapshots submission ON submission.id=attempt.snapshot_id
            WHERE version.id=ANY($1::uuid[])
            """,
            [item.media_version_id for item in references],
            [item.adoption_id for item in references],
        )
        values = {row["media_version_id"]: media_lineage(row) for row in rows}
        if set(values) != {item.media_version_id for item in references}:
            raise RuntimeError("delivery input media lineage is incomplete")
        return tuple(values[item.media_version_id] for item in references)


def _object(value: object) -> dict[str, object]:
    if isinstance(value, str):
        value = json.loads(value)
    if not isinstance(value, dict):
        raise RuntimeError("attempt metadata is invalid")
    return {str(key): item for key, item in value.items()}
