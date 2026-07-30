from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.projects.contracts import DeletePreflightResponse, DeleteResponse
from app.modules.projects.episodes import service
from app.modules.projects.episodes.schemas import (
    EpisodeCreateRequest,
    EpisodeOrderResponse,
    EpisodeReorderRequest,
    EpisodeResponse,
    EpisodeStateRequest,
    EpisodeUpdateRequest,
)

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
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeResponse]:
    return ApiResponse(data=await service.create_episode(session, claims, project_id, payload))


@router.post(
    "/projects/{project_id}/episodes/reorder",
    response_model=ApiResponse[EpisodeOrderResponse],
)
async def reorder_episodes(
    project_id: UUID,
    payload: EpisodeReorderRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeOrderResponse]:
    return ApiResponse(data=await service.reorder_episodes(session, claims, project_id, payload))


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
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeResponse]:
    return ApiResponse(data=await service.update_episode(session, claims, episode_id, payload))


@router.post("/episodes/{episode_id}/archive", response_model=ApiResponse[EpisodeResponse])
async def archive_episode(
    episode_id: UUID,
    payload: EpisodeStateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeResponse]:
    return ApiResponse(
        data=await service.set_episode_archived(session, claims, episode_id, payload, archived=True)
    )


@router.post("/episodes/{episode_id}/restore", response_model=ApiResponse[EpisodeResponse])
async def restore_episode(
    episode_id: UUID,
    payload: EpisodeStateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeResponse]:
    return ApiResponse(
        data=await service.set_episode_archived(
            session, claims, episode_id, payload, archived=False
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
    return ApiResponse(data=await service.episode_delete_preflight(session, claims, episode_id))


@router.delete("/episodes/{episode_id}", response_model=ApiResponse[DeleteResponse])
async def delete_episode(
    episode_id: UUID,
    expected_revision: Annotated[int, Query(ge=1)],
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DeleteResponse]:
    await service.delete_episode(session, claims, episode_id, expected_revision)
    return ApiResponse(data=DeleteResponse())
