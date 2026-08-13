from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.assets import impact, service
from app.modules.assets.contracts import (
    AssetOccurrenceNarrativeReader,
    AssetOccurrenceNarrativeSnapshot,
    AssetProductionImpactReader,
    AssetPromptSnapshot,
    AssetShotUsageReader,
    AssetShotUsageSnapshot,
    AssetTaskSnapshot,
)
from app.modules.assets.schemas import (
    AssetAvailabilityResponse,
    AssetBibleResponse,
    AssetCreateRequest,
    AssetDeletePreflightResponse,
    AssetDeleteResponse,
    AssetDisablePreflightRequest,
    AssetDisableRequest,
    AssetEnableRequest,
    AssetImpactResponse,
    AssetKind,
    AssetOccurrenceDecisionResponse,
    AssetOccurrenceRequest,
    AssetReadinessResponse,
    AssetRenamePreflightRequest,
    AssetRenameRequest,
    AssetRenameResponse,
    AssetResponse,
    AssetStateAvailabilityResponse,
    AssetStateCreateRequest,
    AssetStateCreateResponse,
    AssetStateCurrentPreflightRequest,
    AssetStateCurrentRequest,
    AssetStateCurrentResponse,
    AssetStateEnableRequest,
    AssetStateReadinessResponse,
    AssetStateResponse,
    AssetStateUpdateRequest,
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


def _shot_usage_reader(session: AsyncSession) -> AssetShotUsageReader:
    async def read_usages(
        *,
        workspace_id: UUID,
        asset_version_ids: list[UUID],
        for_update: bool,
    ) -> list[AssetShotUsageSnapshot]:
        from app.modules.storyboards import read_asset_usages

        rows = await read_asset_usages(
            session,
            workspace_id=workspace_id,
            asset_version_ids=asset_version_ids,
            for_update=for_update,
        )
        return [
            AssetShotUsageSnapshot(
                shot_id=item.shot_id,
                shot_title=item.shot_title,
                episode_id=item.episode_id,
                spec_version_id=item.spec_version_id,
                spec_version_no=item.spec_version_no,
                current_spec_version_id=item.current_spec_version_id,
                shot_status=item.shot_status,
                slot_keys=item.slot_keys,
            )
            for item in rows
        ]

    return read_usages


def _production_impact_reader(session: AsyncSession) -> AssetProductionImpactReader:
    async def read_impacts(
        *,
        workspace_id: UUID,
        project_id: UUID,
        asset_version_ids: list[UUID],
        for_update: bool,
    ) -> tuple[list[AssetPromptSnapshot], list[AssetTaskSnapshot]]:
        from app.modules.production import read_asset_production_impacts

        prompt_rows, task_rows = await read_asset_production_impacts(
            session,
            workspace_id=workspace_id,
            project_id=project_id,
            asset_version_ids=asset_version_ids,
            for_update=for_update,
        )
        return (
            [
                AssetPromptSnapshot(
                    generation_request_id=item.generation_request_id,
                    episode_id=item.episode_id,
                    shot_id=item.shot_id,
                    shot_spec_version_id=item.shot_spec_version_id,
                    input_hash=item.input_hash,
                )
                for item in prompt_rows
            ],
            [
                AssetTaskSnapshot(
                    task_id=item.task_id,
                    generation_request_id=item.generation_request_id,
                    status=item.status,
                    revision=item.revision,
                )
                for item in task_rows
            ],
        )

    return read_impacts


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


@router.patch(
    "/asset-states/{state_id}",
    response_model=ApiResponse[AssetStateResponse],
)
async def update_asset_state(
    state_id: UUID,
    payload: AssetStateUpdateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetStateResponse]:
    return ApiResponse(
        data=await impact.update_state(
            session,
            claims,
            state_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


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
    "/assets/{asset_id}/rename-preflight",
    response_model=ApiResponse[AssetImpactResponse],
)
async def asset_rename_preflight(
    asset_id: UUID,
    payload: AssetRenamePreflightRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetImpactResponse]:
    return ApiResponse(
        data=await impact.rename_preflight(
            session,
            claims,
            asset_id,
            payload,
            _shot_usage_reader(session),
            _production_impact_reader(session),
        )
    )


@router.post(
    "/assets/{asset_id}/rename",
    response_model=ApiResponse[AssetRenameResponse],
)
async def rename_asset(
    asset_id: UUID,
    payload: AssetRenameRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetRenameResponse]:
    return ApiResponse(
        data=await impact.rename_asset(
            session,
            claims,
            asset_id,
            payload,
            _shot_usage_reader(session),
            _production_impact_reader(session),
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/assets/{asset_id}/disable-preflight",
    response_model=ApiResponse[AssetImpactResponse],
)
async def asset_disable_preflight(
    asset_id: UUID,
    payload: AssetDisablePreflightRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetImpactResponse]:
    return ApiResponse(
        data=await impact.asset_disable_preflight(
            session,
            claims,
            asset_id,
            payload,
            _shot_usage_reader(session),
            _production_impact_reader(session),
        )
    )


@router.post(
    "/assets/{asset_id}/disable",
    response_model=ApiResponse[AssetAvailabilityResponse],
)
async def disable_asset(
    asset_id: UUID,
    payload: AssetDisableRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetAvailabilityResponse]:
    return ApiResponse(
        data=await impact.disable_asset(
            session,
            claims,
            asset_id,
            payload,
            _shot_usage_reader(session),
            _production_impact_reader(session),
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/assets/{asset_id}/enable",
    response_model=ApiResponse[AssetResponse],
)
async def enable_asset(
    asset_id: UUID,
    payload: AssetEnableRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetResponse]:
    return ApiResponse(
        data=await impact.enable_asset(
            session,
            claims,
            asset_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post("/assets/{asset_id}/archive", response_model=ApiResponse[AssetResponse])
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


@router.post("/assets/{asset_id}/restore", response_model=ApiResponse[AssetResponse])
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


@router.delete("/assets/{asset_id}", response_model=ApiResponse[AssetDeleteResponse])
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


@router.post(
    "/asset-states/{state_id}/disable-preflight",
    response_model=ApiResponse[AssetImpactResponse],
)
async def asset_state_disable_preflight(
    state_id: UUID,
    payload: AssetDisablePreflightRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetImpactResponse]:
    return ApiResponse(
        data=await impact.state_disable_preflight(
            session,
            claims,
            state_id,
            payload,
            _shot_usage_reader(session),
            _production_impact_reader(session),
        )
    )


@router.post(
    "/asset-states/{state_id}/disable",
    response_model=ApiResponse[AssetStateAvailabilityResponse],
)
async def disable_asset_state(
    state_id: UUID,
    payload: AssetDisableRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetStateAvailabilityResponse]:
    return ApiResponse(
        data=await impact.disable_state(
            session,
            claims,
            state_id,
            payload,
            _shot_usage_reader(session),
            _production_impact_reader(session),
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/asset-states/{state_id}/enable",
    response_model=ApiResponse[AssetStateResponse],
)
async def enable_asset_state(
    state_id: UUID,
    payload: AssetStateEnableRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetStateResponse]:
    return ApiResponse(
        data=await impact.enable_state(
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
    return ApiResponse(data=await service.get_version(session, claims, version_id))


@router.post(
    "/asset-states/{state_id}/current-version-preflight",
    response_model=ApiResponse[AssetImpactResponse],
)
async def current_asset_version_preflight(
    state_id: UUID,
    payload: AssetStateCurrentPreflightRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetImpactResponse]:
    return ApiResponse(
        data=await impact.current_version_preflight(
            session,
            claims,
            state_id,
            payload,
            _shot_usage_reader(session),
            _production_impact_reader(session),
        )
    )


@router.post(
    "/asset-states/{state_id}/current-version",
    response_model=ApiResponse[AssetStateCurrentResponse],
)
async def set_current_asset_version(
    state_id: UUID,
    payload: AssetStateCurrentRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[AssetStateCurrentResponse]:
    return ApiResponse(
        data=await impact.set_current_version(
            session,
            claims,
            state_id,
            payload,
            _shot_usage_reader(session),
            _production_impact_reader(session),
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
