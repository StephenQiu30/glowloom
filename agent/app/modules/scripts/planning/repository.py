from uuid import UUID

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.scripts.models import (
    DocumentRevision,
    EpisodePlan,
    EpisodeProposal,
    EpisodeSegmentOrigin,
    FormatIssue,
    ImportCommit,
    NarrativeBlock,
    ScriptDocument,
)


async def find_plan_by_idempotency(
    session: AsyncSession,
    project_id: UUID,
    idempotency_key: str,
) -> EpisodePlan | None:
    return await session.scalar(
        select(EpisodePlan).where(
            EpisodePlan.project_id == project_id,
            EpisodePlan.idempotency_key == idempotency_key,
        )
    )


async def find_plan(
    session: AsyncSession,
    plan_id: UUID,
    *,
    for_update: bool = False,
) -> EpisodePlan | None:
    query = select(EpisodePlan).where(EpisodePlan.id == plan_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_revision_document(
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


async def list_proposals(
    session: AsyncSession,
    plan_id: UUID,
    *,
    for_update: bool = False,
) -> list[EpisodeProposal]:
    query = (
        select(EpisodeProposal)
        .where(EpisodeProposal.plan_id == plan_id)
        .order_by(EpisodeProposal.position, EpisodeProposal.id)
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return list(await session.scalars(query))


async def list_blocks(
    session: AsyncSession,
    revision_id: UUID,
) -> list[NarrativeBlock]:
    return list(
        await session.scalars(
            select(NarrativeBlock)
            .where(NarrativeBlock.document_revision_id == revision_id)
            .order_by(NarrativeBlock.position, NarrativeBlock.id)
        )
    )


async def list_blocking_issues(
    session: AsyncSession,
    revision_id: UUID,
) -> list[FormatIssue]:
    return list(
        await session.scalars(
            select(FormatIssue)
            .where(
                FormatIssue.document_revision_id == revision_id,
                FormatIssue.severity == "blocking",
            )
            .order_by(FormatIssue.position, FormatIssue.id)
        )
    )


async def find_import_commit_by_idempotency(
    session: AsyncSession,
    workspace_id: UUID,
    idempotency_key: str,
    *,
    for_update: bool = False,
) -> ImportCommit | None:
    query = select(ImportCommit).where(
        ImportCommit.workspace_id == workspace_id,
        ImportCommit.idempotency_key == idempotency_key,
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_import_commit(
    session: AsyncSession,
    commit_id: UUID,
    *,
    for_update: bool = False,
) -> ImportCommit | None:
    query = select(ImportCommit).where(ImportCommit.id == commit_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def list_segment_origins(
    session: AsyncSession,
    commit_id: UUID,
    *,
    for_update: bool = False,
) -> list[EpisodeSegmentOrigin]:
    query = (
        select(EpisodeSegmentOrigin)
        .where(EpisodeSegmentOrigin.import_commit_id == commit_id)
        .order_by(EpisodeSegmentOrigin.position, EpisodeSegmentOrigin.id)
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return list(await session.scalars(query))
