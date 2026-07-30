from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import ActorContext, Capability, actor_context
from app.modules.projects import repository
from app.modules.projects.authorization import (
    owned_project,
    require_project_revision,
)
from app.modules.projects.contracts import (
    DeleteBlocker,
    DeletePreflightResponse,
    EpisodeContentContext,
)
from app.modules.projects.episodes.schemas import (
    EpisodeCreateRequest,
    EpisodeOrderResponse,
    EpisodeReorderRequest,
    EpisodeResponse,
    EpisodeStateRequest,
    EpisodeUpdateRequest,
)
from app.modules.projects.models import Episode, Project


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


async def episode_for_read(
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
        project_id=episode.project_id,
        current_script_version_id=episode.current_script_version_id,
        revision=episode.revision,
    )


async def lock_active_episode_for_content_write(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> EpisodeContentContext:
    episode, _, _ = await _owned_episode(session, claims, episode_id, for_update=True)
    if episode.status != "active":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Episode is archived",
            status_code=409,
            next_action="restore_episode",
        )
    return _episode_content_context(episode)


async def episode_for_content_read(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> EpisodeContentContext:
    episode, _, _ = await episode_for_read(session, claims, episode_id)
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
        project, _ = await owned_project(
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
    await owned_project(session, claims, project_id, Capability.CONTENT_READ)
    episodes = await repository.list_episodes(
        session, project_id, include_archived=include_archived
    )
    return [_episode_response(episode) for episode in episodes]


async def get_episode(
    session: AsyncSession, claims: AccessTokenClaims, episode_id: UUID
) -> EpisodeResponse:
    episode, _, _ = await episode_for_read(session, claims, episode_id)
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
        episode, _, _ = await _owned_episode(session, claims, episode_id, for_update=True)
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
        project, _ = await owned_project(
            session,
            claims,
            project_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        require_project_revision(project, request.expected_revision)
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
        episode, project, _ = await _owned_episode(session, claims, episode_id, for_update=True)
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
    episode, _, _ = await episode_for_read(session, claims, episode_id)
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
        episode, project, _ = await _owned_episode(session, claims, episode_id, for_update=True)
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
