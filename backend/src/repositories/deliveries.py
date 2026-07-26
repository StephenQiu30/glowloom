from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Literal, cast
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from schemas.deliveries import (
    DeliveryArtifactSnapshot,
    DeliveryArtifactType,
    DeliveryVersionSnapshot,
)
from schemas.delivery_quality import DeliveryProbeSummaryV1


@dataclass(frozen=True, slots=True)
class RenderTaskDeliveryInput:
    episode_id: UUID
    render_snapshot_id: UUID
    retry_of_task_id: UUID | None


class DeliveryRepository:
    async def lock_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> bool:
        return (
            await connection.fetchval(
                "SELECT true FROM episodes WHERE id=$1 FOR UPDATE", episode_id
            )
            is True
        )

    async def task_render_input(
        self, connection: asyncpg.Connection[asyncpg.Record], task_id: UUID
    ) -> RenderTaskDeliveryInput | None:
        row = await connection.fetchrow(
            """
            SELECT task.episode_id,task.retry_of_task_id,submission.input_refs_json
            FROM production_tasks task
            JOIN submission_snapshots submission ON submission.id=task.snapshot_id
            WHERE task.id=$1 AND task.type='render_episode'
              AND submission.type='render_episode'
            """,
            task_id,
        )
        if row is None:
            return None
        inputs = row["input_refs_json"]
        if isinstance(inputs, str):
            inputs = json.loads(inputs)
        try:
            snapshot_id = UUID(str(inputs["render_snapshot_id"]))
        except (KeyError, TypeError, ValueError) as error:
            raise RuntimeError("render task input is invalid") from error
        return RenderTaskDeliveryInput(row["episode_id"], snapshot_id, row["retry_of_task_id"])

    async def get_by_task(
        self, connection: asyncpg.Connection[asyncpg.Record], task_id: UUID
    ) -> DeliveryVersionSnapshot | None:
        row = await connection.fetchrow(
            "SELECT * FROM delivery_versions WHERE render_task_id=$1", task_id
        )
        return self._map(row) if row else None

    async def get(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        delivery_id: UUID,
    ) -> DeliveryVersionSnapshot | None:
        row = await connection.fetchrow("SELECT * FROM delivery_versions WHERE id=$1", delivery_id)
        return self._map(row) if row else None

    async def list_for_episode(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        episode_id: UUID,
    ) -> tuple[DeliveryVersionSnapshot, ...]:
        rows = await connection.fetch(
            """
            SELECT * FROM delivery_versions WHERE episode_id=$1
            ORDER BY version DESC,id DESC
            """,
            episode_id,
        )
        return tuple(self._map(row) for row in rows)

    async def artifacts(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        delivery: DeliveryVersionSnapshot,
    ) -> tuple[DeliveryArtifactSnapshot, ...]:
        identities: dict[DeliveryArtifactType, UUID | None] = {
            "mp4": delivery.mp4_media_version_id,
            "srt": delivery.srt_media_version_id,
            "manifest": delivery.manifest_media_version_id,
        }
        expected = {value: key for key, value in identities.items() if value is not None}
        if not expected:
            return ()
        rows = await connection.fetch(
            """
            SELECT version.*,object.source_kind FROM media_versions version
            JOIN media_objects object ON object.id=version.media_object_id
            WHERE version.id=ANY($1::uuid[])
            """,
            list(expected),
        )
        values = {row["id"]: row for row in rows}
        if set(values) != set(expected):
            raise RuntimeError("delivery artifact facts are incomplete")
        return tuple(
            self._artifact(values[media_id], artifact_type)
            for artifact_type, media_id in identities.items()
            if media_id is not None
        )

    async def insert_rendering(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        episode_id: UUID,
        task_id: UUID,
        render_snapshot_id: UUID,
        retry_of_delivery_id: UUID | None = None,
    ) -> DeliveryVersionSnapshot:
        version = await connection.fetchval(
            "SELECT coalesce(max(version),0)+1 FROM delivery_versions WHERE episode_id=$1",
            episode_id,
        )
        row = await connection.fetchrow(
            """
            INSERT INTO delivery_versions(
                id,episode_id,version,render_task_id,render_snapshot_id,
                retry_of_delivery_id
            ) VALUES($1,$2,$3,$4,$5,$6) RETURNING *
            """,
            new_id(),
            episode_id,
            version,
            task_id,
            render_snapshot_id,
            retry_of_delivery_id,
        )
        if row is None:
            raise RuntimeError("created delivery could not be read")
        return self._map(row)

    @staticmethod
    def _map(row: asyncpg.Record) -> DeliveryVersionSnapshot:
        probe = row["ffprobe_summary_json"]
        if isinstance(probe, str):
            probe = json.loads(probe)
        return DeliveryVersionSnapshot(
            id=row["id"],
            episode_id=row["episode_id"],
            version=row["version"],
            render_task_id=row["render_task_id"],
            final_attempt_id=row["final_attempt_id"],
            retry_of_delivery_id=row["retry_of_delivery_id"],
            render_snapshot_id=row["render_snapshot_id"],
            mp4_media_version_id=row["mp4_media_version_id"],
            srt_media_version_id=row["srt_media_version_id"],
            manifest_media_version_id=row["manifest_media_version_id"],
            ffmpeg_version=row["ffmpeg_version"],
            ffprobe_summary=(
                DeliveryProbeSummaryV1.model_validate(probe) if probe is not None else None
            ),
            status=row["status"],
            error_code=row["error_code"],
            created_at=row["created_at"],
            updated_at=row["updated_at"],
            finished_at=row["finished_at"],
        )

    @staticmethod
    def _artifact(
        row: asyncpg.Record, artifact_type: DeliveryArtifactType
    ) -> DeliveryArtifactSnapshot:
        return DeliveryArtifactSnapshot(
            artifact_type=artifact_type,
            media_version_id=row["id"],
            source_kind=cast(Literal["ffmpeg", "application"], row["source_kind"]),
            mime_type=row["mime_type"],
            byte_size=row["byte_size"],
            sha256=row["sha256"],
            width=row["width"],
            height=row["height"],
            duration_ticks=row["duration_ticks"],
            timebase=row["timebase"],
            bucket=row["bucket"],
            object_key=row["object_key"],
        )
