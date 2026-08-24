from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.projects.contracts import (
    DeletePreflightResponse,
    DeleteResponse,
    EpisodeStoryboardReferenceSummary,
)
from app.modules.projects.episodes import service
from app.modules.projects.episodes.schemas import (
    EpisodeCreateRequest,
    EpisodeOrderResponse,
    EpisodeReorderRequest,
    EpisodeResponse,
    EpisodeStateRequest,
    EpisodeUpdateRequest,
)
from app.modules.scripts import count_episode_script_versions
from app.modules.storyboards import summarize_episode_storyboard_references

router = APIRouter(prefix="/api/v1", tags=["projects"])


@router.get(
    "/projects/{project_id}/episodes",
    response_model=ApiResponse[list[EpisodeResponse]],
)
async def list_episodes(
    project_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    include_archived: bool = False,
) -> ApiResponse[list[EpisodeResponse]]:
    return ApiResponse(
        data=await service.list_episodes(
            session, claims, project_id, include_archived=include_archived
        )
    )


@router.post(
    "/projects/{project_id}/episodes",
    response_model=ApiResponse[EpisodeResponse],
    status_code=status.HTTP_201_CREATED,
)
async def create_episode(
    project_id: UUID,
    payload: EpisodeCreateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeResponse]:
    return ApiResponse(
        data=await service.create_episode(
            session,
            claims,
            project_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/projects/{project_id}/episodes/reorder",
    response_model=ApiResponse[EpisodeOrderResponse],
)
async def reorder_episodes(
    project_id: UUID,
    payload: EpisodeReorderRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeOrderResponse]:
    return ApiResponse(
        data=await service.reorder_episodes(
            session,
            claims,
            project_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get("/episodes/{episode_id}", response_model=ApiResponse[EpisodeResponse])
async def get_episode(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeResponse]:
    return ApiResponse(data=await service.get_episode(session, claims, episode_id))


@router.patch("/episodes/{episode_id}", response_model=ApiResponse[EpisodeResponse])
async def update_episode(
    episode_id: UUID,
    payload: EpisodeUpdateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeResponse]:
    return ApiResponse(
        data=await service.update_episode(
            session,
            claims,
            episode_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post("/episodes/{episode_id}/archive", response_model=ApiResponse[EpisodeResponse])
async def archive_episode(
    episode_id: UUID,
    payload: EpisodeStateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeResponse]:
    return ApiResponse(
        data=await service.set_episode_archived(
            session,
            claims,
            episode_id,
            payload,
            archived=True,
            trace_id=str(request.state.request_id),
        )
    )


@router.post("/episodes/{episode_id}/restore", response_model=ApiResponse[EpisodeResponse])
async def restore_episode(
    episode_id: UUID,
    payload: EpisodeStateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeResponse]:
    return ApiResponse(
        data=await service.set_episode_archived(
            session,
            claims,
            episode_id,
            payload,
            archived=False,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/episodes/{episode_id}/delete-preflight",
    response_model=ApiResponse[DeletePreflightResponse],
)
async def episode_delete_preflight(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DeletePreflightResponse]:
    async def read_script_version_counts(
        *, workspace_id: UUID, episode_ids: list[UUID]
    ) -> dict[UUID, int]:
        return await count_episode_script_versions(session, workspace_id, episode_ids)

    async def read_storyboard_references(
        *, workspace_id: UUID, episode_ids: list[UUID]
    ) -> dict[UUID, EpisodeStoryboardReferenceSummary]:
        summaries = await summarize_episode_storyboard_references(
            session,
            workspace_id,
            episode_ids,
        )
        return {
            episode_id: EpisodeStoryboardReferenceSummary(
                shot_count=summary.shot_count,
                spec_version_count=summary.spec_version_count,
            )
            for episode_id, summary in summaries.items()
        }

    return ApiResponse(
        data=await service.episode_delete_preflight(
            session,
            claims,
            episode_id,
            read_script_version_counts,
            read_storyboard_references,
        )
    )


@router.delete("/episodes/{episode_id}", response_model=ApiResponse[DeleteResponse])
async def delete_episode(
    episode_id: UUID,
    expected_revision: Annotated[int, Query(ge=1)],
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DeleteResponse]:
    async def read_script_version_counts(
        *, workspace_id: UUID, episode_ids: list[UUID]
    ) -> dict[UUID, int]:
        return await count_episode_script_versions(session, workspace_id, episode_ids)

    async def read_storyboard_references(
        *, workspace_id: UUID, episode_ids: list[UUID]
    ) -> dict[UUID, EpisodeStoryboardReferenceSummary]:
        summaries = await summarize_episode_storyboard_references(
            session,
            workspace_id,
            episode_ids,
        )
        return {
            episode_id: EpisodeStoryboardReferenceSummary(
                shot_count=summary.shot_count,
                spec_version_count=summary.spec_version_count,
            )
            for episode_id, summary in summaries.items()
        }

    await service.delete_episode(
        session,
        claims,
        episode_id,
        expected_revision,
        read_script_version_counts,
        read_storyboard_references,
        trace_id=str(request.state.request_id),
    )
    return ApiResponse(data=DeleteResponse())
