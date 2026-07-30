from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import repository
from app.modules.identity.authentication.service import authenticated_user
from app.modules.identity.contracts import ActorContext, Capability
from app.modules.identity.models import Membership, Workspace
from app.modules.identity.policy import require_capability, require_workspace_capability
from app.modules.identity.presenters import workspace_response
from app.modules.identity.workspaces.schemas import (
    WorkspaceCreateRequest,
    WorkspaceResponse,
    WorkspaceStateRequest,
    WorkspaceUpdateRequest,
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
    return [workspace_response(workspace, membership) for workspace, membership in workspaces]


async def get_workspace(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
) -> WorkspaceResponse:
    user = await authenticated_user(session, claims)
    result = await repository.find_workspace_for_user(session, user.id, workspace_id)
    if result is None:
        raise _workspace_not_found()
    return workspace_response(result[0], result[1])


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
    return workspace_response(workspace, membership)


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
    return workspace_response(workspace, membership)


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
    return workspace_response(workspace, membership)
