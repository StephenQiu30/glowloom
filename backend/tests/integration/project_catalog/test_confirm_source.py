from __future__ import annotations

import asyncio
from uuid import UUID

import pytest

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.modules.project_catalog.application.contracts import SourceRevisionSnapshot
from lanverse.modules.project_catalog.application.create_project import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from lanverse.modules.project_catalog.application.sources import (
    ConfirmSourceCommand,
    ConfirmSourceHandler,
    CreateSourceRevisionCommand,
    CreateSourceRevisionHandler,
    GetSourceRevisionHandler,
    VersionConflict,
)


def source_text(label: str) -> str:
    return "汉字来源" + label + "a" * 295


async def create_source(
    database: DatabasePool, label: str, key: str
) -> tuple[UUID, SourceRevisionSnapshot]:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="确认测试", idempotency_key="project:confirm:1")
    )
    source = await CreateSourceRevisionHandler(database).execute(
        CreateSourceRevisionCommand(
            episode_id=project.episode.id,
            content=source_text(label),
            rights_basis="original",
            parent_id=None,
            idempotency_key=key,
        )
    )
    return project.episode.id, source


@pytest.mark.asyncio
async def test_confirming_a_new_source_supersedes_current_without_mutating_content(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, first = await create_source(database, "甲", "source:confirm:01")
        confirm = ConfirmSourceHandler(database)
        confirmed_first = await confirm.execute(
            ConfirmSourceCommand(version_id=first.id, expected_resource_version=1)
        )
        second = await CreateSourceRevisionHandler(database).execute(
            CreateSourceRevisionCommand(
                episode_id=episode_id,
                content=source_text("乙"),
                rights_basis="licensed",
                parent_id=first.id,
                idempotency_key="source:confirm:02",
            )
        )

        confirmed_second = await confirm.execute(
            ConfirmSourceCommand(version_id=second.id, expected_resource_version=1)
        )
        restored_first = await GetSourceRevisionHandler(database).execute(first.id)

        assert confirmed_first.status == "confirmed"
        assert confirmed_second.status == "confirmed"
        assert confirmed_second.resource_version == 2
        assert restored_first.status == "superseded"
        assert restored_first.resource_version == 3
        assert restored_first.content == first.content
        assert restored_first.sha256 == first.sha256
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_concurrent_confirmation_keeps_exactly_one_current_source(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        episode_id, first = await create_source(database, "甲", "source:race:0001")
        second = await CreateSourceRevisionHandler(database).execute(
            CreateSourceRevisionCommand(
                episode_id=episode_id,
                content=source_text("乙"),
                rights_basis="original",
                parent_id=first.id,
                idempotency_key="source:race:0002",
            )
        )
        confirm = ConfirmSourceHandler(database)

        await asyncio.gather(
            confirm.execute(ConfirmSourceCommand(first.id, 1)),
            confirm.execute(ConfirmSourceCommand(second.id, 1)),
        )

        async with database.transaction() as connection:
            statuses = await connection.fetch(
                "SELECT status, count(*) count FROM source_revisions "
                "WHERE episode_id = $1 GROUP BY status",
                episode_id,
            )
        assert {row["status"]: row["count"] for row in statuses} == {
            "confirmed": 1,
            "superseded": 1,
        }
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_same_source_confirmation_rejects_stale_resource_version(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        _, source = await create_source(database, "甲", "source:etag:0001")
        confirm = ConfirmSourceHandler(database)
        outcomes = await asyncio.gather(
            confirm.execute(ConfirmSourceCommand(source.id, 1)),
            confirm.execute(ConfirmSourceCommand(source.id, 1)),
            return_exceptions=True,
        )

        assert sum(not isinstance(item, Exception) for item in outcomes) == 1
        assert sum(isinstance(item, VersionConflict) for item in outcomes) == 1
    finally:
        await database.close()
