from __future__ import annotations

import json

import pytest

from db.pool import DatabasePool
from schemas.story_content import canonical_content_hash
from services.media_generation import (
    GenerateMediaCommand,
    GenerateMediaHandler,
    UnsupportedMediaUsage,
)
from services.storyboards import ConfirmStoryboardCommand, ConfirmStoryboardHandler
from tests.integration.story_development.support import storyboard_draft


@pytest.mark.asyncio
async def test_asset_image_submission_freezes_exact_confirmed_asset_input(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "media:asset:image")
        confirmed = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(
                generated.storyboard.id, generated.storyboard.resource_version
            )
        )
        asset = next(item for item in confirmed.assets if item.asset_type == "character")
        command = GenerateMediaCommand(
            episode_id=episode_id,
            usage_type="asset_image",
            usage_id=asset.asset_id,
            input_version_id=asset.id,
            idempotency_key="media:asset:image:001",
        )
        handler = GenerateMediaHandler(database, release_version="test-release")

        first = await handler.execute(command)
        replay = await handler.execute(command)

        assert first == replay
        async with database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT snapshot.input_refs_json,snapshot.model_profile_id,
                       snapshot.provider_id,snapshot.model_id,snapshot.schema_version,
                       task.scope_json
                FROM production_tasks task
                JOIN submission_snapshots snapshot ON snapshot.id=task.snapshot_id
                WHERE task.id=$1
                """,
                first.task_id,
            )
        input_refs = json.loads(row["input_refs_json"])
        expected_input = {
            "usage_type": "asset_image",
            "asset_id": str(asset.asset_id),
            "input_version_id": str(asset.id),
            "content_hash": asset.content_hash,
        }
        assert input_refs == {
            **expected_input,
            "input_hash": canonical_content_hash(expected_input),
        }
        assert json.loads(row["scope_json"]) == {
            "episode_id": str(episode_id),
            "usage_type": "asset_image",
            "usage_id": str(asset.asset_id),
            "input_version_id": str(asset.id),
            "input_hash": canonical_content_hash(expected_input),
        }
        assert (
            row["model_profile_id"],
            row["provider_id"],
            row["model_id"],
            row["schema_version"],
        ) == ("mock-image-v1", "mock", "deterministic-image", "image-v1")
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_shot_image_hash_includes_related_assets_and_style_but_style_has_no_slot(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "media:shot:image")
        confirmed = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(
                generated.storyboard.id, generated.storyboard.resource_version
            )
        )
        shot = confirmed.storyboard.content.shots[0]
        assets_by_version = {item.id: item for item in confirmed.assets}
        related = tuple(
            sorted(
                (assets_by_version[item] for item in shot.asset_version_ids),
                key=lambda item: item.asset_id,
            )
        )
        handler = GenerateMediaHandler(database, release_version="test-release")

        accepted = await handler.execute(
            GenerateMediaCommand(
                episode_id=episode_id,
                usage_type="shot_image",
                usage_id=shot.shot_id,
                input_version_id=confirmed.storyboard.id,
                idempotency_key="media:shot:image:001",
            )
        )

        async with database.transaction() as connection:
            raw = await connection.fetchval(
                """
                SELECT snapshot.input_refs_json FROM production_tasks task
                JOIN submission_snapshots snapshot ON snapshot.id=task.snapshot_id
                WHERE task.id=$1
                """,
                accepted.task_id,
            )
        input_refs = json.loads(raw)
        expected_input = {
            "usage_type": "shot_image",
            "shot_id": str(shot.shot_id),
            "input_version_id": str(confirmed.storyboard.id),
            "shot_content_hash": shot.content_hash,
            "asset_versions": [
                {
                    "asset_id": str(item.asset_id),
                    "version_id": str(item.id),
                    "content_hash": item.content_hash,
                }
                for item in related
            ],
        }
        assert input_refs == {
            **expected_input,
            "input_hash": canonical_content_hash(expected_input),
        }
        assert any(item.asset_type == "visual_style" for item in related)

        style = next(item for item in confirmed.assets if item.asset_type == "visual_style")
        with pytest.raises(UnsupportedMediaUsage, match="visual style"):
            await handler.execute(
                GenerateMediaCommand(
                    episode_id=episode_id,
                    usage_type="asset_image",
                    usage_id=style.asset_id,
                    input_version_id=style.id,
                    idempotency_key="media:style:image:001",
                )
            )
        async with database.transaction() as connection:
            assert await connection.fetchval(
                "SELECT count(*) FROM production_tasks WHERE episode_id=$1", episode_id
            ) == 3
    finally:
        await database.close()
