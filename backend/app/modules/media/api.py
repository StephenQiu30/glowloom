from datetime import datetime
from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import (
    AccessTokenClaims,
    get_access_token_claims,
    get_request_settings,
)
from app.core.config import Settings
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.integrations.dependencies import get_media_storage
from app.modules.media import service
from app.modules.media.schemas import (
    AppendVersionRequest,
    ArchiveMediaRequest,
    CurrentMediaVersionRequest,
    MediaAccessRequest,
    MediaAccessResponse,
    MediaKind,
    MediaLocationMigrationRequest,
    MediaLocationRollbackRequest,
    MediaLocationsResponse,
    MediaObjectResponse,
    MediaSource,
    MediaVersionResponse,
    PaginatedMedia,
    ProbeRetryRequest,
    UploadCompletionResponse,
    UploadDeclaration,
    UploadInitializationResponse,
)
from app.modules.media.storage import MediaStorage
from app.modules.production import TaskResponse

router = APIRouter(prefix="/api/v1", tags=["media"])


@router.post(
    "/media/uploads",
    response_model=ApiResponse[UploadInitializationResponse],
    status_code=status.HTTP_201_CREATED,
)
async def initialize_upload(
    payload: UploadDeclaration,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    storage: Annotated[MediaStorage, Depends(get_media_storage)],
    settings: Annotated[Settings, Depends(get_request_settings)],
) -> ApiResponse[UploadInitializationResponse]:
    return ApiResponse(
        data=await service.initialize_upload(
            session, claims, payload, storage, settings
        )
    )


@router.post(
    "/media-objects/{media_object_id}/versions",
    response_model=ApiResponse[UploadInitializationResponse],
    status_code=status.HTTP_201_CREATED,
)
async def initialize_version_upload(
    media_object_id: UUID,
    payload: AppendVersionRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    storage: Annotated[MediaStorage, Depends(get_media_storage)],
    settings: Annotated[Settings, Depends(get_request_settings)],
) -> ApiResponse[UploadInitializationResponse]:
    return ApiResponse(
        data=await service.initialize_version_upload(
            session,
            claims,
            media_object_id,
            payload,
            storage,
            settings,
        )
    )


@router.post(
    "/media/uploads/{upload_session_id}/complete",
    response_model=ApiResponse[UploadCompletionResponse],
)
async def complete_upload(
    upload_session_id: UUID,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    storage: Annotated[MediaStorage, Depends(get_media_storage)],
) -> ApiResponse[UploadCompletionResponse]:
    return ApiResponse(
        data=await service.complete_upload(
            session,
            claims,
            upload_session_id,
            storage,
            trace_id=str(request.state.request_id),
        )
    )


@router.get("/media", response_model=ApiResponse[PaginatedMedia])
async def list_media(
    workspace_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    kind: MediaKind | None = None,
    source_type: MediaSource | None = None,
    include_archived: bool = False,
    created_from: datetime | None = None,
    created_to: datetime | None = None,
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedMedia]:
    return ApiResponse(
        data=await service.list_media(
            session,
            claims,
            workspace_id,
            kind=kind,
            source_type=source_type,
            include_archived=include_archived,
            created_from=created_from,
            created_to=created_to,
            limit=limit or 20,
            offset=offset,
        )
    )


@router.get("/media/{version_id}", response_model=ApiResponse[MediaVersionResponse])
async def get_media(
    version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[MediaVersionResponse]:
    return ApiResponse(data=await service.get_media(session, claims, version_id))


@router.get(
    "/media/{version_id}/locations",
    response_model=ApiResponse[MediaLocationsResponse],
)
async def list_media_locations(
    version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[MediaLocationsResponse]:
    return ApiResponse(
        data=await service.list_media_locations(session, claims, version_id)
    )


@router.post(
    "/media/{version_id}/location-migrations",
    response_model=ApiResponse[TaskResponse],
    status_code=status.HTTP_202_ACCEPTED,
)
async def request_media_location_migration(
    version_id: UUID,
    payload: MediaLocationMigrationRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[TaskResponse]:
    return ApiResponse(
        data=await service.request_media_location_migration(
            session,
            claims,
            version_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/media/{version_id}/location-rollbacks",
    response_model=ApiResponse[TaskResponse],
    status_code=status.HTTP_202_ACCEPTED,
)
async def request_media_location_rollback(
    version_id: UUID,
    payload: MediaLocationRollbackRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[TaskResponse]:
    return ApiResponse(
        data=await service.request_media_location_rollback(
            session,
            claims,
            version_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/media/{version_id}/access",
    response_model=ApiResponse[MediaAccessResponse],
)
async def create_access(
    version_id: UUID,
    payload: MediaAccessRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    storage: Annotated[MediaStorage, Depends(get_media_storage)],
    settings: Annotated[Settings, Depends(get_request_settings)],
) -> ApiResponse[MediaAccessResponse]:
    return ApiResponse(
        data=await service.create_access(
            session, claims, version_id, payload, storage, settings
        )
    )


@router.post(
    "/media/{version_id}/probe-retry",
    response_model=ApiResponse[TaskResponse],
    status_code=status.HTTP_202_ACCEPTED,
)
async def retry_probe(
    version_id: UUID,
    payload: ProbeRetryRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[TaskResponse]:
    return ApiResponse(
        data=await service.retry_probe(
            session,
            claims,
            version_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/media-objects/{media_object_id}/archive",
    response_model=ApiResponse[MediaObjectResponse],
)
async def archive_media(
    media_object_id: UUID,
    payload: ArchiveMediaRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[MediaObjectResponse]:
    return ApiResponse(
        data=await service.archive_media(
            session,
            claims,
            media_object_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/media-objects/{media_object_id}/restore",
    response_model=ApiResponse[MediaObjectResponse],
)
async def restore_media(
    media_object_id: UUID,
    payload: ArchiveMediaRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[MediaObjectResponse]:
    return ApiResponse(
        data=await service.restore_media(
            session,
            claims,
            media_object_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/media-objects/{media_object_id}/current-version",
    response_model=ApiResponse[MediaObjectResponse],
)
async def set_current_media_version(
    media_object_id: UUID,
    payload: CurrentMediaVersionRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[MediaObjectResponse]:
    return ApiResponse(
        data=await service.set_current_version(
            session,
            claims,
            media_object_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )
