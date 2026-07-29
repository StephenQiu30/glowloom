from typing import Annotated

from fastapi import APIRouter, Depends, Request, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, decode_access_token
from app.core.config import Settings
from app.core.database import get_async_session
from app.core.errors import ApiError, ErrorCode
from app.core.schemas import ApiResponse
from app.modules.identity import service
from app.modules.identity.schemas import (
    AuthResponse,
    ChangePasswordRequest,
    LoginRequest,
    MeResponse,
    RegisterRequest,
    RevocationResponse,
)

router = APIRouter(prefix="/api/v1", tags=["identity"])
bearer = HTTPBearer(auto_error=False)


def _settings(request: Request) -> Settings:
    return request.app.state.settings


def _claims(
    credentials: Annotated[HTTPAuthorizationCredentials | None, Depends(bearer)],
    settings: Annotated[Settings, Depends(_settings)],
) -> AccessTokenClaims:
    claims = (
        decode_access_token(credentials.credentials, settings)
        if credentials is not None and credentials.scheme.lower() == "bearer"
        else None
    )
    if claims is None:
        raise ApiError(
            ErrorCode.UNAUTHENTICATED,
            "Invalid credentials",
            status_code=401,
            next_action="login",
        )
    return claims


@router.post(
    "/auth/register",
    response_model=ApiResponse[AuthResponse],
    status_code=status.HTTP_201_CREATED,
)
async def register(
    payload: RegisterRequest,
    session: Annotated[AsyncSession, Depends(get_async_session)],
    settings: Annotated[Settings, Depends(_settings)],
) -> ApiResponse[AuthResponse]:
    return ApiResponse(data=await service.register(session, payload, settings))


@router.post("/auth/login", response_model=ApiResponse[AuthResponse])
async def login(
    payload: LoginRequest,
    session: Annotated[AsyncSession, Depends(get_async_session)],
    settings: Annotated[Settings, Depends(_settings)],
) -> ApiResponse[AuthResponse]:
    return ApiResponse(data=await service.login(session, payload, settings))


@router.post("/auth/logout", response_model=ApiResponse[RevocationResponse])
async def logout(
    claims: Annotated[AccessTokenClaims, Depends(_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[RevocationResponse]:
    await service.logout(session, claims)
    return ApiResponse(data=RevocationResponse())


@router.post("/auth/change-password", response_model=ApiResponse[RevocationResponse])
async def change_password(
    payload: ChangePasswordRequest,
    claims: Annotated[AccessTokenClaims, Depends(_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[RevocationResponse]:
    await service.change_password(session, claims, payload)
    return ApiResponse(data=RevocationResponse())


@router.get("/me", response_model=ApiResponse[MeResponse])
async def me(
    claims: Annotated[AccessTokenClaims, Depends(_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[MeResponse]:
    return ApiResponse(data=await service.me(session, claims))
