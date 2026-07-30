from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class MessageEnvelope(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    event_id: UUID
    event_type: str = Field(min_length=1, max_length=100)
    schema_version: int = Field(ge=1)
    aggregate_id: UUID
    workspace_id: UUID
    occurred_at: datetime
    trace_id: str = Field(min_length=1, max_length=64)
    causation_event_id: UUID | None
    payload: dict[str, str]


@dataclass(frozen=True, slots=True)
class OutboxEventCommand:
    workspace_id: UUID
    event_type: str
    schema_version: int
    aggregate_type: str
    aggregate_id: UUID
    routing_key: str
    payload: dict[str, str]
    trace_id: str
    available_at: datetime
    occurred_at: datetime
    causation_event_id: UUID | None = None
