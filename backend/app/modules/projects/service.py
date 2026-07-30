from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import (
    ActorContext,
    Capability,
    actor_context,
)
from app.modules.projects import repository
from app.modules.projects.contracts import EpisodeContentContext
from app.modules.projects.models import Episode, Project
from app.modules.projects.schemas import (
    BlockingReason,
    BudgetLimitRequest,
    CostSummary,
    DeleteBlocker,
    DeletePreflightResponse,
    EpisodeCreateRequest,
    EpisodeOrderResponse,
    EpisodeProductionSnapshot,
    EpisodeReorderRequest,
    EpisodeResponse,
    EpisodeStateRequest,
    EpisodeUpdateRequest,
    NextAction,
    PaginatedProjects,
    ProjectCreateRequest,
    ProjectProductionSnapshot,
    ProjectResponse,
    ProjectStateRequest,
    ProjectUpdateRequest,
    ReviewSummary,
    TaskSummary,
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


def _require_project_state(project: Project, capability: Capability) -> None:
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
) -> tuple[Project, ActorContext]:
    project = await repository.find_project(session, project_id, for_update=for_update)
    if project is None:
        raise _not_found()
    try:
        actor = await actor_context(session, claims, project.workspace_id, capability)
    except ApiError as error:
        if error.code == ErrorCode.NOT_FOUND:
            raise _not_found() from error
        raise
    if not allow_archived_command:
        _require_project_state(project, capability)
    return project, actor


async def create_project(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: ProjectCreateRequest,
) -> ProjectResponse:
    async with session.begin():
        await actor_context(
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
    project, _ = await _owned_project(
        session, claims, project_id, Capability.WORKSPACE_MANAGE
    )
    count = await repository.count_episodes(session, project_id)
    blockers = (
        [
            DeleteBlocker(
                code="HAS_EPISODES",
                resource_type="project",
                resource_id=project.id,
                summary=f"项目包含 {count} 个单集",
            )
        ]
        if count
        else []
    )
    return DeletePreflightResponse(allowed=not blockers, blockers=blockers)


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
        if await repository.count_episodes(session, project_id):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Project has dependent episodes",
                status_code=409,
                next_action="review_delete_blockers",
            )
        await session.delete(project)


def _episode_response(episode: Episode) -> EpisodeResponse:
    return EpisodeResponse(
        id=episode.id,
        workspace_id=episode.workspace_id,
        project_id=episode.project_id,
        name=episode.name,
        position=episode.position,
        target_duration_ms=episode.target_duration_ms,
        status=cast(Literal["active", "archived"], episode.status),
        revision=episode.revision,
        current_script_version_id=episode.current_script_version_id,
        current_timeline_version_id=episode.current_timeline_version_id,
    )


def _episode_not_found() -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, "Episode not found", status_code=404)


def _episode_revision(episode: Episode, expected: int) -> None:
    if episode.revision != expected:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Episode has changed",
            status_code=409,
            details={"current_revision": episode.revision},
        )


async def _owned_episode(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    *,
    for_update: bool,
) -> tuple[Episode, Project, ActorContext]:
    result = await repository.find_episode(session, episode_id, for_update=for_update)
    if result is None:
        raise _episode_not_found()
    try:
        actor = await actor_context(
            session,
            claims,
            result[0].workspace_id,
            Capability.CONTENT_WRITE,
        )
    except ApiError as error:
        if error.code == ErrorCode.NOT_FOUND:
            raise _episode_not_found() from error
        raise
    if result[1].status != "active":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Project is archived",
            status_code=409,
            next_action="restore_project",
        )
    return result[0], result[1], actor


async def _episode_for_read(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> tuple[Episode, Project, ActorContext]:
    result = await repository.find_episode(session, episode_id)
    if result is None:
        raise _episode_not_found()
    try:
        actor = await actor_context(
            session,
            claims,
            result[0].workspace_id,
            Capability.CONTENT_READ,
        )
    except ApiError as error:
        if error.code == ErrorCode.NOT_FOUND:
            raise _episode_not_found() from error
        raise
    return result[0], result[1], actor


def _episode_content_context(episode: Episode) -> EpisodeContentContext:
    return EpisodeContentContext(
        episode_id=episode.id,
        workspace_id=episode.workspace_id,
        current_script_version_id=episode.current_script_version_id,
        revision=episode.revision,
    )


async def lock_active_episode_for_content_write(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> EpisodeContentContext:
    episode, _, _ = await _owned_episode(
        session, claims, episode_id, for_update=True
    )
    if episode.status != "active":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Episode is archived",
            status_code=409,
            next_action="restore_episode",
        )
    return _episode_content_context(episode)


async def compare_and_set_current_script_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    expected_current_version_id: UUID | None,
    version_id: UUID,
) -> EpisodeContentContext:
    episode, _, _ = await _owned_episode(
        session,
        claims,
        episode_id,
        for_update=True,
    )
    if episode.status != "active":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Episode is archived",
            status_code=409,
            next_action="restore_episode",
        )
    if episode.current_script_version_id != expected_current_version_id:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Current script version has changed",
            status_code=409,
            details={
                "current_script_version_id": (
                    str(episode.current_script_version_id)
                    if episode.current_script_version_id is not None
                    else None
                )
            },
        )
    if episode.current_script_version_id == version_id:
        return _episode_content_context(episode)
    episode.current_script_version_id = version_id
    episode.revision += 1
    await session.flush()
    return _episode_content_context(episode)


async def create_episode(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: EpisodeCreateRequest,
) -> EpisodeResponse:
    async with session.begin():
        project, _ = await _owned_project(
            session,
            claims,
            project_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        episode = Episode(
            id=uuid7(),
            workspace_id=project.workspace_id,
            project_id=project.id,
            name=request.name.strip(),
            position=await repository.next_episode_position(session, project.id),
            target_duration_ms=request.target_duration_ms,
        )
        session.add(episode)
        project.revision += 1
        await session.flush()
    return _episode_response(episode)


async def list_episodes(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    *,
    include_archived: bool,
) -> list[EpisodeResponse]:
    await _owned_project(session, claims, project_id, Capability.CONTENT_READ)
    episodes = await repository.list_episodes(
        session, project_id, include_archived=include_archived
    )
    return [_episode_response(episode) for episode in episodes]


async def get_episode(
    session: AsyncSession, claims: AccessTokenClaims, episode_id: UUID
) -> EpisodeResponse:
    episode, _, _ = await _episode_for_read(session, claims, episode_id)
    return _episode_response(episode)


async def update_episode(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: EpisodeUpdateRequest,
) -> EpisodeResponse:
    values = request.model_dump(exclude={"expected_revision"}, exclude_unset=True)
    if not values:
        raise ApiError(ErrorCode.INVALID_REQUEST, "No episode changes supplied", status_code=422)
    async with session.begin():
        episode, _, _ = await _owned_episode(
            session, claims, episode_id, for_update=True
        )
        if episode.status != "active":
            raise ApiError(ErrorCode.STATE_CONFLICT, "Episode is archived", status_code=409)
        _episode_revision(episode, request.expected_revision)
        for field, value in values.items():
            setattr(episode, field, value.strip() if isinstance(value, str) else value)
        episode.revision += 1
        await session.flush()
    return _episode_response(episode)


async def reorder_episodes(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: EpisodeReorderRequest,
) -> EpisodeOrderResponse:
    if len(request.episode_ids) != len(set(request.episode_ids)):
        raise ApiError(ErrorCode.INVALID_REQUEST, "Episode IDs must be unique", status_code=422)
    async with session.begin():
        project, _ = await _owned_project(
            session,
            claims,
            project_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        _revision(project, request.expected_revision)
        episodes = await repository.list_episodes(
            session, project_id, include_archived=False, for_update=True
        )
        by_id = {episode.id: episode for episode in episodes}
        if set(request.episode_ids) != set(by_id):
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Episode order must contain every active episode exactly once",
                status_code=422,
                details={"current_episode_ids": [str(episode.id) for episode in episodes]},
            )
        temporary_start = len(episodes) * 2 + 1
        for index, episode_id in enumerate(request.episode_ids):
            by_id[episode_id].position = temporary_start + index
        await session.flush()
        ordered = [by_id[episode_id] for episode_id in request.episode_ids]
        for position, episode in enumerate(ordered, start=1):
            episode.position = position
        project.revision += 1
        await session.flush()
    return EpisodeOrderResponse(
        items=[_episode_response(episode) for episode in ordered],
        project_revision=project.revision,
    )


async def _compact_episode_positions(session: AsyncSession, project_id: UUID) -> None:
    episodes = await repository.list_episodes(
        session, project_id, include_archived=False, for_update=True
    )
    temporary_start = len(episodes) * 2 + 1
    for index, episode in enumerate(episodes):
        episode.position = temporary_start + index
    await session.flush()
    for position, episode in enumerate(episodes, start=1):
        episode.position = position
    await session.flush()


async def set_episode_archived(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: EpisodeStateRequest,
    *,
    archived: bool,
) -> EpisodeResponse:
    expected_status = "active" if archived else "archived"
    async with session.begin():
        episode, project, _ = await _owned_episode(
            session, claims, episode_id, for_update=True
        )
        _episode_revision(episode, request.expected_revision)
        if episode.status != expected_status:
            raise ApiError(ErrorCode.STATE_CONFLICT, "Episode state conflict", status_code=409)
        if archived:
            episode.status = "archived"
            episode.archived_at = datetime.now(UTC)
            episode.archived_by = claims.sub
            await session.flush()
            await _compact_episode_positions(session, project.id)
        else:
            next_position = await repository.next_episode_position(session, project.id)
            episode.position = next_position
            episode.status = "active"
            episode.archived_at = None
            episode.archived_by = None
        episode.revision += 1
        project.revision += 1
        await session.flush()
    return _episode_response(episode)


async def episode_delete_preflight(
    session: AsyncSession, claims: AccessTokenClaims, episode_id: UUID
) -> DeletePreflightResponse:
    episode, _, _ = await _episode_for_read(session, claims, episode_id)
    blockers: list[DeleteBlocker] = []
    if episode.current_script_version_id or episode.current_timeline_version_id:
        blockers.append(
            DeleteBlocker(
                code="HAS_VERSION_REFERENCE",
                resource_type="episode",
                resource_id=episode.id,
                summary="单集已有版本引用",
            )
        )
    return DeletePreflightResponse(allowed=not blockers, blockers=blockers)


async def delete_episode(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    expected_revision: int,
) -> None:
    async with session.begin():
        episode, project, _ = await _owned_episode(
            session, claims, episode_id, for_update=True
        )
        _episode_revision(episode, expected_revision)
        if episode.current_script_version_id or episode.current_timeline_version_id:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Episode has version references",
                status_code=409,
            )
        await session.delete(episode)
        await session.flush()
        await _compact_episode_positions(session, project.id)
        project.revision += 1


def _episode_snapshot(
    episode: Episode,
    currency: str,
    computed_at: datetime,
) -> EpisodeProductionSnapshot:
    return EpisodeProductionSnapshot(
        episode_id=episode.id,
        current_stage="script_import",
        completion=0,
        blocking_reasons=[
            BlockingReason(
                code="SCRIPT_MISSING",
                summary="单集尚未导入剧本",
                resource_type="episode",
                resource_id=episode.id,
            )
        ],
        next_actions=[
            NextAction(
                code="import_script",
                label="导入剧本",
                href=f"/studio/{episode.id}/script",
            )
        ],
        task_summary=TaskSummary(),
        review_summary=ReviewSummary(),
        cost_summary=CostSummary(currency=currency),
        partial_failures=[],
        computed_at=computed_at,
    )


async def episode_production_snapshot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> EpisodeProductionSnapshot:
    episode, project, _ = await _episode_for_read(session, claims, episode_id)
    return _episode_snapshot(episode, project.currency, datetime.now(UTC))


async def project_production_snapshot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
) -> ProjectProductionSnapshot:
    project, _ = await _owned_project(
        session, claims, project_id, Capability.CONTENT_READ
    )
    episodes = await repository.list_episodes(
        session, project_id, include_archived=False
    )
    computed_at = datetime.now(UTC)
    episode_snapshots = [
        _episode_snapshot(episode, project.currency, computed_at) for episode in episodes
    ]
    if episode_snapshots:
        blockers = [
            reason
            for snapshot in episode_snapshots
            for reason in snapshot.blocking_reasons
        ]
        actions = [episode_snapshots[0].next_actions[0]]
        current_stage: Literal["project_setup", "script_import"] = "script_import"
    else:
        blockers = [
            BlockingReason(
                code="EPISODE_MISSING",
                summary="项目尚未创建有效单集",
                resource_type="project",
                resource_id=project.id,
            )
        ]
        actions = [
            NextAction(
                code="create_episode",
                label="创建单集",
                href=f"/projects/{project.id}",
            )
        ]
        current_stage = "project_setup"
    return ProjectProductionSnapshot(
        project_id=project.id,
        current_stage=current_stage,
        completion=0,
        blocking_reasons=blockers,
        next_actions=actions,
        episodes=episode_snapshots,
        partial_failures=[],
        computed_at=computed_at,
    )
