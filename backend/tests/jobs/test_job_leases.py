from __future__ import annotations

import asyncio
from datetime import UTC, datetime, timedelta
from uuid import UUID

import pytest

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.jobs.lease_queue import JobQueue
from lanverse.modules.production_jobs.public import SubmitTaskCommand, TaskSubmitter
from lanverse.modules.project_catalog.application.create_project import (
    CreateProjectCommand,
    CreateProjectHandler,
)

NOW = datetime(2030, 7, 25, 12, 0, tzinfo=UTC)


async def create_job(database: DatabasePool) -> UUID:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="租约测试", idempotency_key="lease:project:001")
    )
    episode_id = project.episode.id
    task = await TaskSubmitter(database, release_version="test-release").submit(
        SubmitTaskCommand(
            episode_id=episode_id,
            task_type="generate_script",
            capability="text",
            scope={"episode_id": str(episode_id)},
            input_refs={},
            prompt="生成剧本",
            parameters={"temperature": 0},
            model_profile_id="mock-text-v1",
            provider_id="mock",
            model_id="deterministic-text",
            route_version="text-route-v1",
            schema_version="script-v1",
            operation_scope=f"generateScript/{episode_id}",
            idempotency_key="lease:submit:0001",
            handler_version="1",
        )
    )
    return task.task_id


@pytest.mark.asyncio
async def test_two_workers_compete_with_skip_locked_for_one_job(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id = await create_job(database)
        queue = JobQueue(database, lease_duration=timedelta(seconds=10))

        claims = await asyncio.gather(
            queue.claim("worker-a", now=NOW),
            queue.claim("worker-b", now=NOW),
        )

        acquired = [claim for claim in claims if claim is not None]
        assert len(acquired) == 1
        assert acquired[0].task_id == task_id
        assert acquired[0].attempts == 1
        assert acquired[0].lease_until == NOW + timedelta(seconds=10)
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_heartbeat_requires_live_current_owner_and_extends_from_now(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        await create_job(database)
        queue = JobQueue(database, lease_duration=timedelta(seconds=10))
        claim = await queue.claim("worker-a", now=NOW)
        assert claim is not None

        assert await queue.heartbeat(claim.id, "worker-b", now=NOW) is False
        assert await queue.heartbeat(
            claim.id, "worker-a", now=NOW + timedelta(seconds=5)
        )
        assert await queue.heartbeat(
            claim.id, "worker-a", now=NOW + timedelta(seconds=16)
        ) is False
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_expired_lease_is_recovered_and_old_owner_cannot_complete(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        await create_job(database)
        queue = JobQueue(database, lease_duration=timedelta(seconds=10))
        first = await queue.claim("worker-a", now=NOW)
        assert first is not None

        recovered = await queue.claim("worker-b", now=NOW + timedelta(seconds=11))

        assert recovered is not None
        assert recovered.id == first.id
        assert recovered.attempts == 2
        assert await queue.complete(first.id, "worker-a", now=NOW + timedelta(seconds=12)) is False
        assert await queue.complete(
            recovered.id, "worker-b", now=NOW + timedelta(seconds=12)
        )
    finally:
        await database.close()
