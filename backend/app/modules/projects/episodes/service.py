import hashlib
import json
from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import ActorContext, Capability, actor_context
from app.modules.production import count_episode_task_references
from app.modules.projects import repository
from app.modules.projects.authorization import (
    owned_project,
    require_project_revision,
)
from app.modules.projects.contracts import (
    DeleteBlocker,
    DeletePreflightResponse,
    EpisodeBatchMaterializeCommand,
    EpisodeBatchMaterializeResult,
    EpisodeContentContext,
    EpisodeScriptPublishBatchCommand,
    EpisodeScriptPublishBatchResult,
    EpisodeScriptVersionCountReader,
    EpisodeStoryboardReferenceReader,
    GenerationProjectContext,
    MaterializedEpisodeReference,
    ProjectEpisodeOrderSnapshot,
    PublishedEpisodeScriptReference,
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


def _active_order_hash(episodes: list[Episode]) -> str:
    payload = [
        {
            "id": str(episode.id),
            "position": episode.position,
            "revision": episode.revision,
            "current_script_version_id": (
                str(episode.current_script_version_id)
                if episode.current_script_version_id is not None
                else None
            ),
        }
        for episode in episodes
        if episode.status == "active"
    ]
    serialized = json.dumps(payload, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(serialized.encode("utf-8")).hexdigest()


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


async def resolve_episode_content_context(
    session: AsyncSession,
    workspace_id: UUID,
    episode_id: UUID,
) -> EpisodeContentContext | None:
    result = await repository.find_episode(session, episode_id)
    if result is None:
        return None
    episode, project = result
    if (
        episode.workspace_id != workspace_id
        or project.workspace_id != workspace_id
        or episode.status != "active"
        or project.status != "active"
    ):
        return None
    return _episode_content_context(episode)


async def lock_episode_content_context(
    session: AsyncSession,
    workspace_id: UUID,
    episode_id: UUID,
) -> EpisodeContentContext | None:
    """Resolve an active internal worker context while serializing current changes."""
    result = await repository.find_episode(session, episode_id, for_update=True)
    if result is None:
        return None
    episode, project = result
    if (
        episode.workspace_id != workspace_id
        or project.workspace_id != workspace_id
        or episode.status != "active"
        or project.status != "active"
    ):
        return None
    return _episode_content_context(episode)


async def resolve_episode_generation_context(
    session: AsyncSession,
    workspace_id: UUID,
    episode_id: UUID,
    *,
    for_update: bool = False,
) -> GenerationProjectContext | None:
    result = await repository.find_episode(session, episode_id, for_update=for_update)
    if result is None:
        return None
    episode, project = result
    if episode.workspace_id != workspace_id or project.workspace_id != workspace_id:
        return None
    return GenerationProjectContext(
        project_id=project.id,
        episode_id=episode.id,
        workspace_id=workspace_id,
        project_status=cast(Literal["active", "archived"], project.status),
        episode_status=cast(Literal["active", "archived"], episode.status),
        budget_limit=project.budget_limit,
        currency=project.currency,
        project_revision=project.revision,
        episode_revision=episode.revision,
    )


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


async def project_episode_order_snapshot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
) -> ProjectEpisodeOrderSnapshot:
    project, _ = await owned_project(
        session,
        claims,
        project_id,
        Capability.CONTENT_READ,
    )
    episodes = await repository.list_episodes(
        session,
        project.id,
        include_archived=False,
    )
    return ProjectEpisodeOrderSnapshot(
        project_id=project.id,
        workspace_id=project.workspace_id,
        project_revision=project.revision,
        active_episode_count=len(episodes),
        active_order_hash=_active_order_hash(episodes),
    )


async def materialize_episode_batch(
    session: AsyncSession,
    claims: AccessTokenClaims,
    command: EpisodeBatchMaterializeCommand,
    *,
    trace_id: str,
) -> EpisodeBatchMaterializeResult:
    if len({item.client_reference_id for item in command.items}) != len(command.items):
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Episode batch references must be unique",
            status_code=422,
        )
    project, _ = await owned_project(
        session,
        claims,
        command.project_id,
        Capability.CONTENT_WRITE,
        for_update=True,
    )
    require_project_revision(project, command.expected_project_revision)
    active = await repository.list_episodes(
        session,
        project.id,
        include_archived=False,
        for_update=True,
    )
    current_order_hash = _active_order_hash(active)
    if current_order_hash != command.expected_active_order_hash:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Episode order has changed",
            status_code=409,
            next_action="refresh_episode_plan_impact",
            details={"active_order_hash": current_order_hash},
        )
    if len(active) + len(command.items) > 10:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Episode limit would be exceeded",
            status_code=409,
            next_action="reduce_episode_count",
            details={
                "active_episode_count": len(active),
                "requested_episode_count": len(command.items),
                "maximum_episode_count": 10,
            },
        )
    next_position = max((episode.position for episode in active), default=0) + 1
    created: list[tuple[Episode, UUID]] = []
    now = datetime.now(UTC)
    for offset, item in enumerate(command.items):
        episode = Episode(
            id=uuid7(),
            workspace_id=project.workspace_id,
            project_id=project.id,
            name=item.name.strip(),
            position=next_position + offset,
            target_duration_ms=item.target_duration_ms,
            status="active",
            revision=1,
            created_at=now,
            updated_at=now,
        )
        session.add(episode)
        created.append((episode, item.client_reference_id))
    project.revision += 1
    project.updated_at = now
    await session.flush()
    for episode, _ in created:
        append_audit_event(
            session,
            workspace_id=episode.workspace_id,
            actor_id=claims.sub,
            action="episode.created",
            target_type="episode",
            target_id=episode.id,
            trace_id=trace_id,
            metadata={
                "project_id": str(project.id),
                "project_revision": project.revision,
                "revision": episode.revision,
                "position": episode.position,
                "status": episode.status,
                "source": "episode_plan_batch",
            },
            occurred_at=now,
        )
    all_active = [*active, *(episode for episode, _ in created)]
    append_audit_event(
        session,
        workspace_id=project.workspace_id,
        actor_id=claims.sub,
        action="episode.batch_materialized",
        target_type="project",
        target_id=project.id,
        trace_id=trace_id,
        metadata={
            "project_revision": project.revision,
            "created_episode_count": len(created),
            "active_episode_count": len(all_active),
        },
        occurred_at=now,
    )
    await session.flush()
    return EpisodeBatchMaterializeResult(
        project_revision=project.revision,
        active_order_hash=_active_order_hash(all_active),
        items=tuple(
            MaterializedEpisodeReference(
                client_reference_id=reference_id,
                episode_id=episode.id,
                revision=episode.revision,
                position=episode.position,
                current_script_version_id=episode.current_script_version_id,
            )
            for episode, reference_id in created
        ),
    )


async def publish_episode_script_version_batch(
    session: AsyncSession,
    claims: AccessTokenClaims,
    command: EpisodeScriptPublishBatchCommand,
    *,
    trace_id: str,
) -> EpisodeScriptPublishBatchResult:
    if len({item.episode_id for item in command.items}) != len(command.items):
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Episode publish items must be unique",
            status_code=422,
        )
    project, _ = await owned_project(
        session,
        claims,
        command.project_id,
        Capability.CONTENT_WRITE,
        for_update=True,
    )
    require_project_revision(project, command.expected_project_revision)
    active = await repository.list_episodes(
        session,
        project.id,
        include_archived=False,
        for_update=True,
    )
    episode_by_id = {episode.id: episode for episode in active}
    for item in command.items:
        episode = episode_by_id.get(item.episode_id)
        if episode is None:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Episode batch has changed",
                status_code=409,
                next_action="refresh_import_commit",
            )
        _episode_revision(episode, item.expected_revision)
        if episode.current_script_version_id != item.expected_current_script_version_id:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Current script version has changed",
                status_code=409,
                next_action="refresh_import_commit",
                details={
                    "episode_id": str(episode.id),
                    "current_script_version_id": (
                        str(episode.current_script_version_id)
                        if episode.current_script_version_id is not None
                        else None
                    ),
                },
            )
    now = datetime.now(UTC)
    published: list[PublishedEpisodeScriptReference] = []
    for item in command.items:
        episode = episode_by_id[item.episode_id]
        previous = episode.current_script_version_id
        episode.current_script_version_id = item.script_version_id
        episode.revision += 1
        episode.updated_at = now
        published.append(
            PublishedEpisodeScriptReference(
                episode_id=episode.id,
                revision=episode.revision,
                previous_script_version_id=previous,
                current_script_version_id=item.script_version_id,
            )
        )
    for item in published:
        append_audit_event(
            session,
            workspace_id=project.workspace_id,
            actor_id=claims.sub,
            action="episode.current_script_changed",
            target_type="episode",
            target_id=item.episode_id,
            trace_id=trace_id,
            metadata={
                "project_id": str(project.id),
                "revision": item.revision,
                "previous_version_id": (
                    str(item.previous_script_version_id)
                    if item.previous_script_version_id is not None
                    else None
                ),
                "current_version_id": str(item.current_script_version_id),
                "source": "episode_plan_batch",
            },
            occurred_at=now,
        )
    await session.flush()
    return EpisodeScriptPublishBatchResult(
        project_revision=project.revision,
        items=tuple(published),
    )


async def create_episode(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: EpisodeCreateRequest,
    *,
    trace_id: str,
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
        append_audit_event(
            session,
            workspace_id=episode.workspace_id,
            actor_id=claims.sub,
            action="episode.created",
            target_type="episode",
            target_id=episode.id,
            trace_id=trace_id,
            metadata={
                "project_id": str(project.id),
                "project_revision": project.revision,
                "revision": episode.revision,
                "position": episode.position,
                "status": episode.status,
            },
        )
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
    *,
    trace_id: str,
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
        append_audit_event(
            session,
            workspace_id=episode.workspace_id,
            actor_id=claims.sub,
            action="episode.updated",
            target_type="episode",
            target_id=episode.id,
            trace_id=trace_id,
            metadata={
                "project_id": str(episode.project_id),
                "revision": episode.revision,
                "changed_fields": sorted(values),
            },
        )
        await session.flush()
    return _episode_response(episode)


async def reorder_episodes(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: EpisodeReorderRequest,
    *,
    trace_id: str,
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
        append_audit_event(
            session,
            workspace_id=project.workspace_id,
            actor_id=claims.sub,
            action="episode.reordered",
            target_type="project",
            target_id=project.id,
            trace_id=trace_id,
            metadata={
                "project_revision": project.revision,
                "episode_count": len(ordered),
            },
        )
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
    trace_id: str,
) -> EpisodeResponse:
    expected_status = "active" if archived else "archived"
    async with session.begin():
        episode, project, _ = await _owned_episode(session, claims, episode_id, for_update=True)
        _episode_revision(episode, request.expected_revision)
        if episode.status != expected_status:
            raise ApiError(ErrorCode.STATE_CONFLICT, "Episode state conflict", status_code=409)
        previous_status = episode.status
        now = datetime.now(UTC)
        if archived:
            episode.status = "archived"
            episode.archived_at = now
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
        append_audit_event(
            session,
            workspace_id=episode.workspace_id,
            actor_id=claims.sub,
            action="episode.archived" if archived else "episode.restored",
            target_type="episode",
            target_id=episode.id,
            trace_id=trace_id,
            metadata={
                "project_id": str(project.id),
                "project_revision": project.revision,
                "revision": episode.revision,
                "position": episode.position,
                "previous_status": previous_status,
                "status": episode.status,
            },
            occurred_at=now,
        )
        await session.flush()
    return _episode_response(episode)


async def episode_delete_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    read_script_version_counts: EpisodeScriptVersionCountReader,
    read_storyboard_references: EpisodeStoryboardReferenceReader,
) -> DeletePreflightResponse:
    episode, _, _ = await episode_for_read(session, claims, episode_id)
    blockers: list[DeleteBlocker] = []
    script_version_count = (
        await read_script_version_counts(
            workspace_id=episode.workspace_id,
            episode_ids=[episode.id],
        )
    )[episode.id]
    if episode.current_timeline_version_id or (
        episode.current_script_version_id and not script_version_count
    ):
        blockers.append(
            DeleteBlocker(
                code="HAS_VERSION_REFERENCE",
                resource_type="episode",
                resource_id=episode.id,
                summary="单集已有版本引用",
            )
        )
    if script_version_count:
        blockers.append(
            DeleteBlocker(
                code="HAS_SCRIPT_VERSIONS",
                resource_type="episode",
                resource_id=episode.id,
                summary=f"单集已有 {script_version_count} 个剧本版本",
            )
        )
    storyboard = (
        await read_storyboard_references(
            workspace_id=episode.workspace_id,
            episode_ids=[episode.id],
        )
    )[episode.id]
    if storyboard.shot_count:
        blockers.append(
            DeleteBlocker(
                code="HAS_STORYBOARD_SHOTS",
                resource_type="episode",
                resource_id=episode.id,
                summary=(
                    f"单集已有 {storyboard.shot_count} 个分镜镜头"
                    f"（{storyboard.spec_version_count} 个规格版本）"
                ),
            )
        )
    task_count = (
        await count_episode_task_references(
            session,
            episode.workspace_id,
            [episode.id],
        )
    )[episode.id]
    if task_count:
        blockers.append(
            DeleteBlocker(
                code="HAS_TASKS",
                resource_type="episode",
                resource_id=episode.id,
                summary=f"单集已有 {task_count} 个任务",
            )
        )
    return DeletePreflightResponse(allowed=not blockers, blockers=blockers)


async def delete_episode(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    expected_revision: int,
    read_script_version_counts: EpisodeScriptVersionCountReader,
    read_storyboard_references: EpisodeStoryboardReferenceReader,
    *,
    trace_id: str,
) -> None:
    async with session.begin():
        episode, project, _ = await _owned_episode(session, claims, episode_id, for_update=True)
        _episode_revision(episode, expected_revision)
        storyboard = (
            await read_storyboard_references(
                workspace_id=episode.workspace_id,
                episode_ids=[episode.id],
            )
        )[episode.id]
        if storyboard.shot_count:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Episode has dependent storyboard facts",
                status_code=409,
                next_action="review_delete_blockers",
            )
        script_version_count = (
            await read_script_version_counts(
                workspace_id=episode.workspace_id,
                episode_ids=[episode.id],
            )
        )[episode.id]
        if script_version_count:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Episode has dependent script versions",
                status_code=409,
                next_action="review_delete_blockers",
            )
        if episode.current_script_version_id or episode.current_timeline_version_id:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Episode has version references",
                status_code=409,
                next_action="review_delete_blockers",
            )
        task_count = (
            await count_episode_task_references(
                session,
                episode.workspace_id,
                [episode.id],
            )
        )[episode.id]
        if task_count:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Episode has dependent tasks",
                status_code=409,
                next_action="review_delete_blockers",
            )
        await session.delete(episode)
        await session.flush()
        await _compact_episode_positions(session, project.id)
        project.revision += 1
        append_audit_event(
            session,
            workspace_id=episode.workspace_id,
            actor_id=claims.sub,
            action="episode.deleted",
            target_type="episode",
            target_id=episode.id,
            trace_id=trace_id,
            metadata={
                "project_id": str(project.id),
                "project_revision": project.revision,
                "revision": episode.revision,
                "position": episode.position,
                "status": episode.status,
            },
        )
