from __future__ import annotations

import asyncio
import json
from uuid import UUID

import asyncpg
import pytest

from db.pool import DatabasePool
from repositories.idempotency import IdempotencyKeyReused
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.task_submission import SubmitTaskCommand, TaskSubmitter


async def create_episode(database: DatabasePool, key: str) -> UUID:
    result = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="任务测试", idempotency_key=key)
    )
    return result.episode.id


def command(episode_id: UUID, *, key: str, prompt: str = "生成剧本") -> SubmitTaskCommand:
    return SubmitTaskCommand(
        episode_id=episode_id,
        task_type="generate_script",
        capability="text",
        scope={"episode_id": str(episode_id)},
        input_refs={"source_revision_id": "00000000-0000-0000-0000-000000000001"},
        prompt=prompt,
        parameters={"temperature": 0},
        model_profile_id="mock-text-v1",
        provider_id="mock",
        model_id="deterministic-text",
        route_version="text-route-v1",
        schema_version="script-v1",
        operation_scope=f"generateScript/{episode_id}",
        idempotency_key=key,
        handler_version="1",
    )


@pytest.mark.asyncio
async def test_twenty_replays_atomically_create_one_complete_task_bundle(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=20)
    await database.start()
    try:
        episode_id = await create_episode(database, "task:project:0001")
        submitter = TaskSubmitter(database, release_version="test-release")

        results = await asyncio.gather(
            *(submitter.submit(command(episode_id, key="task:submit:00001")) for _ in range(20))
        )

        assert {result.task_id for result in results} == {results[0].task_id}
        assert all(result.status == "queued" and result.poll_after_ms == 2000 for result in results)
        async with database.transaction() as connection:
            counts = []
            for table in (
                "submission_snapshots",
                "production_tasks",
                "production_attempts",
                "task_events",
                "task_jobs",
            ):
                counts.append(await connection.fetchval(f"SELECT count(*) FROM {table}"))
            idempotency_count = await connection.fetchval(
                "SELECT count(*) FROM idempotency_records "
                "WHERE owner_module = 'production_jobs'"
            )
            payload = await connection.fetchval("SELECT payload_json FROM task_jobs")
        assert tuple(counts) == (1, 1, 1, 1, 1)
        assert idempotency_count == 1
        if isinstance(payload, str):
            payload = json.loads(payload)
        assert payload == {
            "release_version": "test-release",
            "handler_version": "1",
            "task_id": str(results[0].task_id),
            "snapshot_id": str(results[0].snapshot_id),
        }
        assert "生成剧本" not in json.dumps(payload, ensure_ascii=False)
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_same_task_key_with_changed_prompt_is_rejected(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        episode_id = await create_episode(database, "task:project:0002")
        submitter = TaskSubmitter(database, release_version="test-release")
        await submitter.submit(command(episode_id, key="task:reuse:00001"))

        with pytest.raises(IdempotencyKeyReused):
            await submitter.submit(
                command(episode_id, key="task:reuse:00001", prompt="不同剧本")
            )
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_job_insert_failure_rolls_back_the_entire_task_bundle(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=1)
    await database.start()
    try:
        episode_id = await create_episode(database, "task:project:0003")
        async with database.transaction() as connection:
            await connection.execute(
                """
                CREATE FUNCTION reject_test_job() RETURNS trigger LANGUAGE plpgsql AS $$
                BEGIN RAISE EXCEPTION 'test job failure'; END $$;
                CREATE TRIGGER reject_test_job BEFORE INSERT ON task_jobs
                FOR EACH ROW EXECUTE FUNCTION reject_test_job();
                """
            )

        with pytest.raises(asyncpg.RaiseError):
            await TaskSubmitter(database, release_version="test-release").submit(
                command(episode_id, key="task:atomic:0001")
            )

        async with database.transaction() as connection:
            counts = []
            for table in (
                "submission_snapshots",
                "production_tasks",
                "production_attempts",
                "task_events",
                "task_jobs",
            ):
                counts.append(await connection.fetchval(f"SELECT count(*) FROM {table}"))
        assert tuple(counts) == (0, 0, 0, 0, 0)
    finally:
        await database.close()
