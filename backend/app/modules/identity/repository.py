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
