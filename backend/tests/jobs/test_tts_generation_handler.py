from __future__ import annotations

from typing import Protocol

import pytest

from db.pool import DatabasePool
from integrations.ai.deterministic_media import DeterministicTtsProvider, GeneratedMedia
from integrations.ai.deterministic_video import DockerFfmpegRuntime
from integrations.ai.registry import create_mvp_registry
from integrations.object_storage import MinioObjectStore
from services.media_registration import MediaRegistrationService
from services.media_validation import MediaValidationService
from services.tasks import TaskQueryService
from tests.integration.media_library.support import MemoryTransport, accepted_tts_task
from workers.media_generation import GenerateMediaJobHandler
from workers.provider_execution import FaultInjector, InjectedFault


class TtsProvider(Protocol):
    async def generate(
        self,
        text_hash: str,
        output_slot: str,
        *,
        text: str,
        voice_id: str,
    ) -> GeneratedMedia: ...


class RecordingTtsProvider:
    def __init__(self) -> None:
        self.calls: list[tuple[str, str, str, str]] = []
        self._provider = DeterministicTtsProvider()

    async def generate(
        self,
        text_hash: str,
        output_slot: str,
        *,
        text: str,
        voice_id: str,
    ) -> GeneratedMedia:
        self.calls.append((text_hash, output_slot, text, voice_id))
        return await self._provider.generate(
            text_hash, output_slot, text=text, voice_id=voice_id
        )


class CorruptTtsProvider:
    def __init__(self) -> None:
        self.call_count = 0

    async def generate(
        self,
        text_hash: str,
        output_slot: str,
        *,
        text: str,
        voice_id: str,
    ) -> GeneratedMedia:
        assert len(text_hash) == 64 and text and voice_id
        self.call_count += 1
        return GeneratedMedia(output_slot, "audio/wav", b"invalid-wav")


class FailOnce(FaultInjector):
    def __init__(self) -> None:
        self.failed = False

    def hit(self, point: str) -> None:
        if point == "after_media_registration" and not self.failed:
            self.failed = True
            raise InjectedFault(point)


def tts_job_handler(
    database: DatabasePool,
    provider: TtsProvider,
    transport: MemoryTransport,
    fault: FaultInjector | None = None,
) -> GenerateMediaJobHandler:
    registry = create_mvp_registry({("tts", "mock"): lambda _profile: provider})
    registration = MediaRegistrationService(
        database,
        MediaValidationService(DockerFfmpegRuntime()),
        MinioObjectStore(transport, bucket="lanverse"),
    )
    return GenerateMediaJobHandler(
        database,
        registry=registry,
        registration=registration,
        fault=fault or FaultInjector(),
    )


@pytest.mark.asyncio
async def test_tts_job_maps_logical_voice_and_registers_decodable_audio(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id, text, context = await accepted_tts_task(database, "ready")
        provider = RecordingTtsProvider()
        transport = MemoryTransport()

        await tts_job_handler(database, provider, transport).handle(context)

        task = await TaskQueryService(database).get(task_id)
        assert task.status == "succeeded"
        assert len(provider.calls) == 1
        assert provider.calls[0][1:] == (
            "primary",
            text,
            "mock.narrator_female",
        )
        assert len(provider.calls[0][0]) == 64
        assert len(transport.objects) == 1
        async with database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT candidate.status,version.status media_status,
                       version.duration_ticks,version.timebase,
                       version.probe_summary_json->>'sample_rate' sample_rate
                FROM generation_candidates candidate
                JOIN media_versions version ON version.id=candidate.media_version_id
                WHERE candidate.task_id=$1
                """,
                task_id,
            )
        assert row["status"] == row["media_status"] == "ready"
        assert row["duration_ticks"] > 0 and row["timebase"] == 90000
        assert row["sample_rate"] == "48000"
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_invalid_tts_is_blocked_without_affecting_other_outputs(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id, _, context = await accepted_tts_task(database, "invalid")
        provider = CorruptTtsProvider()
        transport = MemoryTransport()

        await tts_job_handler(database, provider, transport).handle(context)

        task = await TaskQueryService(database).get(task_id)
        assert task.status == "failed" and task.error_code == "OUTPUT_INVALID"
        assert provider.call_count == 1 and transport.objects == {}
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_tts_recovery_reuses_registered_candidate_without_provider_recall(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id, _, context = await accepted_tts_task(database, "recovery")
        provider = RecordingTtsProvider()
        handler = tts_job_handler(database, provider, MemoryTransport(), FailOnce())

        with pytest.raises(InjectedFault):
            await handler.handle(context)
        await handler.handle(context)

        task = await TaskQueryService(database).get(task_id)
        assert task.status == "succeeded"
        assert len(provider.calls) == 1
    finally:
        await database.close()
