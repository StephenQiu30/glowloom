from datetime import UTC, datetime

from pwdlib import PasswordHash
from pydantic import SecretStr
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims, create_access_token
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import repository
from app.modules.identity.authentication.schemas import (
    AuthResponse,
    ChangePasswordRequest,
    DeactivateAccountRequest,
    LoginRequest,
    MeResponse,
    ProfileUpdateRequest,
    RegisterRequest,
    UserResponse,
)
from app.modules.identity.contracts import AuthenticatedUser
from app.modules.identity.models import Membership, UserAccount, Workspace
from app.modules.identity.presenters import workspace_response
from app.modules.identity.registration_verifications.contracts import (
    RegistrationVerificationStore,
)
from app.modules.identity.registration_verifications.crypto import normalize_email
from app.modules.identity.registration_verifications.service import (
    consume_registration_ticket,
)

_password_hash = PasswordHash.recommended()
_dummy_password_hash = _password_hash.hash("not-a-real-lanverse-password")


def _plain_password(secret: SecretStr) -> str:
    value = secret.get_secret_value()
    if not 12 <= len(value) <= 128:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Password must contain between 12 and 128 characters",
            status_code=422,
        )
    return value


def _display_name(value: str) -> str:
    normalized = value.strip()
    if not normalized:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Display name must not be empty",
            status_code=422,
        )
    return normalized


def _user_response(user: UserAccount) -> UserResponse:
    return UserResponse(
        id=user.id,
        email=user.email_normalized,
        display_name=user.display_name,
        avatar_url=user.avatar_url,
    )


def _auth_response(
    user: UserAccount,
    workspace: Workspace,
    membership: Membership,
    settings: Settings,
) -> AuthResponse:
    return AuthResponse(
        user=_user_response(user),
        workspace=workspace_response(workspace, membership),
        access_token=create_access_token(user.id, user.token_version, settings),
        expires_in=settings.jwt_access_token_minutes * 60,
    )


async def register(
    session: AsyncSession,
    request: RegisterRequest,
    settings: Settings,
    verification_store: RegistrationVerificationStore,
    *,
    trace_id: str,
) -> AuthResponse:
    password = _plain_password(request.password)
    display_name = _display_name(request.display_name)
    email = await consume_registration_ticket(
        request.registration_ticket,
        settings,
        verification_store,
    )
    user_id = uuid7()
    workspace_id = uuid7()
    now = datetime.now(UTC)
    user = UserAccount(
        id=user_id,
        email_normalized=email,
        password_hash=_password_hash.hash(password),
        display_name=display_name,
        last_login_at=now,
    )
    workspace = Workspace(id=workspace_id, name=f"{display_name}的工作空间")
    membership = Membership(user_id=user_id, workspace_id=workspace_id, role="owner")
    try:
        async with session.begin():
            session.add_all((user, workspace))
            await session.flush()
            session.add(membership)
            await session.flush()
            append_audit_event(
                session,
                workspace_id=workspace.id,
                actor_id=user.id,
                action="identity.registered",
                target_type="user_account",
                target_id=user.id,
                trace_id=trace_id,
                metadata={
                    "token_version": user.token_version,
                    "workspace_revision": workspace.revision,
                },
                occurred_at=now,
            )
            await session.flush()
    except IntegrityError as error:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Account already exists",
            status_code=409,
        ) from error
    return _auth_response(user, workspace, membership, settings)


async def login(
    session: AsyncSession,
    request: LoginRequest,
    settings: Settings,
    *,
    trace_id: str,
) -> AuthResponse:
    email = normalize_email(str(request.email))
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
        now = datetime.now(UTC)
        user.last_login_at = now
        append_audit_event(
            session,
            workspace_id=primary[0].id,
            actor_id=user.id,
            action="identity.login_succeeded",
            target_type="user_account",
            target_id=user.id,
            trace_id=trace_id,
            metadata={"token_version": user.token_version},
            occurred_at=now,
        )
        await session.flush()
    return _auth_response(user, primary[0], primary[1], settings)


async def authenticated_user(session: AsyncSession, claims: AccessTokenClaims) -> UserAccount:
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
        workspace=workspace_response(primary[0], primary[1]),
    )


async def logout(
    session: AsyncSession,
    claims: AccessTokenClaims,
    *,
    trace_id: str,
) -> None:
    async with session.begin():
        user = await authenticated_user(session, claims)
        primary = await repository.find_primary_workspace(session, user.id)
        if primary is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Account workspace is unavailable",
                status_code=500,
            )
        previous_token_version = user.token_version
        user.token_version += 1
        append_audit_event(
            session,
            workspace_id=primary[0].id,
            actor_id=user.id,
            action="identity.logged_out",
            target_type="user_account",
            target_id=user.id,
            trace_id=trace_id,
            metadata={
                "previous_token_version": previous_token_version,
                "token_version": user.token_version,
            },
        )
        await session.flush()


async def change_password(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: ChangePasswordRequest,
    *,
    trace_id: str,
) -> None:
    new_password = _plain_password(request.new_password)
    async with session.begin():
        user = await authenticated_user(session, claims)
        if not _password_hash.verify(
            request.current_password.get_secret_value(), user.password_hash
        ):
            raise _unauthenticated()
        primary = await repository.find_primary_workspace(session, user.id)
        if primary is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Account workspace is unavailable",
                status_code=500,
            )
        previous_token_version = user.token_version
        user.password_hash = _password_hash.hash(new_password)
        user.token_version += 1
        append_audit_event(
            session,
            workspace_id=primary[0].id,
            actor_id=user.id,
            action="identity.password_changed",
            target_type="user_account",
            target_id=user.id,
            trace_id=trace_id,
            metadata={
                "previous_token_version": previous_token_version,
                "token_version": user.token_version,
            },
        )
        await session.flush()


def _unauthenticated() -> ApiError:
    return ApiError(
        ErrorCode.UNAUTHENTICATED,
        "Invalid credentials",
        status_code=401,
        next_action="login",
    )


async def update_profile(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: ProfileUpdateRequest,
    *,
    trace_id: str,
) -> MeResponse:
    if request.display_name is None and request.avatar_url is None:
        raise ApiError(ErrorCode.INVALID_REQUEST, "No profile changes supplied", status_code=422)
    async with session.begin():
        user = await authenticated_user(session, claims)
        changed_fields: list[str] = []
        if request.display_name is not None:
            user.display_name = request.display_name.strip()
            changed_fields.append("display_name")
        if request.avatar_url is not None:
            user.avatar_url = str(request.avatar_url)
            changed_fields.append("avatar_url")
        primary = await repository.find_primary_workspace(session, user.id)
        if primary is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Account workspace is unavailable",
                status_code=500,
            )
        append_audit_event(
            session,
            workspace_id=primary[0].id,
            actor_id=user.id,
            action="identity.profile_updated",
            target_type="user_account",
            target_id=user.id,
            trace_id=trace_id,
            metadata={"changed_fields": changed_fields},
        )
        await session.flush()
    return MeResponse(
        user=_user_response(user),
        workspace=workspace_response(primary[0], primary[1]),
    )


async def deactivate_account(
    session: AsyncSession,
    claims: AccessTokenClaims,
    _: DeactivateAccountRequest,
    *,
    trace_id: str,
) -> None:
    async with session.begin():
        user = await authenticated_user(session, claims)
        primary = await repository.find_primary_workspace(session, user.id)
        if primary is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Account workspace is unavailable",
                status_code=500,
            )
        previous_status = user.status
        previous_token_version = user.token_version
        user.status = "deactivated"
        user.token_version += 1
        append_audit_event(
            session,
            workspace_id=primary[0].id,
            actor_id=user.id,
            action="identity.account_deactivated",
            target_type="user_account",
            target_id=user.id,
            trace_id=trace_id,
            metadata={
                "previous_status": previous_status,
                "status": user.status,
                "previous_token_version": previous_token_version,
                "token_version": user.token_version,
            },
        )
        await session.flush()
