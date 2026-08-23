from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.storyboards.drafts.models import (
    DraftAssetReference,
    DraftDecision,
    DraftInputAsset,
    DraftInputUnit,
    DraftShot,
    DraftShotUnit,
    StoryboardDraftBatch,
)


async def find_batch(
    session: AsyncSession,
    batch_id: UUID,
    *,
    for_update: bool = False,
) -> StoryboardDraftBatch | None:
    query = select(StoryboardDraftBatch).where(StoryboardDraftBatch.id == batch_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_batch_by_key(
    session: AsyncSession,
    episode_id: UUID,
    idempotency_key: str,
) -> StoryboardDraftBatch | None:
    return await session.scalar(
        select(StoryboardDraftBatch).where(
            StoryboardDraftBatch.episode_id == episode_id,
            StoryboardDraftBatch.idempotency_key == idempotency_key,
        )
    )


async def find_draft(
    session: AsyncSession,
    draft_id: UUID,
    *,
    for_update: bool = False,
) -> DraftShot | None:
    query = select(DraftShot).where(DraftShot.id == draft_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def list_input_units(
    session: AsyncSession,
    batch_id: UUID,
) -> list[DraftInputUnit]:
    return list(
        await session.scalars(
            select(DraftInputUnit)
            .where(DraftInputUnit.batch_id == batch_id)
            .order_by(DraftInputUnit.position, DraftInputUnit.id)
        )
    )


async def list_input_assets(
    session: AsyncSession,
    batch_id: UUID,
) -> list[DraftInputAsset]:
    return list(
        await session.scalars(
            select(DraftInputAsset)
            .where(DraftInputAsset.batch_id == batch_id)
            .order_by(DraftInputAsset.position, DraftInputAsset.id)
        )
    )


async def list_drafts(
    session: AsyncSession,
    batch_id: UUID,
) -> list[DraftShot]:
    return list(
        await session.scalars(
            select(DraftShot)
            .where(DraftShot.batch_id == batch_id)
            .order_by(DraftShot.position, DraftShot.id)
        )
    )


async def list_draft_units(
    session: AsyncSession,
    draft_ids: list[UUID],
) -> list[DraftShotUnit]:
    if not draft_ids:
        return []
    return list(
        await session.scalars(
            select(DraftShotUnit)
            .where(DraftShotUnit.draft_shot_id.in_(draft_ids))
            .order_by(DraftShotUnit.draft_shot_id, DraftShotUnit.position)
        )
    )


async def list_draft_assets(
    session: AsyncSession,
    draft_ids: list[UUID],
) -> list[DraftAssetReference]:
    if not draft_ids:
        return []
    return list(
        await session.scalars(
            select(DraftAssetReference)
            .where(DraftAssetReference.draft_shot_id.in_(draft_ids))
            .order_by(DraftAssetReference.draft_shot_id, DraftAssetReference.slot_key)
        )
    )


async def list_decisions(
    session: AsyncSession,
    batch_id: UUID,
) -> list[DraftDecision]:
    return list(
        await session.scalars(
            select(DraftDecision)
            .where(DraftDecision.batch_id == batch_id)
            .order_by(DraftDecision.sequence, DraftDecision.id)
        )
    )


async def find_decision_by_key(
    session: AsyncSession,
    workspace_id: UUID,
    idempotency_key: str,
) -> DraftDecision | None:
    return await session.scalar(
        select(DraftDecision).where(
            DraftDecision.workspace_id == workspace_id,
            DraftDecision.idempotency_key == idempotency_key,
        )
    )


async def next_decision_sequence(session: AsyncSession, batch_id: UUID) -> int:
    return (
        await session.scalar(
            select(func.coalesce(func.max(DraftDecision.sequence), 0)).where(
                DraftDecision.batch_id == batch_id
            )
        )
        or 0
    ) + 1
