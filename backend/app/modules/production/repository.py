from typing import Literal
from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.production.models import Task
from app.modules.production.schemas import TaskStatus


async def find_task(
    session: AsyncSession,
    task_id: UUID,
    *,
    for_update: bool = False,
) -> Task | None:
    query = select(Task).where(Task.id == task_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_idempotent_task(
    session: AsyncSession,
    workspace_id: UUID,
    task_type: str,
    idempotency_key: str,
) -> Task | None:
    return await session.scalar(
        select(Task).where(
            Task.workspace_id == workspace_id,
            Task.task_type == task_type,
            Task.idempotency_key == idempotency_key,
        )
    )


async def list_tasks(
    session: AsyncSession,
    workspace_id: UUID,
    *,
    task_type: Literal["script_extraction"] | None,
    status: TaskStatus | None,
    limit: int,
    offset: int,
) -> tuple[list[Task], int]:
    filters = [Task.workspace_id == workspace_id]
    if task_type is not None:
        filters.append(Task.task_type == task_type)
    if status is not None:
        filters.append(Task.status == status)
    total = await session.scalar(select(func.count()).select_from(Task).where(*filters))
    rows = await session.scalars(
        select(Task)
        .where(*filters)
        .order_by(Task.created_at.desc(), Task.id.desc())
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0
