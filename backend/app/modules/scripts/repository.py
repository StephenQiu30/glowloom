from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.identity.models import Membership
from app.modules.scripts.models import ScriptSource, ScriptVersion


async def find_source_by_idempotency(
    session: AsyncSession,
    episode_id: UUID,
    idempotency_key: str,
) -> ScriptSource | None:
    return await session.scalar(
        select(ScriptSource).where(
            ScriptSource.episode_id == episode_id,
            ScriptSource.idempotency_key == idempotency_key,
        )
    )


async def find_initial_version(
    session: AsyncSession,
    source_id: UUID,
) -> ScriptVersion | None:
    return await session.scalar(
        select(ScriptVersion).where(
            ScriptVersion.source_id == source_id,
            ScriptVersion.version_no == 1,
        )
    )


async def find_source_for_user(
    session: AsyncSession,
    user_id: UUID,
    source_id: UUID,
    *,
    for_update: bool = False,
) -> ScriptSource | None:
    query = (
        select(ScriptSource)
        .join(Membership, Membership.workspace_id == ScriptSource.workspace_id)
        .where(
            ScriptSource.id == source_id,
            Membership.user_id == user_id,
            Membership.status == "active",
        )
    )
    if for_update:
        query = query.with_for_update(of=ScriptSource)
    return await session.scalar(query)


async def latest_version_number(
    session: AsyncSession,
    source_id: UUID,
) -> int:
    latest = await session.scalar(
        select(func.max(ScriptVersion.version_no)).where(
            ScriptVersion.source_id == source_id
        )
    )
    return latest or 0


async def find_version_for_user(
    session: AsyncSession,
    user_id: UUID,
    version_id: UUID,
) -> ScriptVersion | None:
    return await session.scalar(
        select(ScriptVersion)
        .join(ScriptSource, ScriptSource.id == ScriptVersion.source_id)
        .join(Membership, Membership.workspace_id == ScriptVersion.workspace_id)
        .where(
            ScriptVersion.id == version_id,
            Membership.user_id == user_id,
            Membership.status == "active",
        )
    )


async def list_versions_for_user(
    session: AsyncSession,
    user_id: UUID,
    source_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[ScriptVersion], int] | None:
    source = await find_source_for_user(session, user_id, source_id)
    if source is None:
        return None
    total = await session.scalar(
        select(func.count())
        .select_from(ScriptVersion)
        .where(ScriptVersion.source_id == source_id)
    )
    rows = await session.scalars(
        select(ScriptVersion)
        .where(ScriptVersion.source_id == source_id)
        .order_by(ScriptVersion.version_no, ScriptVersion.id)
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0
