from __future__ import annotations

from uuid import UUID

from db.pool import DatabasePool
from integrations.ai.deterministic_video import VideoProbe
from integrations.ffmpeg_recipe import RenderSources
from integrations.object_storage import MinioObjectStore
from schemas.rendering import RenderRecipeV1
from services.render_submission import RenderEpisodeCommand, RenderEpisodeCoordinator
from tests.integration.delivery.support import render_ready_story
from tests.integration.media_library.support import MemoryTransport, media_job_context
from workers.provider_execution import FaultInjector
from workers.render_episode import RenderEpisodeJobHandler


class _RenderRuntime:
    async def render(self, sources: RenderSources, recipe: RenderRecipeV1) -> bytes:
        assert sources.videos and sources.audios and recipe.ffmpeg_version == "8.1"
        return b"delivery-api-final-mp4"


class _ProbeRuntime:
    async def probe(self, data: bytes) -> VideoProbe:
        assert data == b"delivery-api-final-mp4"
        return VideoProbe(
            codec_name="h264",
            pixel_format="yuv420p",
            width=720,
            height=1280,
            frame_rate="24/1",
            duration_seconds=30.0,
            audio_stream_count=1,
            video_start_seconds=0.0,
            video_duration_seconds=29.833333,
            audio_codec_name="aac",
            audio_sample_rate=48000,
            audio_channels=2,
            audio_start_seconds=0.0,
            audio_duration_seconds=30.0,
        )


def render_recipe() -> RenderRecipeV1:
    return RenderRecipeV1(
        runtime_image="sha256:" + "a" * 64,
        ffmpeg_version="8.1",
        ffprobe_version="8.1",
        font_name="Noto Sans CJK SC",
        font_file="/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf",
        font_sha256="b" * 64,
        font_license="OFL-1.1",
    )


async def complete_ready_delivery(
    database: DatabasePool,
    key: str,
    transport: MemoryTransport | None = None,
) -> tuple[UUID, UUID, MemoryTransport]:
    transport = transport or MemoryTransport()
    episode_id, _, _, _ = await render_ready_story(database, key, transport)
    accepted = await RenderEpisodeCoordinator(
        database,
        recipe=render_recipe(),
        release_version="test-release",
        fault=FaultInjector(),
    ).execute(RenderEpisodeCommand(episode_id, f"delivery-api:{key}"))
    await RenderEpisodeJobHandler(
        database,
        object_store=MinioObjectStore(transport, bucket="lanverse"),
        render_runtime=_RenderRuntime(),
        probe_runtime=_ProbeRuntime(),
    ).handle(await media_job_context(database, accepted.task_id))
    async with database.transaction() as connection:
        delivery_id = await connection.fetchval(
            "SELECT id FROM delivery_versions WHERE render_task_id=$1",
            accepted.task_id,
        )
    assert isinstance(delivery_id, UUID)
    return episode_id, delivery_id, transport
