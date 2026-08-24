from typing import Annotated, Literal
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
from app.modules.caching import HighCostGuardPort, get_high_cost_guard
from app.modules.production import generation
from app.modules.production.generation_schemas import (
    CostQueryResponse,
    GenerationPreflightRequest,
    GenerationPreflightResponse,
    GenerationSubmissionRequest,
    GenerationSubmissionResponse,
    ModelCapabilityResponse,
)

router = APIRouter(prefix="/api/v1", tags=["production"])


@router.get(
    "/model-capabilities",
    response_model=ApiResponse[list[ModelCapabilityResponse]],
)
async def list_model_capabilities(
    workspace_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    settings: Annotated[Settings, Depends(get_request_settings)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    kind: Literal["image", "video"] | None = None,
    model: Annotated[str | None, Query(min_length=1, max_length=160)] = None,
) -> ApiResponse[list[ModelCapabilityResponse]]:
    return ApiResponse(
        data=await generation.list_model_capabilities(
            session,
            claims,
            settings,
            workspace_id,
            kind=kind,
            model=model,
        )
    )


@router.post(
    "/shots/{shot_id}/generation-preflight",
    response_model=ApiResponse[GenerationPreflightResponse],
)
async def preflight_generation(
    shot_id: UUID,
    payload: GenerationPreflightRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    settings: Annotated[Settings, Depends(get_request_settings)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[GenerationPreflightResponse]:
    return ApiResponse(
        data=await generation.preflight_generation(
            session,
            claims,
            settings,
            shot_id,
            payload,
        )
    )


@router.post(
    "/shots/{shot_id}/generation-requests",
    response_model=ApiResponse[GenerationSubmissionResponse],
    status_code=status.HTTP_201_CREATED,
)
async def submit_generation(
    shot_id: UUID,
    payload: GenerationSubmissionRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    settings: Annotated[Settings, Depends(get_request_settings)],
    high_cost_guard: Annotated[HighCostGuardPort, Depends(get_high_cost_guard)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[GenerationSubmissionResponse]:
    return ApiResponse(
        data=await generation.submit_generation(
            session,
            claims,
            settings,
            shot_id,
            payload,
            trace_id=str(request.state.request_id),
            high_cost_guard=high_cost_guard,
        )
    )


@router.get("/costs", response_model=ApiResponse[CostQueryResponse])
async def get_costs(
    workspace_id: UUID,
    project_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[CostQueryResponse]:
    return ApiResponse(
        data=await generation.get_costs(
            session,
            claims,
            workspace_id,
            project_id,
            limit=limit or 20,
            offset=offset,
        )
    )
