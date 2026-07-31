from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.scripts.structure import service
from app.modules.scripts.structure.schemas import (
    ConfirmedStructureResponse,
    StructureConfirmationResponse,
)

router = APIRouter(prefix="/api/v1", tags=["scripts"])


@router.get(
    "/script-versions/{version_id}/structure",
    response_model=ApiResponse[ConfirmedStructureResponse],
)
async def get_confirmed_structure(
    version_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ConfirmedStructureResponse]:
    return ApiResponse(
        data=await service.get_confirmed_structure(
            session,
            claims,
            version_id,
        )
    )


@router.post(
    "/extraction-batches/{batch_id}/confirm-structure",
    response_model=ApiResponse[StructureConfirmationResponse],
    status_code=status.HTTP_201_CREATED,
)
async def confirm_structure(
    batch_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[StructureConfirmationResponse]:
    return ApiResponse(
        data=await service.confirm_structure(session, claims, batch_id)
    )
