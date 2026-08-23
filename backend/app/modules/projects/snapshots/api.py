from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.projects.snapshots import service
from app.modules.projects.snapshots.schemas import (
    EpisodeProductionSnapshot,
    ProjectProductionSnapshot,
)

router = APIRouter(prefix="/api/v1", tags=["projects"])


@router.get(
    "/projects/{project_id}/production-snapshot",
    response_model=ApiResponse[ProjectProductionSnapshot],
)
async def project_production_snapshot(
    project_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProjectProductionSnapshot]:
    return ApiResponse(data=await service.project_production_snapshot(session, claims, project_id))


@router.get(
    "/episodes/{episode_id}/production-snapshot",
    response_model=ApiResponse[EpisodeProductionSnapshot],
)
async def episode_production_snapshot(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[EpisodeProductionSnapshot]:
    return ApiResponse(data=await service.episode_production_snapshot(session, claims, episode_id))
