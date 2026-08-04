from datetime import datetime
from decimal import Decimal
from typing import Any, Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator

from app.modules.production.contracts import TaskResponse


class GenerationCommand(BaseModel):
    model_config = ConfigDict(extra="forbid")

    workspace_id: UUID
    shot_spec_version_id: UUID
    capability_id: UUID
    parameters: dict[str, Any]


class GenerationPreflightRequest(GenerationCommand):
    pass


class GenerationSubmissionRequest(GenerationCommand):
    preflight_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    preflight_expires_at: datetime
    warning_acknowledgements: list[str] = Field(default_factory=list, max_length=20)
    high_cost_confirmed: bool = False
    idempotency_key: str = Field(min_length=1, max_length=200)

    @field_validator("warning_acknowledgements")
    @classmethod
    def normalize_warning_acknowledgements(cls, value: list[str]) -> list[str]:
        normalized = sorted({item.strip() for item in value if item.strip()})
        if len(normalized) != len(value):
            raise ValueError("warning acknowledgements must be unique and non-empty")
        return normalized


class CapabilityPricingResponse(BaseModel):
    unit: Literal["per_request"]
    amount: Decimal
    currency: str
    high_cost_threshold: Decimal | None


class ModelCapabilityResponse(BaseModel):
    id: UUID
    provider: str
    model: str
    kind: Literal["image", "video"]
    config_version: int
    input_types: list[str]
    parameter_schema: dict[str, Any]
    limits: dict[str, Any]
    pricing: CapabilityPricingResponse | None
    status: Literal["active", "inactive", "unavailable"]
    unavailable_reason: str | None


class GenerationBlocker(BaseModel):
    code: str
    summary: str
    next_action: str


class GenerationConfirmationRequirement(BaseModel):
    code: Literal["ACKNOWLEDGE_WARNINGS", "CONFIRM_HIGH_COST"]
    warning_codes: list[str]


class EstimatedCostResponse(BaseModel):
    amount: Decimal
    currency: str
    pricing_version: int
    unit: Literal["per_request"] = "per_request"


class GenerationPreflightResponse(BaseModel):
    shot_id: UUID
    shot_spec_version_id: UUID
    capability_id: UUID
    status: Literal["ready", "blocked", "unavailable"]
    ready: bool
    blocking_reasons: list[GenerationBlocker]
    warning_codes: list[str]
    confirmation_requirements: list[GenerationConfirmationRequirement]
    estimated_cost: EstimatedCostResponse | None
    preflight_hash: str
    expires_at: datetime


class GenerationRequestResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    project_id: UUID
    episode_id: UUID
    shot_id: UUID
    shot_spec_version_id: UUID
    capability_id: UUID
    capability_config_version: int
    parameter_snapshot: dict[str, Any]
    warning_acknowledgements: list[str]
    shot_spec_input_hash: str
    input_hash: str
    high_cost_confirmed: bool
    idempotency_key: str
    requested_by: UUID
    created_at: datetime


class ReservationResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    request_id: UUID
    currency: str
    estimated_amount: Decimal
    reserved_amount: Decimal
    status: Literal["active", "settled", "released"]
    revision: int
    created_at: datetime


class CostEntryResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    reservation_id: UUID
    request_id: UUID
    task_id: UUID
    entry_type: Literal["reserve", "settle", "release", "adjust"]
    amount: Decimal
    currency: str
    provider_bill_ref: str | None
    created_at: datetime


class GenerationSubmissionResponse(BaseModel):
    request: GenerationRequestResponse
    task: TaskResponse
    reservation: ReservationResponse
    initial_cost_entry: CostEntryResponse
    outbox_event_id: UUID
    replayed: bool


class GenerationTaskCancellationRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    expected_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)
    reason: Literal["user_requested", "input_changed", "budget_changed"]


class GenerationTaskCancellationResponse(BaseModel):
    task: TaskResponse
    reservation: ReservationResponse
    release_cost_entry: CostEntryResponse
    replayed: bool


class CostSummaryResponse(BaseModel):
    reserved: Decimal
    settled: Decimal
    released: Decimal
    adjustments: Decimal
    remaining_reserved: Decimal


class CostQueryResponse(BaseModel):
    currency: str
    summary: CostSummaryResponse
    items: list[CostEntryResponse]
    total: int
    limit: int
    offset: int
