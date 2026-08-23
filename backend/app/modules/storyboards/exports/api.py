from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.storyboards.exports import service
from app.modules.storyboards.exports.schemas import (
    ExportHistoryResponse,
    ExportPreflightResponse,
    ExportRequest,
    ExportResponse,
)

router = APIRouter()


@router.post(
    "/episodes/{episode_id}/storyboard-exports/preflight",
    response_model=ApiResponse[ExportPreflightResponse],
)
async def preflight_export(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ExportPreflightResponse]:
    return ApiResponse(data=await service.preflight_export(session, claims, episode_id))


@router.post(
    "/episodes/{episode_id}/storyboard-exports",
    response_model=ApiResponse[ExportResponse],
    status_code=status.HTTP_202_ACCEPTED,
)
async def request_export(
    episode_id: UUID,
    payload: ExportRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ExportResponse]:
    return ApiResponse(
        data=await service.request_export(
            session,
            claims,
            episode_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/episodes/{episode_id}/storyboard-exports",
    response_model=ApiResponse[ExportHistoryResponse],
)
async def list_exports(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ExportHistoryResponse]:
    return ApiResponse(data=await service.list_exports(session, claims, episode_id))
