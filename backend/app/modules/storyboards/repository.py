from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.storyboards.models import AssetReference, Shot, ShotSpecVersion


async def find_shot(
    session: AsyncSession,
    shot_id: UUID,
    *,
    for_update: bool = False,
) -> Shot | None:
    query = select(Shot).where(Shot.id == shot_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_shot_by_creation_key(
    session: AsyncSession,
    workspace_id: UUID,
    creation_key: str,
) -> Shot | None:
    return await session.scalar(
        select(Shot).where(
            Shot.workspace_id == workspace_id,
            Shot.creation_key == creation_key,
        )
    )


async def list_active_shots(
    session: AsyncSession,
    episode_id: UUID,
    *,
    for_update: bool = False,
) -> list[Shot]:
    query = (
        select(Shot)
        .where(Shot.episode_id == episode_id, Shot.status == "active")
        .order_by(Shot.position, Shot.id)
    )
    if for_update:
        query = query.with_for_update()
    return list(await session.scalars(query))


async def latest_spec_version_number(
    session: AsyncSession,
    shot_id: UUID,
) -> int:
    return (
        await session.scalar(
            select(func.coalesce(func.max(ShotSpecVersion.version_no), 0)).where(
                ShotSpecVersion.shot_id == shot_id
            )
        )
        or 0
    )


async def find_spec_version(
    session: AsyncSession,
    version_id: UUID,
) -> tuple[ShotSpecVersion, Shot] | None:
    row = (
        await session.execute(
            select(ShotSpecVersion, Shot)
            .join(Shot, Shot.id == ShotSpecVersion.shot_id)
            .where(ShotSpecVersion.id == version_id)
        )
    ).one_or_none()
    return None if row is None else (row[0], row[1])


async def list_spec_versions(
    session: AsyncSession,
    shot_id: UUID,
) -> list[ShotSpecVersion]:
    return list(
        await session.scalars(
            select(ShotSpecVersion)
            .where(ShotSpecVersion.shot_id == shot_id)
            .order_by(ShotSpecVersion.version_no, ShotSpecVersion.id)
        )
    )


async def list_asset_references(
    session: AsyncSession,
    spec_version_ids: list[UUID],
) -> list[AssetReference]:
    if not spec_version_ids:
        return []
    return list(
        await session.scalars(
            select(AssetReference)
            .where(AssetReference.shot_spec_version_id.in_(spec_version_ids))
            .order_by(AssetReference.shot_spec_version_id, AssetReference.slot_key)
        )
    )
