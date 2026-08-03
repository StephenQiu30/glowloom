from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import Capability, actor_context
from app.modules.production import count_episode_task_references
from app.modules.projects import repository
from app.modules.projects.authorization import (
    owned_project,
    require_project_revision,
)
from app.modules.projects.contracts import (
    DeleteBlocker,
    DeletePreflightResponse,
    ProjectContentContext,
)
from app.modules.projects.models import Project
from app.modules.projects.projects.schemas import (
    BudgetLimitRequest,
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


async def create_project(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: ProjectCreateRequest,
    *,
    trace_id: str,
) -> ProjectResponse:
    async with session.begin():
        await actor_context(session, claims, request.workspace_id, Capability.CONTENT_WRITE)
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
        append_audit_event(
            session,
            workspace_id=project.workspace_id,
            actor_id=claims.sub,
            action="project.created",
            target_type="project",
            target_id=project.id,
            trace_id=trace_id,
            metadata={"revision": project.revision, "status": project.status},
        )
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
    await actor_context(session, claims, workspace_id, Capability.CONTENT_READ)
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
    project, _ = await owned_project(session, claims, project_id, Capability.CONTENT_READ)
    return _response(project)


async def update_project(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: ProjectUpdateRequest,
    *,
    trace_id: str,
) -> ProjectResponse:
    values = request.model_dump(exclude={"expected_revision"}, exclude_unset=True)
    if not values:
        raise ApiError(ErrorCode.INVALID_REQUEST, "No project changes supplied", status_code=422)
    async with session.begin():
        project, _ = await owned_project(
            session,
            claims,
            project_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        require_project_revision(project, request.expected_revision)
        for field, value in values.items():
            setattr(project, field, value.strip() if isinstance(value, str) else value)
        project.revision += 1
        append_audit_event(
            session,
            workspace_id=project.workspace_id,
            actor_id=claims.sub,
            action="project.updated",
            target_type="project",
            target_id=project.id,
            trace_id=trace_id,
            metadata={
                "revision": project.revision,
                "changed_fields": sorted(values),
            },
        )
        await session.flush()
    return _response(project)


async def update_budget(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: BudgetLimitRequest,
    *,
    trace_id: str,
) -> ProjectResponse:
    async with session.begin():
        project, _ = await owned_project(
            session,
            claims,
            project_id,
            Capability.BUDGET_MANAGE,
            for_update=True,
        )
        require_project_revision(project, request.expected_revision)
        project.budget_limit = request.amount
        project.currency = request.currency
        project.revision += 1
        append_audit_event(
            session,
            workspace_id=project.workspace_id,
            actor_id=claims.sub,
            action="project.budget_updated",
            target_type="project",
            target_id=project.id,
            trace_id=trace_id,
            metadata={
                "revision": project.revision,
                "changed_fields": ["budget_limit", "currency"],
            },
        )
        await session.flush()
    return _response(project)


async def set_archived(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: ProjectStateRequest,
    *,
    archived: bool,
    trace_id: str,
) -> ProjectResponse:
    expected_status = "active" if archived else "archived"
    async with session.begin():
        project, _ = await owned_project(
            session,
            claims,
            project_id,
            Capability.WORKSPACE_MANAGE,
            for_update=True,
            allow_archived_command=True,
        )
        require_project_revision(project, request.expected_revision)
        if project.status != expected_status:
            raise ApiError(ErrorCode.STATE_CONFLICT, "Project state conflict", status_code=409)
        previous_status = project.status
        now = datetime.now(UTC)
        project.status = "archived" if archived else "active"
        project.archived_at = now if archived else None
        project.archived_by = claims.sub if archived else None
        project.revision += 1
        append_audit_event(
            session,
            workspace_id=project.workspace_id,
            actor_id=claims.sub,
            action="project.archived" if archived else "project.restored",
            target_type="project",
            target_id=project.id,
            trace_id=trace_id,
            metadata={
                "revision": project.revision,
                "previous_status": previous_status,
                "status": project.status,
            },
            occurred_at=now,
        )
        await session.flush()
    return _response(project)


async def delete_preflight(
    session: AsyncSession, claims: AccessTokenClaims, project_id: UUID
) -> DeletePreflightResponse:
    project, _ = await owned_project(session, claims, project_id, Capability.WORKSPACE_MANAGE)
    episodes = await repository.list_episodes(
        session,
        project_id,
        include_archived=True,
    )
    episode_ids = [episode.id for episode in episodes]
    task_counts = await count_episode_task_references(
        session,
        project.workspace_id,
        episode_ids,
    )
    task_count = sum(task_counts.values())
    blockers: list[DeleteBlocker] = []
    if episodes:
        blockers.append(
            DeleteBlocker(
                code="HAS_EPISODES",
                resource_type="project",
                resource_id=project.id,
                summary=f"项目包含 {len(episodes)} 个单集",
            )
        )
    if task_count:
        blockers.append(
            DeleteBlocker(
                code="HAS_TASKS",
                resource_type="project",
                resource_id=project.id,
                summary=f"项目关联 {task_count} 个任务",
            )
        )
    return DeletePreflightResponse(allowed=not blockers, blockers=blockers)


async def delete_project(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    expected_revision: int,
    *,
    trace_id: str,
) -> None:
    async with session.begin():
        project, _ = await owned_project(
            session,
            claims,
            project_id,
            Capability.WORKSPACE_MANAGE,
            for_update=True,
            allow_archived_command=True,
        )
        require_project_revision(project, expected_revision)
        if await repository.count_episodes(session, project_id):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Project has dependent episodes",
                status_code=409,
                next_action="review_delete_blockers",
            )
        append_audit_event(
            session,
            workspace_id=project.workspace_id,
            actor_id=claims.sub,
            action="project.deleted",
            target_type="project",
            target_id=project.id,
            trace_id=trace_id,
            metadata={"revision": project.revision, "status": project.status},
        )
        await session.delete(project)


def _content_context(project: Project) -> ProjectContentContext:
    return ProjectContentContext(
        project_id=project.id,
        workspace_id=project.workspace_id,
        status=cast(Literal["active", "archived"], project.status),
        revision=project.revision,
    )


async def project_for_content_read(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
) -> ProjectContentContext:
    project, _ = await owned_project(
        session, claims, project_id, Capability.CONTENT_READ
    )
    return _content_context(project)


async def lock_active_project_for_content_write(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
) -> ProjectContentContext:
    project, _ = await owned_project(
        session,
        claims,
        project_id,
        Capability.CONTENT_WRITE,
        for_update=True,
    )
    return _content_context(project)
