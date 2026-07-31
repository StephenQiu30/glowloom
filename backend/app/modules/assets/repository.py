from uuid import UUID

from sqlalchemy import func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.assets.models import Asset, AssetMediaReference, AssetVersion


async def find_asset(
    session: AsyncSession,
    asset_id: UUID,
    *,
    for_update: bool = False,
) -> Asset | None:
    query = select(Asset).where(Asset.id == asset_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


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
    total = await session.scalar(
        select(func.count()).select_from(Asset).where(*filters)
    )
    rows = await session.scalars(
        select(Asset)
        .where(*filters)
        .order_by(Asset.updated_at.desc(), Asset.id)
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


async def list_active_assets_with_current_version(
    session: AsyncSession,
    project_id: UUID,
) -> tuple[list[tuple[Asset, AssetVersion]], int]:
    total = await session.scalar(
        select(func.count())
        .select_from(Asset)
        .where(Asset.project_id == project_id, Asset.status == "active")
    )
    rows = await session.execute(
        select(Asset, AssetVersion)
        .join(AssetVersion, AssetVersion.id == Asset.current_version_id)
        .where(Asset.project_id == project_id, Asset.status == "active")
        .order_by(Asset.kind, Asset.id)
    )
    return [(asset, version) for asset, version in rows], total or 0


async def find_version(
    session: AsyncSession,
    version_id: UUID,
) -> tuple[AssetVersion, Asset] | None:
    row = (
        await session.execute(
            select(AssetVersion, Asset)
            .join(Asset, Asset.id == AssetVersion.asset_id)
            .where(AssetVersion.id == version_id)
        )
    ).one_or_none()
    return None if row is None else (row[0], row[1])


async def find_versions(
    session: AsyncSession,
    version_ids: list[UUID],
) -> list[tuple[AssetVersion, Asset]]:
    if not version_ids:
        return []
    rows = await session.execute(
        select(AssetVersion, Asset)
        .join(Asset, Asset.id == AssetVersion.asset_id)
        .where(AssetVersion.id.in_(version_ids))
    )
    by_id = {row[0].id: (row[0], row[1]) for row in rows}
    return [by_id[version_id] for version_id in version_ids if version_id in by_id]


async def list_versions(
    session: AsyncSession,
    asset_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[AssetVersion], int]:
    total = await session.scalar(
        select(func.count())
        .select_from(AssetVersion)
        .where(AssetVersion.asset_id == asset_id)
    )
    rows = await session.scalars(
        select(AssetVersion)
        .where(AssetVersion.asset_id == asset_id)
        .order_by(AssetVersion.version_no.desc())
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


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
            select(func.count())
            .select_from(AssetVersion)
            .where(AssetVersion.asset_id == asset_id)
        )
        or 0
    )


async def find_candidate_version(
    session: AsyncSession,
    candidate_id: UUID,
) -> AssetVersion | None:
    return await session.scalar(
        select(AssetVersion).where(
            AssetVersion.source_type == "candidate",
            AssetVersion.source_id == candidate_id,
        )
    )
