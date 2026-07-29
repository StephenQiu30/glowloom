from typing import Literal
from uuid import UUID

from sqlalchemy import func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.identity.models import Membership
from app.modules.projects.models import Episode, Project


async def list_projects(
    session: AsyncSession,
    workspace_id: UUID,
    *,
    include_archived: bool,
    search: str | None,
    sort: Literal["name", "created_at", "updated_at"],
    order: Literal["asc", "desc"],
    limit: int,
    offset: int,
) -> tuple[list[Project], int]:
    filters = [Project.workspace_id == workspace_id]
    if not include_archived:
        filters.append(Project.status == "active")
    if search:
        pattern = f"%{search}%"
        filters.append(or_(Project.name.ilike(pattern), Project.description.ilike(pattern)))
    total = await session.scalar(select(func.count()).select_from(Project).where(*filters))
    sort_column = {
        "name": Project.name,
        "created_at": Project.created_at,
        "updated_at": Project.updated_at,
    }[sort]
    direction = sort_column.asc() if order == "asc" else sort_column.desc()
    rows = await session.scalars(
        select(Project)
        .where(*filters)
        .order_by(direction, Project.id)
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


async def find_project_for_user(
    session: AsyncSession,
    user_id: UUID,
    project_id: UUID,
    *,
    for_update: bool = False,
) -> tuple[Project, Membership] | None:
    query = (
        select(Project, Membership)
        .join(Membership, Membership.workspace_id == Project.workspace_id)
        .where(
            Project.id == project_id,
            Membership.user_id == user_id,
            Membership.status == "active",
        )
    )
    if for_update:
        query = query.with_for_update(of=Project)
    row = (await session.execute(query)).one_or_none()
    return None if row is None else (row[0], row[1])


async def count_episodes(session: AsyncSession, project_id: UUID) -> int:
    count = await session.scalar(
        select(func.count()).select_from(Episode).where(Episode.project_id == project_id)
    )
    return count or 0


async def next_episode_position(session: AsyncSession, project_id: UUID) -> int:
    current = await session.scalar(
        select(func.max(Episode.position)).where(
            Episode.project_id == project_id,
            Episode.status == "active",
        )
    )
    return (current or 0) + 1


async def list_episodes(
    session: AsyncSession,
    project_id: UUID,
    *,
    include_archived: bool,
    for_update: bool = False,
) -> list[Episode]:
    query = select(Episode).where(Episode.project_id == project_id)
    if not include_archived:
        query = query.where(Episode.status == "active")
    query = query.order_by(Episode.position, Episode.id)
    if for_update:
        query = query.with_for_update()
    return list(await session.scalars(query))


async def find_episode_for_user(
    session: AsyncSession,
    user_id: UUID,
    episode_id: UUID,
    *,
    for_update: bool = False,
) -> tuple[Episode, Project, Membership] | None:
    query = (
        select(Episode, Project, Membership)
        .join(
            Project,
            (Project.id == Episode.project_id)
            & (Project.workspace_id == Episode.workspace_id),
        )
        .join(Membership, Membership.workspace_id == Episode.workspace_id)
        .where(
            Episode.id == episode_id,
            Membership.user_id == user_id,
            Membership.status == "active",
        )
    )
    if for_update:
        query = query.with_for_update(of=(Episode, Project))
    row = (await session.execute(query)).one_or_none()
    return None if row is None else (row[0], row[1], row[2])
