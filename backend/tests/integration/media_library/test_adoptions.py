from __future__ import annotations

import asyncio
from dataclasses import replace
from uuid import UUID, uuid4

import pytest

from db.pool import DatabasePool
from repositories.idempotency import IdempotencyKeyReused
from services.adoptions import (
    AdoptCandidateCommand,
    AdoptCandidateHandler,
    AdoptionInputOutdated,
    CandidateNotAdoptable,
)
from services.media_generation import GenerateMediaCommand, GenerateMediaHandler
from services.script_versions import (
    ConfirmScriptCommand,
    ConfirmScriptHandler,
    DeriveScriptDraftCommand,
    DeriveScriptDraftHandler,
    GetScriptVersionHandler,
)
from services.storyboards import ConfirmStoryboardCommand, ConfirmStoryboardHandler
from tests.integration.media_library.support import (
    media_job_handler,
    run_media_job,
)
from tests.integration.story_development.support import storyboard_draft


async def ready_tts_candidates(
    database: DatabasePool, key: str, count: int = 2
) -> tuple[UUID, object, object, tuple[dict[str, object], ...]]:
    episode_id, generated = await storyboard_draft(database, key)
    script = await GetScriptVersionHandler(database).execute(
        generated.storyboard.content.script_version_id
    )
    line = script.content.scenes[0].speech_lines[0]
    generator = GenerateMediaHandler(database, release_version="test-release")
    jobs = media_job_handler(database)
    candidates = []
    for index in range(count):
        task = await generator.execute(
            GenerateMediaCommand(
                episode_id,
                "speech_audio",
                line.speech_line_id,
                script.id,
                f"adoption:{key}:{index}",
            )
        )
        await run_media_job(database, task.task_id, jobs)
        async with database.transaction() as connection:
            row = await connection.fetchrow(
                "SELECT * FROM generation_candidates WHERE task_id=$1", task.task_id
            )
        assert row is not None
        candidates.append(dict(row))
    return episode_id, script, line, tuple(candidates)


def command(candidate: dict[str, object], key: str) -> AdoptCandidateCommand:
    return AdoptCandidateCommand(
        usage_type=str(candidate["usage_type"]),
        usage_id=UUID(str(candidate["usage_id"])),
        input_version_id=UUID(str(candidate["input_version_id"])),
        input_hash=str(candidate["input_hash"]),
        candidate_id=UUID(str(candidate["id"])),
        idempotency_key=key,
    )


@pytest.mark.asyncio
async def test_adoption_replaces_history_and_serializes_twenty_concurrent_choices(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=25)
    await database.start()
    try:
        _, _, _, candidates = await ready_tts_candidates(database, "concurrency")
        handler = AdoptCandidateHandler(database)
        first = await handler.execute(command(candidates[0], "adopt:first:0001"))
        second = await handler.execute(command(candidates[1], "adopt:second:001"))
        replay = await handler.execute(command(candidates[1], "adopt:second:001"))

        assert first.status == "active" and first.version == 1
        assert second.status == "active" and second.version == 2
        assert second.supersedes_id == first.id
        assert replay.id == second.id
        with pytest.raises(IdempotencyKeyReused):
            await handler.execute(command(candidates[0], "adopt:second:001"))

        await asyncio.gather(
            *(
                handler.execute(command(candidates[index % 2], f"adopt:parallel:{index:02d}"))
                for index in range(20)
            )
        )
        async with database.transaction() as connection:
            rows = await connection.fetch("SELECT * FROM adoptions ORDER BY version")
            unchanged = await connection.fetchval(
                "SELECT count(*) FROM generation_candidates WHERE status='ready'"
            )

        assert sum(row["status"] == "active" for row in rows) == 1
        assert [row["version"] for row in rows] == list(range(1, len(rows) + 1))
        assert all(
            rows[index]["supersedes_id"] == rows[index - 1]["id"] for index in range(1, len(rows))
        )
        assert unchanged == 2
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_adoption_rejects_non_ready_wrong_slot_and_outdated_input(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        _, script, _, candidates = await ready_tts_candidates(database, "rejections")
        handler = AdoptCandidateHandler(database)
        async with database.transaction() as connection:
            await connection.execute(
                "UPDATE generation_candidates SET status='blocked',blocked_reason='test' "
                "WHERE id=$1",
                candidates[0]["id"],
            )
        with pytest.raises(CandidateNotAdoptable):
            await handler.execute(command(candidates[0], "adopt:blocked:01"))
        with pytest.raises(CandidateNotAdoptable):
            await handler.execute(
                replace(command(candidates[1], "adopt:wrong-slot"), usage_id=uuid4())
            )
        async with database.transaction() as connection:
            style = await connection.fetchrow(
                "SELECT asset_id,id FROM creative_asset_versions "
                "WHERE episode_id=$1 AND asset_type='visual_style'",
                candidates[1]["episode_id"],
            )
        assert style is not None
        with pytest.raises(CandidateNotAdoptable):
            await handler.execute(
                replace(
                    command(candidates[1], "adopt:style:0001"),
                    usage_type="asset_image",
                    usage_id=style["asset_id"],
                    input_version_id=style["id"],
                )
            )

        draft = await DeriveScriptDraftHandler(database).execute(
            DeriveScriptDraftCommand(script.id, "adopt:derive:001")
        )
        await ConfirmScriptHandler(database).execute(
            ConfirmScriptCommand(draft.id, draft.resource_version)
        )
        with pytest.raises(AdoptionInputOutdated):
            await handler.execute(command(candidates[1], "adopt:outdated:1"))
        async with database.transaction() as connection:
            assert await connection.fetchval("SELECT count(*) FROM adoptions") == 0
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_asset_candidate_cannot_be_adopted_after_its_script_is_outdated(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "asset-outdated")
        confirmed = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(
                generated.storyboard.id, generated.storyboard.resource_version
            )
        )
        asset = next(item for item in confirmed.assets if item.asset_type == "character")
        task = await GenerateMediaHandler(database, release_version="test-release").execute(
            GenerateMediaCommand(
                episode_id,
                "asset_image",
                asset.asset_id,
                asset.id,
                "adoption:asset:outdated",
            )
        )
        await run_media_job(database, task.task_id, media_job_handler(database))
        async with database.transaction() as connection:
            candidate = await connection.fetchrow(
                "SELECT * FROM generation_candidates WHERE task_id=$1", task.task_id
            )
        assert candidate is not None
        script_id = confirmed.storyboard.content.script_version_id
        draft = await DeriveScriptDraftHandler(database).execute(
            DeriveScriptDraftCommand(script_id, "adopt:asset:derive")
        )
        await ConfirmScriptHandler(database).execute(
            ConfirmScriptCommand(draft.id, draft.resource_version)
        )

        with pytest.raises(AdoptionInputOutdated):
            await AdoptCandidateHandler(database).execute(
                command(dict(candidate), "adopt:asset:stale")
            )
    finally:
        await database.close()
