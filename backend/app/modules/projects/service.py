from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import service as identity_service
from app.modules.identity.models import Membership
from app.modules.identity.policy import Capability, require_workspace_capability
from app.modules.projects import repository
from app.modules.projects.models import Project
from app.modules.projects.schemas import (
    BudgetLimitRequest,
    DeletePreflightResponse,
    PaginatedProjects,
    ProjectCreateRequest,
    ProjectResponse,
    ProjectStateRequest,
    ProjectUpdateRequest,
)


def _response(project: Project) -> ProjectResponse:
    return ProjectResponse(
        id=project.id,
        workspace_id=project.workspace_id,
        name=project.name,
        description=project.description,
        aspect_ratio=cast(Literal["9:16", "16:9", "1:1"], project.aspect_ratio),
        language=project.language,
        visual_style=project.visual_style,
        target_duration_ms=project.target_duration_ms,
        budget_limit=project.budget_limit,
        currency=project.currency,
        status=cast(Literal["active", "archived"], project.status),
        revision=project.revision,
    )


def _not_found() -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, "Project not found", status_code=404)


def _revision(project: Project, expected: int) -> None:
    if project.revision != expected:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Project has changed",
            status_code=409,
            details={"current_revision": project.revision},
        )


def _capability(project: Project, membership: Membership, capability: Capability) -> None:
    try:
        require_workspace_capability(membership.role, "active", capability)
    except PermissionError as error:
        raise ApiError(ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403) from error
    if project.status == "archived" and capability != Capability.CONTENT_READ:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Project is archived",
            status_code=409,
            next_action="restore_project",
        )


async def _owned_project(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    capability: Capability,
    *,
    for_update: bool = False,
    allow_archived_command: bool = False,
) -> tuple[Project, Membership]:
    user = await identity_service.authenticated_user(session, claims)
    result = await repository.find_project_for_user(
        session, user.id, project_id, for_update=for_update
    )
    if result is None:
        raise _not_found()
    if allow_archived_command:
        try:
            require_workspace_capability(result[1].role, "active", capability)
        except PermissionError as error:
            raise ApiError(
                ErrorCode.FORBIDDEN, "Action is not allowed", status_code=403
            ) from error
    else:
        _capability(result[0], result[1], capability)
    return result


async def create_project(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: ProjectCreateRequest,
) -> ProjectResponse:
    async with session.begin():
        await identity_service.actor_context(
            session, claims, request.workspace_id, Capability.CONTENT_WRITE
        )
        project = Project(
            id=uuid7(),
            workspace_id=request.workspace_id,
            name=request.name.strip(),
            description=request.description,
            aspect_ratio=request.aspect_ratio,
            language=request.language,
            visual_style=request.visual_style,
            target_duration_ms=request.target_duration_ms,
        )
        session.add(project)
        await session.flush()
    return _response(project)


async def list_projects(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    *,
    include_archived: bool,
    search: str | None,
    sort: Literal["name", "created_at", "updated_at"],
    order: Literal["asc", "desc"],
    limit: int,
    offset: int,
) -> PaginatedProjects:
    await identity_service.actor_context(session, claims, workspace_id, Capability.CONTENT_READ)
    projects, total = await repository.list_projects(
        session,
        workspace_id,
        include_archived=include_archived,
        search=search,
        sort=sort,
        order=order,
        limit=limit,
        offset=offset,
    )
    return PaginatedProjects(
        items=[_response(project) for project in projects],
        total=total,
        limit=limit,
        offset=offset,
    )


async def get_project(
    session: AsyncSession, claims: AccessTokenClaims, project_id: UUID
) -> ProjectResponse:
    project, _ = await _owned_project(
        session, claims, project_id, Capability.CONTENT_READ
    )
    return _response(project)


async def update_project(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: ProjectUpdateRequest,
) -> ProjectResponse:
    values = request.model_dump(exclude={"expected_revision"}, exclude_unset=True)
    if not values:
        raise ApiError(ErrorCode.INVALID_REQUEST, "No project changes supplied", status_code=422)
    async with session.begin():
        project, _ = await _owned_project(
            session,
            claims,
            project_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        _revision(project, request.expected_revision)
        for field, value in values.items():
            setattr(project, field, value.strip() if isinstance(value, str) else value)
        project.revision += 1
        await session.flush()
    return _response(project)


async def update_budget(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: BudgetLimitRequest,
) -> ProjectResponse:
    async with session.begin():
        project, _ = await _owned_project(
            session,
            claims,
            project_id,
            Capability.BUDGET_MANAGE,
            for_update=True,
        )
        _revision(project, request.expected_revision)
        project.budget_limit = request.amount
        project.currency = request.currency
        project.revision += 1
        await session.flush()
    return _response(project)


async def set_archived(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: ProjectStateRequest,
    *,
    archived: bool,
) -> ProjectResponse:
    expected_status = "active" if archived else "archived"
    async with session.begin():
        project, _ = await _owned_project(
            session,
            claims,
            project_id,
            Capability.WORKSPACE_MANAGE,
            for_update=True,
            allow_archived_command=True,
        )
        _revision(project, request.expected_revision)
        if project.status != expected_status:
            raise ApiError(ErrorCode.STATE_CONFLICT, "Project state conflict", status_code=409)
        project.status = "archived" if archived else "active"
        project.archived_at = datetime.now(UTC) if archived else None
        project.archived_by = claims.sub if archived else None
        project.revision += 1
        await session.flush()
    return _response(project)


async def delete_preflight(
    session: AsyncSession, claims: AccessTokenClaims, project_id: UUID
) -> DeletePreflightResponse:
    await _owned_project(session, claims, project_id, Capability.WORKSPACE_MANAGE)
    return DeletePreflightResponse(allowed=True, blockers=[])


async def delete_project(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    expected_revision: int,
) -> None:
    async with session.begin():
        project, _ = await _owned_project(
            session,
            claims,
            project_id,
            Capability.WORKSPACE_MANAGE,
            for_update=True,
            allow_archived_command=True,
        )
        _revision(project, expected_revision)
        await session.delete(project)
