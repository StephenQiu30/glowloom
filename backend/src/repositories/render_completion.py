from __future__ import annotations

import json
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from db.pool import DatabasePool
from domain.task_states import transition_task
from repositories.render_artifacts import (
    RenderArtifactRepository,
    UploadedDeliveryArtifacts,
)
from repositories.render_executions import RenderExecutionPlan
from repositories.task_events import TaskEventRepository
from schemas.delivery_manifest import DeliveryArtifactSummaryV1
from schemas.delivery_quality import DeliveryProbeSummaryV1


class RenderCompletionStore:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._events = TaskEventRepository()
        self._artifacts = RenderArtifactRepository()

    async def mark_ready(
        self,
        plan: RenderExecutionPlan,
        *,
        delivery_id: UUID,
        artifacts: UploadedDeliveryArtifacts,
        quality: DeliveryProbeSummaryV1,
        ffmpeg_version: str,
    ) -> None:
        async with self._database.transaction() as connection:
            state = await self._lock(connection, plan, delivery_id)
            if state["delivery_status"] == "ready":
                return
            if state["task_status"] != "running" or state["attempt_status"] != "postprocessing":
                raise RuntimeError("render completion state is invalid")
            stored = await self._artifacts.register_exact(connection, plan, artifacts, quality)
            summary = DeliveryArtifactSummaryV1(
                mp4=stored["mp4"].reference(),
                srt=stored["srt"].reference(),
                manifest=stored["manifest"].reference(),
            )
            await connection.execute(
                """
                UPDATE delivery_versions SET final_attempt_id=$2,
                    mp4_media_version_id=$3,srt_media_version_id=$4,
                    manifest_media_version_id=$5,artifact_summary_json=$6::jsonb,
                    ffmpeg_version=$7,ffprobe_summary_json=$8::jsonb,
                    status='ready',error_code=NULL,updated_at=now(),finished_at=now()
                WHERE id=$1 AND status='rendering'
                """,
                delivery_id,
                plan.attempt_id,
                stored["mp4"].media_version_id,
                stored["srt"].media_version_id,
                stored["manifest"].media_version_id,
                summary.model_dump_json(),
                ffmpeg_version,
                quality.model_dump_json(),
            )
            await connection.execute(
                """
                INSERT INTO task_outputs(id,task_id,output_type,output_id,ordinal)
                VALUES($1,$2,'delivery_version',$3,0) ON CONFLICT DO NOTHING
                """,
                new_id(),
                plan.task_id,
                delivery_id,
            )
            output = await connection.fetchval(
                """
                SELECT output_id FROM task_outputs
                WHERE task_id=$1 AND output_type='delivery_version' AND ordinal=0
                """,
                plan.task_id,
            )
            if output != delivery_id:
                raise RuntimeError("render task output conflicts")
            await connection.execute(
                """
                UPDATE production_attempts SET status='succeeded',finished_at=now()
                WHERE id=$1 AND status='postprocessing'
                """,
                plan.attempt_id,
            )
            await self._finish_task(
                connection,
                plan,
                state["task_status"],
                state["resource_version"],
                "succeeded",
            )

    async def mark_failed(
        self,
        plan: RenderExecutionPlan,
        *,
        delivery_id: UUID,
        error_code: str,
        summary: str,
    ) -> None:
        async with self._database.transaction() as connection:
            state = await self._lock(connection, plan, delivery_id)
            if state["task_status"] in {"failed", "cancelled", "succeeded"}:
                return
            await connection.execute(
                """
                UPDATE delivery_versions SET status='failed',error_code=$2,
                    updated_at=now(),finished_at=now()
                WHERE id=$1 AND status='rendering'
                """,
                delivery_id,
                error_code,
            )
            await connection.execute(
                """
                UPDATE production_attempts SET status='failed',error_code=$2,
                    error_summary=$3,finished_at=now()
                WHERE id=$1 AND status NOT IN ('succeeded','failed','cancelled')
                """,
                plan.attempt_id,
                error_code,
                summary,
            )
            await self._finish_task(
                connection,
                plan,
                state["task_status"],
                state["resource_version"],
                "failed",
                error_code,
                summary,
            )

    async def _lock(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        plan: RenderExecutionPlan,
        delivery_id: UUID,
    ) -> asyncpg.Record:
        row = await connection.fetchrow(
            """
            SELECT task.status task_status,task.resource_version,
                   attempt.status attempt_status,delivery.status delivery_status
            FROM production_tasks task
            JOIN production_attempts attempt ON attempt.task_id=task.id AND attempt.id=$2
            JOIN delivery_versions delivery ON delivery.render_task_id=task.id
                AND delivery.id=$3 AND delivery.render_snapshot_id=$4
            WHERE task.id=$1 FOR UPDATE OF task,attempt,delivery
            """,
            plan.task_id,
            plan.attempt_id,
            delivery_id,
            plan.render_snapshot_id,
        )
        if row is None:
            raise RuntimeError("render completion facts do not match")
        return row

    async def _finish_task(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        plan: RenderExecutionPlan,
        previous_status: str,
        resource_version: int,
        status: str,
        error_code: str | None = None,
        summary: str | None = None,
    ) -> None:
        transition = transition_task(previous_status, status, resource_version)
        progress = {
            "phase": "completed" if status == "succeeded" else "failed",
            "completed": int(status == "succeeded"),
            "total": 1,
        }
        error = json.dumps({"retryable": False, "summary": summary}) if summary else None
        await connection.execute(
            """
            UPDATE production_tasks SET status=$2,resource_version=$3,
                progress_json=$4::jsonb,error_code=$5,error_json=$6::jsonb,
                next_action=NULL,updated_at=now(),finished_at=now()
            WHERE id=$1 AND resource_version=$7
            """,
            plan.task_id,
            status,
            transition.resource_version,
            json.dumps(progress),
            error_code,
            error,
            transition.previous_resource_version,
        )
        await self._events.record(
            connection, task_id=plan.task_id, resource_version=transition.resource_version,
            event_type=transition.event_type,
        )
