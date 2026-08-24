from datetime import datetime
from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.governance.audit.models import AuditEvent


async def list_audit_events(
    session: AsyncSession,
    workspace_id: UUID,
    *,
    actor_id: UUID | None,
    target_type: str | None,
    target_id: UUID | None,
    action: str | None,
    occurred_from: datetime | None,
    occurred_to: datetime | None,
    limit: int,
    offset: int,
) -> tuple[list[AuditEvent], int]:
    filters = [AuditEvent.workspace_id == workspace_id]
    if actor_id is not None:
        filters.append(AuditEvent.actor_id == actor_id)
    if target_type is not None:
        filters.append(AuditEvent.target_type == target_type)
    if target_id is not None:
        filters.append(AuditEvent.target_id == target_id)
    if action is not None:
        filters.append(AuditEvent.action == action)
    if occurred_from is not None:
        filters.append(AuditEvent.occurred_at >= occurred_from)
    if occurred_to is not None:
        filters.append(AuditEvent.occurred_at <= occurred_to)
    total = await session.scalar(select(func.count()).select_from(AuditEvent).where(*filters))
    events = list(
        await session.scalars(
            select(AuditEvent)
            .where(*filters)
            .order_by(AuditEvent.occurred_at.desc(), AuditEvent.id.desc())
            .limit(limit)
            .offset(offset)
        )
    )
    return events, total or 0
