from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.storyboards.drafts import service
from app.modules.storyboards.drafts.schemas import (
    DraftApplyPreflightRequest,
    DraftApplyPreflightResponse,
    DraftApplyRequest,
    DraftApplyResponse,
    DraftApproveRequest,
    DraftBatchCreateRequest,
    DraftBatchResponse,
    DraftDecisionRequest,
    DraftDecisionResult,
)

router = APIRouter()


@router.post(
    "/episodes/{episode_id}/storyboard-draft-batches",
    response_model=ApiResponse[DraftBatchResponse],
    status_code=status.HTTP_202_ACCEPTED,
)
async def create_batch(
    episode_id: UUID,
    payload: DraftBatchCreateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DraftBatchResponse]:
    return ApiResponse(
        data=await service.create_batch(
            session,
            claims,
            episode_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/storyboard-draft-batches/{batch_id}",
    response_model=ApiResponse[DraftBatchResponse],
)
async def get_batch(
    batch_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DraftBatchResponse]:
    return ApiResponse(data=await service.get_batch(session, claims, batch_id))


@router.post(
    "/storyboard-drafts/{draft_id}/decisions",
    response_model=ApiResponse[DraftDecisionResult],
    status_code=status.HTTP_201_CREATED,
)
async def decide_draft(
    draft_id: UUID,
    payload: DraftDecisionRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DraftDecisionResult]:
    return ApiResponse(
        data=await service.decide_draft(
            session,
            claims,
            draft_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/storyboard-draft-batches/{batch_id}/approve",
    response_model=ApiResponse[DraftBatchResponse],
)
async def approve_batch(
    batch_id: UUID,
    payload: DraftApproveRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DraftBatchResponse]:
    return ApiResponse(
        data=await service.approve_batch(
            session,
            claims,
            batch_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/storyboard-draft-batches/{batch_id}/apply-preflight",
    response_model=ApiResponse[DraftApplyPreflightResponse],
)
async def preflight_apply(
    batch_id: UUID,
    payload: DraftApplyPreflightRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DraftApplyPreflightResponse]:
    return ApiResponse(data=await service.preflight_apply(session, claims, batch_id, payload))


@router.post(
    "/storyboard-draft-batches/{batch_id}/apply",
    response_model=ApiResponse[DraftApplyResponse],
    status_code=status.HTTP_201_CREATED,
)
async def apply_batch(
    batch_id: UUID,
    payload: DraftApplyRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DraftApplyResponse]:
    return ApiResponse(
        data=await service.apply_batch(
            session,
            claims,
            batch_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )
