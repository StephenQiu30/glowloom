from __future__ import annotations

import pytest

from db.pool import DatabasePool
from services.project_reader import ProjectCatalogReader
from services.script_versions import GetScriptVersionHandler
from services.sources import (
    ConfirmSourceCommand,
    ConfirmSourceHandler,
    CreateSourceRevisionCommand,
    CreateSourceRevisionHandler,
)
from services.storyboards import (
    GetCreativeAssetVersionHandler,
    GetStoryboardVersionHandler,
)
from services.tasks import TaskQueryService
from tests.integration.story_development.support import storyboard_draft


@pytest.mark.asyncio
async def test_new_source_only_projects_old_story_and_tasks_as_outdated(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "story:outdated:1")
        script_id = generated.storyboard.content.script_version_id
        asset_ids = tuple(item.id for item in generated.assets)
        board_id = generated.storyboard.id
        before_tasks = await TaskQueryService(database).list(episode_id)
        before_script = await GetScriptVersionHandler(database).execute(script_id)
        before_board = await GetStoryboardVersionHandler(database).execute(board_id)
        before_assets = tuple(
            [
                await GetCreativeAssetVersionHandler(database).execute(item)
                for item in asset_ids
            ]
        )
        current_source = await ProjectCatalogReader(database).confirmed_source(episode_id)
        replacement = await CreateSourceRevisionHandler(database).execute(
            CreateSourceRevisionCommand(
                episode_id=episode_id,
                content="汉字新来源" + "e" * 295,
                rights_basis="original",
                parent_id=current_source.id,
                idempotency_key="story:source:new01",
            )
        )
        await ConfirmSourceHandler(database).execute(
            ConfirmSourceCommand(replacement.id, replacement.resource_version)
        )

        script = await GetScriptVersionHandler(database).execute(script_id)
        board = await GetStoryboardVersionHandler(database).execute(board_id)
        assets = tuple(
            [
                await GetCreativeAssetVersionHandler(database).execute(item)
                for item in asset_ids
            ]
        )
        tasks = await TaskQueryService(database).list(episode_id)

        assert script.input_outdated is True
        assert board.input_outdated is True
        assert all(item.input_outdated for item in assets)
        assert all(item.input_outdated for item in tasks)
        assert [(item.id, item.status, item.resource_version) for item in tasks] == [
            (item.id, item.status, item.resource_version) for item in before_tasks
        ]
        assert script.content_hash == before_script.content_hash
        assert board.content_hash == before_board.content_hash
        assert [item.content_hash for item in assets] == [
            item.content_hash for item in before_assets
        ]
    finally:
        await database.close()
