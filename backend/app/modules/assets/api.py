from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.assets import service
from app.modules.assets.contracts import (
    AssetOccurrenceNarrativeReader,
    AssetOccurrenceNarrativeSnapshot,
)
from app.modules.assets.schemas import (
    AssetBibleResponse,
    AssetCreateRequest,
    AssetDeletePreflightResponse,
    AssetDeleteResponse,
    AssetKind,
    AssetOccurrenceDecisionResponse,
    AssetOccurrenceRequest,
    AssetReadinessResponse,
    AssetResponse,
    AssetStateCreateRequest,
    AssetStateCreateResponse,
    AssetStateCurrentRequest,
    AssetStateReadinessResponse,
    AssetStateResponse,
    AssetStatusRequest,
    AssetUpdateRequest,
    AssetVersionCreateRequest,
    AssetVersionCreateResponse,
    AssetVersionResponse,
    PaginatedAssetOccurrences,
    PaginatedAssets,
    PaginatedAssetStates,
    PaginatedAssetVersions,
)

router = APIRouter(prefix="/api/v1", tags=["assets"])


def _narrative_reader(session: AsyncSession) -> AssetOccurrenceNarrativeReader:
    async def read_narratives(
        *,
        workspace_id: UUID,
        unit_version_ids: list[UUID],
    ) -> dict[UUID, AssetOccurrenceNarrativeSnapshot]:
        from app.modules.scripts import resolve_narrative_unit_versions

        resolved = await resolve_narrative_unit_versions(
            session,
            workspace_id,
            unit_version_ids,
        )
        return {
            version_id: AssetOccurrenceNarrativeSnapshot(
                workspace_id=item.workspace_id,
                project_id=item.project_id,
                episode_id=item.episode_id,
                script_version_id=item.script_version_id,
                narrative_unit_id=item.narrative_unit_id,
                narrative_unit_version_id=item.narrative_unit_version_id,
                current_script_version_id=item.current_script_version_id,
                current_unit_version_id=item.current_unit_version_id,
                text_hash=item.text_hash,
            )
            for version_id, item in resolved.items()
        }

    return read_narratives


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


@router.get(
    "/projects/{project_id}/asset-bible",
    response_model=ApiResponse[AssetBibleResponse],
)
async def get_asset_bible(
    project_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    purpose: Annotated[str, Query(min_length=1, max_length=100)],
    channel: Annotated[str, Query(min_length=1, max_length=100)],
    region: Annotated[str, Query(min_length=2, max_length=10)],
) -> ApiResponse[AssetBibleResponse]:
    return ApiResponse(
        data=await service.get_asset_bible(
            session,
            claims,
            project_id,
            _narrative_reader(session),
            purpose=purpose,
            channel=channel,
            region=region,
        )
    )


@router.get("/assets/{asset_id}", response_model=ApiResponse[AssetResponse])
async def get_asset(
    asset_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetResponse]:
    return ApiResponse(data=await service.get_asset(session, claims, asset_id))


@router.post(
    "/assets/{asset_id}/states",
    response_model=ApiResponse[AssetStateCreateResponse],
    status_code=status.HTTP_201_CREATED,
)
async def create_asset_state(
    asset_id: UUID,
    payload: AssetStateCreateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetStateCreateResponse]:
    return ApiResponse(
        data=await service.create_state(
            session,
            claims,
            asset_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/assets/{asset_id}/states",
    response_model=ApiResponse[PaginatedAssetStates],
)
async def list_asset_states(
    asset_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[PaginatedAssetStates]:
    return ApiResponse(data=await service.list_states(session, claims, asset_id))


@router.post(
    "/asset-states/{state_id}/occurrence-decisions",
    response_model=ApiResponse[AssetOccurrenceDecisionResponse],
    status_code=status.HTTP_201_CREATED,
)
async def decide_asset_occurrence(
    state_id: UUID,
    payload: AssetOccurrenceRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetOccurrenceDecisionResponse]:
    return ApiResponse(
        data=await service.decide_occurrence(
            session,
            claims,
            state_id,
            payload,
            _narrative_reader(session),
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/asset-states/{state_id}/occurrences",
    response_model=ApiResponse[PaginatedAssetOccurrences],
)
async def list_asset_occurrences(
    state_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    include_history: bool = False,
) -> ApiResponse[PaginatedAssetOccurrences]:
    return ApiResponse(
        data=await service.list_occurrences(
            session,
            claims,
            state_id,
            _narrative_reader(session),
            include_history=include_history,
        )
    )


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
    payload: AssetStatusRequest,
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
    payload: AssetStatusRequest,
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
    async def read_candidate_decision_counts(
        *, workspace_id: UUID, asset_ids: list[UUID]
    ) -> dict[UUID, int]:
        from app.modules.scripts import count_asset_candidate_decisions

        return await count_asset_candidate_decisions(session, workspace_id, asset_ids)

    return ApiResponse(
        data=await service.delete_preflight(
            session,
            claims,
            asset_id,
            read_candidate_decision_counts,
        )
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
    async def read_candidate_decision_counts(
        *, workspace_id: UUID, asset_ids: list[UUID]
    ) -> dict[UUID, int]:
        from app.modules.scripts import count_asset_candidate_decisions

        return await count_asset_candidate_decisions(session, workspace_id, asset_ids)

    await service.delete_asset(
        session,
        claims,
        asset_id,
        expected_revision,
        read_candidate_decision_counts,
        trace_id=str(request.state.request_id),
    )
    return ApiResponse(data=AssetDeleteResponse())


@router.post(
    "/asset-states/{state_id}/versions",
    response_model=ApiResponse[AssetVersionCreateResponse],
    status_code=status.HTTP_201_CREATED,
)
async def append_asset_version(
    state_id: UUID,
    payload: AssetVersionCreateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetVersionCreateResponse]:
    return ApiResponse(
        data=await service.append_version(
            session,
            claims,
            state_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/asset-states/{state_id}/versions",
    response_model=ApiResponse[PaginatedAssetVersions],
)
async def list_asset_versions(
    state_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedAssetVersions]:
    return ApiResponse(
        data=await service.list_versions(
            session, claims, state_id, limit=limit or 20, offset=offset
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
    "/asset-states/{state_id}/current-version",
    response_model=ApiResponse[AssetStateResponse],
)
async def set_current_asset_version(
    state_id: UUID,
    payload: AssetStateCurrentRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetStateResponse]:
    return ApiResponse(
        data=await service.set_current_version(
            session,
            claims,
            state_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/asset-states/{state_id}/readiness",
    response_model=ApiResponse[AssetStateReadinessResponse],
)
async def get_asset_state_readiness(
    state_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    purpose: Annotated[str, Query(min_length=1, max_length=100)],
    channel: Annotated[str, Query(min_length=1, max_length=100)],
    region: Annotated[str, Query(min_length=2, max_length=10)],
) -> ApiResponse[AssetStateReadinessResponse]:
    return ApiResponse(
        data=await service.get_state_readiness(
            session,
            claims,
            state_id,
            purpose=purpose,
            channel=channel,
            region=region,
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
