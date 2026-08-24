from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.scripts.production_bibles import service
from app.modules.scripts.production_bibles.materialization import (
    confirm_and_materialize_production_bible,
)
from app.modules.scripts.production_bibles.review import (
    resolve_production_bible_review_issue,
)
from app.modules.scripts.production_bibles.schemas import (
    ProductionBibleConfirmRequest,
    ProductionBibleCreateRequest,
    ProductionBibleResponse,
    ProductionBibleResumeRequest,
    ProductionBibleReviewIssueResolutionRequest,
)

router = APIRouter(prefix="/api/v1", tags=["production-bibles"])


@router.post(
    "/document-revisions/{revision_id}/production-bibles",
    response_model=ApiResponse[ProductionBibleResponse],
    status_code=status.HTTP_202_ACCEPTED,
)
async def create_bible(
    revision_id: UUID,
    payload: ProductionBibleCreateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProductionBibleResponse]:
    return ApiResponse(
        data=await service.create_bible(
            session,
            claims,
            revision_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/production-bibles/{bible_id}",
    response_model=ApiResponse[ProductionBibleResponse],
)
async def get_bible(
    bible_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProductionBibleResponse]:
    return ApiResponse(data=await service.get_bible(session, claims, bible_id))


@router.post(
    "/production-bibles/{bible_id}/resume",
    response_model=ApiResponse[ProductionBibleResponse],
    status_code=status.HTTP_202_ACCEPTED,
)
async def resume_bible(
    bible_id: UUID,
    payload: ProductionBibleResumeRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProductionBibleResponse]:
    return ApiResponse(
        data=await service.resume_bible(
            session,
            claims,
            bible_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/production-bibles/{bible_id}/review-issue-resolutions",
    response_model=ApiResponse[ProductionBibleResponse],
)
async def resolve_bible_review_issue(
    bible_id: UUID,
    payload: ProductionBibleReviewIssueResolutionRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProductionBibleResponse]:
    await resolve_production_bible_review_issue(
        session,
        claims,
        bible_id,
        payload,
        trace_id=str(request.state.request_id),
    )
    return ApiResponse(data=await service.get_bible(session, claims, bible_id))


@router.post(
    "/production-bibles/{bible_id}/confirm",
    response_model=ApiResponse[ProductionBibleResponse],
)
async def confirm_bible(
    bible_id: UUID,
    payload: ProductionBibleConfirmRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProductionBibleResponse]:
    await confirm_and_materialize_production_bible(
        session,
        claims,
        bible_id,
        payload,
        trace_id=str(request.state.request_id),
    )
    return ApiResponse(data=await service.get_bible(session, claims, bible_id))


@router.get(
    "/projects/{project_id}/production-bible",
    response_model=ApiResponse[ProductionBibleResponse],
)
async def get_current_bible(
    project_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProductionBibleResponse]:
    return ApiResponse(data=await service.get_current_bible(session, claims, project_id))
