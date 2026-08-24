from datetime import datetime, timedelta
from uuid import UUID

from sqlalchemy import func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.scheduling.models import Schedule, ScheduleFire


async def current_database_time(session: AsyncSession) -> datetime:
    value = await session.scalar(select(func.current_timestamp()))
    if value is None:
        raise RuntimeError("database clock is unavailable")
    return value


async def find_schedule(
    session: AsyncSession,
    schedule_id: UUID,
    *,
    for_update: bool = False,
) -> Schedule | None:
    query = select(Schedule).where(Schedule.id == schedule_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_schedule_by_key(
    session: AsyncSession,
    workspace_id: UUID,
    schedule_key: str,
    *,
    for_update: bool = False,
) -> Schedule | None:
    query = select(Schedule).where(
        Schedule.workspace_id == workspace_id,
        Schedule.schedule_key == schedule_key,
    )
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def list_schedules(
    session: AsyncSession,
    workspace_id: UUID,
    *,
    status: str | None,
    limit: int,
    offset: int,
) -> tuple[list[Schedule], int]:
    filters = [Schedule.workspace_id == workspace_id]
    if status is not None:
        filters.append(Schedule.status == status)
    total = await session.scalar(select(func.count()).select_from(Schedule).where(*filters))
    rows = await session.scalars(
        select(Schedule)
        .where(*filters)
        .order_by(Schedule.created_at.desc(), Schedule.id.desc())
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


async def find_fire(
    session: AsyncSession,
    schedule_id: UUID,
    fire_key: str,
) -> ScheduleFire | None:
    return await session.scalar(
        select(ScheduleFire).where(
            ScheduleFire.schedule_id == schedule_id,
            ScheduleFire.fire_key == fire_key,
        )
    )


async def claim_due_schedules(
    session: AsyncSession,
    *,
    dispatcher_id: str,
    now: datetime,
    batch_size: int,
    lease_duration: timedelta,
) -> list[UUID]:
    rows = await session.scalars(
        select(Schedule)
        .where(
            Schedule.status == "active",
            Schedule.next_fire_at.is_not(None),
            Schedule.next_fire_at <= now,
            or_(Schedule.next_attempt_at.is_(None), Schedule.next_attempt_at <= now),
            or_(Schedule.lease_until.is_(None), Schedule.lease_until <= now),
        )
        .order_by(Schedule.next_fire_at, Schedule.id)
        .limit(batch_size)
        .with_for_update(skip_locked=True)
    )
    schedules = list(rows)
    for schedule in schedules:
        schedule.leased_by = dispatcher_id
        schedule.lease_until = now + lease_duration
        schedule.updated_at = now
    await session.flush()
    return [schedule.id for schedule in schedules]


async def find_claimed_schedule(
    session: AsyncSession,
    schedule_id: UUID,
    *,
    dispatcher_id: str,
) -> Schedule | None:
    return await session.scalar(
        select(Schedule)
        .where(
            Schedule.id == schedule_id,
            Schedule.leased_by == dispatcher_id,
        )
        .with_for_update()
    )
