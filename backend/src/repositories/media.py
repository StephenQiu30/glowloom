from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from repositories.tasks import object_value


@dataclass(frozen=True, slots=True)
class MediaRegistrationRow:
    media_object_id: UUID
    media_version_id: UUID
    candidate_id: UUID
    episode_id: UUID
    task_id: UUID
    attempt_id: UUID
    output_slot: str
    usage_type: str
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    media_kind: str
    bucket: str
    object_key: str
    content_type: str
    byte_size: int
    sha256: str
    width: int | None
    height: int | None
    duration_ticks: int | None
    timebase: int | None
    probe_summary: Mapping[str, object]
    media_status: str
    candidate_status: str
    blocked_reason: str | None


class MediaRepository:
    async def task_context(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        task_id: UUID,
        attempt_id: UUID,
        for_update: bool = False,
    ) -> tuple[UUID, str] | None:
        lock = " FOR UPDATE OF task,attempt" if for_update else ""
        row = await connection.fetchrow(
            """
            SELECT task.episode_id,snapshot.capability
            FROM production_attempts attempt
            JOIN production_tasks task ON task.id=attempt.task_id
            JOIN submission_snapshots snapshot ON snapshot.id=attempt.snapshot_id
            WHERE task.id=$1 AND attempt.id=$2
            """
            + lock,
            task_id,
            attempt_id,
        )
        return (row["episode_id"], row["capability"]) if row else None

    async def find_by_attempt_slot(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        attempt_id: UUID,
        output_slot: str,
    ) -> MediaRegistrationRow | None:
        row = await connection.fetchrow(
            """
            SELECT object.id media_object_id,version.id media_version_id,
                   candidate.id candidate_id,object.episode_id,candidate.task_id,
                   candidate.attempt_id,candidate.output_slot,candidate.usage_type,
                   candidate.usage_id,candidate.input_version_id,candidate.input_hash,
                   object.media_kind,version.bucket,version.object_key,
                   version.mime_type,version.byte_size,version.sha256,version.width,
                   version.height,version.duration_ticks,version.timebase,
                   version.probe_summary_json,version.status media_status,
                   candidate.status candidate_status,candidate.blocked_reason
            FROM media_versions version
            JOIN media_objects object ON object.id=version.media_object_id
            JOIN generation_candidates candidate ON candidate.media_version_id=version.id
            WHERE version.origin_attempt_id=$1 AND version.output_slot=$2
            """,
            attempt_id,
            output_slot,
        )
        return self._map(row) if row else None

    async def insert_finalized(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        episode_id: UUID,
        task_id: UUID,
        attempt_id: UUID,
        output_slot: str,
        usage_type: str,
        usage_id: UUID,
        input_version_id: UUID,
        input_hash: str,
        media_kind: str,
        bucket: str,
        object_key: str,
        content_type: str,
        byte_size: int,
        sha256: str,
        width: int | None,
        height: int | None,
        duration_ticks: int | None,
        timebase: int | None,
        probe_summary: Mapping[str, object] | None,
        media_status: str,
        candidate_status: str,
        blocked_reason: str | None,
    ) -> MediaRegistrationRow:
        media_object_id = new_id()
        media_version_id = new_id()
        candidate_id = new_id()
        await connection.execute(
            """
            INSERT INTO media_objects(id,episode_id,media_kind,source_kind)
            VALUES($1,$2,$3,'provider')
            """,
            media_object_id,
            episode_id,
            media_kind,
        )
        await connection.execute(
            """
            INSERT INTO media_versions(
                id,media_object_id,version,origin_attempt_id,output_slot,bucket,
                object_key,mime_type,byte_size,sha256,status,width,height,
                duration_ticks,timebase,probe_summary_json,finalized_at
            ) VALUES($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,now())
            """,
            media_version_id, media_object_id, attempt_id, output_slot, bucket,
            object_key, content_type, byte_size, sha256, media_status, width, height,
            duration_ticks, timebase,
            json.dumps(probe_summary, separators=(",", ":")) if probe_summary else None,
        )
        await connection.execute(
            """
            INSERT INTO generation_candidates(
                id,episode_id,task_id,attempt_id,output_slot,usage_type,usage_id,
                input_version_id,input_hash,media_version_id,status,blocked_reason,finalized_at
            ) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
            """,
            candidate_id, episode_id, task_id, attempt_id, output_slot, usage_type,
            usage_id, input_version_id, input_hash, media_version_id,
            candidate_status, blocked_reason,
        )
        stored = await self.find_by_attempt_slot(
            connection, attempt_id=attempt_id, output_slot=output_slot
        )
        if stored is None:
            raise RuntimeError("registered media could not be read")
        return stored

    @staticmethod
    def _map(row: asyncpg.Record) -> MediaRegistrationRow:
        return MediaRegistrationRow(
            media_object_id=row["media_object_id"], media_version_id=row["media_version_id"],
            candidate_id=row["candidate_id"], episode_id=row["episode_id"],
            task_id=row["task_id"], attempt_id=row["attempt_id"],
            output_slot=row["output_slot"], usage_type=row["usage_type"],
            usage_id=row["usage_id"], input_version_id=row["input_version_id"],
            input_hash=row["input_hash"], media_kind=row["media_kind"], bucket=row["bucket"],
            object_key=row["object_key"], content_type=row["mime_type"],
            byte_size=row["byte_size"], sha256=row["sha256"], width=row["width"],
            height=row["height"], duration_ticks=row["duration_ticks"],
            timebase=row["timebase"],
            probe_summary=(
                object_value(row["probe_summary_json"])
                if row["probe_summary_json"] is not None
                else {}
            ),
            media_status=row["media_status"], candidate_status=row["candidate_status"],
            blocked_reason=row["blocked_reason"],
        )
