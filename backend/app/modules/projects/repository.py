from typing import Literal
from uuid import UUID

from sqlalchemy import func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.identity.models import Membership
from app.modules.projects.models import Project


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
