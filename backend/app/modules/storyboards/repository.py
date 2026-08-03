from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.storyboards.models import (
    AssetReference,
    Shot,
    ShotSpecVersion,
    ShotTransform,
)


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


async def find_shot_by_candidate(
    session: AsyncSession,
    workspace_id: UUID,
    candidate_id: UUID,
) -> Shot | None:
    return await session.scalar(
        select(Shot).where(
            Shot.workspace_id == workspace_id,
            Shot.source_candidate_id == candidate_id,
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


async def list_archived_shots(
    session: AsyncSession,
    episode_id: UUID,
) -> list[Shot]:
    return list(
        await session.scalars(
            select(Shot)
            .where(Shot.episode_id == episode_id, Shot.status == "archived")
            .order_by(Shot.updated_at.desc(), Shot.id.desc())
        )
    )


async def count_storyboard_references_by_episode(
    session: AsyncSession,
    workspace_id: UUID,
    episode_ids: list[UUID],
) -> list[tuple[UUID, int, int]]:
    if not episode_ids:
        return []
    rows = await session.execute(
        select(
            Shot.episode_id,
            func.count(func.distinct(Shot.id)),
            func.count(ShotSpecVersion.id),
        )
        .outerjoin(ShotSpecVersion, ShotSpecVersion.shot_id == Shot.id)
        .where(
            Shot.workspace_id == workspace_id,
            Shot.episode_id.in_(episode_ids),
        )
        .group_by(Shot.episode_id)
    )
    return [
        (episode_id, shot_count, spec_version_count)
        for episode_id, shot_count, spec_version_count in rows
    ]


async def list_active_shot_ids_not_using_script_version(
    session: AsyncSession,
    episode_id: UUID,
    current_script_version_id: UUID,
) -> list[UUID]:
    return list(
        await session.scalars(
            select(Shot.id)
            .where(
                Shot.episode_id == episode_id,
                Shot.status == "active",
                Shot.source_script_version_id != current_script_version_id,
            )
            .order_by(Shot.position, Shot.id)
        )
    )


async def list_active_shots_with_current_specs(
    session: AsyncSession,
    episode_id: UUID,
) -> list[tuple[Shot, ShotSpecVersion | None]]:
    return await list_active_shots_with_current_specs_for_episodes(
        session,
        workspace_id=None,
        episode_ids=[episode_id],
    )


async def list_active_shots_with_current_specs_for_episodes(
    session: AsyncSession,
    *,
    workspace_id: UUID | None,
    episode_ids: list[UUID],
) -> list[tuple[Shot, ShotSpecVersion | None]]:
    if not episode_ids:
        return []
    conditions = [Shot.episode_id.in_(episode_ids), Shot.status == "active"]
    if workspace_id is not None:
        conditions.append(Shot.workspace_id == workspace_id)
    rows = await session.execute(
        select(Shot, ShotSpecVersion)
        .outerjoin(
            ShotSpecVersion,
            (ShotSpecVersion.id == Shot.current_spec_version_id)
            & (ShotSpecVersion.workspace_id == Shot.workspace_id),
        )
        .where(*conditions)
        .order_by(Shot.episode_id, Shot.position, Shot.id)
    )
    return [(row[0], row[1]) for row in rows]


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


async def count_spec_versions(session: AsyncSession, shot_id: UUID) -> int:
    return (
        await session.scalar(
            select(func.count())
            .select_from(ShotSpecVersion)
            .where(ShotSpecVersion.shot_id == shot_id)
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


async def find_transform_by_idempotency(
    session: AsyncSession,
    workspace_id: UUID,
    idempotency_key: str,
) -> ShotTransform | None:
    return await session.scalar(
        select(ShotTransform).where(
            ShotTransform.workspace_id == workspace_id,
            ShotTransform.idempotency_key == idempotency_key,
        )
    )


async def find_shots(
    session: AsyncSession,
    shot_ids: list[UUID],
    *,
    for_update: bool = False,
) -> list[Shot]:
    if not shot_ids:
        return []
    query = select(Shot).where(Shot.id.in_(shot_ids))
    if for_update:
        query = query.with_for_update()
    rows = {shot.id: shot for shot in await session.scalars(query)}
    return [rows[shot_id] for shot_id in shot_ids if shot_id in rows]


async def find_shots_with_current_specs(
    session: AsyncSession,
    shot_ids: list[UUID],
    *,
    for_update: bool = False,
) -> list[tuple[Shot, ShotSpecVersion | None]]:
    if not shot_ids:
        return []
    query = (
        select(Shot, ShotSpecVersion)
        .outerjoin(
            ShotSpecVersion,
            (ShotSpecVersion.id == Shot.current_spec_version_id)
            & (ShotSpecVersion.workspace_id == Shot.workspace_id),
        )
        .where(Shot.id.in_(shot_ids))
    )
    if for_update:
        query = query.with_for_update(of=Shot)
    rows = {
        row[0].id: (row[0], row[1]) for row in await session.execute(query)
    }
    return [rows[shot_id] for shot_id in shot_ids if shot_id in rows]


async def latest_spec_version_numbers(
    session: AsyncSession,
    shot_ids: list[UUID],
) -> dict[UUID, int]:
    if not shot_ids:
        return {}
    rows = await session.execute(
        select(
            ShotSpecVersion.shot_id,
            func.max(ShotSpecVersion.version_no),
        )
        .where(ShotSpecVersion.shot_id.in_(shot_ids))
        .group_by(ShotSpecVersion.shot_id)
    )
    return {row[0]: row[1] for row in rows}


async def list_asset_version_usages(
    session: AsyncSession,
    asset_version_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[tuple[ShotSpecVersion, Shot, list[str]]], int]:
    version_ids_query = (
        select(ShotSpecVersion.id)
        .join(
            AssetReference,
            AssetReference.shot_spec_version_id == ShotSpecVersion.id,
        )
        .where(AssetReference.asset_version_id == asset_version_id)
        .distinct()
    )
    total = await session.scalar(
        select(func.count()).select_from(version_ids_query.subquery())
    )
    version_ids = list(
        await session.scalars(
            version_ids_query.order_by(ShotSpecVersion.id).limit(limit).offset(offset)
        )
    )
    if not version_ids:
        return [], total or 0
    rows = await session.execute(
        select(ShotSpecVersion, Shot)
        .join(Shot, Shot.id == ShotSpecVersion.shot_id)
        .where(ShotSpecVersion.id.in_(version_ids))
    )
    by_version = {row[0].id: (row[0], row[1]) for row in rows}
    references = await list_asset_references(session, version_ids)
    slot_keys_by_version: dict[UUID, list[str]] = {}
    for reference in references:
        if reference.asset_version_id == asset_version_id:
            slot_keys_by_version.setdefault(
                reference.shot_spec_version_id,
                [],
            ).append(reference.slot_key)
    return [
        (
            by_version[version_id][0],
            by_version[version_id][1],
            slot_keys_by_version.get(version_id, []),
        )
        for version_id in version_ids
    ], total or 0
