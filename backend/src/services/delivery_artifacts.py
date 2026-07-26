from __future__ import annotations

from uuid import UUID

from integrations.object_storage import ObjectStore, StoredObject
from repositories.render_artifacts import UploadedDeliveryArtifacts
from schemas.delivery_manifest import (
    DeliveryArtifactDigestV1,
    DeliveryManifestArtifactsV1,
    DeliveryManifestV1,
)
from schemas.delivery_quality import DeliveryProbeSummaryV1
from services.render_sources import RenderSourceBundle


class DeliveryArtifactWriter:
    def __init__(self, object_store: ObjectStore) -> None:
        self._objects = object_store

    async def write(
        self,
        *,
        bundle: RenderSourceBundle,
        task_id: UUID,
        attempt_id: UUID,
        mp4: bytes,
        quality: DeliveryProbeSummaryV1,
    ) -> UploadedDeliveryArtifacts:
        srt = bundle.subtitles_srt.encode()
        stored_mp4 = await self._put(
            bundle.snapshot.episode_id, attempt_id, "mp4", "video/mp4", mp4
        )
        stored_srt = await self._put(
            bundle.snapshot.episode_id,
            attempt_id,
            "srt",
            "application/x-subrip",
            srt,
        )
        manifest = DeliveryManifestV1(
            episode_id=bundle.snapshot.episode_id,
            render_snapshot_id=bundle.snapshot.id,
            render_task_id=task_id,
            final_attempt_id=attempt_id,
            snapshot_content_hash=bundle.snapshot.content_hash,
            recipe_hash=bundle.snapshot.recipe_hash,
            inputs=bundle.snapshot.input_refs,
            subtitle_input_refs=bundle.subtitle.input_refs,
            media_lineage=bundle.media_lineage,
            segments=bundle.snapshot.segments,
            recipe=bundle.snapshot.recipe,
            artifacts=DeliveryManifestArtifactsV1(
                mp4=_digest(stored_mp4),
                srt=_digest(stored_srt),
            ),
            quality=quality,
        ).canonical_bytes()
        stored_manifest = await self._put(
            bundle.snapshot.episode_id,
            attempt_id,
            "manifest",
            "application/json",
            manifest,
        )
        return UploadedDeliveryArtifacts(stored_mp4, stored_srt, stored_manifest)

    async def _put(
        self,
        episode_id: UUID,
        attempt_id: UUID,
        slot: str,
        content_type: str,
        data: bytes,
    ) -> StoredObject:
        return await self._objects.put(
            episode_id=episode_id,
            attempt_id=attempt_id,
            output_slot=slot,
            content_type=content_type,
            data=data,
        )


def _digest(value: StoredObject) -> DeliveryArtifactDigestV1:
    return DeliveryArtifactDigestV1(
        content_type=value.content_type,
        byte_size=value.byte_size,
        sha256=value.sha256,
    )
