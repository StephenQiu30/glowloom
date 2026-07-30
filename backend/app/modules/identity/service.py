from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from pwdlib import PasswordHash
from pydantic import SecretStr
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims, create_access_token
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import repository
from app.modules.identity.contracts import ActorContext, AuthenticatedUser, Capability
from app.modules.identity.models import Membership, UserAccount, Workspace
from app.modules.identity.policy import (
    require_capability,
    require_workspace_capability,
)
from app.modules.identity.schemas import (
    AuthResponse,
    ChangePasswordRequest,
    DeactivateAccountRequest,
    LoginRequest,
    MeResponse,
    ProfileUpdateRequest,
    RegisterRequest,
    UserResponse,
    WorkspaceCreateRequest,
    WorkspaceResponse,
    WorkspaceStateRequest,
    WorkspaceUpdateRequest,
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
        revision=workspace.revision,
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


async def get_authenticated_user(
    session: AsyncSession, claims: AccessTokenClaims
) -> AuthenticatedUser:
    user = await authenticated_user(session, claims)
    return AuthenticatedUser(id=user.id)


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


def _workspace_not_found() -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, "Workspace not found", status_code=404)


def _require_owner(membership: Membership) -> None:
    try:
        require_capability(membership.role, Capability.WORKSPACE_MANAGE)
    except PermissionError as error:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403) from error


def _require_revision(workspace: Workspace, expected_revision: int) -> None:
    if workspace.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Workspace has changed",
            status_code=409,
            details={"current_revision": workspace.revision},
        )


async def update_profile(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: ProfileUpdateRequest,
) -> MeResponse:
    if request.display_name is None and request.avatar_url is None:
        raise ApiError(ErrorCode.INVALID_REQUEST, "No profile changes supplied", status_code=422)
    async with session.begin():
        user = await authenticated_user(session, claims)
        if request.display_name is not None:
            user.display_name = request.display_name.strip()
        if request.avatar_url is not None:
            user.avatar_url = str(request.avatar_url)
        primary = await repository.find_primary_workspace(session, user.id)
        if primary is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Account workspace is unavailable",
                status_code=500,
            )
        await session.flush()
    return MeResponse(
        user=_user_response(user),
        workspace=_workspace_response(primary[0], primary[1]),
    )


async def list_workspaces(
    session: AsyncSession,
    claims: AccessTokenClaims,
    *,
    include_archived: bool,
) -> list[WorkspaceResponse]:
    user = await authenticated_user(session, claims)
    workspaces = await repository.list_workspaces(
        session, user.id, include_archived=include_archived
    )
    return [_workspace_response(workspace, membership) for workspace, membership in workspaces]


async def get_workspace(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
) -> WorkspaceResponse:
    user = await authenticated_user(session, claims)
    result = await repository.find_workspace_for_user(session, user.id, workspace_id)
    if result is None:
        raise _workspace_not_found()
    return _workspace_response(result[0], result[1])


async def actor_context(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    capability: Capability,
) -> ActorContext:
    user = await authenticated_user(session, claims)
    result = await repository.find_workspace_for_user(session, user.id, workspace_id)
    if result is None:
        raise _workspace_not_found()
    workspace, membership = result
    try:
        require_workspace_capability(membership.role, workspace.status, capability)
    except PermissionError as error:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403) from error
    return ActorContext(
        user_id=user.id,
        workspace_id=workspace.id,
        membership_id=membership.id,
        role=membership.role,
        workspace_status=workspace.status,
    )


async def create_workspace(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: WorkspaceCreateRequest,
) -> WorkspaceResponse:
    workspace_id = uuid7()
    async with session.begin():
        user = await authenticated_user(session, claims)
        workspace = Workspace(id=workspace_id, name=request.name.strip())
        membership = Membership(
            user_id=user.id,
            workspace_id=workspace_id,
            role="owner",
        )
        session.add(workspace)
        await session.flush()
        session.add(membership)
        await session.flush()
    return _workspace_response(workspace, membership)


async def update_workspace(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    request: WorkspaceUpdateRequest,
) -> WorkspaceResponse:
    async with session.begin():
        user = await authenticated_user(session, claims)
        result = await repository.find_workspace_for_user(
            session, user.id, workspace_id, for_update=True
        )
        if result is None:
            raise _workspace_not_found()
        workspace, membership = result
        _require_owner(membership)
        _require_revision(workspace, request.expected_revision)
        if workspace.status != "active":
            raise ApiError(ErrorCode.STATE_CONFLICT, "Workspace is archived", status_code=409)
        workspace.name = request.name.strip()
        workspace.revision += 1
        await session.flush()
    return _workspace_response(workspace, membership)


async def set_workspace_archived(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    request: WorkspaceStateRequest,
    *,
    archived: bool,
) -> WorkspaceResponse:
    expected_status = "active" if archived else "archived"
    async with session.begin():
        user = await authenticated_user(session, claims)
        result = await repository.find_workspace_for_user(
            session, user.id, workspace_id, for_update=True
        )
        if result is None:
            raise _workspace_not_found()
        workspace, membership = result
        _require_owner(membership)
        _require_revision(workspace, request.expected_revision)
        if workspace.status != expected_status:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Workspace state does not allow this action",
                status_code=409,
            )
        workspace.status = "archived" if archived else "active"
        workspace.archived_at = datetime.now(UTC) if archived else None
        workspace.revision += 1
        await session.flush()
    return _workspace_response(workspace, membership)


async def deactivate_account(
    session: AsyncSession,
    claims: AccessTokenClaims,
    _: DeactivateAccountRequest,
) -> None:
    async with session.begin():
        user = await authenticated_user(session, claims)
        user.status = "deactivated"
        user.token_version += 1
        await session.flush()
