from typing import Annotated

from fastapi import APIRouter, Depends, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims, get_request_settings
from app.core.config import Settings
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.identity.authentication import service
from app.modules.identity.authentication.schemas import (
    AuthResponse,
    ChangePasswordRequest,
    DeactivateAccountRequest,
    LoginRequest,
    MeResponse,
    ProfileUpdateRequest,
    RegisterRequest,
    RevocationResponse,
)

router = APIRouter(prefix="/api/v1", tags=["identity"])


@router.post(
    "/auth/register",
    response_model=ApiResponse[AuthResponse],
    status_code=status.HTTP_201_CREATED,
)
async def register(
    payload: RegisterRequest,
    session: Annotated[AsyncSession, Depends(get_async_session)],
    settings: Annotated[Settings, Depends(get_request_settings)],
) -> ApiResponse[AuthResponse]:
    return ApiResponse(data=await service.register(session, payload, settings))


@router.post("/auth/login", response_model=ApiResponse[AuthResponse])
async def login(
    payload: LoginRequest,
    session: Annotated[AsyncSession, Depends(get_async_session)],
    settings: Annotated[Settings, Depends(get_request_settings)],
) -> ApiResponse[AuthResponse]:
    return ApiResponse(data=await service.login(session, payload, settings))


@router.post("/auth/logout", response_model=ApiResponse[RevocationResponse])
async def logout(
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[RevocationResponse]:
    await service.logout(session, claims)
    return ApiResponse(data=RevocationResponse())


@router.post("/auth/change-password", response_model=ApiResponse[RevocationResponse])
async def change_password(
    payload: ChangePasswordRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[RevocationResponse]:
    await service.change_password(session, claims, payload)
    return ApiResponse(data=RevocationResponse())


@router.get("/me", response_model=ApiResponse[MeResponse])
async def me(
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[MeResponse]:
    return ApiResponse(data=await service.me(session, claims))


@router.patch("/me", response_model=ApiResponse[MeResponse])
async def update_me(
    payload: ProfileUpdateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[MeResponse]:
    return ApiResponse(data=await service.update_profile(session, claims, payload))


@router.post("/me/deactivate", response_model=ApiResponse[RevocationResponse])
async def deactivate_me(
    payload: DeactivateAccountRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[RevocationResponse]:
    await service.deactivate_account(session, claims, payload)
    return ApiResponse(data=RevocationResponse())
