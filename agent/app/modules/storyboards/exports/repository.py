from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.storyboards.exports.models import (
    StoryboardExportJob,
    StoryboardExportManifest,
)


async def find_job(
    session: AsyncSession,
    job_id: UUID,
    *,
    for_update: bool = False,
) -> StoryboardExportJob | None:
    query = select(StoryboardExportJob).where(StoryboardExportJob.id == job_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_job_by_key(
    session: AsyncSession,
    episode_id: UUID,
    idempotency_key: str,
) -> StoryboardExportJob | None:
    return await session.scalar(
        select(StoryboardExportJob).where(
            StoryboardExportJob.episode_id == episode_id,
            StoryboardExportJob.idempotency_key == idempotency_key,
        )
    )


async def list_jobs(
    session: AsyncSession,
    episode_id: UUID,
) -> tuple[list[StoryboardExportJob], int]:
    rows = await session.scalars(
        select(StoryboardExportJob)
        .where(StoryboardExportJob.episode_id == episode_id)
        .order_by(StoryboardExportJob.created_at.desc(), StoryboardExportJob.id.desc())
    )
    total = await session.scalar(
        select(func.count())
        .select_from(StoryboardExportJob)
        .where(StoryboardExportJob.episode_id == episode_id)
    )
    return list(rows), int(total or 0)


async def find_manifests(
    session: AsyncSession,
    job_ids: list[UUID],
) -> dict[UUID, StoryboardExportManifest]:
    if not job_ids:
        return {}
    rows = await session.scalars(
        select(StoryboardExportManifest).where(StoryboardExportManifest.job_id.in_(job_ids))
    )
    return {row.job_id: row for row in rows}
