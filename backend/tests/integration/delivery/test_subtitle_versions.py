from __future__ import annotations

import asyncio
from uuid import UUID

import pytest
from services.subtitles import (
    ConfirmSubtitleCommand,
    ConfirmSubtitleHandler,
    CreateSubtitlesCommand,
    CreateSubtitlesHandler,
    DeriveSubtitleDraftCommand,
    DeriveSubtitleDraftHandler,
    GetSubtitleVersionHandler,
    ListSubtitleVersionsHandler,
    SaveSubtitleCommand,
    SaveSubtitleHandler,
)

from db.pool import DatabasePool
from schemas.subtitles import SubtitleContentV1, SubtitleCueV1
from services.script_versions import VersionConflict, VersionImmutable
from tests.integration.delivery.support import story_with_tts_adoptions


def revised_content(content: SubtitleContentV1, text: str) -> SubtitleContentV1:
    first = content.cues[0]
    changed = SubtitleCueV1.model_validate(
        {
            **first.model_dump(),
            "text": text,
            "start_ticks": first.start_ticks + 3750,
            "end_ticks": first.end_ticks + 3750,
        }
    )
    return SubtitleContentV1(
        language=content.language,
        cues=(changed, *content.cues[1:]),
    )


@pytest.mark.asyncio
async def test_subtitle_create_save_confirm_and_derive_are_versioned(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=8)
    await database.start()
    try:
        episode_id, script, board = await story_with_tts_adoptions(database, "versions")
        create = CreateSubtitlesHandler(database)
        command = CreateSubtitlesCommand(episode_id, "subtitle:create:0001")

        draft = await create.execute(command)
        replay = await create.execute(command)

        assert replay == draft
        assert draft.status == "draft"
        assert draft.script_version_id == script.id
        assert draft.shot_spec_version_id == board.id
        assert len(draft.content.cues) == 6
        assert [cue.start_ticks for cue in draft.content.cues] == [
            0,
            450000,
            900000,
            1350000,
            1800000,
            2250000,
        ]
        assert all(cue.tts_duration_ticks == 90000 for cue in draft.content.cues)

        changed = revised_content(draft.content, "已校对的第一句")
        saved = await SaveSubtitleHandler(database).execute(
            SaveSubtitleCommand(draft.id, draft.resource_version, changed)
        )
        with pytest.raises(VersionConflict):
            await SaveSubtitleHandler(database).execute(
                SaveSubtitleCommand(draft.id, draft.resource_version, changed)
            )
        confirmed = await ConfirmSubtitleHandler(database).execute(
            ConfirmSubtitleCommand(saved.id, saved.resource_version)
        )
        assert confirmed.status == "confirmed"
        with pytest.raises(VersionImmutable):
            await SaveSubtitleHandler(database).execute(
                SaveSubtitleCommand(confirmed.id, confirmed.resource_version, changed)
            )

        derive = DeriveSubtitleDraftHandler(database)
        first = await derive.execute(
            DeriveSubtitleDraftCommand(confirmed.id, "subtitle:derive:0001")
        )
        replay_derived = await derive.execute(
            DeriveSubtitleDraftCommand(confirmed.id, "subtitle:derive:0001")
        )
        second = await derive.execute(
            DeriveSubtitleDraftCommand(confirmed.id, "subtitle:derive:0002")
        )
        assert replay_derived == first
        assert first.parent_id == confirmed.id and second.parent_id == confirmed.id
        assert first.content == confirmed.content == second.content

        results = await asyncio.gather(
            ConfirmSubtitleHandler(database).execute(
                ConfirmSubtitleCommand(first.id, first.resource_version)
            ),
            ConfirmSubtitleHandler(database).execute(
                ConfirmSubtitleCommand(second.id, second.resource_version)
            ),
        )
        assert all(item.status == "confirmed" for item in results)
        versions = await ListSubtitleVersionsHandler(database).execute(episode_id)
        stored = {
            item.id: await GetSubtitleVersionHandler(database).execute(item.id)
            for item in versions
        }
        assert len([item for item in stored.values() if item.status == "confirmed"]) == 1
        assert len([item for item in stored.values() if item.status == "superseded"]) == 2
        assert await database_count(database, episode_id, "confirmed") == 1
    finally:
        await database.close()


async def database_count(database: DatabasePool, episode_id: UUID, status: str) -> int:
    async with database.transaction() as connection:
        value = await connection.fetchval(
            "SELECT count(*) FROM subtitle_versions WHERE episode_id=$1 AND status=$2",
            episode_id,
            status,
        )
    return int(value)
