from uuid import UUID

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.scripts.models import AdaptationRun


async def find_run(
    session: AsyncSession,
    run_id: UUID,
    *,
    for_update: bool = False,
) -> AdaptationRun | None:
    query = select(AdaptationRun).where(AdaptationRun.id == run_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_run_by_idempotency(
    session: AsyncSession,
    episode_id: UUID,
    idempotency_key: str,
) -> AdaptationRun | None:
    return await session.scalar(
        select(AdaptationRun).where(
            AdaptationRun.episode_id == episode_id,
            AdaptationRun.idempotency_key == idempotency_key,
        )
    )
