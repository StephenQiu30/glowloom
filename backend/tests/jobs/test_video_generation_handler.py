from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

import pytest

from db.pool import DatabasePool
from integrations.ai.deterministic_media import GeneratedMedia
from integrations.ai.deterministic_video import VideoProbe
from integrations.ai.registry import create_mvp_registry
from integrations.object_storage import MinioObjectStore
from services.media_registration import MediaRegistrationService
from services.media_validation import TIMEBASE, MediaValidationService
from services.tasks import TaskQueryService
from tests.integration.media_library.support import (
    MemoryTransport,
    accepted_video_task,
)
from workers.media_generation import GenerateMediaJobHandler
from workers.provider_execution import FaultInjector


class RecordingVideoProvider:
    def __init__(self) -> None:
        self.calls: list[tuple[str, str, int]] = []

    async def generate(
        self,
        input_hash: str,
        output_slot: str,
        *,
        duration_ticks: int,
    ) -> GeneratedMedia:
        self.calls.append((input_hash, output_slot, duration_ticks))
        return GeneratedMedia(
            output_slot,
            "video/mp4",
            b"deterministic-video-output",
            width=720,
            height=1280,
            duration_ticks=duration_ticks,
        )


class VideoProvider(Protocol):
    async def generate(
        self,
        input_hash: str,
        output_slot: str,
        *,
        duration_ticks: int,
    ) -> GeneratedMedia: ...


@dataclass(slots=True)
class StubVideoRuntime:
    width: int
    height: int
    duration_ticks: int

    async def probe(self, data: bytes) -> VideoProbe:
        assert data == b"deterministic-video-output"
        return VideoProbe(
            codec_name="h264",
            pixel_format="yuv420p",
            width=self.width,
            height=self.height,
            frame_rate="24/1",
            duration_seconds=self.duration_ticks / TIMEBASE,
            audio_stream_count=0,
        )

    async def verify_video_decode(self, data: bytes) -> None:
        assert data == b"deterministic-video-output"


def video_job_handler(
    database: DatabasePool,
    provider: VideoProvider,
    runtime: StubVideoRuntime,
    transport: MemoryTransport,
) -> GenerateMediaJobHandler:
    registry = create_mvp_registry({("video", "mock"): lambda _profile: provider})
    registration = MediaRegistrationService(
        database,
        MediaValidationService(runtime),
        MinioObjectStore(transport, bucket="lanverse"),
    )
    return GenerateMediaJobHandler(
        database,
        registry=registry,
        registration=registration,
        fault=FaultInjector(),
    )


@pytest.mark.asyncio
async def test_video_job_uses_frozen_duration_and_registers_ready_candidate(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id, duration_ticks, context = await accepted_video_task(database, "ready")
        provider = RecordingVideoProvider()
        transport = MemoryTransport()

        await video_job_handler(
            database,
            provider,
            StubVideoRuntime(720, 1280, duration_ticks),
            transport,
        ).handle(context)

        task = await TaskQueryService(database).get(task_id)
        assert task.status == "succeeded"
        assert len(provider.calls) == 1
        assert len(provider.calls[0][0]) == 64
        assert provider.calls[0][1:] == ("primary", duration_ticks)
        assert len(transport.objects) == 1
        async with database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT candidate.status,version.status media_status,
                       version.duration_ticks,version.width,version.height,
                       version.probe_summary_json->>'codec' codec
                FROM generation_candidates candidate
                JOIN media_versions version ON version.id=candidate.media_version_id
                WHERE candidate.task_id=$1
                """,
                task_id,
            )
        assert tuple(row) == (
            "ready",
            "ready",
            duration_ticks,
            720,
            1280,
            "h264",
        )
    finally:
        await database.close()


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("width", "height", "duration_delta"),
    [(1280, 720, 0), (720, 1280, TIMEBASE + 1)],
)
async def test_invalid_video_probe_is_blocked_before_upload(
    migrated_database_url: str,
    width: int,
    height: int,
    duration_delta: int,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        key = f"invalid:{width}:{height}:{duration_delta}"
        task_id, duration_ticks, context = await accepted_video_task(database, key)
        provider = RecordingVideoProvider()
        transport = MemoryTransport()

        await video_job_handler(
            database,
            provider,
            StubVideoRuntime(width, height, duration_ticks + duration_delta),
            transport,
        ).handle(context)

        task = await TaskQueryService(database).get(task_id)
        assert task.status == "failed"
        assert task.error_code == "OUTPUT_INVALID"
        assert len(provider.calls) == 1
        assert transport.objects == {}
        async with database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT candidate.status,candidate.blocked_reason,version.status
                FROM generation_candidates candidate
                JOIN media_versions version ON version.id=candidate.media_version_id
                WHERE candidate.task_id=$1
                """,
                task_id,
            )
        assert tuple(row) == ("blocked", "OUTPUT_INVALID", "invalid")
    finally:
        await database.close()
