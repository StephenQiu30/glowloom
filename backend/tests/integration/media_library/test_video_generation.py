from __future__ import annotations

import json

import pytest

from db.pool import DatabasePool
from schemas.story_content import canonical_content_hash
from services.media_generation import (
    GenerateMediaCommand,
    GenerateMediaHandler,
    MediaInputOutdated,
)
from services.storyboards import ConfirmStoryboardCommand, ConfirmStoryboardHandler
from tests.integration.media_library.support import (
    adopt_task_candidate,
    media_job_handler,
    run_media_job,
)
from tests.integration.story_development.support import storyboard_draft


@pytest.mark.asyncio
async def test_video_submission_rejects_missing_active_image_adoptions(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "media:video:missing")
        confirmed = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(
                generated.storyboard.id, generated.storyboard.resource_version
            )
        )
        shot = confirmed.storyboard.content.shots[0]

        with pytest.raises(MediaInputOutdated, match="active image adoption"):
            await GenerateMediaHandler(database, release_version="test-release").execute(
                GenerateMediaCommand(
                    episode_id=episode_id,
                    usage_type="shot_video",
                    usage_id=shot.shot_id,
                    input_version_id=confirmed.storyboard.id,
                    idempotency_key="media:video:missing:001",
                )
            )

        async with database.transaction() as connection:
            assert await connection.fetchval(
                "SELECT count(*) FROM production_tasks WHERE episode_id=$1", episode_id
            ) == 2
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_video_submission_freezes_current_image_adoptions_and_target_duration(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "media:video:ready")
        confirmed = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(
                generated.storyboard.id, generated.storyboard.resource_version
            )
        )
        shot = confirmed.storyboard.content.shots[0]
        assets = {item.id: item for item in confirmed.assets}
        related = tuple(
            sorted(
                (assets[version_id] for version_id in shot.asset_version_ids),
                key=lambda item: item.asset_id,
            )
        )
        handler = GenerateMediaHandler(database, release_version="test-release")
        job_handler = media_job_handler(database)
        adoptions: list[dict[str, object]] = []
        for asset in related:
            if asset.asset_type == "visual_style":
                continue
            accepted = await handler.execute(
                GenerateMediaCommand(
                    episode_id=episode_id,
                    usage_type="asset_image",
                    usage_id=asset.asset_id,
                    input_version_id=asset.id,
                    idempotency_key=f"media:video:asset:{asset.asset_id}",
                )
            )
            await run_media_job(database, accepted.task_id, job_handler)
            adoptions.append(await adopt_task_candidate(database, accepted.task_id))
        image = await handler.execute(
            GenerateMediaCommand(
                episode_id=episode_id,
                usage_type="shot_image",
                usage_id=shot.shot_id,
                input_version_id=confirmed.storyboard.id,
                idempotency_key="media:video:shot-image:001",
            )
        )
        await run_media_job(database, image.task_id, job_handler)
        adoptions.append(await adopt_task_candidate(database, image.task_id))

        video = await handler.execute(
            GenerateMediaCommand(
                episode_id=episode_id,
                usage_type="shot_video",
                usage_id=shot.shot_id,
                input_version_id=confirmed.storyboard.id,
                idempotency_key="media:video:ready:001",
            )
        )

        async with database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT snapshot.capability,snapshot.input_refs_json,
                       snapshot.model_profile_id,snapshot.provider_id,
                       snapshot.model_id,snapshot.schema_version
                FROM production_tasks task
                JOIN submission_snapshots snapshot ON snapshot.id=task.snapshot_id
                WHERE task.id=$1
                """,
                video.task_id,
            )
        expected = {
            "usage_type": "shot_video",
            "shot_id": str(shot.shot_id),
            "input_version_id": str(confirmed.storyboard.id),
            "shot_content_hash": shot.content_hash,
            "duration_ticks": shot.duration_ticks,
            "asset_versions": [
                {
                    "asset_id": str(item.asset_id),
                    "version_id": str(item.id),
                    "content_hash": item.content_hash,
                }
                for item in related
            ],
            "image_adoptions": sorted(
                adoptions, key=lambda item: (str(item["usage_type"]), str(item["usage_id"]))
            ),
        }
        assert json.loads(row["input_refs_json"]) == {
            **expected,
            "input_hash": canonical_content_hash(expected),
        }
        assert (
            row["capability"],
            row["model_profile_id"],
            row["provider_id"],
            row["model_id"],
            row["schema_version"],
        ) == ("video", "mock-video-v1", "mock", "deterministic-video", "video-v1")
    finally:
        await database.close()
