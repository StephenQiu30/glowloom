from __future__ import annotations

from uuid import UUID

from db.pool import DatabasePool
from schemas.story_snapshots import ScriptVersionSnapshot, StoryboardVersionSnapshot
from schemas.subtitle_versions import SubtitleVersionSnapshot
from services.media_generation import GenerateMediaCommand, GenerateMediaHandler
from services.script_versions import GetScriptVersionHandler
from services.storyboards import (
    ConfirmStoryboardCommand,
    ConfirmStoryboardHandler,
    ListCreativeAssetsHandler,
)
from services.subtitles import (
    ConfirmSubtitleCommand,
    ConfirmSubtitleHandler,
    CreateSubtitlesCommand,
    CreateSubtitlesHandler,
)
from tests.integration.media_library.support import (
    MemoryTransport,
    adopt_task_candidate,
    media_job_handler,
    run_media_job,
)
from tests.integration.story_development.support import storyboard_draft


async def story_with_tts_adoptions(
    database: DatabasePool,
    key: str,
    transport: MemoryTransport | None = None,
) -> tuple[UUID, ScriptVersionSnapshot, StoryboardVersionSnapshot]:
    episode_id, generated = await storyboard_draft(database, f"subtitle:{key}")
    confirmed = await ConfirmStoryboardHandler(database).execute(
        ConfirmStoryboardCommand(generated.storyboard.id, generated.storyboard.resource_version)
    )
    script = await GetScriptVersionHandler(database).execute(
        confirmed.storyboard.content.script_version_id
    )
    submit = GenerateMediaHandler(database, release_version="test-release")
    jobs = media_job_handler(database, transport=transport)
    for line in (item for scene in script.content.scenes for item in scene.speech_lines):
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


async def render_ready_story(
    database: DatabasePool,
    key: str,
    transport: MemoryTransport | None = None,
) -> tuple[UUID, StoryboardVersionSnapshot, SubtitleVersionSnapshot, MemoryTransport]:
    transport = transport or MemoryTransport()
    episode_id, _, board = await story_with_tts_adoptions(database, key, transport)
    submit = GenerateMediaHandler(database, release_version="test-release")
    jobs = media_job_handler(database, transport=transport)
    assets = await ListCreativeAssetsHandler(database).execute(episode_id)
    for asset in assets:
        if asset.asset_type == "visual_style":
            continue
        accepted = await submit.execute(
            GenerateMediaCommand(
                episode_id,
                "asset_image",
                asset.asset_id,
                asset.id,
                f"render:{key}:asset:{asset.asset_id}",
            )
        )
        await run_media_job(database, accepted.task_id, jobs)
        await adopt_task_candidate(database, accepted.task_id)
    for shot in board.content.shots:
        image = await submit.execute(
            GenerateMediaCommand(
                episode_id,
                "shot_image",
                shot.shot_id,
                board.id,
                f"render:{key}:image:{shot.ordinal}",
            )
        )
        await run_media_job(database, image.task_id, jobs)
        await adopt_task_candidate(database, image.task_id)
        video = await submit.execute(
            GenerateMediaCommand(
                episode_id,
                "shot_video",
                shot.shot_id,
                board.id,
                f"render:{key}:video:{shot.ordinal}",
            )
        )
        await run_media_job(database, video.task_id, jobs)
        await adopt_task_candidate(database, video.task_id)
    draft = await CreateSubtitlesHandler(database).execute(
        CreateSubtitlesCommand(episode_id, f"render:{key}:subtitles")
    )
    subtitle = await ConfirmSubtitleHandler(database).execute(
        ConfirmSubtitleCommand(draft.id, draft.resource_version)
    )
    return episode_id, board, subtitle, transport
