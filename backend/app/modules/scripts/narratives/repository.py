from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.scripts.narratives.models import (
    NarrativeImpactAssessment,
    NarrativeStructure,
    NarrativeUnit,
    NarrativeUnitVersion,
)


async def find_structure_by_script(
    session: AsyncSession,
    script_version_id: UUID,
    *,
    for_update: bool = False,
) -> NarrativeStructure | None:
    query = select(NarrativeStructure).where(
        NarrativeStructure.script_version_id == script_version_id
    )
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_structure(
    session: AsyncSession,
    structure_id: UUID,
    *,
    for_update: bool = False,
) -> NarrativeStructure | None:
    query = select(NarrativeStructure).where(NarrativeStructure.id == structure_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def list_structures_by_scripts(
    session: AsyncSession,
    workspace_id: UUID,
    script_version_ids: list[UUID],
) -> list[NarrativeStructure]:
    if not script_version_ids:
        return []
    return list(
        await session.scalars(
            select(NarrativeStructure).where(
                NarrativeStructure.workspace_id == workspace_id,
                NarrativeStructure.script_version_id.in_(script_version_ids),
            )
        )
    )


async def list_versions(
    session: AsyncSession,
    structure_id: UUID,
    revision: int,
) -> list[NarrativeUnitVersion]:
    return list(
        await session.scalars(
            select(NarrativeUnitVersion)
            .where(
                NarrativeUnitVersion.structure_id == structure_id,
                NarrativeUnitVersion.structure_revision == revision,
            )
            .order_by(NarrativeUnitVersion.position, NarrativeUnitVersion.id)
        )
    )


async def find_units(
    session: AsyncSession,
    unit_ids: list[UUID],
    *,
    for_update: bool = False,
) -> list[NarrativeUnit]:
    if not unit_ids:
        return []
    query = select(NarrativeUnit).where(NarrativeUnit.id.in_(unit_ids))
    if for_update:
        query = query.with_for_update()
    return list(await session.scalars(query))


async def next_impact_sequence(session: AsyncSession, episode_id: UUID) -> int:
    return (
        await session.scalar(
            select(func.coalesce(func.max(NarrativeImpactAssessment.sequence), 0)).where(
                NarrativeImpactAssessment.episode_id == episode_id
            )
        )
        or 0
    ) + 1


async def latest_impact(
    session: AsyncSession,
    episode_id: UUID,
) -> NarrativeImpactAssessment | None:
    return await session.scalar(
        select(NarrativeImpactAssessment)
        .where(NarrativeImpactAssessment.episode_id == episode_id)
        .order_by(
            NarrativeImpactAssessment.sequence.desc(),
            NarrativeImpactAssessment.id.desc(),
        )
        .limit(1)
    )
