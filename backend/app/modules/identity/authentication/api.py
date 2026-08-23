from typing import Annotated

from fastapi import APIRouter, Depends, Request, Response, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims, get_request_settings
from app.core.config import Settings
from app.core.database import get_async_session
from app.core.errors import ApiError, ErrorCode
from app.core.schemas import ApiResponse
from app.modules.identity.authentication import service
from app.modules.identity.authentication.contracts import AuthSessionStore
from app.modules.identity.authentication.dependencies import get_auth_session_store
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
from app.modules.identity.registration_verifications.contracts import (
    RegistrationVerificationStore,
)
from app.modules.identity.registration_verifications.dependencies import (
    get_registration_verification_store,
)

router = APIRouter(prefix="/api/v1", tags=["identity"])
_REFRESH_COOKIE_NAME = "lanverse_refresh_token"


def _set_refresh_cookie(response: Response, refresh_token: str, settings: Settings) -> None:
    response.set_cookie(
        key=_REFRESH_COOKIE_NAME,
        value=refresh_token,
        max_age=settings.auth_session_ttl_seconds,
        httponly=True,
        secure=settings.environment == "production",
        samesite="lax",
        path="/api/v1/auth",
    )


def _clear_refresh_cookie(response: Response) -> None:
    response.delete_cookie(
        key=_REFRESH_COOKIE_NAME,
        httponly=True,
        samesite="lax",
        path="/api/v1/auth",
    )


@router.post(
    "/auth/register",
    response_model=ApiResponse[AuthResponse],
    status_code=status.HTTP_201_CREATED,
)
async def register(
    payload: RegisterRequest,
    request: Request,
    response: Response,
    session: Annotated[AsyncSession, Depends(get_async_session)],
    settings: Annotated[Settings, Depends(get_request_settings)],
    auth_sessions: Annotated[AuthSessionStore, Depends(get_auth_session_store)],
    verification_store: Annotated[
        RegistrationVerificationStore,
        Depends(get_registration_verification_store),
    ],
) -> ApiResponse[AuthResponse]:
    result = await service.register(
        session,
        payload,
        settings,
        verification_store,
        auth_sessions,
        trace_id=str(request.state.request_id),
    )
    _set_refresh_cookie(response, result.refresh_token, settings)
    return ApiResponse(data=result.response)


@router.post("/auth/login", response_model=ApiResponse[AuthResponse])
async def login(
    payload: LoginRequest,
    request: Request,
    response: Response,
    session: Annotated[AsyncSession, Depends(get_async_session)],
    settings: Annotated[Settings, Depends(get_request_settings)],
    auth_sessions: Annotated[AuthSessionStore, Depends(get_auth_session_store)],
) -> ApiResponse[AuthResponse]:
    result = await service.login(
        session,
        payload,
        settings,
        auth_sessions,
        trace_id=str(request.state.request_id),
    )
    _set_refresh_cookie(response, result.refresh_token, settings)
    return ApiResponse(data=result.response)


@router.post("/auth/refresh", response_model=ApiResponse[AuthResponse])
async def refresh(
    request: Request,
    response: Response,
    session: Annotated[AsyncSession, Depends(get_async_session)],
    settings: Annotated[Settings, Depends(get_request_settings)],
    auth_sessions: Annotated[AuthSessionStore, Depends(get_auth_session_store)],
) -> ApiResponse[AuthResponse]:
    refresh_token = request.cookies.get(_REFRESH_COOKIE_NAME)
    if refresh_token is None:
        raise ApiError(
            ErrorCode.UNAUTHENTICATED,
            "Invalid credentials",
            status_code=401,
            next_action="login",
        )
    try:
        result = await service.refresh(
            session,
            refresh_token,
            settings,
            auth_sessions,
        )
    except ApiError as error:
        if error.code == ErrorCode.UNAUTHENTICATED:
            _clear_refresh_cookie(response)
        raise
    _set_refresh_cookie(response, result.refresh_token, settings)
    return ApiResponse(data=result.response)


@router.post("/auth/logout", response_model=ApiResponse[RevocationResponse])
async def logout(
    request: Request,
    response: Response,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    auth_sessions: Annotated[AuthSessionStore, Depends(get_auth_session_store)],
) -> ApiResponse[RevocationResponse]:
    await service.logout(
        session,
        claims,
        trace_id=str(request.state.request_id),
        auth_sessions=auth_sessions,
        refresh_token=request.cookies.get(_REFRESH_COOKIE_NAME),
    )
    _clear_refresh_cookie(response)
    return ApiResponse(data=RevocationResponse())


@router.post("/auth/change-password", response_model=ApiResponse[RevocationResponse])
async def change_password(
    payload: ChangePasswordRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[RevocationResponse]:
    await service.change_password(
        session,
        claims,
        payload,
        trace_id=str(request.state.request_id),
    )
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
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[MeResponse]:
    return ApiResponse(
        data=await service.update_profile(
            session,
            claims,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post("/me/deactivate", response_model=ApiResponse[RevocationResponse])
async def deactivate_me(
    payload: DeactivateAccountRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[RevocationResponse]:
    await service.deactivate_account(
        session,
        claims,
        payload,
        trace_id=str(request.state.request_id),
    )
    return ApiResponse(data=RevocationResponse())
