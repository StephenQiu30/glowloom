from __future__ import annotations

from typing import Protocol

from db.pool import DatabasePool
from integrations.ai.deterministic_video import MediaRuntimeError, VideoProbe
from integrations.ffmpeg_recipe import RenderSources
from integrations.ffmpeg_render import RenderRuntimeError
from integrations.object_storage import ObjectIntegrityError, ObjectKeyConflict, ObjectStore
from repositories.render_completion import RenderCompletionStore
from repositories.render_executions import RenderExecutionStore
from repositories.render_recovery import RENDER_RETRY_ERROR, RenderRecoveryStore
from schemas.delivery_media_lineage import DeliveryMediaLineageInvalid
from schemas.rendering import RenderRecipeV1
from services.delivery_artifacts import DeliveryArtifactWriter
from services.delivery_quality import DeliveryQualityInvalid, DeliveryQualityPolicy
from services.render_delivery import StartRenderDeliveryHandler
from services.render_sources import RenderSourceInvalid, RenderSourceLoader
from workers.dispatch import JobContext
from workers.errors import RetryableJobError
from workers.provider_execution import FaultInjector


class RenderRuntime(Protocol):
    async def render(self, sources: RenderSources, recipe: RenderRecipeV1) -> bytes: ...


class ProbeRuntime(Protocol):
    async def probe(self, data: bytes) -> VideoProbe: ...


class RenderEpisodeJobHandler:
    def __init__(
        self,
        database: DatabasePool,
        *,
        object_store: ObjectStore,
        render_runtime: RenderRuntime,
        probe_runtime: ProbeRuntime,
        fault: FaultInjector | None = None,
    ) -> None:
        self._executions = RenderExecutionStore(database)
        self._deliveries = StartRenderDeliveryHandler(database)
        self._sources = RenderSourceLoader(database, object_store)
        self._artifacts = DeliveryArtifactWriter(object_store)
        self._completion = RenderCompletionStore(database)
        self._recovery = RenderRecoveryStore(database)
        self._render = render_runtime
        self._probe = probe_runtime
        self._quality = DeliveryQualityPolicy()
        self._fault = fault or FaultInjector()

    async def handle(self, context: JobContext) -> None:
        plan = await self._executions.prepare(context.payload)
        if plan.skip:
            return
        delivery = await self._deliveries.execute(plan.task_id)
        try:
            bundle = await self._sources.load(plan.render_snapshot_id)
            self._fault.hit("before_ffmpeg")
            mp4 = await self._render.render(bundle.sources, bundle.snapshot.recipe)
            self._fault.hit("after_ffmpeg")
            probe = await self._probe.probe(mp4)
            quality = self._quality.validate(
                probe, target_duration_ticks=bundle.target_duration_ticks
            )
            self._fault.hit("before_artifact_upload")
            artifacts = await self._artifacts.write(
                bundle=bundle,
                task_id=plan.task_id,
                attempt_id=plan.attempt_id,
                mp4=mp4,
                quality=quality,
            )
            self._fault.hit("after_artifact_upload")
        except RenderRuntimeError as error:
            scheduled = await self._recovery.retry_or_fail(
                plan,
                delivery_id=delivery.id,
                summary=str(error)[:500],
            )
            if scheduled:
                raise RetryableJobError(RENDER_RETRY_ERROR) from error
            return
        except (
            DeliveryQualityInvalid,
            DeliveryMediaLineageInvalid,
            MediaRuntimeError,
            ObjectIntegrityError,
            ObjectKeyConflict,
            RenderSourceInvalid,
        ) as error:
            await self._completion.mark_failed(
                plan,
                delivery_id=delivery.id,
                error_code="RENDER_OUTPUT_INVALID",
                summary=str(error)[:500],
            )
            return
        await self._completion.mark_ready(
            plan,
            delivery_id=delivery.id,
            artifacts=artifacts,
            quality=quality,
            ffmpeg_version=bundle.snapshot.recipe.ffmpeg_version,
        )
        self._fault.hit("after_delivery_ready")
