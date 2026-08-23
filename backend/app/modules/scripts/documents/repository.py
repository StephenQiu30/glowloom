from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.scripts.models import (
    DocumentRevision,
    FormatIssue,
    NarrativeBlock,
    ScriptDocument,
)


async def find_document_by_idempotency(
    session: AsyncSession,
    project_id: UUID,
    idempotency_key: str,
) -> ScriptDocument | None:
    return await session.scalar(
        select(ScriptDocument).where(
            ScriptDocument.project_id == project_id,
            ScriptDocument.idempotency_key == idempotency_key,
        )
    )


async def find_document(
    session: AsyncSession,
    document_id: UUID,
) -> ScriptDocument | None:
    return await session.scalar(select(ScriptDocument).where(ScriptDocument.id == document_id))


async def find_initial_revision(
    session: AsyncSession,
    document_id: UUID,
) -> DocumentRevision | None:
    return await session.scalar(
        select(DocumentRevision).where(
            DocumentRevision.document_id == document_id,
            DocumentRevision.version_no == 1,
        )
    )


async def find_revision_with_document(
    session: AsyncSession,
    revision_id: UUID,
) -> tuple[DocumentRevision, ScriptDocument] | None:
    row = (
        await session.execute(
            select(DocumentRevision, ScriptDocument)
            .join(
                ScriptDocument,
                (ScriptDocument.id == DocumentRevision.document_id)
                & (ScriptDocument.workspace_id == DocumentRevision.workspace_id),
            )
            .where(DocumentRevision.id == revision_id)
        )
    ).one_or_none()
    return None if row is None else (row[0], row[1])


async def list_blocks(
    session: AsyncSession,
    revision_id: UUID,
) -> list[NarrativeBlock]:
    rows = await session.scalars(
        select(NarrativeBlock)
        .where(NarrativeBlock.document_revision_id == revision_id)
        .order_by(NarrativeBlock.position, NarrativeBlock.id)
    )
    return list(rows)


async def list_issues(
    session: AsyncSession,
    revision_id: UUID,
) -> list[FormatIssue]:
    rows = await session.scalars(
        select(FormatIssue)
        .where(FormatIssue.document_revision_id == revision_id)
        .order_by(FormatIssue.position, FormatIssue.id)
    )
    return list(rows)


async def list_documents(
    session: AsyncSession,
    project_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[ScriptDocument], int]:
    total = await session.scalar(
        select(func.count())
        .select_from(ScriptDocument)
        .where(ScriptDocument.project_id == project_id)
    )
    rows = await session.scalars(
        select(ScriptDocument)
        .where(ScriptDocument.project_id == project_id)
        .order_by(ScriptDocument.created_at.desc(), ScriptDocument.id.desc())
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0
