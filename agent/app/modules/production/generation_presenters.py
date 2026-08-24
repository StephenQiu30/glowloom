from decimal import Decimal
from typing import Literal, cast

from app.modules.production.generation_schemas import (
    CostEntryResponse,
    GenerationRequestResponse,
    ReservationResponse,
)
from app.modules.production.models import (
    CostEntry,
    GenerationRequest,
    Reservation,
    Task,
)

AMOUNT_QUANTUM = Decimal("0.000001")


def decimal_amount(value: Decimal) -> Decimal:
    return Decimal(value).quantize(AMOUNT_QUANTUM)


def generation_request_response(request: GenerationRequest) -> GenerationRequestResponse:
    return GenerationRequestResponse(
        id=request.id,
        workspace_id=request.workspace_id,
        project_id=request.project_id,
        episode_id=request.episode_id,
        shot_id=request.shot_id,
        shot_spec_version_id=request.shot_spec_version_id,
        capability_id=request.capability_id,
        capability_config_version=request.capability_config_version,
        parameter_snapshot=request.parameter_snapshot,
        warning_acknowledgements=request.warning_acknowledgements,
        shot_spec_input_hash=request.shot_spec_input_hash,
        input_hash=request.input_hash,
        high_cost_confirmed=request.high_cost_confirmed,
        idempotency_key=request.idempotency_key,
        requested_by=request.requested_by,
        created_at=request.created_at,
    )


def reservation_response(reservation: Reservation) -> ReservationResponse:
    return ReservationResponse(
        id=reservation.id,
        workspace_id=reservation.workspace_id,
        request_id=reservation.request_id,
        currency=reservation.currency,
        estimated_amount=decimal_amount(reservation.estimated_amount),
        reserved_amount=decimal_amount(reservation.reserved_amount),
        status=cast(Literal["active", "settled", "released"], reservation.status),
        revision=reservation.revision,
        created_at=reservation.created_at,
    )


def cost_entry_response(
    entry: CostEntry,
    reservation: Reservation,
    request: GenerationRequest,
    task: Task,
) -> CostEntryResponse:
    return CostEntryResponse(
        id=entry.id,
        workspace_id=entry.workspace_id,
        reservation_id=reservation.id,
        request_id=request.id,
        task_id=task.id,
        entry_type=cast(
            Literal["reserve", "settle", "release", "adjust"],
            entry.entry_type,
        ),
        amount=decimal_amount(entry.amount),
        currency=entry.currency,
        provider_bill_ref=entry.provider_bill_ref,
        created_at=entry.created_at,
    )
