from __future__ import annotations

import json

import pytest

from db.pool import DatabasePool
from integrations.ai.deterministic_video import VideoProbe
from integrations.ffmpeg_recipe import RenderSources
from integrations.object_storage import MinioObjectStore, ObjectStoreUnavailable
from schemas.rendering import RenderRecipeV1
from services.render_submission import RenderEpisodeCommand, RenderEpisodeCoordinator
from tests.integration.delivery.support import render_ready_story
from tests.integration.media_library.support import MemoryTransport, media_job_context
from tests.jobs.render_support import delivery_facts, historical_counts, render_state
from workers.provider_execution import FaultInjector
from workers.render_episode import RenderEpisodeJobHandler


class CapturingRenderRuntime:
    def __init__(self) -> None:
        self.calls = 0
        self.sources: RenderSources | None = None

    async def render(self, sources: RenderSources, recipe: RenderRecipeV1) -> bytes:
        self.calls += 1
        self.sources = sources
        return b"deterministic-final-mp4"


class ValidProbeRuntime:
    async def probe(self, data: bytes) -> VideoProbe:
        assert data == b"deterministic-final-mp4"
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


class FailManifestOnceTransport(MemoryTransport):
    def __init__(self) -> None:
        super().__init__()
        self.fail_manifest_once = False

    def put(
        self,
        bucket: str,
        object_key: str,
        data: bytes,
        content_type: str,
        sha256: str,
    ) -> None:
        if self.fail_manifest_once and object_key.endswith("/manifest.json"):
            self.fail_manifest_once = False
            raise OSError("interrupted manifest upload")
        super().put(bucket, object_key, data, content_type, sha256)


def recipe() -> RenderRecipeV1:
    return RenderRecipeV1(
        runtime_image="sha256:" + "a" * 64,
        ffmpeg_version="8.1",
        ffprobe_version="8.1",
        font_name="Noto Sans CJK SC",
        font_file="/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf",
        font_sha256="b" * 64,
        font_license="OFL-1.1",
    )


@pytest.mark.asyncio
async def test_render_job_replays_upload_and_registers_exact_delivery_artifacts(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=12)
    await database.start()
    try:
        transport = FailManifestOnceTransport()
        episode_id, _, subtitle, _ = await render_ready_story(database, "render-job", transport)
        accepted = await RenderEpisodeCoordinator(
            database,
            recipe=recipe(),
            release_version="test-release",
            fault=FaultInjector(),
        ).execute(RenderEpisodeCommand(episode_id, "render-job:exact-artifacts"))
        context = await media_job_context(database, accepted.task_id)
        runtime = CapturingRenderRuntime()
        handler = RenderEpisodeJobHandler(
            database,
            object_store=MinioObjectStore(transport, bucket="lanverse"),
            render_runtime=runtime,
            probe_runtime=ValidProbeRuntime(),
        )
        before = await historical_counts(database, episode_id)

        transport.fail_manifest_once = True
        with pytest.raises(ObjectStoreUnavailable, match="upload"):
            await handler.handle(context)
        assert await render_state(database, accepted.task_id) == ("running", "rendering", 0)

        await handler.handle(context)
        await handler.handle(context)

        state = await delivery_facts(database, accepted.task_id)
        assert state["task_status"] == "succeeded"
        assert state["attempt_status"] == "succeeded"
        assert state["delivery_status"] == "ready"
        assert state["slots"] == ["manifest", "mp4", "srt"]
        assert state["source_kinds"] == ["application", "ffmpeg", "application"]
        assert state["task_outputs"] == 1
        assert await historical_counts(database, episode_id) == before

        artifacts = state["artifacts"]
        manifest_key = artifacts["manifest"]["object_key"]
        manifest = json.loads(transport.contents[("lanverse", manifest_key)])
        assert manifest["schema_version"] == "delivery-manifest-v1"
        assert manifest["render_snapshot_id"] == str(state["render_snapshot_id"])
        assert manifest["render_task_id"] == str(accepted.task_id)
        assert manifest["final_attempt_id"] == str(state["attempt_id"])
        assert manifest["inputs"]["subtitle_version_id"] == str(subtitle.id)
        assert len(manifest["inputs"]["video_adoptions"]) == 6
        assert manifest["subtitle_input_refs"]["script_version_id"] == str(
            subtitle.script_version_id
        )
        lineage = manifest["media_lineage"]
        assert len(lineage) == len(manifest["inputs"]["video_adoptions"]) + len(
            manifest["inputs"]["tts_adoptions"]
        )
        assert all(
            item["origin_attempt_id"]
            and item["origin_task_id"]
            and item["origin_submission_snapshot_id"]
            and item["provider_id"]
            and item["model_id"]
            for item in lineage
        )
        assert manifest["artifacts"]["mp4"]["sha256"] == artifacts["mp4"]["sha256"]
        assert manifest["artifacts"]["srt"]["sha256"] == artifacts["srt"]["sha256"]
        assert runtime.calls == 2
    finally:
        await database.close()
