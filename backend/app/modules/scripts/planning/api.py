from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Request, Response, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.scripts.planning import service
from app.modules.scripts.planning.schemas import (
    ConfirmEpisodePlanRequest,
    EpisodePlanCreateRequest,
    EpisodePlanDetailResponse,
    ImportCommitDetailResponse,
    MaterializeEpisodePlanRequest,
    MergeEpisodeProposalRequest,
    MoveEpisodeBoundaryRequest,
    PublishImportCommitRequest,
    RenameEpisodeProposalRequest,
    SplitEpisodeProposalRequest,
)

router = APIRouter(prefix="/api/v1", tags=["episode-planning"])


@router.post(
    "/document-revisions/{revision_id}/episode-plans",
    response_model=ApiResponse[EpisodePlanDetailResponse],
    status_code=status.HTTP_201_CREATED,
)
async def create_episode_plan(
    revision_id: UUID,
    payload: EpisodePlanCreateRequest,
    request: Request,
    response: Response,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodePlanDetailResponse]:
    result = await service.create_plan(
        session,
        claims,
        revision_id,
        payload,
        trace_id=str(request.state.request_id),
    )
    if result.plan.planning_task_id is not None and result.plan.status == "draft":
        response.status_code = status.HTTP_202_ACCEPTED
    return ApiResponse(data=result)


@router.get(
    "/episode-plans/{plan_id}",
    response_model=ApiResponse[EpisodePlanDetailResponse],
)
async def get_episode_plan(
    plan_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodePlanDetailResponse]:
    return ApiResponse(data=await service.get_plan(session, claims, plan_id))


@router.post(
    "/episode-plans/{plan_id}/move-boundary",
    response_model=ApiResponse[EpisodePlanDetailResponse],
)
async def move_episode_boundary(
    plan_id: UUID,
    payload: MoveEpisodeBoundaryRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodePlanDetailResponse]:
    return ApiResponse(
        data=await service.move_boundary(
            session,
            claims,
            plan_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/episode-plans/{plan_id}/split",
    response_model=ApiResponse[EpisodePlanDetailResponse],
)
async def split_episode_proposal(
    plan_id: UUID,
    payload: SplitEpisodeProposalRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodePlanDetailResponse]:
    return ApiResponse(
        data=await service.split_proposal(
            session,
            claims,
            plan_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/episode-plans/{plan_id}/merge",
    response_model=ApiResponse[EpisodePlanDetailResponse],
)
async def merge_episode_proposals(
    plan_id: UUID,
    payload: MergeEpisodeProposalRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodePlanDetailResponse]:
    return ApiResponse(
        data=await service.merge_proposals(
            session,
            claims,
            plan_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/episode-plans/{plan_id}/rename",
    response_model=ApiResponse[EpisodePlanDetailResponse],
)
async def rename_episode_proposal(
    plan_id: UUID,
    payload: RenameEpisodeProposalRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodePlanDetailResponse]:
    return ApiResponse(
        data=await service.rename_proposal(
            session,
            claims,
            plan_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/episode-plans/{plan_id}/confirm",
    response_model=ApiResponse[EpisodePlanDetailResponse],
)
async def confirm_episode_plan(
    plan_id: UUID,
    payload: ConfirmEpisodePlanRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodePlanDetailResponse]:
    return ApiResponse(
        data=await service.confirm_plan(
            session,
            claims,
            plan_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/episode-plans/{plan_id}/materializations",
    response_model=ApiResponse[ImportCommitDetailResponse],
    status_code=status.HTTP_201_CREATED,
)
async def materialize_episode_plan(
    plan_id: UUID,
    payload: MaterializeEpisodePlanRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ImportCommitDetailResponse]:
    return ApiResponse(
        data=await service.materialize_plan(
            session,
            claims,
            plan_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/import-commits/{commit_id}/publish",
    response_model=ApiResponse[ImportCommitDetailResponse],
)
async def publish_import_commit(
    commit_id: UUID,
    payload: PublishImportCommitRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ImportCommitDetailResponse]:
    return ApiResponse(
        data=await service.publish_import_commit(
            session,
            claims,
            commit_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )
