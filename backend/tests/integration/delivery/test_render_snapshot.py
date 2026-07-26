from __future__ import annotations

from uuid import UUID

import pytest
from pydantic import ValidationError

from db.pool import DatabasePool
from integrations.ai.deterministic_video import FFMPEG_IMAGE
from schemas.rendering import RenderRecipeV1
from services.render_snapshots import (
    CreateRenderSnapshotCommand,
    CreateRenderSnapshotHandler,
    RenderInputInvalid,
)
from tests.integration.delivery.support import render_ready_story


def render_recipe() -> RenderRecipeV1:
    return RenderRecipeV1(
        runtime_image=FFMPEG_IMAGE,
        ffmpeg_version="mock-ffmpeg-7.1",
        ffprobe_version="mock-ffprobe-7.1",
        font_name="Noto Sans CJK SC",
        font_file="/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
        font_sha256="a" * 64,
        font_license="OFL-1.1",
    )


@pytest.mark.asyncio
async def test_render_snapshot_freezes_current_media_and_recipe(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=8)
    await database.start()
    try:
        episode_id, board, subtitle, _ = await render_ready_story(database, "snapshot")
        command = CreateRenderSnapshotCommand(
            episode_id, "renderEpisode/test", "render:snapshot:0001"
        )
        handler = CreateRenderSnapshotHandler(database, recipe=render_recipe())

        snapshot = await handler.execute(command)
        replay = await handler.execute(command)

        assert replay == snapshot
        assert snapshot.shot_spec_version_id == board.id
        assert snapshot.subtitle_version_id == subtitle.id
        assert len(snapshot.segments) == 6
        assert [item.ordinal for item in snapshot.segments] == list(range(1, 7))
        assert snapshot.segments[-1].end_ticks == 2_700_000
        assert len(snapshot.input_refs.video_adoptions) == 6
        assert len(snapshot.input_refs.tts_adoptions) == 6
        assert snapshot.recipe.width == 720 and snapshot.recipe.height == 1280
        assert snapshot.recipe.fps == 24 and snapshot.recipe.remove_source_audio is True
        assert len(snapshot.recipe_hash) == len(snapshot.content_hash) == 64
        assert await stored_count(database, episode_id, "render_snapshots") == 1
        assert await stored_count(database, episode_id, "production_tasks") == 0

        tts_ref = subtitle.input_refs.tts_adoptions[0]
        await replace_subtitle_tts_hash(database, subtitle.id, "b" * 64)
        with pytest.raises(RenderInputInvalid, match=str(tts_ref.speech_line_id)):
            await handler.execute(
                CreateRenderSnapshotCommand(
                    episode_id, "renderEpisode/test", "render:snapshot:0002"
                )
            )
        assert await stored_count(database, episode_id, "render_snapshots") == 1
        await replace_subtitle_tts_hash(database, subtitle.id, tts_ref.input_hash)

        missing_shot_id = board.content.shots[0].shot_id
        await supersede_video(database, episode_id, missing_shot_id)
        with pytest.raises(RenderInputInvalid, match=str(missing_shot_id)):
            await handler.execute(
                CreateRenderSnapshotCommand(
                    episode_id, "renderEpisode/test", "render:snapshot:0003"
                )
            )
        assert await stored_count(database, episode_id, "render_snapshots") == 1
    finally:
        await database.close()


def test_render_recipe_requires_pinned_tools_and_font() -> None:
    with pytest.raises(ValidationError):
        RenderRecipeV1(
            runtime_image=FFMPEG_IMAGE,
            ffmpeg_version="",
            ffprobe_version="mock-ffprobe-7.1",
            font_name="Noto Sans CJK SC",
            font_file="/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
            font_sha256="not-a-digest",
            font_license="OFL-1.1",
        )


async def supersede_video(database: DatabasePool, episode_id: UUID, shot_id: UUID) -> None:
    async with database.transaction() as connection:
        await connection.execute(
            """
            UPDATE adoptions SET status='superseded',superseded_at=now()
            WHERE episode_id=$1 AND usage_type='shot_video' AND usage_id=$2
              AND status='active'
            """,
            episode_id,
            shot_id,
        )


async def replace_subtitle_tts_hash(
    database: DatabasePool, subtitle_id: UUID, input_hash: str
) -> None:
    async with database.transaction() as connection:
        await connection.execute(
            """
            UPDATE subtitle_versions
            SET input_refs_json=jsonb_set(
                input_refs_json,'{tts_adoptions,0,input_hash}',to_jsonb($2::text)
            )
            WHERE id=$1
            """,
            subtitle_id,
            input_hash,
        )


async def stored_count(database: DatabasePool, episode_id: UUID, table_name: str) -> int:
    if table_name not in {"render_snapshots", "production_tasks"}:
        raise ValueError("unsupported test table")
    async with database.transaction() as connection:
        condition = " AND type='render_episode'" if table_name == "production_tasks" else ""
        value = await connection.fetchval(
            f"SELECT count(*) FROM {table_name} WHERE episode_id=$1{condition}",
            episode_id,
        )
    return int(value)
