from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator

from app.core.telemetry import is_valid_traceparent


class MessageEnvelope(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    event_id: UUID
    event_type: str = Field(min_length=1, max_length=100)
    schema_version: int = Field(ge=1)
    aggregate_id: UUID
    workspace_id: UUID
    occurred_at: datetime
    trace_id: str = Field(min_length=1, max_length=64)
    traceparent: str | None = Field(default=None, min_length=55, max_length=55)
    causation_event_id: UUID | None
    payload: dict[str, str]

    @field_validator("traceparent")
    @classmethod
    def validate_traceparent(cls, value: str | None) -> str | None:
        if value is not None and not is_valid_traceparent(value):
            raise ValueError("traceparent is invalid")
        return value


@dataclass(frozen=True, slots=True)
class OutboxEventCommand:
    workspace_id: UUID
    event_type: str
    schema_version: int
    aggregate_type: str
    aggregate_id: UUID
    topic: str
    payload: dict[str, str]
    trace_id: str
    available_at: datetime
    occurred_at: datetime
    causation_event_id: UUID | None = None
    traceparent: str | None = None
