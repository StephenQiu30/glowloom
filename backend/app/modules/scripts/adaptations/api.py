from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.scripts.adaptations import service
from app.modules.scripts.adaptations.schemas import (
    AdaptationCancelRequest,
    AdaptationDiffResponse,
    AdaptationDraftUpdateRequest,
    AdaptationPublishRequest,
    AdaptationPublishResponse,
    AdaptationRunCreateRequest,
    AdaptationRunResponse,
)
from app.modules.storyboards import list_script_version_affected_shot_ids

router = APIRouter(prefix="/api/v1", tags=["script-adaptations"])


@router.post(
    "/episodes/{episode_id}/adaptation-runs",
    response_model=ApiResponse[AdaptationRunResponse],
    status_code=status.HTTP_202_ACCEPTED,
)
async def create_run(
    episode_id: UUID,
    payload: AdaptationRunCreateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AdaptationRunResponse]:
    return ApiResponse(
        data=await service.create_run(
            session,
            claims,
            episode_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/adaptation-runs/{run_id}",
    response_model=ApiResponse[AdaptationRunResponse],
)
async def get_run(
    run_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AdaptationRunResponse]:
    return ApiResponse(data=await service.get_run(session, claims, run_id))


@router.patch(
    "/adaptation-runs/{run_id}/draft",
    response_model=ApiResponse[AdaptationRunResponse],
)
async def update_draft(
    run_id: UUID,
    payload: AdaptationDraftUpdateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AdaptationRunResponse]:
    return ApiResponse(
        data=await service.update_draft(
            session,
            claims,
            run_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/adaptation-runs/{run_id}/diff",
    response_model=ApiResponse[AdaptationDiffResponse],
)
async def diff_run(
    run_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AdaptationDiffResponse]:
    return ApiResponse(data=await service.diff_run(session, claims, run_id))


@router.post(
    "/adaptation-runs/{run_id}/publish",
    response_model=ApiResponse[AdaptationPublishResponse],
)
async def publish_run(
    run_id: UUID,
    payload: AdaptationPublishRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AdaptationPublishResponse]:
    async def read_impact(*, episode_id: UUID, current_script_version_id: UUID) -> list[UUID]:
        return await list_script_version_affected_shot_ids(
            session,
            episode_id=episode_id,
            current_script_version_id=current_script_version_id,
        )

    return ApiResponse(
        data=await service.publish_run(
            session,
            claims,
            run_id,
            payload,
            read_impact,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/adaptation-runs/{run_id}/cancel",
    response_model=ApiResponse[AdaptationRunResponse],
)
async def cancel_run(
    run_id: UUID,
    payload: AdaptationCancelRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AdaptationRunResponse]:
    return ApiResponse(
        data=await service.cancel_run(
            session,
            claims,
            run_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )
