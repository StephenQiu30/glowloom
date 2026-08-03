from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.assets import service
from app.modules.assets.schemas import (
    AssetCreateRequest,
    AssetCurrentVersionRequest,
    AssetDeletePreflightResponse,
    AssetDeleteResponse,
    AssetKind,
    AssetReadinessResponse,
    AssetResponse,
    AssetStateRequest,
    AssetUpdateRequest,
    AssetVersionCreateRequest,
    AssetVersionCreateResponse,
    AssetVersionResponse,
    PaginatedAssets,
    PaginatedAssetVersions,
)

router = APIRouter(prefix="/api/v1", tags=["assets"])


@router.post(
    "/projects/{project_id}/assets",
    response_model=ApiResponse[AssetResponse],
    status_code=status.HTTP_201_CREATED,
)
async def create_asset(
    project_id: UUID,
    payload: AssetCreateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetResponse]:
    return ApiResponse(
        data=await service.create_asset(
            session,
            claims,
            project_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/projects/{project_id}/assets",
    response_model=ApiResponse[PaginatedAssets],
)
async def list_assets(
    project_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    kind: AssetKind | None = None,
    include_archived: bool = False,
    query: Annotated[str | None, Query(max_length=200)] = None,
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedAssets]:
    return ApiResponse(
        data=await service.list_assets(
            session,
            claims,
            project_id,
            kind=kind,
            include_archived=include_archived,
            query=query,
            limit=limit or 20,
            offset=offset,
        )
    )


@router.get("/assets/{asset_id}", response_model=ApiResponse[AssetResponse])
async def get_asset(
    asset_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetResponse]:
    return ApiResponse(data=await service.get_asset(session, claims, asset_id))


@router.patch("/assets/{asset_id}", response_model=ApiResponse[AssetResponse])
async def update_asset(
    asset_id: UUID,
    payload: AssetUpdateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetResponse]:
    return ApiResponse(
        data=await service.update_asset(
            session,
            claims,
            asset_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/assets/{asset_id}/archive", response_model=ApiResponse[AssetResponse]
)
async def archive_asset(
    asset_id: UUID,
    payload: AssetStateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetResponse]:
    return ApiResponse(
        data=await service.set_asset_archived(
            session,
            claims,
            asset_id,
            payload,
            archived=True,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/assets/{asset_id}/restore", response_model=ApiResponse[AssetResponse]
)
async def restore_asset(
    asset_id: UUID,
    payload: AssetStateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetResponse]:
    return ApiResponse(
        data=await service.set_asset_archived(
            session,
            claims,
            asset_id,
            payload,
            archived=False,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/assets/{asset_id}/delete-preflight",
    response_model=ApiResponse[AssetDeletePreflightResponse],
)
async def asset_delete_preflight(
    asset_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetDeletePreflightResponse]:
    return ApiResponse(
        data=await service.delete_preflight(session, claims, asset_id)
    )


@router.delete(
    "/assets/{asset_id}", response_model=ApiResponse[AssetDeleteResponse]
)
async def delete_asset(
    asset_id: UUID,
    expected_revision: Annotated[int, Query(ge=1)],
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetDeleteResponse]:
    await service.delete_asset(
        session,
        claims,
        asset_id,
        expected_revision,
        trace_id=str(request.state.request_id),
    )
    return ApiResponse(data=AssetDeleteResponse())


@router.post(
    "/assets/{asset_id}/versions",
    response_model=ApiResponse[AssetVersionCreateResponse],
    status_code=status.HTTP_201_CREATED,
)
async def append_asset_version(
    asset_id: UUID,
    payload: AssetVersionCreateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetVersionCreateResponse]:
    return ApiResponse(
        data=await service.append_version(
            session,
            claims,
            asset_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/assets/{asset_id}/versions",
    response_model=ApiResponse[PaginatedAssetVersions],
)
async def list_asset_versions(
    asset_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedAssetVersions]:
    return ApiResponse(
        data=await service.list_versions(
            session, claims, asset_id, limit=limit or 20, offset=offset
        )
    )


@router.get(
    "/asset-versions/{version_id}",
    response_model=ApiResponse[AssetVersionResponse],
)
async def get_asset_version(
    version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetVersionResponse]:
    return ApiResponse(
        data=await service.get_version(session, claims, version_id)
    )


@router.post(
    "/assets/{asset_id}/current-version",
    response_model=ApiResponse[AssetResponse],
)
async def set_current_asset_version(
    asset_id: UUID,
    payload: AssetCurrentVersionRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetResponse]:
    return ApiResponse(
        data=await service.set_current_version(
            session,
            claims,
            asset_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/asset-versions/{version_id}/readiness",
    response_model=ApiResponse[AssetReadinessResponse],
)
async def get_asset_readiness(
    version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    purpose: Annotated[str, Query(min_length=1, max_length=100)],
    channel: Annotated[str, Query(min_length=1, max_length=100)],
    region: Annotated[str, Query(min_length=2, max_length=10)],
) -> ApiResponse[AssetReadinessResponse]:
    return ApiResponse(
        data=await service.get_readiness(
            session,
            claims,
            version_id,
            purpose=purpose,
            channel=channel,
            region=region,
        )
    )
