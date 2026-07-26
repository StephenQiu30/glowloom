from __future__ import annotations

from uuid import UUID, uuid4

import pytest

from db.pool import DatabasePool
from integrations.ai.deterministic_media import DeterministicImageProvider
from integrations.ai.deterministic_video import DockerFfmpegRuntime
from integrations.object_storage import MinioObjectStore, RemoteObject
from schemas.tasks import SubmitTaskCommand
from services.media_registration import (
    MediaRegistrationCommand,
    MediaRegistrationConflict,
    MediaRegistrationService,
)
from services.media_validation import MediaValidationService
from services.projects import CreateProjectCommand, CreateProjectHandler
from services.task_submission import TaskSubmitter

INPUT_HASH = "a" * 64


class MemoryTransport:
    def __init__(self) -> None:
        self.objects: dict[tuple[str, str], RemoteObject] = {}
        self.write_count = 0

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
        self.write_count += 1
        self.objects[(bucket, object_key)] = RemoteObject(len(data), sha256, content_type)


async def create_media_task(database: DatabasePool) -> tuple[UUID, UUID, UUID]:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="媒体登记", idempotency_key=f"media-project:{uuid4()}")
    )
    episode_id = project.episode.id
    accepted = await TaskSubmitter(database, release_version="test-release").submit(
        SubmitTaskCommand(
            episode_id=episode_id,
            task_type="generate_media",
            capability="image",
            scope={"episode_id": str(episode_id)},
            input_refs={"input_hash": INPUT_HASH},
            prompt="生成图片",
            parameters={"width": 720, "height": 1280},
            model_profile_id="mock-image-v1",
            provider_id="mock",
            model_id="deterministic-image",
            route_version="image-route-v1",
            schema_version="image-v1",
            operation_scope=f"generateMedia/{episode_id}/{uuid4()}",
            idempotency_key=f"media-task:{uuid4()}",
            handler_version="1",
        )
    )
    async with database.transaction() as connection:
        attempt_id = await connection.fetchval(
            "SELECT id FROM production_attempts WHERE task_id=$1", accepted.task_id
        )
    assert isinstance(attempt_id, UUID)
    return episode_id, accepted.task_id, attempt_id


@pytest.mark.asyncio
async def test_registers_ready_media_and_candidate_idempotently(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    transport = MemoryTransport()
    try:
        episode_id, task_id, attempt_id = await create_media_task(database)
        media = await DeterministicImageProvider().generate(INPUT_HASH, "shot/1")
        service = MediaRegistrationService(
            database,
            MediaValidationService(DockerFfmpegRuntime()),
            MinioObjectStore(transport, bucket="lanverse"),
        )
        command = MediaRegistrationCommand(
            episode_id=episode_id,
            task_id=task_id,
            attempt_id=attempt_id,
            output_slot="primary",
            usage_type="shot_image",
            usage_id=uuid4(),
            input_version_id=uuid4(),
            input_hash=INPUT_HASH,
            media_kind="image",
            content_type=media.content_type,
            data=media.data,
        )

        first = await service.register(command)
        replay = await service.register(command)

        assert first == replay
        assert first.media_status == "ready"
        assert first.candidate_status == "ready"
        assert first.sha256 == media.sha256
        assert transport.write_count == 1
        async with database.transaction() as connection:
            counts = await connection.fetchrow(
                """
                SELECT (SELECT count(*) FROM media_objects) media_objects,
                       (SELECT count(*) FROM media_versions) media_versions,
                       (SELECT count(*) FROM generation_candidates) candidates
                """
            )
            row = await connection.fetchrow(
                "SELECT * FROM media_versions WHERE id=$1", first.media_version_id
            )
        assert counts == {"media_objects": 1, "media_versions": 1, "candidates": 1}
        assert row["bucket"] == "lanverse"
        assert row["object_key"] == first.object_key
        assert row["probe_summary_json"] == {"codec": "png", "width": 720, "height": 1280}
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_replay_with_different_usage_facts_is_rejected(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        episode_id, task_id, attempt_id = await create_media_task(database)
        media = await DeterministicImageProvider().generate(INPUT_HASH, "shot/1")
        service = MediaRegistrationService(
            database,
            MediaValidationService(DockerFfmpegRuntime()),
            MinioObjectStore(MemoryTransport(), bucket="lanverse"),
        )
        command = MediaRegistrationCommand(
            episode_id, task_id, attempt_id, "primary", "shot_image", uuid4(), uuid4(),
            INPUT_HASH, "image", media.content_type, media.data,
        )
        await service.register(command)

        with pytest.raises(MediaRegistrationConflict):
            await service.register(
                MediaRegistrationCommand(
                    episode_id, task_id, attempt_id, "primary", "shot_image", uuid4(),
                    command.input_version_id, INPUT_HASH, "image", media.content_type, media.data,
                )
            )
    finally:
        await database.close()
