from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.scripts.narratives import service
from app.modules.scripts.narratives.schemas import (
    NarrativeDependencyResponse,
    NarrativeImpactResponse,
    NarrativeRevisionResponse,
    NarrativeStructureResponse,
    NarrativeStructureRevisionRequest,
)

router = APIRouter(prefix="/api/v1", tags=["scripts"])


@router.get(
    "/script-versions/{version_id}/narrative-structure",
    response_model=ApiResponse[NarrativeStructureResponse],
)
async def get_narrative_structure(
    version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    revision: Annotated[int | None, Query(ge=1)] = None,
) -> ApiResponse[NarrativeStructureResponse]:
    return ApiResponse(
        data=await service.get_structure(
            session,
            claims,
            version_id,
            revision=revision,
        )
    )


@router.post(
    "/narrative-structures/{structure_id}/revisions",
    response_model=ApiResponse[NarrativeRevisionResponse],
    status_code=status.HTTP_201_CREATED,
)
async def revise_narrative_structure(
    structure_id: UUID,
    payload: NarrativeStructureRevisionRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[NarrativeRevisionResponse]:
    return ApiResponse(
        data=await service.revise_structure(
            session,
            claims,
            structure_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/episodes/{episode_id}/narrative-dependency",
    response_model=ApiResponse[NarrativeDependencyResponse],
)
async def get_narrative_dependency(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    evaluation_hash: Annotated[str | None, Query(min_length=64, max_length=64)] = None,
) -> ApiResponse[NarrativeDependencyResponse]:
    return ApiResponse(
        data=await service.get_dependency(
            session,
            claims,
            episode_id,
            evaluation_hash=evaluation_hash,
        )
    )


@router.get(
    "/episodes/{episode_id}/narrative-impacts/latest",
    response_model=ApiResponse[NarrativeImpactResponse],
)
async def get_latest_narrative_impact(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[NarrativeImpactResponse]:
    return ApiResponse(data=await service.get_latest_impact(session, claims, episode_id))
