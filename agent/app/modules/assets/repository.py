from uuid import UUID

from sqlalchemy import func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.assets.models import (
    Asset,
    AssetMediaReference,
    AssetNameRevision,
    AssetOccurrenceDecision,
    AssetState,
    AssetVersion,
)


async def find_asset(
    session: AsyncSession,
    asset_id: UUID,
    *,
    for_update: bool = False,
) -> Asset | None:
    query = select(Asset).where(Asset.id == asset_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_state(
    session: AsyncSession,
    state_id: UUID,
    *,
    for_update: bool = False,
) -> AssetState | None:
    query = select(AssetState).where(AssetState.id == state_id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return await session.scalar(query)


async def find_state_scopes(
    session: AsyncSession,
    state_ids: list[UUID],
    *,
    for_update: bool = False,
) -> list[tuple[AssetState, Asset]]:
    if not state_ids:
        return []
    query = (
        select(AssetState, Asset)
        .join(Asset, Asset.id == AssetState.asset_id)
        .where(AssetState.id.in_(state_ids))
    )
    if for_update:
        query = query.with_for_update(of=AssetState)
    rows = await session.execute(query)
    return [(row[0], row[1]) for row in rows]


async def find_state_by_creation_key(
    session: AsyncSession,
    asset_id: UUID,
    creation_key: str,
) -> AssetState | None:
    return await session.scalar(
        select(AssetState).where(
            AssetState.asset_id == asset_id,
            AssetState.creation_key == creation_key,
        )
    )


async def find_state_by_key(
    session: AsyncSession,
    asset_id: UUID,
    state_key: str,
) -> AssetState | None:
    return await session.scalar(
        select(AssetState).where(
            AssetState.asset_id == asset_id,
            AssetState.state_key == state_key,
        )
    )


async def list_states(
    session: AsyncSession,
    asset_id: UUID,
) -> list[AssetState]:
    return list(
        await session.scalars(
            select(AssetState)
            .where(AssetState.asset_id == asset_id)
            .order_by(
                (AssetState.state_key != "base"),
                AssetState.created_at,
                AssetState.id,
            )
        )
    )


async def list_states_for_assets(
    session: AsyncSession,
    asset_ids: list[UUID],
) -> list[AssetState]:
    if not asset_ids:
        return []
    return list(
        await session.scalars(
            select(AssetState)
            .where(AssetState.asset_id.in_(asset_ids))
            .order_by(
                AssetState.asset_id,
                (AssetState.state_key != "base"),
                AssetState.created_at,
                AssetState.id,
            )
        )
    )


async def find_duplicate_name(
    session: AsyncSession,
    project_id: UUID,
    kind: str,
    normalized_name: str,
    *,
    excluding_id: UUID | None = None,
) -> Asset | None:
    filters = [
        Asset.project_id == project_id,
        Asset.kind == kind,
        Asset.normalized_name == normalized_name,
    ]
    if excluding_id is not None:
        filters.append(Asset.id != excluding_id)
    return await session.scalar(select(Asset).where(*filters).limit(1))


async def list_assets(
    session: AsyncSession,
    project_id: UUID,
    *,
    kind: str | None,
    include_archived: bool,
    query: str | None,
    limit: int,
    offset: int,
) -> tuple[list[Asset], int]:
    filters = [Asset.project_id == project_id]
    if kind is not None:
        filters.append(Asset.kind == kind)
    if not include_archived:
        filters.append(Asset.status == "active")
    if query:
        pattern = f"%{query.strip()}%"
        filters.append(or_(Asset.name.ilike(pattern), Asset.normalized_name.ilike(pattern)))
    total = await session.scalar(select(func.count()).select_from(Asset).where(*filters))
    rows = await session.scalars(
        select(Asset)
        .where(*filters)
        .order_by(Asset.updated_at.desc(), Asset.id)
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


async def list_project_assets(
    session: AsyncSession,
    project_id: UUID,
) -> list[Asset]:
    return list(
        await session.scalars(
            select(Asset)
            .where(Asset.project_id == project_id, Asset.status == "active")
            .order_by(Asset.kind, Asset.created_at, Asset.id)
        )
    )


async def list_active_states_with_current_version(
    session: AsyncSession,
    project_id: UUID,
) -> tuple[list[tuple[Asset, AssetState, AssetVersion]], int]:
    total = await session.scalar(
        select(func.count())
        .select_from(Asset)
        .where(Asset.project_id == project_id, Asset.status == "active")
    )
    rows = await session.execute(
        select(Asset, AssetState, AssetVersion)
        .join(AssetState, AssetState.asset_id == Asset.id)
        .join(AssetVersion, AssetVersion.id == AssetState.current_version_id)
        .where(Asset.project_id == project_id, Asset.status == "active")
        .order_by(Asset.kind, Asset.id, AssetState.state_key)
    )
    return [(asset, state, version) for asset, state, version in rows], total or 0


async def count_asset_references_by_project(
    session: AsyncSession,
    workspace_id: UUID,
    project_ids: list[UUID],
) -> list[tuple[UUID, int, int]]:
    if not project_ids:
        return []
    rows = await session.execute(
        select(
            Asset.project_id,
            func.count(func.distinct(Asset.id)),
            func.count(AssetVersion.id),
        )
        .outerjoin(AssetVersion, AssetVersion.asset_id == Asset.id)
        .where(
            Asset.workspace_id == workspace_id,
            Asset.project_id.in_(project_ids),
        )
        .group_by(Asset.project_id)
    )
    return [
        (project_id, asset_count, version_count) for project_id, asset_count, version_count in rows
    ]


async def count_related_asset_versions(
    session: AsyncSession,
    workspace_id: UUID,
    asset_ids: list[UUID],
) -> dict[UUID, int]:
    if not asset_ids:
        return {}
    reference_ids = [str(asset_id) for asset_id in asset_ids]
    holder_id = AssetVersion.spec["holder_character_id"].astext
    wearer_id = AssetVersion.spec["wearer_character_id"].astext
    related_id = func.coalesce(holder_id, wearer_id)
    rows = await session.execute(
        select(related_id, func.count())
        .where(
            AssetVersion.workspace_id == workspace_id,
            or_(
                holder_id.in_(reference_ids),
                wearer_id.in_(reference_ids),
            ),
        )
        .group_by(related_id)
    )
    return {
        UUID(related_asset_id): count
        for related_asset_id, count in rows
        if related_asset_id is not None
    }


async def find_version(
    session: AsyncSession,
    version_id: UUID,
) -> tuple[AssetVersion, AssetState, Asset] | None:
    row = (
        await session.execute(
            select(AssetVersion, AssetState, Asset)
            .join(AssetState, AssetState.id == AssetVersion.asset_state_id)
            .join(Asset, Asset.id == AssetVersion.asset_id)
            .where(AssetVersion.id == version_id)
        )
    ).one_or_none()
    return None if row is None else (row[0], row[1], row[2])


async def find_versions(
    session: AsyncSession,
    version_ids: list[UUID],
) -> list[tuple[AssetVersion, AssetState, Asset]]:
    if not version_ids:
        return []
    rows = await session.execute(
        select(AssetVersion, AssetState, Asset)
        .join(AssetState, AssetState.id == AssetVersion.asset_state_id)
        .join(Asset, Asset.id == AssetVersion.asset_id)
        .where(AssetVersion.id.in_(version_ids))
    )
    by_id = {row[0].id: (row[0], row[1], row[2]) for row in rows}
    return [by_id[version_id] for version_id in version_ids if version_id in by_id]


async def list_versions(
    session: AsyncSession,
    state_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[AssetVersion], int]:
    total = await session.scalar(
        select(func.count())
        .select_from(AssetVersion)
        .where(AssetVersion.asset_state_id == state_id)
    )
    rows = await session.scalars(
        select(AssetVersion)
        .where(AssetVersion.asset_state_id == state_id)
        .order_by(AssetVersion.version_no.desc())
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


async def list_change_versions(
    session: AsyncSession,
    *,
    asset_id: UUID,
    state_id: UUID | None = None,
) -> list[AssetVersion]:
    filters = [AssetVersion.asset_id == asset_id]
    if state_id is not None:
        filters.append(AssetVersion.asset_state_id == state_id)
    return list(
        await session.scalars(
            select(AssetVersion).where(*filters).order_by(AssetVersion.version_no, AssetVersion.id)
        )
    )


async def list_name_revisions(
    session: AsyncSession,
    asset_id: UUID,
) -> list[AssetNameRevision]:
    return list(
        await session.scalars(
            select(AssetNameRevision)
            .where(AssetNameRevision.asset_id == asset_id)
            .order_by(AssetNameRevision.revision_no)
        )
    )


async def latest_version_number(session: AsyncSession, asset_id: UUID) -> int:
    return (
        await session.scalar(
            select(func.coalesce(func.max(AssetVersion.version_no), 0)).where(
                AssetVersion.asset_id == asset_id
            )
        )
        or 0
    )


async def list_media_references(
    session: AsyncSession,
    version_ids: list[UUID],
) -> list[AssetMediaReference]:
    if not version_ids:
        return []
    return list(
        await session.scalars(
            select(AssetMediaReference)
            .where(AssetMediaReference.asset_version_id.in_(version_ids))
            .order_by(
                AssetMediaReference.asset_version_id,
                AssetMediaReference.purpose,
                AssetMediaReference.position,
            )
        )
    )


async def count_versions(session: AsyncSession, asset_id: UUID) -> int:
    return (
        await session.scalar(
            select(func.count()).select_from(AssetVersion).where(AssetVersion.asset_id == asset_id)
        )
        or 0
    )


async def delete_states(session: AsyncSession, asset_id: UUID) -> None:
    for state in await list_states(session, asset_id):
        await session.delete(state)


async def find_occurrence_by_key(
    session: AsyncSession,
    state_id: UUID,
    idempotency_key: str,
) -> AssetOccurrenceDecision | None:
    return await session.scalar(
        select(AssetOccurrenceDecision).where(
            AssetOccurrenceDecision.asset_state_id == state_id,
            AssetOccurrenceDecision.idempotency_key == idempotency_key,
        )
    )


async def latest_occurrence_sequence(
    session: AsyncSession,
    state_id: UUID,
) -> int:
    return (
        await session.scalar(
            select(func.coalesce(func.max(AssetOccurrenceDecision.sequence), 0)).where(
                AssetOccurrenceDecision.asset_state_id == state_id
            )
        )
        or 0
    )


async def list_occurrence_decisions(
    session: AsyncSession,
    state_ids: list[UUID],
) -> list[AssetOccurrenceDecision]:
    if not state_ids:
        return []
    return list(
        await session.scalars(
            select(AssetOccurrenceDecision)
            .where(AssetOccurrenceDecision.asset_state_id.in_(state_ids))
            .order_by(
                AssetOccurrenceDecision.asset_state_id,
                AssetOccurrenceDecision.sequence,
            )
        )
    )


async def list_episode_occurrence_decisions(
    session: AsyncSession,
    episode_id: UUID,
) -> list[AssetOccurrenceDecision]:
    return list(
        await session.scalars(
            select(AssetOccurrenceDecision)
            .where(AssetOccurrenceDecision.episode_id == episode_id)
            .order_by(
                AssetOccurrenceDecision.asset_state_id,
                AssetOccurrenceDecision.narrative_unit_id,
                AssetOccurrenceDecision.sequence,
            )
        )
    )


async def find_candidate_version(
    session: AsyncSession,
    candidate_id: UUID,
) -> AssetVersion | None:
    return await session.scalar(
        select(AssetVersion).where(
            AssetVersion.source_type == "script_extraction_candidate",
            AssetVersion.source_id == candidate_id,
        )
    )
