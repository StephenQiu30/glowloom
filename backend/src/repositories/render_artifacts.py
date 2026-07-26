from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from integrations.object_storage import StoredObject
from repositories.render_executions import RenderExecutionPlan
from schemas.delivery_manifest import DeliveryArtifactRefV1
from schemas.delivery_quality import DeliveryProbeSummaryV1


@dataclass(frozen=True, slots=True)
class UploadedDeliveryArtifacts:
    mp4: StoredObject
    srt: StoredObject
    manifest: StoredObject


@dataclass(frozen=True, slots=True)
class RegisteredArtifact:
    media_version_id: UUID
    stored: StoredObject

    def reference(self) -> DeliveryArtifactRefV1:
        return DeliveryArtifactRefV1(
            media_version_id=self.media_version_id,
            bucket=self.stored.bucket,
            object_key=self.stored.object_key,
            content_type=self.stored.content_type,
            byte_size=self.stored.byte_size,
            sha256=self.stored.sha256,
        )


class RenderArtifactRepository:
    async def register_exact(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        plan: RenderExecutionPlan,
        artifacts: UploadedDeliveryArtifacts,
        quality: DeliveryProbeSummaryV1,
    ) -> dict[str, RegisteredArtifact]:
        rows = await connection.fetch(
            "SELECT id,output_slot FROM media_versions WHERE origin_attempt_id=$1",
            plan.attempt_id,
        )
        if rows:
            raise RuntimeError("render attempt already has partial artifacts")
        values = {}
        for slot, source, kind, stored in (
            ("mp4", "ffmpeg", "video", artifacts.mp4),
            ("srt", "application", "subtitle", artifacts.srt),
            ("manifest", "application", "manifest", artifacts.manifest),
        ):
            values[slot] = await self._insert(
                connection,
                plan,
                slot,
                source,
                kind,
                stored,
                quality if slot == "mp4" else None,
            )
        return values

    async def _insert(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        plan: RenderExecutionPlan,
        slot: str,
        source_kind: str,
        media_kind: str,
        stored: StoredObject,
        quality: DeliveryProbeSummaryV1 | None,
    ) -> RegisteredArtifact:
        object_id, version_id = new_id(), new_id()
        await connection.execute(
            """
            INSERT INTO media_objects(id,episode_id,media_kind,source_kind)
            VALUES($1,$2,$3,$4)
            """,
            object_id,
            plan.episode_id,
            media_kind,
            source_kind,
        )
        await connection.execute(
            """
            INSERT INTO media_versions(
                id,media_object_id,version,origin_attempt_id,output_slot,bucket,
                object_key,mime_type,byte_size,sha256,status,width,height,
                duration_ticks,timebase,probe_summary_json,finalized_at
            ) VALUES(
                $1,$2,1,$3,$4,$5,$6,$7,$8,$9,'ready',$10,$11,$12,$13,$14::jsonb,now()
            )
            """,
            version_id,
            object_id,
            plan.attempt_id,
            slot,
            stored.bucket,
            stored.object_key,
            stored.content_type,
            stored.byte_size,
            stored.sha256,
            quality.width if quality else None,
            quality.height if quality else None,
            quality.duration_ticks if quality else None,
            quality.timebase if quality else None,
            quality.model_dump_json() if quality else None,
        )
        return RegisteredArtifact(version_id, stored)
