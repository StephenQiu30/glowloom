from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims
from app.modules.identity import Capability
from app.modules.projects import repository
from app.modules.projects.authorization import owned_project
from app.modules.projects.episodes.service import episode_for_read
from app.modules.projects.models import Episode
from app.modules.projects.snapshots.schemas import (
    BlockingReason,
    CostSummary,
    EpisodeProductionSnapshot,
    NextAction,
    ProjectProductionSnapshot,
    ReviewSummary,
    TaskSummary,
)


def _episode_snapshot(
    episode: Episode,
    currency: str,
    computed_at: datetime,
) -> EpisodeProductionSnapshot:
    return EpisodeProductionSnapshot(
        episode_id=episode.id,
        current_stage="script_import",
        completion=0,
        blocking_reasons=[
            BlockingReason(
                code="SCRIPT_MISSING",
                summary="单集尚未导入剧本",
                resource_type="episode",
                resource_id=episode.id,
            )
        ],
        next_actions=[
            NextAction(
                code="import_script",
                label="导入剧本",
                href=f"/studio/{episode.id}/script",
            )
        ],
        task_summary=TaskSummary(),
        review_summary=ReviewSummary(),
        cost_summary=CostSummary(currency=currency),
        partial_failures=[],
        computed_at=computed_at,
    )


async def episode_production_snapshot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> EpisodeProductionSnapshot:
    episode, project, _ = await episode_for_read(session, claims, episode_id)
    return _episode_snapshot(episode, project.currency, datetime.now(UTC))


async def project_production_snapshot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
) -> ProjectProductionSnapshot:
    project, _ = await owned_project(session, claims, project_id, Capability.CONTENT_READ)
    episodes = await repository.list_episodes(session, project_id, include_archived=False)
    computed_at = datetime.now(UTC)
    episode_snapshots = [
        _episode_snapshot(episode, project.currency, computed_at) for episode in episodes
    ]
    if episode_snapshots:
        blockers = [
            reason for snapshot in episode_snapshots for reason in snapshot.blocking_reasons
        ]
        actions = [episode_snapshots[0].next_actions[0]]
        current_stage: Literal["project_setup", "script_import"] = "script_import"
    else:
        blockers = [
            BlockingReason(
                code="EPISODE_MISSING",
                summary="项目尚未创建有效单集",
                resource_type="project",
                resource_id=project.id,
            )
        ]
        actions = [
            NextAction(
                code="create_episode",
                label="创建单集",
                href=f"/projects/{project.id}",
            )
        ]
        current_stage = "project_setup"
    return ProjectProductionSnapshot(
        project_id=project.id,
        current_stage=current_stage,
        completion=0,
        blocking_reasons=blockers,
        next_actions=actions,
        episodes=episode_snapshots,
        partial_failures=[],
        computed_at=computed_at,
    )
