from datetime import datetime
from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.governance.audit import service
from app.modules.governance.audit.schemas import PaginatedAuditEvents

router = APIRouter()


@router.get("/audit-events", response_model=ApiResponse[PaginatedAuditEvents])
async def list_audit_events(
    workspace_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    actor_id: UUID | None = None,
    target_type: Annotated[str | None, Query(min_length=1, max_length=60)] = None,
    target_id: UUID | None = None,
    action: Annotated[str | None, Query(min_length=1, max_length=80)] = None,
    occurred_from: datetime | None = None,
    occurred_to: datetime | None = None,
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedAuditEvents]:
    return ApiResponse(
        data=await service.list_audit_events(
            session,
            claims,
            workspace_id,
            actor_id=actor_id,
            target_type=target_type,
            target_id=target_id,
            action=action,
            occurred_from=occurred_from,
            occurred_to=occurred_to,
            limit=limit or 20,
            offset=offset,
        )
    )
