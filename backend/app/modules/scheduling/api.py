from typing import Annotated, Literal
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.scheduling import service
from app.modules.scheduling.schemas import (
    PaginatedSchedules,
    ScheduleConfigurationRequest,
    ScheduleFireResponse,
    ScheduleResponse,
    ScheduleResumeRequest,
    ScheduleStateRequest,
    ScheduleTriggerRequest,
)

router = APIRouter(prefix="/api/v1/schedules", tags=["schedules"])


@router.get("", response_model=ApiResponse[PaginatedSchedules])
async def list_schedules(
    workspace_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    status: Literal["active", "paused", "completed", "manual_attention"] | None = None,
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedSchedules]:
    return ApiResponse(
        data=await service.list_schedules(
            session,
            claims,
            workspace_id,
            status=status,
            limit=limit or 20,
            offset=offset,
        )
    )


@router.post("/{schedule_id}/pause", response_model=ApiResponse[ScheduleResponse])
async def pause_schedule(
    schedule_id: UUID,
    payload: ScheduleStateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScheduleResponse]:
    return ApiResponse(
        data=await service.pause_schedule(
            session,
            claims,
            schedule_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.put(
    "/{schedule_id}/configuration",
    response_model=ApiResponse[ScheduleResponse],
)
async def configure_schedule(
    schedule_id: UUID,
    payload: ScheduleConfigurationRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScheduleResponse]:
    return ApiResponse(
        data=await service.configure_schedule(
            session,
            claims,
            schedule_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post("/{schedule_id}/resume", response_model=ApiResponse[ScheduleResponse])
async def resume_schedule(
    schedule_id: UUID,
    payload: ScheduleResumeRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScheduleResponse]:
    return ApiResponse(
        data=await service.resume_schedule(
            session,
            claims,
            schedule_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post("/{schedule_id}/trigger", response_model=ApiResponse[ScheduleFireResponse])
async def trigger_schedule(
    schedule_id: UUID,
    payload: ScheduleTriggerRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScheduleFireResponse]:
    return ApiResponse(
        data=await service.trigger_schedule(
            session,
            claims,
            schedule_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )
