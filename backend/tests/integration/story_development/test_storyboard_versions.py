from __future__ import annotations

import asyncio
import json
from uuid import uuid4

import pytest

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.modules.story_development.application.storyboards import (
    ConfirmStoryboardCommand,
    ConfirmStoryboardHandler,
    DeriveStoryboardDraftCommand,
    DeriveStoryboardDraftHandler,
    SaveCreativeAssetCommand,
    SaveCreativeAssetHandler,
    SaveStoryboardCommand,
    SaveStoryboardHandler,
    StoryReferenceInvalid,
    VersionConflict,
    VersionImmutable,
)
from tests.integration.story_development.support import storyboard_draft


@pytest.mark.asyncio
async def test_storyboard_edit_confirm_and_derive_preserve_stable_ids(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        _, generated = await storyboard_draft(database, "board:edit:00001")
        asset = generated.assets[0]
        edited_content = asset.content.model_copy(update={"description": "人工角色设定"})
        edited_asset = await SaveCreativeAssetHandler(database).execute(
            SaveCreativeAssetCommand(asset.id, asset.resource_version, edited_content)
        )
        with pytest.raises(VersionConflict):
            await SaveCreativeAssetHandler(database).execute(
                SaveCreativeAssetCommand(asset.id, asset.resource_version, edited_content)
            )
        board = await SaveStoryboardHandler(database).execute(
            SaveStoryboardCommand(
                generated.storyboard.id,
                generated.storyboard.resource_version,
                generated.storyboard.content,
            )
        )
        confirmed = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(board.id, board.resource_version)
        )
        assert confirmed.storyboard.status == "confirmed"
        assert all(item.status == "confirmed" for item in confirmed.assets)
        assert any(item.id == edited_asset.id for item in confirmed.assets)
        with pytest.raises(VersionImmutable):
            await SaveStoryboardHandler(database).execute(
                SaveStoryboardCommand(
                    board.id, confirmed.storyboard.resource_version, board.content
                )
            )

        command = DeriveStoryboardDraftCommand(board.id, "board:derive:key01")
        first, replay = await asyncio.gather(
            DeriveStoryboardDraftHandler(database).execute(command),
            DeriveStoryboardDraftHandler(database).execute(command),
        )
        assert first.storyboard.id == replay.storyboard.id
        assert first.storyboard.parent_id == board.id
        assert {item.asset_id for item in first.assets} == {
            item.asset_id for item in confirmed.assets
        }
        assert {shot.shot_id for shot in first.storyboard.content.shots} == {
            shot.shot_id for shot in confirmed.storyboard.content.shots
        }
        assert set(first.storyboard.content.asset_version_ids).isdisjoint(
            confirmed.storyboard.content.asset_version_ids
        )
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_joint_confirmation_rolls_back_when_an_asset_reference_is_missing(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        _, generated = await storyboard_draft(database, "board:atomic:001")
        refs = [*map(str, generated.storyboard.content.asset_version_ids), str(uuid4())]
        async with database.transaction() as connection:
            await connection.execute(
                "UPDATE shot_spec_versions SET asset_version_refs_json=$2::jsonb WHERE id=$1",
                generated.storyboard.id,
                json.dumps(refs),
            )
        with pytest.raises(StoryReferenceInvalid):
            await ConfirmStoryboardHandler(database).execute(
                ConfirmStoryboardCommand(
                    generated.storyboard.id, generated.storyboard.resource_version
                )
            )
        async with database.transaction() as connection:
            statuses = await connection.fetch(
                "SELECT status FROM creative_asset_versions UNION ALL "
                "SELECT status FROM shot_spec_versions"
            )
        assert {row["status"] for row in statuses} == {"draft"}
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_concurrent_joint_confirmation_keeps_one_current_aggregate(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=6)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "board:race:00001")
        base = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(
                generated.storyboard.id, generated.storyboard.resource_version
            )
        )
        first, second = await asyncio.gather(
            DeriveStoryboardDraftHandler(database).execute(
                DeriveStoryboardDraftCommand(base.storyboard.id, "board:race:derive1")
            ),
            DeriveStoryboardDraftHandler(database).execute(
                DeriveStoryboardDraftCommand(base.storyboard.id, "board:race:derive2")
            ),
        )
        await asyncio.gather(
            ConfirmStoryboardHandler(database).execute(
                ConfirmStoryboardCommand(first.storyboard.id, 1)
            ),
            ConfirmStoryboardHandler(database).execute(
                ConfirmStoryboardCommand(second.storyboard.id, 1)
            ),
        )
        async with database.transaction() as connection:
            boards = await connection.fetchval(
                "SELECT count(*) FROM shot_spec_versions "
                "WHERE episode_id=$1 AND status='confirmed'",
                episode_id,
            )
            assets = await connection.fetch(
                "SELECT asset_id,count(*) count FROM creative_asset_versions "
                "WHERE status='confirmed' GROUP BY asset_id"
            )
        assert boards == 1
        assert len(assets) == 3 and all(row["count"] == 1 for row in assets)
    finally:
        await database.close()
