from __future__ import annotations

from uuid import UUID

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.modules.project_catalog.application.create_project import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from lanverse.modules.project_catalog.application.sources import (
    ConfirmSourceCommand,
    ConfirmSourceHandler,
    CreateSourceRevisionCommand,
    CreateSourceRevisionHandler,
)
from lanverse.modules.story_development.application.contracts.snapshots import (
    StoryboardGenerationSnapshot,
)
from lanverse.modules.story_development.application.generate import (
    GenerateScriptCommand,
    GenerateScriptHandler,
    GenerateStoryboardCommand,
    GenerateStoryboardHandler,
)
from lanverse.modules.story_development.application.results import (
    ScriptResultRegistrar,
    StoryboardResultRegistrar,
)
from lanverse.modules.story_development.application.scripts import (
    ConfirmScriptCommand,
    ConfirmScriptHandler,
)
from lanverse.modules.story_development.infrastructure.text_provider import (
    DeterministicTextProvider,
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
