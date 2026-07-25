from __future__ import annotations

import asyncio

import asyncpg
import pytest

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.modules.project_catalog.application.create_project import (
    CreateProjectCommand,
    CreateProjectHandler,
    IdempotencyKeyReused,
)


async def table_counts(database_url: str) -> tuple[int, int, int]:
    connection = await asyncpg.connect(database_url)
    try:
        return (
            await connection.fetchval("SELECT count(*) FROM projects"),
            await connection.fetchval("SELECT count(*) FROM episodes"),
            await connection.fetchval("SELECT count(*) FROM idempotency_records"),
        )
    finally:
        await connection.close()


@pytest.mark.asyncio
async def test_twenty_concurrent_replays_create_one_project_and_episode(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=20)
    await database.start()
    try:
        handler = CreateProjectHandler(database)
        command = CreateProjectCommand(title="  我的短剧  ", idempotency_key="project:same:001")

        results = await asyncio.gather(*(handler.execute(command) for _ in range(20)))

        assert {result.project.id for result in results} == {results[0].project.id}
        assert {result.episode.id for result in results} == {results[0].episode.id}
        assert all(result.project.title == "我的短剧" for result in results)
        assert results[0].project.production_spec.aspect_ratio == "9:16"
        assert results[0].episode.target_min_ticks == 2700000
        assert await table_counts(migrated_database_url) == (1, 1, 1)
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_same_key_with_a_different_request_is_rejected(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        handler = CreateProjectHandler(database)
        await handler.execute(
            CreateProjectCommand(title="项目甲", idempotency_key="project:reuse:001")
        )

        with pytest.raises(IdempotencyKeyReused):
            await handler.execute(
                CreateProjectCommand(title="项目乙", idempotency_key="project:reuse:001")
            )

        assert await table_counts(migrated_database_url) == (1, 1, 1)
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_episode_failure_rolls_back_project_and_idempotency(
    migrated_database_url: str,
) -> None:
    connection = await asyncpg.connect(migrated_database_url)
    await connection.execute(
        """
        CREATE FUNCTION reject_test_episode() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN RAISE EXCEPTION 'test episode failure'; END $$;
        CREATE TRIGGER reject_test_episode BEFORE INSERT ON episodes
        FOR EACH ROW EXECUTE FUNCTION reject_test_episode();
        """
    )
    await connection.close()
    database = DatabasePool(migrated_database_url, min_size=1, max_size=1)
    await database.start()
    try:
        with pytest.raises(asyncpg.RaiseError):
            await CreateProjectHandler(database).execute(
                CreateProjectCommand(title="事务项目", idempotency_key="project:atomic:01")
            )

        assert await table_counts(migrated_database_url) == (0, 0, 0)
    finally:
        await database.close()
