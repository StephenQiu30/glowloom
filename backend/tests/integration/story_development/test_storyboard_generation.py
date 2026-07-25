from __future__ import annotations

from uuid import UUID

import pytest
from pydantic import ValidationError

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
from lanverse.modules.story_development.application.contracts.snapshots import (
    ScriptVersionSnapshot,
)
from lanverse.modules.story_development.application.generate import (
    GenerateScriptCommand,
    GenerateScriptHandler,
    GenerateStoryboardCommand,
    GenerateStoryboardHandler,
)
from lanverse.modules.story_development.application.results import (
    ScriptResultRegistrar,
    StoryboardResultRegistrar,
)
from lanverse.modules.story_development.application.scripts import (
    ConfirmScriptCommand,
    ConfirmScriptHandler,
)
from lanverse.modules.story_development.infrastructure.text_provider import (
    DeterministicTextProvider,
)


async def confirmed_script(
    database: DatabasePool,
) -> tuple[UUID, ScriptVersionSnapshot]:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="分镜生成", idempotency_key="board:project:0001")
    )
    source = await CreateSourceRevisionHandler(database).execute(
        CreateSourceRevisionCommand(
            episode_id=project.episode.id,
            content="汉字分镜" + "c" * 296,
            rights_basis="original",
            parent_id=None,
            idempotency_key="board:source:00001",
        )
    )
    await ConfirmSourceHandler(database).execute(
        ConfirmSourceCommand(source.id, source.resource_version)
    )
    task = await GenerateScriptHandler(database, release_version="test-release").execute(
        GenerateScriptCommand(project.episode.id, "board:script:00001")
    )
    provider = DeterministicTextProvider()
    draft = await ScriptResultRegistrar(database).register(
        task.task_id, await provider.generate_script(source.id, "分镜剧本")
    )
    script = await ConfirmScriptHandler(database).execute(
        ConfirmScriptCommand(draft.id, draft.resource_version)
    )
    return project.episode.id, script


@pytest.mark.asyncio
async def test_storyboard_generation_has_stable_assets_shots_and_output_slots(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        episode_id, script = await confirmed_script(database)
        accepted = await GenerateStoryboardHandler(
            database, release_version="test-release"
        ).execute(GenerateStoryboardCommand(episode_id, "board:generate:001"))
        provider = DeterministicTextProvider()
        first_output = await provider.generate_storyboard(script.content)
        assert first_output == await provider.generate_storyboard(script.content)

        registrar = StoryboardResultRegistrar(database)
        first = await registrar.register(accepted.task_id, first_output)
        replay = await registrar.register(accepted.task_id, first_output)

        assert first.storyboard.id == replay.storyboard.id
        assert len(first.assets) == 3
        assert {item.asset_type for item in first.assets} == {
            "character",
            "scene",
            "visual_style",
        }
        assert len(first.storyboard.content.shots) == 6
        assert first.storyboard.content.total_duration_ticks == 2700000
        assert first.storyboard.content.script_version_id == script.id
        assert set(first.storyboard.content.asset_version_ids) == {
            item.id for item in first.assets
        }
        async with database.transaction() as connection:
            counts = await connection.fetchrow(
                "SELECT (SELECT count(*) FROM creative_asset_versions) assets, "
                "(SELECT count(*) FROM shot_spec_versions) boards, "
                "(SELECT count(*) FROM task_outputs WHERE task_id=$1) outputs",
                accepted.task_id,
            )
        assert counts is not None and tuple(counts.values()) == (3, 1, 4)
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_invalid_storyboard_output_rolls_back_every_generated_version(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        episode_id, _ = await confirmed_script(database)
        accepted = await GenerateStoryboardHandler(
            database, release_version="test-release"
        ).execute(GenerateStoryboardCommand(episode_id, "board:generate:002"))
        with pytest.raises(ValidationError):
            await StoryboardResultRegistrar(database).register(
                accepted.task_id,
                '{"schema_version":"storyboard-generation-v1","assets":[],"shots":[]}',
            )
        async with database.transaction() as connection:
            assert await connection.fetchval("SELECT count(*) FROM creative_asset_versions") == 0
            assert await connection.fetchval("SELECT count(*) FROM shot_spec_versions") == 0
    finally:
        await database.close()
