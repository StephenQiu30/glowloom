from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.scripts.production_bibles.models import (
    ProductionBible,
    ProductionBibleEntity,
    ProductionBibleEntityState,
    ProductionBibleWorldEntry,
)


async def find_bible(
    session: AsyncSession,
    bible_id: UUID,
    *,
    for_update: bool = False,
) -> ProductionBible | None:
    query = select(ProductionBible).where(ProductionBible.id == bible_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def lock_bible(session: AsyncSession, bible_id: UUID) -> ProductionBible | None:
    return await find_bible(session, bible_id, for_update=True)


async def find_bible_by_idempotency(
    session: AsyncSession,
    document_revision_id: UUID,
    idempotency_key: str,
    *,
    for_update: bool = False,
) -> ProductionBible | None:
    query = select(ProductionBible).where(
        ProductionBible.document_revision_id == document_revision_id,
        ProductionBible.idempotency_key == idempotency_key,
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_bible_by_confirm_idempotency(
    session: AsyncSession,
    workspace_id: UUID,
    idempotency_key: str,
    *,
    for_update: bool = False,
) -> ProductionBible | None:
    query = select(ProductionBible).where(
        ProductionBible.workspace_id == workspace_id,
        ProductionBible.confirm_idempotency_key == idempotency_key,
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_current_confirmed_bible(
    session: AsyncSession,
    project_id: UUID,
    *,
    for_update: bool = False,
) -> ProductionBible | None:
    query = (
        select(ProductionBible)
        .where(
            ProductionBible.project_id == project_id,
            ProductionBible.status == "confirmed",
        )
        .order_by(ProductionBible.confirmed_at.desc(), ProductionBible.id.desc())
        .limit(1)
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_confirmed_bible_for_revision(
    session: AsyncSession,
    project_id: UUID,
    document_revision_id: UUID,
    *,
    for_update: bool = False,
) -> ProductionBible | None:
    query = select(ProductionBible).where(
        ProductionBible.project_id == project_id,
        ProductionBible.document_revision_id == document_revision_id,
        ProductionBible.status == "confirmed",
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def list_bibles(
    session: AsyncSession,
    project_id: UUID,
    *,
    status: str | None = None,
    limit: int = 20,
    offset: int = 0,
) -> tuple[list[ProductionBible], int]:
    filters = [ProductionBible.project_id == project_id]
    if status is not None:
        filters.append(ProductionBible.status == status)
    total = await session.scalar(select(func.count()).select_from(ProductionBible).where(*filters))
    rows = await session.scalars(
        select(ProductionBible)
        .where(*filters)
        .order_by(ProductionBible.created_at.desc(), ProductionBible.id.desc())
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


async def find_entity(
    session: AsyncSession,
    entity_id: UUID,
    *,
    for_update: bool = False,
) -> ProductionBibleEntity | None:
    query = select(ProductionBibleEntity).where(ProductionBibleEntity.id == entity_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_entity_by_key(
    session: AsyncSession,
    bible_id: UUID,
    entity_key: str,
    *,
    for_update: bool = False,
) -> ProductionBibleEntity | None:
    query = select(ProductionBibleEntity).where(
        ProductionBibleEntity.bible_id == bible_id,
        ProductionBibleEntity.entity_key == entity_key,
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def list_entities(
    session: AsyncSession,
    bible_id: UUID,
    *,
    for_update: bool = False,
) -> list[ProductionBibleEntity]:
    query = (
        select(ProductionBibleEntity)
        .where(ProductionBibleEntity.bible_id == bible_id)
        .order_by(
            ProductionBibleEntity.kind,
            ProductionBibleEntity.normalized_name,
            ProductionBibleEntity.entity_key,
        )
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return list(await session.scalars(query))


async def find_entity_state(
    session: AsyncSession,
    state_id: UUID,
    *,
    for_update: bool = False,
) -> ProductionBibleEntityState | None:
    query = select(ProductionBibleEntityState).where(ProductionBibleEntityState.id == state_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_entity_state_by_key(
    session: AsyncSession,
    entity_id: UUID,
    state_key: str,
    *,
    for_update: bool = False,
) -> ProductionBibleEntityState | None:
    query = select(ProductionBibleEntityState).where(
        ProductionBibleEntityState.entity_id == entity_id,
        ProductionBibleEntityState.state_key == state_key,
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def list_entity_states(
    session: AsyncSession,
    bible_id: UUID,
    *,
    entity_ids: list[UUID] | None = None,
    for_update: bool = False,
) -> list[ProductionBibleEntityState]:
    filters = [ProductionBibleEntityState.bible_id == bible_id]
    if entity_ids is not None:
        if not entity_ids:
            return []
        filters.append(ProductionBibleEntityState.entity_id.in_(entity_ids))
    query = (
        select(ProductionBibleEntityState)
        .where(*filters)
        .order_by(
            ProductionBibleEntityState.entity_id,
            ProductionBibleEntityState.state_key,
        )
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return list(await session.scalars(query))


async def find_world_entry(
    session: AsyncSession,
    entry_id: UUID,
    *,
    for_update: bool = False,
) -> ProductionBibleWorldEntry | None:
    query = select(ProductionBibleWorldEntry).where(ProductionBibleWorldEntry.id == entry_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_world_entry_by_key(
    session: AsyncSession,
    bible_id: UUID,
    entry_key: str,
    *,
    for_update: bool = False,
) -> ProductionBibleWorldEntry | None:
    query = select(ProductionBibleWorldEntry).where(
        ProductionBibleWorldEntry.bible_id == bible_id,
        ProductionBibleWorldEntry.entry_key == entry_key,
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def list_world_entries(
    session: AsyncSession,
    bible_id: UUID,
    *,
    for_update: bool = False,
) -> list[ProductionBibleWorldEntry]:
    query = (
        select(ProductionBibleWorldEntry)
        .where(ProductionBibleWorldEntry.bible_id == bible_id)
        .order_by(
            ProductionBibleWorldEntry.category,
            ProductionBibleWorldEntry.entry_key,
        )
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return list(await session.scalars(query))
