from __future__ import annotations

from uuid import UUID

from db.pool import DatabasePool
from schemas.story_snapshots import ScriptVersionSnapshot, StoryboardVersionSnapshot
from services.media_generation import GenerateMediaCommand, GenerateMediaHandler
from services.script_versions import GetScriptVersionHandler
from services.storyboards import ConfirmStoryboardCommand, ConfirmStoryboardHandler
from tests.integration.media_library.support import (
    adopt_task_candidate,
    media_job_handler,
    run_media_job,
)
from tests.integration.story_development.support import storyboard_draft


async def story_with_tts_adoptions(
    database: DatabasePool, key: str
) -> tuple[UUID, ScriptVersionSnapshot, StoryboardVersionSnapshot]:
    episode_id, generated = await storyboard_draft(database, f"subtitle:{key}")
    confirmed = await ConfirmStoryboardHandler(database).execute(
        ConfirmStoryboardCommand(
            generated.storyboard.id, generated.storyboard.resource_version
        )
    )
    script = await GetScriptVersionHandler(database).execute(
        confirmed.storyboard.content.script_version_id
    )
    submit = GenerateMediaHandler(database, release_version="test-release")
    jobs = media_job_handler(database)
    for line in (
        item for scene in script.content.scenes for item in scene.speech_lines
    ):
        accepted = await submit.execute(
            GenerateMediaCommand(
                episode_id,
                "speech_audio",
                line.speech_line_id,
                script.id,
                f"subtitle:{key}:tts:{line.ordinal}",
            )
        )
        await run_media_job(database, accepted.task_id, jobs)
        await adopt_task_candidate(database, accepted.task_id)
    return episode_id, script, confirmed.storyboard
