from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.scripts import service
from app.modules.scripts.schemas import (
    PaginatedScriptVersions,
    ScriptImportRequest,
    ScriptImportResponse,
    ScriptSourceResponse,
    ScriptVersionResponse,
)

router = APIRouter(prefix="/api/v1", tags=["scripts"])


@router.post(
    "/episodes/{episode_id}/script-sources",
    response_model=ApiResponse[ScriptImportResponse],
    status_code=status.HTTP_201_CREATED,
)
async def import_text_source(
    episode_id: UUID,
    payload: ScriptImportRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptImportResponse]:
    return ApiResponse(
        data=await service.import_text_source(session, claims, episode_id, payload)
    )


@router.get(
    "/script-sources/{source_id}",
    response_model=ApiResponse[ScriptSourceResponse],
)
async def get_source(
    source_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptSourceResponse]:
    return ApiResponse(data=await service.get_source(session, claims, source_id))


@router.get(
    "/script-sources/{source_id}/versions",
    response_model=ApiResponse[PaginatedScriptVersions],
)
async def list_versions(
    source_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedScriptVersions]:
    return ApiResponse(
        data=await service.list_versions(
            session, claims, source_id, limit=limit or 20, offset=offset
        )
    )


@router.get(
    "/script-versions/{version_id}",
    response_model=ApiResponse[ScriptVersionResponse],
)
async def get_version(
    version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptVersionResponse]:
    return ApiResponse(data=await service.get_version(session, claims, version_id))
