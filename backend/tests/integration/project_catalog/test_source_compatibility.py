from __future__ import annotations

from uuid import uuid4

import pytest

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.modules.project_catalog.application.create_project import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from lanverse.modules.project_catalog.application.sources import (
    ConfirmSourceCommand,
    ConfirmSourceHandler,
    CreateSourceRevisionCommand,
    CreateSourceRevisionHandler,
)
from lanverse.modules.project_catalog.public import ProjectCatalogReader


def source_text(label: str) -> str:
    return "汉字投影" + label + "a" * 295


@pytest.mark.asyncio
async def test_new_current_source_only_marks_old_downstream_input_outdated(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        project = await CreateProjectHandler(database).execute(
            CreateProjectCommand(title="兼容投影", idempotency_key="compat:project:01")
        )
        create = CreateSourceRevisionHandler(database)
        first = await create.execute(
            CreateSourceRevisionCommand(
                episode_id=project.episode.id,
                content=source_text("甲"),
                rights_basis="original",
                parent_id=None,
                idempotency_key="compat:source:001",
            )
        )
        await ConfirmSourceHandler(database).execute(ConfirmSourceCommand(first.id, 1))
        script_id = uuid4()
        async with database.transaction() as connection:
            await connection.execute(
                """
                INSERT INTO script_versions(
                    id, episode_id, version, source_revision_id, schema_version,
                    content_json, content_hash
                ) VALUES($1, $2, 1, $3, 'script-v1', '{"title":"locked"}', $4)
                """,
                script_id,
                project.episode.id,
                first.id,
                "a" * 64,
            )
            before = dict(
                await connection.fetchrow(
                    "SELECT source_revision_id, content_json, content_hash, status, "
                    "resource_version FROM script_versions WHERE id = $1",
                    script_id,
                )
            )
        second = await create.execute(
            CreateSourceRevisionCommand(
                episode_id=project.episode.id,
                content=source_text("乙"),
                rights_basis="original",
                parent_id=first.id,
                idempotency_key="compat:source:002",
            )
        )
        await ConfirmSourceHandler(database).execute(ConfirmSourceCommand(second.id, 1))

        compatibility = await ProjectCatalogReader(database).source_compatibility(
            project.episode.id, first.id
        )
        async with database.transaction() as connection:
            after = dict(
                await connection.fetchrow(
                    "SELECT source_revision_id, content_json, content_hash, status, "
                    "resource_version FROM script_versions WHERE id = $1",
                    script_id,
                )
            )

        assert compatibility.input_source_revision_id == first.id
        assert compatibility.current_source_revision_id == second.id
        assert compatibility.input_outdated is True
        assert after == before
    finally:
        await database.close()
