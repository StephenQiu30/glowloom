from datetime import UTC, datetime
from typing import Literal, cast

from pwdlib import PasswordHash
from pydantic import SecretStr
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims, create_access_token
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import repository
from app.modules.identity.models import Membership, UserAccount, Workspace
from app.modules.identity.schemas import (
    AuthResponse,
    ChangePasswordRequest,
    LoginRequest,
    MeResponse,
    RegisterRequest,
    UserResponse,
    WorkspaceResponse,
)

_password_hash = PasswordHash.recommended()
_dummy_password_hash = _password_hash.hash("not-a-real-lanverse-password")


def _normalize_email(email: str) -> str:
    return email.strip().casefold()


def _plain_password(secret: SecretStr) -> str:
    value = secret.get_secret_value()
    if not 12 <= len(value) <= 128:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Password must contain between 12 and 128 characters",
            status_code=422,
        )
    return value


def _user_response(user: UserAccount) -> UserResponse:
    return UserResponse(
        id=user.id,
        email=user.email_normalized,
        display_name=user.display_name,
        avatar_url=user.avatar_url,
    )


def _workspace_response(workspace: Workspace, membership: Membership) -> WorkspaceResponse:
    return WorkspaceResponse(
        id=workspace.id,
        name=workspace.name,
        status=cast(Literal["active", "archived"], workspace.status),
        role=cast(Literal["owner", "editor", "viewer"], membership.role),
    )


def _auth_response(
    user: UserAccount,
    workspace: Workspace,
    membership: Membership,
    settings: Settings,
) -> AuthResponse:
    return AuthResponse(
        user=_user_response(user),
        workspace=_workspace_response(workspace, membership),
        access_token=create_access_token(user.id, user.token_version, settings),
        expires_in=settings.jwt_access_token_minutes * 60,
    )


async def register(
    session: AsyncSession, request: RegisterRequest, settings: Settings
) -> AuthResponse:
    email = _normalize_email(str(request.email))
    password = _plain_password(request.password)
    user_id = uuid7()
    workspace_id = uuid7()
    user = UserAccount(
        id=user_id,
        email_normalized=email,
        password_hash=_password_hash.hash(password),
        display_name=request.display_name.strip(),
        last_login_at=datetime.now(UTC),
    )
    workspace = Workspace(id=workspace_id, name=f"{request.display_name.strip()}的工作空间")
    membership = Membership(user_id=user_id, workspace_id=workspace_id, role="owner")
    try:
        async with session.begin():
            session.add_all((user, workspace))
            await session.flush()
            session.add(membership)
            await session.flush()
    except IntegrityError as error:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Account already exists",
            status_code=409,
        ) from error
    return _auth_response(user, workspace, membership, settings)


async def login(
    session: AsyncSession, request: LoginRequest, settings: Settings
) -> AuthResponse:
    email = _normalize_email(str(request.email))
    password = request.password.get_secret_value()
    async with session.begin():
        user = await repository.find_user_by_email(session, email)
        candidate_hash = user.password_hash if user is not None else _dummy_password_hash
        valid = _password_hash.verify(password, candidate_hash)
        if user is None or not valid or user.status != "active":
            raise _unauthenticated()
        primary = await repository.find_primary_workspace(session, user.id)
        if primary is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Account workspace is unavailable",
                status_code=500,
            )
        user.last_login_at = datetime.now(UTC)
        await session.flush()
    return _auth_response(user, primary[0], primary[1], settings)


async def authenticated_user(
    session: AsyncSession, claims: AccessTokenClaims
) -> UserAccount:
    user = await repository.find_user_by_id(session, claims.sub)
    if user is None or user.status != "active" or user.token_version != claims.ver:
        raise _unauthenticated()
    return user


async def me(session: AsyncSession, claims: AccessTokenClaims) -> MeResponse:
    user = await authenticated_user(session, claims)
    primary = await repository.find_primary_workspace(session, user.id)
    if primary is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Account workspace is unavailable",
            status_code=500,
        )
    return MeResponse(
        user=_user_response(user),
        workspace=_workspace_response(primary[0], primary[1]),
    )


async def logout(session: AsyncSession, claims: AccessTokenClaims) -> None:
    async with session.begin():
        user = await authenticated_user(session, claims)
        user.token_version += 1
        await session.flush()


async def change_password(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: ChangePasswordRequest,
) -> None:
    new_password = _plain_password(request.new_password)
    async with session.begin():
        user = await authenticated_user(session, claims)
        if not _password_hash.verify(
            request.current_password.get_secret_value(), user.password_hash
        ):
            raise _unauthenticated()
        user.password_hash = _password_hash.hash(new_password)
        user.token_version += 1
        await session.flush()


def _unauthenticated() -> ApiError:
    return ApiError(
        ErrorCode.UNAUTHENTICATED,
        "Invalid credentials",
        status_code=401,
        next_action="login",
    )
