from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.governance.consents import service
from app.modules.governance.consents.schemas import (
    ConsentCreateRequest,
    ConsentDetailResponse,
    ConsentRevisionRequest,
    ConsentRevokeRequest,
    PaginatedConsents,
)

router = APIRouter()


@router.post(
    "/consents",
    response_model=ApiResponse[ConsentDetailResponse],
    status_code=status.HTTP_201_CREATED,
)
async def create_consent(
    payload: ConsentCreateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ConsentDetailResponse]:
    return ApiResponse(
        data=await service.create_consent(session, claims, payload)
    )


@router.get("/consents", response_model=ApiResponse[PaginatedConsents])
async def list_consents(
    workspace_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedConsents]:
    return ApiResponse(
        data=await service.list_consents(
            session,
            claims,
            workspace_id,
            limit=limit,
            offset=offset,
        )
    )


@router.get(
    "/consents/{consent_id}",
    response_model=ApiResponse[ConsentDetailResponse],
)
async def get_consent(
    consent_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ConsentDetailResponse]:
    return ApiResponse(
        data=await service.get_consent(session, claims, consent_id)
    )


@router.post(
    "/consents/{consent_id}/revisions",
    response_model=ApiResponse[ConsentDetailResponse],
    status_code=status.HTTP_201_CREATED,
)
async def revise_consent(
    consent_id: UUID,
    payload: ConsentRevisionRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ConsentDetailResponse]:
    return ApiResponse(
        data=await service.revise_consent(
            session, claims, consent_id, payload
        )
    )


@router.post(
    "/consents/{consent_id}/revoke",
    response_model=ApiResponse[ConsentDetailResponse],
)
async def revoke_consent(
    consent_id: UUID,
    payload: ConsentRevokeRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ConsentDetailResponse]:
    return ApiResponse(
        data=await service.revoke_consent(
            session, claims, consent_id, payload
        )
    )
