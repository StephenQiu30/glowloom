from __future__ import annotations

import json
from uuid import UUID

import pytest

from db.pool import DatabasePool
from integrations.ai.deterministic_media import DeterministicImageProvider, GeneratedMedia
from integrations.ai.deterministic_video import DockerFfmpegRuntime
from integrations.ai.registry import create_mvp_registry
from integrations.object_storage import MinioObjectStore, RemoteObject
from schemas.jobs import JobPayload
from services.media_generation import GenerateMediaCommand, GenerateMediaHandler
from services.media_registration import MediaRegistrationService
from services.media_validation import MediaValidationService
from services.storyboards import ConfirmStoryboardCommand, ConfirmStoryboardHandler
from services.tasks import TaskQueryService
from tests.integration.story_development.support import storyboard_draft
from workers.dispatch import JobContext
from workers.media_generation import GenerateMediaJobHandler, ImageProvider
from workers.provider_execution import FaultInjector, InjectedFault


class MemoryTransport:
    def __init__(self) -> None:
        self.objects: dict[tuple[str, str], RemoteObject] = {}

    def stat(self, bucket: str, object_key: str) -> RemoteObject | None:
        return self.objects.get((bucket, object_key))

    def put(
        self,
        bucket: str,
        object_key: str,
        data: bytes,
        content_type: str,
        sha256: str,
    ) -> None:
        self.objects[(bucket, object_key)] = RemoteObject(len(data), sha256, content_type)


class FailOnce(FaultInjector):
    def __init__(self, point: str) -> None:
        self._point = point
        self._failed = False

    def hit(self, point: str) -> None:
        if point == self._point and not self._failed:
            self._failed = True
            raise InjectedFault(point)


class CorruptImageProvider:
    def __init__(self) -> None:
        self.call_count = 0

    async def generate(self, input_hash: str, output_slot: str) -> GeneratedMedia:
        assert len(input_hash) == 64 and output_slot == "primary"
        self.call_count += 1
        return GeneratedMedia(output_slot, "image/png", b"not-a-decodable-png")


async def accepted_image_task(
    database: DatabasePool, key: str
) -> tuple[UUID, JobContext]:
    episode_id, generated = await storyboard_draft(database, f"worker:{key}")
    confirmed = await ConfirmStoryboardHandler(database).execute(
        ConfirmStoryboardCommand(
            generated.storyboard.id, generated.storyboard.resource_version
        )
    )
    shot = confirmed.storyboard.content.shots[0]
    accepted = await GenerateMediaHandler(database, release_version="test-release").execute(
        GenerateMediaCommand(
            episode_id=episode_id,
            usage_type="shot_image",
            usage_id=shot.shot_id,
            input_version_id=confirmed.storyboard.id,
            idempotency_key=f"worker:image:{key}",
        )
    )
    async with database.transaction() as connection:
        row = await connection.fetchrow(
            "SELECT id,payload_json FROM task_jobs WHERE task_id=$1", accepted.task_id
        )
    payload = JobPayload.parse(json.loads(row["payload_json"]))
    return accepted.task_id, JobContext(job_id=row["id"], owner="worker-test", payload=payload)


def image_handler(
    database: DatabasePool,
    provider: ImageProvider,
    fault: FaultInjector,
    transport: MemoryTransport | None = None,
) -> GenerateMediaJobHandler:
    registry = create_mvp_registry({("image", "mock"): lambda _profile: provider})
    registration = MediaRegistrationService(
        database,
        MediaValidationService(DockerFfmpegRuntime()),
        MinioObjectStore(transport or MemoryTransport(), bucket="lanverse"),
    )
    return GenerateMediaJobHandler(
        database,
        registry=registry,
        registration=registration,
        fault=fault,
    )


@pytest.mark.asyncio
async def test_image_job_registers_candidate_output_before_succeeding(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id, context = await accepted_image_task(database, "success")
        provider = DeterministicImageProvider()

        await image_handler(database, provider, FaultInjector()).handle(context)

        task = await TaskQueryService(database).get(task_id)
        assert task.status == "succeeded"
        assert provider.call_count == 1
        assert len(task.result_refs) == 1
        assert task.result_refs[0].output_type == "generation_candidate"
        async with database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT candidate.status,version.status media_status,version.sha256
                FROM generation_candidates candidate
                JOIN media_versions version ON version.id=candidate.media_version_id
                WHERE candidate.task_id=$1
                """,
                task_id,
            )
        assert (row["status"], row["media_status"], len(row["sha256"])) == (
            "ready",
            "ready",
            64,
        )
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_invalid_image_is_blocked_without_upload_and_task_fails_cleanly(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id, context = await accepted_image_task(database, "invalid")
        provider = CorruptImageProvider()
        transport = MemoryTransport()

        await image_handler(database, provider, FaultInjector(), transport).handle(context)

        task = await TaskQueryService(database).get(task_id)
        assert task.status == "failed"
        assert task.error_code == "OUTPUT_INVALID"
        assert provider.call_count == 1
        assert transport.objects == {}
        assert len(task.result_refs) == 1
        async with database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT candidate.status,candidate.blocked_reason,
                       version.status media_status,version.byte_size,version.sha256,
                       version.probe_summary_json
                FROM generation_candidates candidate
                JOIN media_versions version ON version.id=candidate.media_version_id
                WHERE candidate.task_id=$1
                """,
                task_id,
            )
        assert (row["status"], row["blocked_reason"], row["media_status"]) == (
            "blocked",
            "OUTPUT_INVALID",
            "invalid",
        )
        assert row["byte_size"] == len(b"not-a-decodable-png")
        assert len(row["sha256"]) == 64
        assert row["probe_summary_json"] is None
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_recovery_after_registration_reuses_candidate_without_provider_recall(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id, context = await accepted_image_task(database, "recovery")
        provider = DeterministicImageProvider()
        handler = image_handler(database, provider, FailOnce("after_media_registration"))

        with pytest.raises(InjectedFault):
            await handler.handle(context)
        await handler.handle(context)

        task = await TaskQueryService(database).get(task_id)
        assert task.status == "succeeded"
        assert provider.call_count == 1
        async with database.transaction() as connection:
            assert await connection.fetchval(
                "SELECT count(*) FROM generation_candidates WHERE task_id=$1", task_id
            ) == 1
    finally:
        await database.close()
