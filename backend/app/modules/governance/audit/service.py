from datetime import datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import repository
from app.modules.governance.audit.models import AuditEvent
from app.modules.governance.audit.schemas import (
    AuditEventResponse,
    PaginatedAuditEvents,
)
from app.modules.identity import Capability, actor_context


def _response(event: AuditEvent) -> AuditEventResponse:
    return AuditEventResponse(
        id=event.id,
        workspace_id=event.workspace_id,
        actor_id=event.actor_id,
        action=event.action,
        target_type=event.target_type,
        target_id=event.target_id,
        result=cast(Literal["succeeded", "denied", "failed"], event.result),
        trace_id=event.trace_id,
        metadata=event.event_metadata,
        occurred_at=event.occurred_at,
    )


async def list_audit_events(
    session: AsyncSession,
    claims: AccessTokenClaims,
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
) -> PaginatedAuditEvents:
    actor = await actor_context(
        session, claims, workspace_id, Capability.CONTENT_READ
    )
    if actor.role != "owner":
        raise ApiError(
            ErrorCode.FORBIDDEN,
            "Only workspace owners can query audit events",
            status_code=403,
        )
    events, total = await repository.list_audit_events(
        session,
        workspace_id,
        actor_id=actor_id,
        target_type=target_type,
        target_id=target_id,
        action=action,
        occurred_from=occurred_from,
        occurred_to=occurred_to,
        limit=limit,
        offset=offset,
    )
    return PaginatedAuditEvents(
        items=[_response(event) for event in events],
        total=total,
        limit=limit,
        offset=offset,
    )
