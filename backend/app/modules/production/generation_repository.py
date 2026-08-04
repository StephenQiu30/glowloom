from decimal import Decimal
from typing import Literal
from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.sql.elements import ColumnElement

from app.modules.production.models import (
    CostEntry,
    GenerationRequest,
    ModelCapability,
    Reservation,
    Task,
)


async def list_capabilities(
    session: AsyncSession,
    *,
    kind: Literal["image", "video"] | None,
    model: str | None,
) -> list[ModelCapability]:
    filters: list[ColumnElement[bool]] = []
    if kind is not None:
        filters.append(ModelCapability.kind == kind)
    if model is not None:
        filters.append(ModelCapability.model == model)
    rows = await session.scalars(
        select(ModelCapability)
        .where(*filters)
        .order_by(
            ModelCapability.kind,
            ModelCapability.model,
            ModelCapability.config_version.desc(),
        )
    )
    return list(rows)


async def find_capability(
    session: AsyncSession,
    capability_id: UUID,
    *,
    for_update: bool = False,
) -> ModelCapability | None:
    query = select(ModelCapability).where(ModelCapability.id == capability_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_idempotent_generation_request(
    session: AsyncSession,
    workspace_id: UUID,
    idempotency_key: str,
) -> GenerationRequest | None:
    return await session.scalar(
        select(GenerationRequest).where(
            GenerationRequest.workspace_id == workspace_id,
            GenerationRequest.idempotency_key == idempotency_key,
        )
    )


async def active_reserved_amount(
    session: AsyncSession,
    workspace_id: UUID,
    project_id: UUID,
) -> Decimal:
    amount = await session.scalar(
        select(func.coalesce(func.sum(Reservation.reserved_amount), Decimal("0")))
        .join(GenerationRequest, GenerationRequest.id == Reservation.request_id)
        .where(
            Reservation.workspace_id == workspace_id,
            Reservation.status == "active",
            GenerationRequest.project_id == project_id,
        )
    )
    return Decimal(amount or 0)


async def generation_submission_facts(
    session: AsyncSession,
    request_id: UUID,
) -> tuple[GenerationRequest, Task, Reservation, CostEntry] | None:
    request = await session.scalar(
        select(GenerationRequest).where(GenerationRequest.id == request_id)
    )
    if request is None:
        return None
    task = await session.scalar(
        select(Task).where(
            Task.request_type == "generation_request",
            Task.request_id == request.id,
            Task.workspace_id == request.workspace_id,
        )
    )
    reservation = await session.scalar(
        select(Reservation).where(Reservation.request_id == request.id)
    )
    cost_entry = (
        None
        if reservation is None
        else await session.scalar(
            select(CostEntry).where(
                CostEntry.reservation_id == reservation.id,
                CostEntry.entry_type == "reserve",
            )
        )
    )
    if task is None or reservation is None or cost_entry is None:
        raise RuntimeError("generation submission facts are incomplete")
    return request, task, reservation, cost_entry


async def find_generation_task_for_cancellation(
    session: AsyncSession,
    workspace_id: UUID,
    task_id: UUID,
) -> Task | None:
    return await session.scalar(
        select(Task)
        .where(Task.id == task_id, Task.workspace_id == workspace_id)
        .with_for_update()
    )


async def find_generation_request(
    session: AsyncSession,
    workspace_id: UUID,
    request_id: UUID,
) -> GenerationRequest | None:
    return await session.scalar(
        select(GenerationRequest).where(
            GenerationRequest.id == request_id,
            GenerationRequest.workspace_id == workspace_id,
        )
    )


async def find_generation_reservation_for_update(
    session: AsyncSession,
    workspace_id: UUID,
    request_id: UUID,
) -> Reservation | None:
    return await session.scalar(
        select(Reservation)
        .where(
            Reservation.workspace_id == workspace_id,
            Reservation.request_id == request_id,
        )
        .with_for_update()
    )


async def find_release_cost_entry(
    session: AsyncSession,
    workspace_id: UUID,
    reservation_id: UUID,
) -> CostEntry | None:
    return await session.scalar(
        select(CostEntry).where(
            CostEntry.workspace_id == workspace_id,
            CostEntry.reservation_id == reservation_id,
            CostEntry.entry_type == "release",
        )
    )


async def list_project_cost_entries(
    session: AsyncSession,
    workspace_id: UUID,
    project_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[tuple[CostEntry, Reservation, GenerationRequest, Task]], int]:
    filters = (
        CostEntry.workspace_id == workspace_id,
        GenerationRequest.project_id == project_id,
    )
    base = (
        select(CostEntry, Reservation, GenerationRequest, Task)
        .join(Reservation, Reservation.id == CostEntry.reservation_id)
        .join(GenerationRequest, GenerationRequest.id == Reservation.request_id)
        .join(
            Task,
            (Task.request_type == "generation_request")
            & (Task.request_id == GenerationRequest.id)
            & (Task.workspace_id == GenerationRequest.workspace_id),
        )
        .where(*filters)
    )
    total = await session.scalar(
        select(func.count())
        .select_from(CostEntry)
        .join(Reservation, Reservation.id == CostEntry.reservation_id)
        .join(GenerationRequest, GenerationRequest.id == Reservation.request_id)
        .where(*filters)
    )
    rows = await session.execute(
        base.order_by(CostEntry.created_at.desc(), CostEntry.id.desc()).limit(limit).offset(offset)
    )
    return [(row[0], row[1], row[2], row[3]) for row in rows.all()], total or 0


async def project_cost_totals(
    session: AsyncSession,
    workspace_id: UUID,
    project_id: UUID,
) -> dict[str, Decimal]:
    rows = await session.execute(
        select(CostEntry.entry_type, func.sum(CostEntry.amount))
        .join(Reservation, Reservation.id == CostEntry.reservation_id)
        .join(GenerationRequest, GenerationRequest.id == Reservation.request_id)
        .where(
            CostEntry.workspace_id == workspace_id,
            GenerationRequest.project_id == project_id,
        )
        .group_by(CostEntry.entry_type)
    )
    return {entry_type: Decimal(amount) for entry_type, amount in rows.all()}
