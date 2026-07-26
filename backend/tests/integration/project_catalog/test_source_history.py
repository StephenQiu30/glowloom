from __future__ import annotations

import asyncio
from hashlib import sha256
from uuid import UUID

import asyncpg
import pytest

from db.pool import DatabasePool
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.sources import (
    CreateSourceRevisionCommand,
    CreateSourceRevisionHandler,
    GetSourceRevisionHandler,
    InvalidRightsBasis,
    ListSourceRevisionsHandler,
)


def valid_source(label: str = "甲") -> str:
    return f"汉字故事{label}" + "a" * 295


async def create_episode(database: DatabasePool) -> UUID:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="来源测试", idempotency_key="project:source:001")
    )
    return project.episode.id


@pytest.mark.asyncio
@pytest.mark.parametrize("rights_basis", [None, "", "unknown"])
async def test_source_requires_an_explicit_supported_rights_basis(
    migrated_database_url: str, rights_basis: str | None
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        episode_id = await create_episode(database)

        with pytest.raises(InvalidRightsBasis):
            await CreateSourceRevisionHandler(database).execute(
                CreateSourceRevisionCommand(
                    episode_id=episode_id,
                    content=valid_source(),
                    rights_basis=rights_basis,
                    parent_id=None,
                    idempotency_key="source:rights:001",
                )
            )

        async with database.transaction() as connection:
            assert await connection.fetchval("SELECT count(*) FROM source_revisions") == 0
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_source_versions_preserve_parent_content_hash_and_newest_first_history(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        episode_id = await create_episode(database)
        create = CreateSourceRevisionHandler(database)
        first = await create.execute(
            CreateSourceRevisionCommand(
                episode_id=episode_id,
                content="\u3000" + valid_source("一") + "\r\n",
                rights_basis="original",
                parent_id=None,
                idempotency_key="source:history:01",
            )
        )
        second = await create.execute(
            CreateSourceRevisionCommand(
                episode_id=episode_id,
                content=valid_source("二"),
                rights_basis="licensed",
                parent_id=first.id,
                idempotency_key="source:history:02",
            )
        )

        history = await ListSourceRevisionsHandler(database).execute(episode_id)
        restored = await GetSourceRevisionHandler(database).execute(first.id)

        assert [item.version for item in history] == [2, 1]
        assert second.parent_id == first.id
        assert first.content.endswith("\n") is False
        assert first.sha256 == sha256(first.content.encode()).hexdigest()
        assert restored == first
        assert first.status == second.status == "draft"
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_twenty_source_replays_create_one_immutable_version(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=20)
    await database.start()
    try:
        episode_id = await create_episode(database)
        handler = CreateSourceRevisionHandler(database)
        command = CreateSourceRevisionCommand(
            episode_id=episode_id,
            content=valid_source(),
            rights_basis="original",
            parent_id=None,
            idempotency_key="source:replay:001",
        )

        results = await asyncio.gather(*(handler.execute(command) for _ in range(20)))

        assert {item.id for item in results} == {results[0].id}
        connection = await asyncpg.connect(migrated_database_url)
        try:
            assert await connection.fetchval("SELECT count(*) FROM source_revisions") == 1
        finally:
            await connection.close()
    finally:
        await database.close()
