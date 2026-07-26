from __future__ import annotations

from uuid import UUID

from db.pool import DatabasePool
from integrations.ai.deterministic_text import (
    DeterministicTextProvider,
)
from schemas.story_snapshots import (
    StoryboardGenerationSnapshot,
)
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.script_versions import (
    ConfirmScriptCommand,
    ConfirmScriptHandler,
)
from services.sources import (
    ConfirmSourceCommand,
    ConfirmSourceHandler,
    CreateSourceRevisionCommand,
    CreateSourceRevisionHandler,
)
from services.story_generation import (
    GenerateScriptCommand,
    GenerateScriptHandler,
    GenerateStoryboardCommand,
    GenerateStoryboardHandler,
)
from services.story_results import (
    ScriptResultRegistrar,
    StoryboardResultRegistrar,
)


async def storyboard_draft(
    database: DatabasePool, key: str
) -> tuple[UUID, StoryboardGenerationSnapshot]:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="分镜版本", idempotency_key=f"project:{key}")
    )
    source = await CreateSourceRevisionHandler(database).execute(
        CreateSourceRevisionCommand(
            episode_id=project.episode.id,
            content="汉字制作" + "d" * 296,
            rights_basis="original",
            parent_id=None,
            idempotency_key=f"source:{key}",
        )
    )
    await ConfirmSourceHandler(database).execute(
        ConfirmSourceCommand(source.id, source.resource_version)
    )
    provider = DeterministicTextProvider()
    script_task = await GenerateScriptHandler(
        database, release_version="test-release"
    ).execute(GenerateScriptCommand(project.episode.id, f"script:{key}"))
    script_draft = await ScriptResultRegistrar(database).register(
        script_task.task_id, await provider.generate_script(source.id, "制作剧本")
    )
    script = await ConfirmScriptHandler(database).execute(
        ConfirmScriptCommand(script_draft.id, script_draft.resource_version)
    )
    board_task = await GenerateStoryboardHandler(
        database, release_version="test-release"
    ).execute(GenerateStoryboardCommand(project.episode.id, f"board:{key}"))
    generated = await StoryboardResultRegistrar(database).register(
        board_task.task_id,
        await provider.generate_storyboard(board_task.task_id, script.id, script.content),
    )
    return project.episode.id, generated
