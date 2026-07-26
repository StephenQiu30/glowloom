from __future__ import annotations

import asyncio
from uuid import UUID

import pytest

from db.pool import DatabasePool
from integrations.ai.deterministic_text import (
    DeterministicTextProvider,
)
from schemas.story_snapshots import (
    ScriptVersionSnapshot,
)
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.script_versions import (
    ConfirmScriptCommand,
    ConfirmScriptHandler,
    DeriveScriptDraftCommand,
    DeriveScriptDraftHandler,
    GetCurrentScriptHandler,
    ListScriptVersionsHandler,
    SaveScriptCommand,
    SaveScriptHandler,
    VersionConflict,
    VersionImmutable,
)
from services.sources import (
    ConfirmSourceCommand,
    ConfirmSourceHandler,
    CreateSourceRevisionCommand,
    CreateSourceRevisionHandler,
)
from services.story_generation import (
    GenerateScriptCommand,
    GenerateScriptHandler,
)
from services.story_results import ScriptResultRegistrar


async def script_draft(
    database: DatabasePool, key: str
) -> tuple[UUID, ScriptVersionSnapshot]:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="剧本版本", idempotency_key=f"project:{key}")
    )
    source = await CreateSourceRevisionHandler(database).execute(
        CreateSourceRevisionCommand(
            episode_id=project.episode.id,
            content="汉字剧本" + "b" * 296,
            rights_basis="original",
            parent_id=None,
            idempotency_key=f"source:{key}",
        )
    )
    await ConfirmSourceHandler(database).execute(
        ConfirmSourceCommand(source.id, source.resource_version)
    )
    task = await GenerateScriptHandler(database, release_version="test-release").execute(
        GenerateScriptCommand(project.episode.id, f"generate:{key}")
    )
    output = await DeterministicTextProvider().generate_script(source.id, "初稿")
    draft = await ScriptResultRegistrar(database).register(task.task_id, output)
    return project.episode.id, draft


@pytest.mark.asyncio
async def test_script_save_requires_fresh_etag_and_confirmed_is_immutable(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, draft = await script_draft(database, "script:version:01")
        edited_content = draft.content.model_copy(update={"title": "人工修订"})
        edited = await SaveScriptHandler(database).execute(
            SaveScriptCommand(draft.id, draft.resource_version, edited_content)
        )
        assert edited.resource_version == 2
        assert edited.content.title == "人工修订"

        with pytest.raises(VersionConflict):
            await SaveScriptHandler(database).execute(
                SaveScriptCommand(draft.id, draft.resource_version, edited_content)
            )
        confirmed = await ConfirmScriptHandler(database).execute(
            ConfirmScriptCommand(edited.id, edited.resource_version)
        )
        assert confirmed.status == "confirmed"
        assert (await GetCurrentScriptHandler(database).execute(episode_id)).id == confirmed.id
        with pytest.raises(VersionImmutable):
            await SaveScriptHandler(database).execute(
                SaveScriptCommand(confirmed.id, confirmed.resource_version, edited_content)
            )
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_script_draft_derivation_is_atomic_and_idempotent(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        episode_id, draft = await script_draft(database, "script:derive:001")
        confirmed = await ConfirmScriptHandler(database).execute(
            ConfirmScriptCommand(draft.id, draft.resource_version)
        )
        handler = DeriveScriptDraftHandler(database)
        command = DeriveScriptDraftCommand(confirmed.id, "script:derive:key01")
        first, replay = await asyncio.gather(handler.execute(command), handler.execute(command))

        assert first.id == replay.id
        assert first.parent_id == confirmed.id
        assert first.origin_task_id is None
        assert first.status == "draft"
        assert first.content_hash == confirmed.content_hash
        versions = await ListScriptVersionsHandler(database).execute(episode_id)
        assert [item.version for item in versions] == [2, 1]
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_concurrent_script_confirmation_keeps_one_current_version(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        episode_id, first = await script_draft(database, "script:race:0001")
        task = await GenerateScriptHandler(database, release_version="test-release").execute(
            GenerateScriptCommand(episode_id, "script:race:generate2")
        )
        output = await DeterministicTextProvider().generate_script(
            first.source_revision_id, "竞争稿"
        )
        second = await ScriptResultRegistrar(database).register(task.task_id, output)

        await asyncio.gather(
            ConfirmScriptHandler(database).execute(
                ConfirmScriptCommand(first.id, first.resource_version)
            ),
            ConfirmScriptHandler(database).execute(
                ConfirmScriptCommand(second.id, second.resource_version)
            ),
        )
        async with database.transaction() as connection:
            rows = await connection.fetch(
                "SELECT status,count(*) count FROM script_versions "
                "WHERE episode_id=$1 GROUP BY status",
                episode_id,
            )
        assert {row["status"]: row["count"] for row in rows} == {
            "confirmed": 1,
            "superseded": 1,
        }
    finally:
        await database.close()
