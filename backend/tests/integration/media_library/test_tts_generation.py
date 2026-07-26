from __future__ import annotations

import json
from uuid import uuid4

import pytest

from db.pool import DatabasePool
from schemas.story_content import canonical_content_hash
from services.media_generation import (
    GenerateMediaCommand,
    GenerateMediaHandler,
    MediaInputNotFound,
)
from services.script_versions import GetScriptVersionHandler
from tests.integration.story_development.support import storyboard_draft


@pytest.mark.asyncio
async def test_tts_submission_freezes_confirmed_line_text_hash_and_logical_voice(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "media:tts:ready")
        script_id = generated.storyboard.content.script_version_id
        script = await GetScriptVersionHandler(database).execute(script_id)
        line = script.content.scenes[0].speech_lines[0]
        handler = GenerateMediaHandler(database, release_version="test-release")

        accepted = await handler.execute(
            GenerateMediaCommand(
                episode_id=episode_id,
                usage_type="speech_audio",
                usage_id=line.speech_line_id,
                input_version_id=script.id,
                idempotency_key="media:tts:ready:001",
            )
        )

        async with database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT snapshot.capability,snapshot.input_refs_json,snapshot.prompt,
                       snapshot.model_profile_id,snapshot.provider_id,
                       snapshot.model_id,snapshot.schema_version
                FROM production_tasks task
                JOIN submission_snapshots snapshot ON snapshot.id=task.snapshot_id
                WHERE task.id=$1
                """,
                accepted.task_id,
            )
        expected = {
            "usage_type": "speech_audio",
            "speech_line_id": str(line.speech_line_id),
            "input_version_id": str(script.id),
            "text_hash": canonical_content_hash(line.text),
            "voice_id": line.voice_id,
        }
        assert json.loads(row["input_refs_json"]) == {
            **expected,
            "input_hash": canonical_content_hash(expected),
        }
        assert row["prompt"] == line.text
        assert (
            row["capability"],
            row["model_profile_id"],
            row["provider_id"],
            row["model_id"],
            row["schema_version"],
        ) == ("tts", "mock-tts-v1", "mock", "deterministic-tts", "tts-v1")

        with pytest.raises(MediaInputNotFound, match="speech line"):
            await handler.execute(
                GenerateMediaCommand(
                    episode_id=episode_id,
                    usage_type="speech_audio",
                    usage_id=uuid4(),
                    input_version_id=script.id,
                    idempotency_key="media:tts:missing:001",
                )
            )
    finally:
        await database.close()
