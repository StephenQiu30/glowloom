from typing import cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.production.contracts import (
    AssetImpactTaskStatus,
    GenerationPromptSnapshot,
    GenerationTaskSnapshot,
)
from app.modules.production.generation_repository import list_asset_impact_rows

_ACTIVE_TASK_STATUSES = frozenset({"queued", "running", "waiting_provider", "unknown"})


async def read_asset_production_impacts(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    project_id: UUID,
    asset_version_ids: list[UUID],
    for_update: bool,
) -> tuple[list[GenerationPromptSnapshot], list[GenerationTaskSnapshot]]:
    rows = await list_asset_impact_rows(
        session,
        workspace_id,
        project_id,
        asset_version_ids,
        for_update=for_update,
    )
    prompts = [
        GenerationPromptSnapshot(
            generation_request_id=request.id,
            episode_id=request.episode_id,
            shot_id=request.shot_id,
            shot_spec_version_id=request.shot_spec_version_id,
            input_hash=request.input_hash,
        )
        for request, _task in rows
    ]
    tasks = [
        GenerationTaskSnapshot(
            task_id=task.id,
            generation_request_id=request.id,
            status=cast(AssetImpactTaskStatus, task.status),
            revision=task.revision,
        )
        for request, task in rows
        if task.status in _ACTIVE_TASK_STATUSES
    ]
    return prompts, tasks
