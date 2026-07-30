from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.scripts.models import (
    CandidateDecision,
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


async def find_source(
    session: AsyncSession,
    source_id: UUID,
    *,
    for_update: bool = False,
) -> ScriptSource | None:
    query = select(ScriptSource).where(ScriptSource.id == source_id)
    if for_update:
        query = query.with_for_update()
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


async def find_version(
    session: AsyncSession,
    version_id: UUID,
) -> ScriptVersion | None:
    return await session.scalar(
        select(ScriptVersion).where(ScriptVersion.id == version_id)
    )


async def list_versions(
    session: AsyncSession,
    source_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[ScriptVersion], int]:
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


async def list_extraction_candidates(
    session: AsyncSession,
    batch_id: UUID,
    *,
    kind: str | None,
    status: str | None,
    limit: int,
    offset: int,
) -> tuple[list[ExtractionCandidate], int]:
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


async def find_extraction_candidate(
    session: AsyncSession,
    candidate_id: UUID,
    *,
    for_update: bool = False,
) -> ExtractionCandidate | None:
    query = select(ExtractionCandidate).where(ExtractionCandidate.id == candidate_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_candidate_decision_by_key(
    session: AsyncSession,
    candidate_id: UUID,
    decision_key: str,
) -> CandidateDecision | None:
    return await session.scalar(
        select(CandidateDecision).where(
            CandidateDecision.candidate_id == candidate_id,
            CandidateDecision.decision_key == decision_key,
        )
    )


async def list_candidate_decisions(
    session: AsyncSession,
    candidate_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[CandidateDecision], int]:
    total = await session.scalar(
        select(func.count())
        .select_from(CandidateDecision)
        .where(CandidateDecision.candidate_id == candidate_id)
    )
    decisions = await session.scalars(
        select(CandidateDecision)
        .where(CandidateDecision.candidate_id == candidate_id)
        .order_by(CandidateDecision.sequence, CandidateDecision.id)
        .limit(limit)
        .offset(offset)
    )
    return list(decisions), total or 0
