from __future__ import annotations

from typing import cast
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from repositories.tasks import object_value
from schemas.candidates import CandidateSnapshot, CandidateStatus, PreviewMediaSnapshot
from schemas.media_registration import UsageType


class CandidateRepository:
    async def list_for_slot(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        episode_id: UUID,
        usage_type: UsageType,
        usage_id: UUID,
        input_version_id: UUID,
        input_hash: str,
    ) -> tuple[CandidateSnapshot, ...]:
        rows = await connection.fetch(
            """
            SELECT candidate.*,version.mime_type,version.byte_size,version.sha256,
                   version.width,version.height,version.duration_ticks,version.timebase,
                   version.probe_summary_json,snapshot.model_profile_id,
                   snapshot.provider_id,snapshot.model_id,snapshot.route_version,
                   snapshot.schema_version,
                   (SELECT adoption.id FROM adoptions adoption
                    WHERE adoption.candidate_id=candidate.id
                      AND adoption.status='active') active_adoption_id
            FROM generation_candidates candidate
            JOIN media_versions version ON version.id=candidate.media_version_id
            JOIN production_tasks task ON task.id=candidate.task_id
            JOIN submission_snapshots snapshot ON snapshot.id=task.snapshot_id
            WHERE candidate.episode_id=$1 AND candidate.usage_type=$2
              AND candidate.usage_id=$3 AND candidate.input_version_id=$4
              AND candidate.input_hash=$5
            ORDER BY candidate.created_at DESC,candidate.id DESC
            """,
            episode_id,
            usage_type,
            usage_id,
            input_version_id,
            input_hash,
        )
        return tuple(self._candidate(row) for row in rows)

    async def preview_media(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        episode_id: UUID,
        media_version_id: UUID,
    ) -> PreviewMediaSnapshot | None:
        row = await connection.fetchrow(
            """
            SELECT version.id media_version_id,version.bucket,version.object_key
            FROM media_versions version
            JOIN generation_candidates candidate ON candidate.media_version_id=version.id
            WHERE version.id=$1 AND candidate.episode_id=$2
              AND version.status='ready' AND candidate.status='ready'
              AND candidate.output_slot='primary'
            """,
            media_version_id,
            episode_id,
        )
        return (
            PreviewMediaSnapshot(row["media_version_id"], row["bucket"], row["object_key"])
            if row
            else None
        )

    @staticmethod
    def _candidate(row: asyncpg.Record) -> CandidateSnapshot:
        probe = (
            object_value(row["probe_summary_json"])
            if row["probe_summary_json"] is not None
            else {}
        )
        return CandidateSnapshot(
            id=row["id"],
            episode_id=row["episode_id"],
            task_id=row["task_id"],
            attempt_id=row["attempt_id"],
            output_slot=row["output_slot"],
            usage_type=cast(UsageType, row["usage_type"]),
            usage_id=row["usage_id"],
            input_version_id=row["input_version_id"],
            input_hash=row["input_hash"],
            media_version_id=row["media_version_id"],
            status=cast(CandidateStatus, row["status"]),
            blocked_reason=row["blocked_reason"],
            mime_type=row["mime_type"],
            byte_size=row["byte_size"],
            sha256=row["sha256"],
            width=row["width"],
            height=row["height"],
            duration_ticks=row["duration_ticks"],
            timebase=row["timebase"],
            probe_summary=probe,
            model_profile_id=row["model_profile_id"],
            provider_id=row["provider_id"],
            model_id=row["model_id"],
            route_version=row["route_version"],
            schema_version=row["schema_version"],
            active_adoption_id=row["active_adoption_id"],
            created_at=row["created_at"],
            finalized_at=row["finalized_at"],
        )
