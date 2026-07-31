from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.storyboards import service
from app.modules.storyboards.schemas import (
    AssetUpgradeApplyRequest,
    AssetUpgradeApplyResponse,
    AssetUpgradePreflightRequest,
    AssetUpgradePreflightResponse,
    CopyShotRequest,
    MergePreflightRequest,
    MergeShotRequest,
    PaginatedAssetShotUsages,
    ShotCreateRequest,
    ShotCurrentSpecRequest,
    ShotDeletePreflightResponse,
    ShotDeleteResponse,
    ShotOrderResponse,
    ShotReadinessBatchResponse,
    ShotReadinessResponse,
    ShotReorderRequest,
    ShotResponse,
    ShotSpecCreateRequest,
    ShotSpecCreateResponse,
    ShotSpecVersionResponse,
    ShotStateRequest,
    ShotStateResponse,
    ShotTransformPreflightResponse,
    ShotTransformResponse,
    ShotUpdateRequest,
    SplitPreflightRequest,
    SplitShotRequest,
)

router = APIRouter(prefix="/api/v1", tags=["storyboards"])


@router.get(
    "/asset-versions/{asset_version_id}/shot-usages",
    response_model=ApiResponse[PaginatedAssetShotUsages],
)
async def list_asset_shot_usages(
    asset_version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedAssetShotUsages]:
    return ApiResponse(
        data=await service.list_asset_shot_usages(
            session,
            claims,
            asset_version_id,
            limit=limit or 20,
            offset=offset,
        )
    )


@router.post(
    "/asset-versions/{asset_version_id}/upgrade-preflight",
    response_model=ApiResponse[AssetUpgradePreflightResponse],
)
async def preflight_asset_upgrade(
    asset_version_id: UUID,
    payload: AssetUpgradePreflightRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetUpgradePreflightResponse]:
    return ApiResponse(
        data=await service.preflight_asset_upgrade(
            session,
            claims,
            asset_version_id,
            payload,
        )
    )


@router.post(
    "/asset-versions/{asset_version_id}/upgrade",
    response_model=ApiResponse[AssetUpgradeApplyResponse],
    status_code=status.HTTP_201_CREATED,
)
async def apply_asset_upgrade(
    asset_version_id: UUID,
    payload: AssetUpgradeApplyRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetUpgradeApplyResponse]:
    return ApiResponse(
        data=await service.apply_asset_upgrade(
            session,
            claims,
            asset_version_id,
            payload,
        )
    )


@router.post(
    "/extraction-candidates/{candidate_id}/shot",
    response_model=ApiResponse[ShotResponse],
    status_code=status.HTTP_201_CREATED,
)
async def create_from_confirmed_candidate(
    candidate_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotResponse]:
    return ApiResponse(
        data=await service.create_from_confirmed_candidate(
            session,
            claims,
            candidate_id,
        )
    )


@router.post(
    "/shots/merge-preflight",
    response_model=ApiResponse[ShotTransformPreflightResponse],
)
async def merge_preflight(
    payload: MergePreflightRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotTransformPreflightResponse]:
    return ApiResponse(
        data=await service.merge_preflight(session, claims, payload)
    )


@router.post(
    "/shots/merge",
    response_model=ApiResponse[ShotTransformResponse],
    status_code=status.HTTP_201_CREATED,
)
async def merge_shots(
    payload: MergeShotRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotTransformResponse]:
    return ApiResponse(data=await service.merge_shots(session, claims, payload))


@router.post(
    "/episodes/{episode_id}/shots",
    response_model=ApiResponse[ShotResponse],
    status_code=status.HTTP_201_CREATED,
)
async def create_manual_shot(
    episode_id: UUID,
    payload: ShotCreateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotResponse]:
    return ApiResponse(
        data=await service.create_manual_shot(session, claims, episode_id, payload)
    )


@router.get(
    "/episodes/{episode_id}/archived-shots",
    response_model=ApiResponse[list[ShotResponse]],
)
async def list_archived_shots(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[list[ShotResponse]]:
    return ApiResponse(
        data=await service.list_archived_shots(
            session,
            claims,
            episode_id,
        )
    )


@router.get(
    "/episodes/{episode_id}/shots",
    response_model=ApiResponse[ShotOrderResponse],
)
async def list_shots(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotOrderResponse]:
    return ApiResponse(data=await service.list_shots(session, claims, episode_id))


@router.get(
    "/episodes/{episode_id}/shot-readiness",
    response_model=ApiResponse[ShotReadinessBatchResponse],
)
async def get_episode_readiness(
    episode_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotReadinessBatchResponse]:
    return ApiResponse(
        data=await service.get_episode_readiness(
            session,
            claims,
            episode_id,
        )
    )


@router.post(
    "/episodes/{episode_id}/shots/reorder",
    response_model=ApiResponse[ShotOrderResponse],
)
async def reorder_shots(
    episode_id: UUID,
    payload: ShotReorderRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotOrderResponse]:
    return ApiResponse(
        data=await service.reorder_shots(session, claims, episode_id, payload)
    )


@router.get(
    "/shots/{shot_id}/readiness",
    response_model=ApiResponse[ShotReadinessResponse],
)
async def get_readiness(
    shot_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    version_id: UUID | None = None,
) -> ApiResponse[ShotReadinessResponse]:
    return ApiResponse(
        data=await service.get_readiness(
            session,
            claims,
            shot_id,
            version_id=version_id,
        )
    )


@router.get("/shots/{shot_id}", response_model=ApiResponse[ShotResponse])
async def get_shot(
    shot_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotResponse]:
    return ApiResponse(data=await service.get_shot(session, claims, shot_id))


@router.patch("/shots/{shot_id}", response_model=ApiResponse[ShotResponse])
async def update_shot(
    shot_id: UUID,
    payload: ShotUpdateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotResponse]:
    return ApiResponse(data=await service.update_shot(session, claims, shot_id, payload))


@router.post(
    "/shots/{shot_id}/archive",
    response_model=ApiResponse[ShotStateResponse],
)
async def archive_shot(
    shot_id: UUID,
    payload: ShotStateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotStateResponse]:
    return ApiResponse(
        data=await service.set_shot_archived(
            session, claims, shot_id, payload, archived=True
        )
    )


@router.post(
    "/shots/{shot_id}/restore",
    response_model=ApiResponse[ShotStateResponse],
)
async def restore_shot(
    shot_id: UUID,
    payload: ShotStateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotStateResponse]:
    return ApiResponse(
        data=await service.set_shot_archived(
            session, claims, shot_id, payload, archived=False
        )
    )


@router.post(
    "/shots/{shot_id}/copy",
    response_model=ApiResponse[ShotTransformResponse],
    status_code=status.HTTP_201_CREATED,
)
async def copy_shot(
    shot_id: UUID,
    payload: CopyShotRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotTransformResponse]:
    return ApiResponse(data=await service.copy_shot(session, claims, shot_id, payload))


@router.post(
    "/shots/{shot_id}/split-preflight",
    response_model=ApiResponse[ShotTransformPreflightResponse],
)
async def split_preflight(
    shot_id: UUID,
    payload: SplitPreflightRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotTransformPreflightResponse]:
    return ApiResponse(
        data=await service.split_preflight(session, claims, shot_id, payload)
    )


@router.post(
    "/shots/{shot_id}/split",
    response_model=ApiResponse[ShotTransformResponse],
    status_code=status.HTTP_201_CREATED,
)
async def split_shot(
    shot_id: UUID,
    payload: SplitShotRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotTransformResponse]:
    return ApiResponse(
        data=await service.split_shot(session, claims, shot_id, payload)
    )


@router.post(
    "/shots/{shot_id}/spec-versions",
    response_model=ApiResponse[ShotSpecCreateResponse],
    status_code=status.HTTP_201_CREATED,
)
async def append_spec_version(
    shot_id: UUID,
    payload: ShotSpecCreateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotSpecCreateResponse]:
    return ApiResponse(
        data=await service.append_spec_version(session, claims, shot_id, payload)
    )


@router.get(
    "/shots/{shot_id}/spec-versions",
    response_model=ApiResponse[list[ShotSpecVersionResponse]],
)
async def list_spec_versions(
    shot_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[list[ShotSpecVersionResponse]]:
    return ApiResponse(
        data=await service.list_spec_versions(session, claims, shot_id)
    )


@router.get(
    "/shot-spec-versions/{version_id}",
    response_model=ApiResponse[ShotSpecVersionResponse],
)
async def get_spec_version(
    version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotSpecVersionResponse]:
    return ApiResponse(
        data=await service.get_spec_version(session, claims, version_id)
    )


@router.post(
    "/shots/{shot_id}/current-spec-version",
    response_model=ApiResponse[ShotResponse],
)
async def set_current_spec_version(
    shot_id: UUID,
    payload: ShotCurrentSpecRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotResponse]:
    return ApiResponse(
        data=await service.set_current_spec_version(session, claims, shot_id, payload)
    )


@router.get(
    "/shots/{shot_id}/delete-preflight",
    response_model=ApiResponse[ShotDeletePreflightResponse],
)
async def shot_delete_preflight(
    shot_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotDeletePreflightResponse]:
    return ApiResponse(data=await service.delete_preflight(session, claims, shot_id))


@router.delete(
    "/shots/{shot_id}",
    response_model=ApiResponse[ShotDeleteResponse],
)
async def delete_shot(
    shot_id: UUID,
    expected_revision: Annotated[int, Query(ge=1)],
    expected_order_hash: Annotated[str, Query(min_length=64, max_length=64)],
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ShotDeleteResponse]:
    return ApiResponse(
        data=await service.delete_shot(
            session,
            claims,
            shot_id,
            expected_revision=expected_revision,
            expected_order_hash=expected_order_hash,
        )
    )
