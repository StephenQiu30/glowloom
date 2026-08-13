from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.storyboards.coverage.models import (
    CoverageDecision,
    ShotNarrativeReference,
)


async def list_references(
    session: AsyncSession,
    spec_version_ids: list[UUID],
) -> list[ShotNarrativeReference]:
    if not spec_version_ids:
        return []
    return list(
        await session.scalars(
            select(ShotNarrativeReference)
            .where(
                ShotNarrativeReference.shot_spec_version_id.in_(spec_version_ids)
            )
            .order_by(
                ShotNarrativeReference.shot_spec_version_id,
                ShotNarrativeReference.unit_version_id,
                ShotNarrativeReference.channel,
                ShotNarrativeReference.segment_key,
            )
        )
    )


async def list_decisions(
    session: AsyncSession,
    episode_id: UUID,
) -> list[CoverageDecision]:
    return list(
        await session.scalars(
            select(CoverageDecision)
            .where(CoverageDecision.episode_id == episode_id)
            .order_by(CoverageDecision.sequence, CoverageDecision.id)
        )
    )


async def next_decision_sequence(
    session: AsyncSession,
    episode_id: UUID,
) -> int:
    current = await session.scalar(
        select(func.coalesce(func.max(CoverageDecision.sequence), 0)).where(
            CoverageDecision.episode_id == episode_id
        )
    )
    return int(current or 0) + 1


async def find_decision_by_key(
    session: AsyncSession,
    workspace_id: UUID,
    idempotency_key: str,
) -> CoverageDecision | None:
    return await session.scalar(
        select(CoverageDecision).where(
            CoverageDecision.workspace_id == workspace_id,
            CoverageDecision.idempotency_key == idempotency_key,
        )
    )
