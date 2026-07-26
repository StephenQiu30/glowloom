from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from db.pool import DatabasePool
from integrations.ai.registry import AiModelRegistry, create_mvp_registry
from schemas.tasks import SubmitTaskCommand, TaskAcceptedSnapshot
from services.project_reader import ProjectCatalogReader
from services.script_versions import (
    GetCurrentScriptHandler,
    VersionConflict,
)
from services.task_submission import TaskSubmitter


@dataclass(frozen=True, slots=True)
class GenerateScriptCommand:
    episode_id: UUID
    idempotency_key: str
    model_profile_id: str = "mock-text-v1"


class GenerateScriptHandler:
    def __init__(
        self,
        database: DatabasePool,
        *,
        release_version: str,
        registry: AiModelRegistry | None = None,
    ) -> None:
        self._catalog = ProjectCatalogReader(database)
        self._tasks = TaskSubmitter(database, release_version=release_version)
        self._registry = registry or create_mvp_registry()

    async def execute(self, command: GenerateScriptCommand) -> TaskAcceptedSnapshot:
        source = await self._catalog.confirmed_source(command.episode_id)
        profile = self._registry.select(
            "text",
            command.model_profile_id,
            schema_version="script-v1",
        )
        return await self._tasks.submit(
            SubmitTaskCommand(
                episode_id=command.episode_id,
                task_type="generate_script",
                capability="text",
                scope={"episode_id": str(command.episode_id)},
                input_refs={"source_revision_id": str(source.id)},
                prompt=f"将以下来源转换为 script-v1：\n{source.content}",
                parameters=profile.parameters,
                model_profile_id=profile.model_profile_id,
                provider_id=profile.provider_id,
                model_id=profile.model_id,
                route_version=profile.route_version,
                schema_version=profile.schema_version,
                operation_scope=f"generateScript/{command.episode_id}",
                idempotency_key=command.idempotency_key,
                handler_version="1",
            )
        )


@dataclass(frozen=True, slots=True)
class GenerateStoryboardCommand:
    episode_id: UUID
    idempotency_key: str
    model_profile_id: str = "mock-text-v1"


class GenerateStoryboardHandler:
    def __init__(
        self,
        database: DatabasePool,
        *,
        release_version: str,
        registry: AiModelRegistry | None = None,
    ) -> None:
        self._scripts = GetCurrentScriptHandler(database)
        self._tasks = TaskSubmitter(database, release_version=release_version)
        self._registry = registry or create_mvp_registry()

    async def execute(self, command: GenerateStoryboardCommand) -> TaskAcceptedSnapshot:
        script = await self._scripts.execute(command.episode_id)
        if script.input_outdated:
            raise VersionConflict
        profile = self._registry.select(
            "text",
            command.model_profile_id,
            schema_version="storyboard-generation-v1",
        )
        return await self._tasks.submit(
            SubmitTaskCommand(
                episode_id=command.episode_id,
                task_type="generate_storyboard",
                capability="text",
                scope={"episode_id": str(command.episode_id)},
                input_refs={"script_version_id": str(script.id)},
                prompt=(
                    "将以下剧本转换为 storyboard-generation-v1：\n"
                    f"{script.content.model_dump_json()}"
                ),
                parameters=profile.parameters,
                model_profile_id=profile.model_profile_id,
                provider_id=profile.provider_id,
                model_id=profile.model_id,
                route_version=profile.route_version,
                schema_version=profile.schema_version,
                operation_scope=f"generateStoryboard/{command.episode_id}",
                idempotency_key=command.idempotency_key,
                handler_version="1",
            )
        )
