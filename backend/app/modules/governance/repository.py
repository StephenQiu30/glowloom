from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.governance.models import Consent, ConsentProof, ConsentRevision


async def find_consent(
    session: AsyncSession,
    consent_id: UUID,
    *,
    for_update: bool = False,
) -> Consent | None:
    query = select(Consent).where(Consent.id == consent_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_consent_by_idempotency(
    session: AsyncSession, workspace_id: UUID, idempotency_key: str
) -> Consent | None:
    return await session.scalar(
        select(Consent).where(
            Consent.workspace_id == workspace_id,
            Consent.idempotency_key == idempotency_key,
        )
    )


async def list_consents(
    session: AsyncSession,
    workspace_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[Consent], int]:
    filters = [Consent.workspace_id == workspace_id]
    total = await session.scalar(
        select(func.count()).select_from(Consent).where(*filters)
    )
    rows = await session.scalars(
        select(Consent)
        .where(*filters)
        .order_by(Consent.updated_at.desc(), Consent.id)
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


async def list_current_consents_for_subject(
    session: AsyncSession,
    workspace_id: UUID,
    subject_type: str,
    subject_id: UUID,
) -> list[tuple[Consent, ConsentRevision]]:
    rows = await session.execute(
        select(Consent, ConsentRevision)
        .join(
            ConsentRevision,
            (ConsentRevision.id == Consent.current_revision_id)
            & (ConsentRevision.workspace_id == Consent.workspace_id),
        )
        .where(
            Consent.workspace_id == workspace_id,
            Consent.subject_type == subject_type,
            Consent.subject_id == subject_id,
        )
        .order_by(Consent.created_at, Consent.id)
    )
    return [(row[0], row[1]) for row in rows]


async def find_revision(
    session: AsyncSession, revision_id: UUID
) -> ConsentRevision | None:
    return await session.scalar(
        select(ConsentRevision).where(ConsentRevision.id == revision_id)
    )


async def list_revisions(
    session: AsyncSession, consent_id: UUID
) -> list[ConsentRevision]:
    return list(
        await session.scalars(
            select(ConsentRevision)
            .where(ConsentRevision.consent_id == consent_id)
            .order_by(ConsentRevision.revision_no)
        )
    )


async def find_revisions(
    session: AsyncSession, revision_ids: list[UUID]
) -> list[ConsentRevision]:
    if not revision_ids:
        return []
    return list(
        await session.scalars(
            select(ConsentRevision).where(ConsentRevision.id.in_(revision_ids))
        )
    )


async def list_proofs(
    session: AsyncSession, revision_ids: list[UUID]
) -> list[ConsentProof]:
    if not revision_ids:
        return []
    return list(
        await session.scalars(
            select(ConsentProof)
            .where(ConsentProof.consent_revision_id.in_(revision_ids))
            .order_by(ConsentProof.consent_revision_id, ConsentProof.position)
        )
    )
