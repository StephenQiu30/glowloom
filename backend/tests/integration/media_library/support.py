from __future__ import annotations

import json
from uuid import UUID, uuid4

from db.pool import DatabasePool
from integrations.ai.deterministic_video import DockerFfmpegRuntime
from integrations.ai.registry import AiModelRegistry, create_mvp_registry
from integrations.object_storage import MinioObjectStore, RemoteObject
from schemas.jobs import JobPayload
from services.media_generation import GenerateMediaCommand, GenerateMediaHandler
from services.media_registration import MediaRegistrationService
from services.media_validation import MediaValidationService
from services.script_versions import GetScriptVersionHandler
from services.storyboards import ConfirmStoryboardCommand, ConfirmStoryboardHandler
from tests.integration.story_development.support import storyboard_draft
from workers.dispatch import JobContext
from workers.media_generation import GenerateMediaJobHandler
from workers.provider_execution import FaultInjector


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


def media_job_handler(
    database: DatabasePool,
    *,
    registry: AiModelRegistry | None = None,
    transport: MemoryTransport | None = None,
) -> GenerateMediaJobHandler:
    registration = MediaRegistrationService(
        database,
        MediaValidationService(DockerFfmpegRuntime()),
        MinioObjectStore(transport or MemoryTransport(), bucket="lanverse"),
    )
    return GenerateMediaJobHandler(
        database,
        registry=registry or create_mvp_registry(),
        registration=registration,
        fault=FaultInjector(),
    )


async def run_media_job(
    database: DatabasePool, task_id: UUID, handler: GenerateMediaJobHandler
) -> None:
    await handler.handle(await media_job_context(database, task_id))


async def media_job_context(database: DatabasePool, task_id: UUID) -> JobContext:
    async with database.transaction() as connection:
        row = await connection.fetchrow(
            "SELECT id,payload_json FROM task_jobs WHERE task_id=$1", task_id
        )
    assert row is not None
    return JobContext(
        job_id=row["id"],
        owner="media-test",
        payload=JobPayload.parse(json.loads(row["payload_json"])),
    )


async def adopt_task_candidate(database: DatabasePool, task_id: UUID) -> dict[str, object]:
    async with database.transaction() as connection:
        candidate = await connection.fetchrow(
            """
            SELECT candidate.*,version.sha256
            FROM generation_candidates candidate
            JOIN media_versions version ON version.id=candidate.media_version_id
            WHERE candidate.task_id=$1 AND candidate.status='ready'
            """,
            task_id,
        )
        assert candidate is not None
        adoption_id = uuid4()
        await connection.execute(
            """
            INSERT INTO adoptions (
                id,episode_id,usage_type,usage_id,input_version_id,input_hash,
                version,candidate_id,status
            ) VALUES ($1,$2,$3,$4,$5,$6,1,$7,'active')
            """,
            adoption_id,
            candidate["episode_id"],
            candidate["usage_type"],
            candidate["usage_id"],
            candidate["input_version_id"],
            candidate["input_hash"],
            candidate["id"],
        )
    return {
        "usage_type": candidate["usage_type"],
        "usage_id": str(candidate["usage_id"]),
        "input_version_id": str(candidate["input_version_id"]),
        "input_hash": candidate["input_hash"],
        "adoption_id": str(adoption_id),
        "candidate_id": str(candidate["id"]),
        "media_version_id": str(candidate["media_version_id"]),
        "sha256": candidate["sha256"],
    }


async def accepted_video_task(
    database: DatabasePool, key: str
) -> tuple[UUID, int, JobContext]:
    episode_id, generated = await storyboard_draft(database, f"video-worker:{key}")
    confirmed = await ConfirmStoryboardHandler(database).execute(
        ConfirmStoryboardCommand(
            generated.storyboard.id, generated.storyboard.resource_version
        )
    )
    shot = confirmed.storyboard.content.shots[0]
    assets = {item.id: item for item in confirmed.assets}
    handler = GenerateMediaHandler(database, release_version="test-release")
    jobs = media_job_handler(database)
    for version_id in shot.asset_version_ids:
        asset = assets[version_id]
        if asset.asset_type == "visual_style":
            continue
        accepted = await handler.execute(
            GenerateMediaCommand(
                episode_id,
                "asset_image",
                asset.asset_id,
                asset.id,
                f"video-worker:{key}:asset:{asset.asset_id}",
            )
        )
        await run_media_job(database, accepted.task_id, jobs)
        await adopt_task_candidate(database, accepted.task_id)
    image = await handler.execute(
        GenerateMediaCommand(
            episode_id,
            "shot_image",
            shot.shot_id,
            confirmed.storyboard.id,
            f"video-worker:{key}:shot-image",
        )
    )
    await run_media_job(database, image.task_id, jobs)
    await adopt_task_candidate(database, image.task_id)
    video = await handler.execute(
        GenerateMediaCommand(
            episode_id,
            "shot_video",
            shot.shot_id,
            confirmed.storyboard.id,
            f"video-worker:{key}:video",
        )
    )
    return video.task_id, shot.duration_ticks, await media_job_context(
        database, video.task_id
    )


async def accepted_tts_task(
    database: DatabasePool, key: str
) -> tuple[UUID, str, JobContext]:
    episode_id, generated = await storyboard_draft(database, f"tts-worker:{key}")
    script = await GetScriptVersionHandler(database).execute(
        generated.storyboard.content.script_version_id
    )
    line = script.content.scenes[0].speech_lines[0]
    accepted = await GenerateMediaHandler(
        database, release_version="test-release"
    ).execute(
        GenerateMediaCommand(
            episode_id,
            "speech_audio",
            line.speech_line_id,
            script.id,
            f"tts-worker:{key}:audio",
        )
    )
    return accepted.task_id, line.text, await media_job_context(database, accepted.task_id)
