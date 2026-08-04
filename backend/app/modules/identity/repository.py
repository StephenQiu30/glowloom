from uuid import UUID

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.identity.models import Membership, UserAccount, Workspace


async def find_user_by_email(session: AsyncSession, email: str) -> UserAccount | None:
    return await session.scalar(
        select(UserAccount).where(UserAccount.email_normalized == email)
    )


async def find_user_by_id(session: AsyncSession, user_id: UUID) -> UserAccount | None:
    return await session.get(UserAccount, user_id)


async def find_primary_workspace(
    session: AsyncSession, user_id: UUID
) -> tuple[Workspace, Membership] | None:
    result = await session.execute(
        select(Workspace, Membership)
        .join(Membership, Membership.workspace_id == Workspace.id)
        .where(
            Membership.user_id == user_id,
            Membership.status == "active",
        )
        .order_by(Membership.joined_at, Membership.id)
        .limit(1)
    )
    row = result.one_or_none()
    return None if row is None else (row[0], row[1])


async def list_workspaces(
    session: AsyncSession,
    user_id: UUID,
    *,
    include_archived: bool,
) -> list[tuple[Workspace, Membership]]:
    query = (
        select(Workspace, Membership)
        .join(Membership, Membership.workspace_id == Workspace.id)
        .where(Membership.user_id == user_id, Membership.status == "active")
        .order_by(Workspace.created_at, Workspace.id)
    )
    if not include_archived:
        query = query.where(Workspace.status == "active")
    rows = await session.execute(query)
    return [(row[0], row[1]) for row in rows.all()]


async def find_workspace_for_user(
    session: AsyncSession,
    user_id: UUID,
    workspace_id: UUID,
    *,
    for_update: bool = False,
) -> tuple[Workspace, Membership] | None:
    query = (
        select(Workspace, Membership)
        .join(Membership, Membership.workspace_id == Workspace.id)
        .where(
            Workspace.id == workspace_id,
            Membership.user_id == user_id,
            Membership.status == "active",
        )
    )
    if for_update:
        query = query.with_for_update(of=Workspace)
    row = (await session.execute(query)).one_or_none()
    return None if row is None else (row[0], row[1])


async def find_membership_for_user(
    session: AsyncSession,
    user_id: UUID,
    workspace_id: UUID,
) -> Membership | None:
    return await session.scalar(
        select(Membership).where(
            Membership.workspace_id == workspace_id,
            Membership.user_id == user_id,
            Membership.status == "active",
        )
    )


async def find_workspace_by_id(
    session: AsyncSession,
    workspace_id: UUID,
) -> Workspace | None:
    return await session.get(Workspace, workspace_id)
