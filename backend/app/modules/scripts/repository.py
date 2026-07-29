from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.identity.models import Membership
from app.modules.scripts.models import (
    ExtractionBatch,
    ExtractionCandidate,
    ScriptSource,
    ScriptVersion,
)


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


async def find_version(
    session: AsyncSession,
    version_id: UUID,
) -> ScriptVersion | None:
    return await session.scalar(
        select(ScriptVersion).where(ScriptVersion.id == version_id)
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


async def find_idempotent_extraction_batch(
    session: AsyncSession,
    script_version_id: UUID,
    idempotency_key: str,
) -> ExtractionBatch | None:
    return await session.scalar(
        select(ExtractionBatch).where(
            ExtractionBatch.script_version_id == script_version_id,
            ExtractionBatch.idempotency_key == idempotency_key,
        )
    )


async def find_extraction_batch(
    session: AsyncSession,
    batch_id: UUID,
    *,
    for_update: bool = False,
) -> ExtractionBatch | None:
    query = select(ExtractionBatch).where(ExtractionBatch.id == batch_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_extraction_batch_for_user(
    session: AsyncSession,
    user_id: UUID,
    batch_id: UUID,
) -> ExtractionBatch | None:
    return await session.scalar(
        select(ExtractionBatch)
        .join(
            ScriptVersion,
            ScriptVersion.id == ExtractionBatch.script_version_id,
        )
        .join(Membership, Membership.workspace_id == ExtractionBatch.workspace_id)
        .where(
            ExtractionBatch.id == batch_id,
            Membership.user_id == user_id,
            Membership.status == "active",
        )
    )


async def list_extraction_candidates_for_user(
    session: AsyncSession,
    user_id: UUID,
    batch_id: UUID,
    *,
    kind: str | None,
    status: str | None,
    limit: int,
    offset: int,
) -> tuple[list[ExtractionCandidate], int] | None:
    batch = await find_extraction_batch_for_user(session, user_id, batch_id)
    if batch is None:
        return None
    filters = [ExtractionCandidate.batch_id == batch_id]
    if kind is not None:
        filters.append(ExtractionCandidate.kind == kind)
    if status is not None:
        filters.append(ExtractionCandidate.status == status)
    total = await session.scalar(
        select(func.count()).select_from(ExtractionCandidate).where(*filters)
    )
    candidates = await session.scalars(
        select(ExtractionCandidate)
        .where(*filters)
        .order_by(
            ExtractionCandidate.source_start,
            ExtractionCandidate.source_end,
            ExtractionCandidate.id,
        )
        .limit(limit)
        .offset(offset)
    )
    return list(candidates), total or 0


async def find_extraction_candidate_for_user(
    session: AsyncSession,
    user_id: UUID,
    candidate_id: UUID,
) -> ExtractionCandidate | None:
    return await session.scalar(
        select(ExtractionCandidate)
        .join(
            ExtractionBatch,
            ExtractionBatch.id == ExtractionCandidate.batch_id,
        )
        .join(Membership, Membership.workspace_id == ExtractionCandidate.workspace_id)
        .where(
            ExtractionCandidate.id == candidate_id,
            Membership.user_id == user_id,
            Membership.status == "active",
        )
    )
