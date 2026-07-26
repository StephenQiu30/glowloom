from __future__ import annotations

import json
from uuid import UUID, uuid4

from db.pool import DatabasePool
from integrations.ai.deterministic_video import DockerFfmpegRuntime
from integrations.ai.registry import AiModelRegistry, create_mvp_registry
from integrations.object_storage import MinioObjectStore, RemoteObject
from schemas.jobs import JobPayload
from services.media_registration import MediaRegistrationService
from services.media_validation import MediaValidationService
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
    async with database.transaction() as connection:
        row = await connection.fetchrow(
            "SELECT id,payload_json FROM task_jobs WHERE task_id=$1", task_id
        )
    assert row is not None
    context = JobContext(
        job_id=row["id"],
        owner="media-test",
        payload=JobPayload.parse(json.loads(row["payload_json"])),
    )
    await handler.handle(context)


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
