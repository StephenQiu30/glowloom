from __future__ import annotations

from datetime import timedelta
from uuid import UUID

from core.clock import Clock
from db.pool import DatabasePool
from integrations.object_storage import ObjectStore
from repositories.asset_versions import CreativeAssetVersionRepository
from repositories.deliveries import DeliveryRepository
from repositories.delivery_lineage import DeliveryLineageRepository
from repositories.render_snapshots import RenderSnapshotRepository
from repositories.script_versions import ScriptVersioningRepository
from repositories.sources import SourceRevisionRepository
from repositories.storyboard_versions import StoryboardVersionRepository
from repositories.subtitles import SubtitleRepository
from repositories.tasks import TaskRepository
from schemas.deliveries import (
    DeliveryArtifactType,
    DeliveryDownloadAuthorization,
    DeliveryViewSnapshot,
)
from schemas.delivery_lineage import DeliveryDetailSnapshot, DeliveryLineageSnapshot

DOWNLOAD_TTL_SECONDS = 900


class DeliveryNotFound(LookupError):
    pass


class DeliveryQueryService:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._deliveries = DeliveryRepository()
        self._lineage = DeliveryLineageRepository()
        self._snapshots = RenderSnapshotRepository()
        self._subtitles = SubtitleRepository()
        self._boards = StoryboardVersionRepository()
        self._scripts = ScriptVersioningRepository()
        self._assets = CreativeAssetVersionRepository()
        self._sources = SourceRevisionRepository()
        self._tasks = TaskRepository()

    async def list(self, episode_id: UUID) -> tuple[DeliveryViewSnapshot, ...]:
        async with self._database.transaction() as connection:
            deliveries = await self._deliveries.list_for_episode(connection, episode_id)
            values = []
            for delivery in deliveries:
                values.append(
                    DeliveryViewSnapshot(
                        delivery=delivery,
                        artifacts=await self._deliveries.artifacts(connection, delivery),
                    )
                )
            return tuple(values)

    async def get(self, delivery_id: UUID) -> DeliveryDetailSnapshot:
        async with self._database.transaction() as connection:
            delivery = await self._deliveries.get(connection, delivery_id)
            if delivery is None:
                raise DeliveryNotFound
            snapshot = await self._snapshots.get(connection, delivery.render_snapshot_id)
            if snapshot is None:
                raise RuntimeError("delivery render snapshot is missing")
            subtitle = await self._subtitles.get(connection, snapshot.subtitle_version_id)
            board = await self._boards.get(connection, snapshot.shot_spec_version_id)
            if subtitle is None or board is None:
                raise RuntimeError("delivery story lineage is incomplete")
            script = await self._scripts.get(connection, board.content.script_version_id)
            if script is None:
                raise RuntimeError("delivery script lineage is missing")
            source = await self._sources.get(connection, script.source_revision_id)
            assets = await self._assets.get_many(connection, board.content.asset_version_ids)
            task = await self._tasks.get(connection, delivery.render_task_id)
            if (
                source is None
                or task is None
                or len(assets) != len(board.content.asset_version_ids)
            ):
                raise RuntimeError("delivery root lineage is incomplete")
            attempts = await self._lineage.attempts(connection, task.id)
            input_media = await self._lineage.input_media(connection, snapshot)
            delivery_media = await self._deliveries.artifacts(connection, delivery)
        return DeliveryDetailSnapshot(
            delivery=delivery,
            lineage=DeliveryLineageSnapshot(
                source_revision=source,
                script_version=script,
                creative_asset_versions=assets,
                shot_spec_version=board,
                subtitle_version=subtitle,
                render_snapshot=snapshot,
                render_task=task,
                render_attempts=attempts,
                input_media=input_media,
                delivery_media=delivery_media,
            ),
        )


class DeliveryDownloadService:
    def __init__(
        self,
        database: DatabasePool,
        object_store: ObjectStore,
        clock: Clock,
    ) -> None:
        self._database = database
        self._objects = object_store
        self._clock = clock
        self._deliveries = DeliveryRepository()

    async def authorize(
        self,
        *,
        delivery_id: UUID,
        episode_id: UUID,
        artifact_types: tuple[DeliveryArtifactType, ...],
    ) -> tuple[DeliveryDownloadAuthorization, ...]:
        async with self._database.transaction() as connection:
            delivery = await self._deliveries.get(connection, delivery_id)
            if delivery is None or delivery.episode_id != episode_id or delivery.status != "ready":
                raise DeliveryNotFound
            artifacts = await self._deliveries.artifacts(connection, delivery)
        selected = {item.artifact_type: item for item in artifacts}
        if any(item not in selected for item in artifact_types):
            raise DeliveryNotFound
        expires_at = self._clock.now() + timedelta(seconds=DOWNLOAD_TTL_SECONDS)
        values = []
        for artifact_type in artifact_types:
            artifact = selected[artifact_type]
            url = await self._objects.authorize_read(
                bucket=artifact.bucket,
                object_key=artifact.object_key,
                expires_seconds=DOWNLOAD_TTL_SECONDS,
            )
            values.append(
                DeliveryDownloadAuthorization(
                    artifact_type=artifact_type,
                    media_version_id=artifact.media_version_id,
                    url=url,
                    expires_in_seconds=DOWNLOAD_TTL_SECONDS,
                    expires_at=expires_at,
                )
            )
        return tuple(values)
