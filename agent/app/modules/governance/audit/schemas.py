from datetime import datetime
from typing import Any, Literal
from uuid import UUID

from pydantic import BaseModel


class AuditEventResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    actor_id: UUID
    action: str
    target_type: str
    target_id: UUID
    result: Literal["succeeded", "denied", "failed"]
    trace_id: str
    metadata: dict[str, Any]
    occurred_at: datetime


class PaginatedAuditEvents(BaseModel):
    items: list[AuditEventResponse]
    total: int
    limit: int
    offset: int
