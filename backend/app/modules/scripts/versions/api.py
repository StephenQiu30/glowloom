from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.scripts.versions import service
from app.modules.scripts.versions.schemas import (
    CurrentScriptVersionRequest,
    CurrentScriptVersionResponse,
    PaginatedScriptVersions,
    ScriptImportRequest,
    ScriptImportResponse,
    ScriptSourceResponse,
    ScriptSourceStateRequest,
    ScriptVersionDeleteResponse,
    ScriptVersionDiffResponse,
    ScriptVersionPublishRequest,
    ScriptVersionPublishResponse,
    ScriptVersionResponse,
)

router = APIRouter(prefix="/api/v1", tags=["scripts"])
lookup_router = APIRouter(prefix="/api/v1", tags=["scripts"])


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


@router.post(
    "/script-sources/{source_id}/versions",
    response_model=ApiResponse[ScriptVersionPublishResponse],
    status_code=status.HTTP_201_CREATED,
)
async def publish_version(
    source_id: UUID,
    payload: ScriptVersionPublishRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptVersionPublishResponse]:
    return ApiResponse(
        data=await service.publish_version(session, claims, source_id, payload)
    )


@router.post(
    "/episodes/{episode_id}/current-script-version",
    response_model=ApiResponse[CurrentScriptVersionResponse],
)
async def set_current_version(
    episode_id: UUID,
    payload: CurrentScriptVersionRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[CurrentScriptVersionResponse]:
    return ApiResponse(
        data=await service.set_current_version(session, claims, episode_id, payload)
    )


@router.post(
    "/script-sources/{source_id}/archive",
    response_model=ApiResponse[ScriptSourceResponse],
)
async def archive_source(
    source_id: UUID,
    payload: ScriptSourceStateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptSourceResponse]:
    return ApiResponse(
        data=await service.set_source_archived(
            session, claims, source_id, payload, archived=True
        )
    )


@router.post(
    "/script-sources/{source_id}/restore",
    response_model=ApiResponse[ScriptSourceResponse],
)
async def restore_source(
    source_id: UUID,
    payload: ScriptSourceStateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptSourceResponse]:
    return ApiResponse(
        data=await service.set_source_archived(
            session, claims, source_id, payload, archived=False
        )
    )


@router.delete(
    "/script-versions/{version_id}",
    response_model=ApiResponse[ScriptVersionDeleteResponse],
)
async def delete_draft_version(
    version_id: UUID,
    confirm: Annotated[bool, Query()],
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptVersionDeleteResponse]:
    return ApiResponse(
        data=await service.delete_draft_version(
            session,
            claims,
            version_id,
            confirmed=confirm,
        )
    )


@router.get(
    "/script-versions/{version_id}/diff",
    response_model=ApiResponse[ScriptVersionDiffResponse],
)
async def diff_versions(
    version_id: UUID,
    other_version_id: Annotated[UUID, Query()],
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptVersionDiffResponse]:
    return ApiResponse(
        data=await service.diff_versions(
            session, claims, version_id, other_version_id
        )
    )


@lookup_router.get(
    "/script-versions/{version_id}",
    response_model=ApiResponse[ScriptVersionResponse],
)
async def get_version(
    version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptVersionResponse]:
    return ApiResponse(data=await service.get_version(session, claims, version_id))
