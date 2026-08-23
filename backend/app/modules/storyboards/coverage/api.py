from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.storyboards.coverage import service
from app.modules.storyboards.coverage.schemas import (
    CoverageDecisionApplyResponse,
    CoverageDecisionRequest,
    CoverageReportResponse,
    NarrativeReferenceReplaceRequest,
    NarrativeReferenceReplaceResponse,
)

router = APIRouter()


@router.get(
    "/episodes/{episode_id}/coverage",
    response_model=ApiResponse[CoverageReportResponse],
)
async def get_coverage(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[CoverageReportResponse]:
    return ApiResponse(data=await service.get_report(session, claims, episode_id))


@router.post(
    "/shots/{shot_id}/narrative-references",
    response_model=ApiResponse[NarrativeReferenceReplaceResponse],
    status_code=status.HTTP_201_CREATED,
)
async def replace_references(
    shot_id: UUID,
    payload: NarrativeReferenceReplaceRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[NarrativeReferenceReplaceResponse]:
    return ApiResponse(
        data=await service.replace_references(
            session,
            claims,
            shot_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/episodes/{episode_id}/coverage-decisions",
    response_model=ApiResponse[CoverageDecisionApplyResponse],
    status_code=status.HTTP_201_CREATED,
)
async def decide_coverage(
    episode_id: UUID,
    payload: CoverageDecisionRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[CoverageDecisionApplyResponse]:
    return ApiResponse(
        data=await service.decide_coverage(
            session,
            claims,
            episode_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )
