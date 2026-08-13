from datetime import UTC, datetime
from difflib import unified_diff
from hashlib import sha256
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.projects import (
    compare_and_set_current_script_version,
    episode_for_content_read,
    lock_active_episode_for_content_write,
)
from app.modules.scripts import repository
from app.modules.scripts.authorization import (
    require_resource_access,
    resource_not_found,
)
from app.modules.scripts.contracts import (
    ConfirmedStructureQuery,
    ConfirmedStructureReference,
    EpisodeConfirmedStructureQuery,
    NarrativeImpactRecorder,
    NarrativeImpactSnapshot,
    ScriptVersionImpactReader,
    ScriptVersionSnapshot,
)
from app.modules.scripts.models import ExtractionBatch, ScriptSource, ScriptVersion
from app.modules.scripts.versions.schemas import (
    CurrentScriptVersionRequest,
    CurrentScriptVersionResponse,
    PaginatedScriptSources,
    PaginatedScriptVersions,
    ScriptImportRequest,
    ScriptImportResponse,
    ScriptSourceResponse,
    ScriptSourceStateRequest,
    ScriptVersionDeleteBlocker,
    ScriptVersionDeleteResponse,
    ScriptVersionDiffResponse,
    ScriptVersionImpactResponse,
    ScriptVersionPublishRequest,
    ScriptVersionPublishResponse,
    ScriptVersionResponse,
)


def _source_response(source: ScriptSource) -> ScriptSourceResponse:
    return ScriptSourceResponse(
        id=source.id,
        workspace_id=source.workspace_id,
        episode_id=source.episode_id,
        input_type=cast(Literal["text", "media"], source.input_type),
        title=source.title,
        source_media_version_id=source.source_media_version_id,
        rights_declaration=source.rights_declaration,
        status=cast(Literal["active", "archived"], source.status),
        revision=source.revision,
        created_at=source.created_at,
    )


def _version_response(version: ScriptVersion) -> ScriptVersionResponse:
    return ScriptVersionResponse(
        id=version.id,
        workspace_id=version.workspace_id,
        source_id=version.source_id,
        version_no=version.version_no,
        status=cast(Literal["draft", "published"], version.status),
        body=version.body,
        content_hash=version.content_hash,
        created_by=version.created_by,
        created_at=version.created_at,
    )


def _current_response(
    episode_id: UUID,
    previous_script_version_id: UUID | None,
    current_script_version_id: UUID,
    episode_revision: int,
    affected_shot_ids: list[UUID],
    narrative_impact: NarrativeImpactSnapshot,
) -> CurrentScriptVersionResponse:
    return CurrentScriptVersionResponse(
        episode_id=episode_id,
        current_script_version_id=current_script_version_id,
        episode_revision=episode_revision,
        impact=ScriptVersionImpactResponse(
            previous_script_version_id=previous_script_version_id,
            current_script_version_id=current_script_version_id,
            affected_shot_ids=affected_shot_ids,
            narrative_impact_id=narrative_impact.impact_id,
            previous_narrative_dependency_hash=narrative_impact.previous_dependency_hash,
            current_narrative_dependency_hash=narrative_impact.current_dependency_hash,
            invalidated_scopes=cast(
                list[Literal["shot_readiness", "coverage", "export"]],
                list(narrative_impact.invalidated_scopes),
            ),
        ),
    )


async def count_episode_script_versions(
    session: AsyncSession,
    workspace_id: UUID,
    episode_ids: list[UUID],
) -> dict[UUID, int]:
    counts = {episode_id: 0 for episode_id in episode_ids}
    for episode_id, count in await repository.count_versions_by_episode(
        session,
        workspace_id,
        episode_ids,
    ):
        counts[episode_id] = count
    return counts


def _source_revision(source: ScriptSource, expected_revision: int) -> None:
    if source.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Script source has changed",
            status_code=409,
            details={"current_revision": source.revision},
        )


def _same_import(
    source: ScriptSource,
    version: ScriptVersion,
    request: ScriptImportRequest,
    content_hash: str,
) -> bool:
    return (
        source.input_type == request.input_type
        and source.title == request.title.strip()
        and source.rights_declaration == request.rights_declaration.strip()
        and version.body == request.body
        and version.content_hash == content_hash
    )


async def import_text_source(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: ScriptImportRequest,
    *,
    trace_id: str,
) -> ScriptImportResponse:
    title = request.title.strip()
    rights_declaration = request.rights_declaration.strip()
    if not title or not rights_declaration:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Script metadata is required", status_code=422)
    content_hash = sha256(request.body.encode("utf-8")).hexdigest()
    now = datetime.now(UTC)
    async with session.begin():
        episode = await lock_active_episode_for_content_write(session, claims, episode_id)
        source_id = uuid7()
        inserted_id = await session.scalar(
            insert(ScriptSource)
            .values(
                id=source_id,
                workspace_id=episode.workspace_id,
                episode_id=episode.episode_id,
                input_type=request.input_type,
                title=title,
                rights_declaration=rights_declaration,
                status="active",
                revision=1,
                idempotency_key=request.idempotency_key,
                created_at=now,
                updated_at=now,
            )
            .on_conflict_do_nothing(constraint="uq_scr_source_episode_idempotency")
            .returning(ScriptSource.id)
        )
        if inserted_id is None:
            source = await repository.find_source_by_idempotency(
                session, episode.episode_id, request.idempotency_key
            )
            if source is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Script source state is unavailable",
                    status_code=500,
                )
            version = await repository.find_initial_version(session, source.id)
            if version is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Script version state is unavailable",
                    status_code=500,
                )
            if not _same_import(source, version, request, content_hash):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            return ScriptImportResponse(
                source=_source_response(source),
                version=_version_response(version),
            )

        source = ScriptSource(
            id=inserted_id,
            workspace_id=episode.workspace_id,
            episode_id=episode.episode_id,
            input_type=request.input_type,
            title=title,
            rights_declaration=rights_declaration,
            status="active",
            revision=1,
            idempotency_key=request.idempotency_key,
            created_at=now,
            updated_at=now,
        )
        version = ScriptVersion(
            id=uuid7(),
            workspace_id=episode.workspace_id,
            source_id=inserted_id,
            version_no=1,
            status="draft",
            body=request.body,
            content_hash=content_hash,
            structure_summary={},
            created_by=claims.sub,
            created_at=now,
        )
        session.add(version)
        append_audit_event(
            session,
            workspace_id=source.workspace_id,
            actor_id=claims.sub,
            action="script.version_created",
            target_type="script_version",
            target_id=version.id,
            trace_id=trace_id,
            metadata={
                "source_id": str(source.id),
                "episode_id": str(source.episode_id),
                "version_no": version.version_no,
                "status": version.status,
            },
            occurred_at=now,
        )
        await session.flush()
    return ScriptImportResponse(
        source=_source_response(source),
        version=_version_response(version),
    )


async def get_source(
    session: AsyncSession,
    claims: AccessTokenClaims,
    source_id: UUID,
) -> ScriptSourceResponse:
    source = await repository.find_source(session, source_id)
    if source is None:
        raise resource_not_found("Script source")
    await require_resource_access(session, claims, source.workspace_id, "Script source")
    return _source_response(source)


async def list_sources(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    *,
    limit: int,
    offset: int,
) -> PaginatedScriptSources:
    await episode_for_content_read(session, claims, episode_id)
    sources, total = await repository.list_sources(session, episode_id, limit=limit, offset=offset)
    return PaginatedScriptSources(
        items=[_source_response(source) for source in sources],
        total=total,
        limit=limit,
        offset=offset,
    )


async def get_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
) -> ScriptVersionResponse:
    version = await repository.find_version(session, version_id)
    if version is None:
        raise resource_not_found("Script version")
    await require_resource_access(session, claims, version.workspace_id, "Script version")
    return _version_response(version)


async def script_version_exists(
    session: AsyncSession, workspace_id: UUID, version_id: UUID
) -> bool:
    version = await repository.find_version(session, version_id)
    return version is not None and version.workspace_id == workspace_id


async def resolve_script_version_snapshot(
    session: AsyncSession,
    workspace_id: UUID,
    episode_id: UUID,
    version_id: UUID,
) -> ScriptVersionSnapshot | None:
    version = await repository.find_version(session, version_id)
    if version is None or version.workspace_id != workspace_id:
        return None
    source = await repository.find_source(session, version.source_id)
    if (
        source is None
        or source.workspace_id != workspace_id
        or source.episode_id != episode_id
    ):
        return None
    return ScriptVersionSnapshot(
        workspace_id=workspace_id,
        episode_id=episode_id,
        version_id=version.id,
        version_no=version.version_no,
        status=cast(Literal["draft", "published"], version.status),
        content_hash=version.content_hash,
    )


async def resolve_confirmed_structure(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    episode_id: UUID,
    script_version_id: UUID,
    scene_id: UUID,
    dialogue_ids: list[UUID],
) -> ConfirmedStructureReference | None:
    version = await repository.find_version(session, script_version_id)
    if (
        version is None
        or version.workspace_id != workspace_id
        or version.status != "published"
        or not version.structure_summary.get("confirmation_batch_id")
    ):
        return None
    source = await repository.find_source(session, version.source_id)
    if (
        source is None
        or source.workspace_id != workspace_id
        or source.episode_id != episode_id
        or source.status != "active"
    ):
        return None
    scene = await repository.find_scene(session, scene_id)
    if (
        scene is None
        or scene.workspace_id != workspace_id
        or scene.script_version_id != script_version_id
    ):
        return None
    dialogues = await repository.list_dialogues(session, [scene_id])
    available_ids = {dialogue.id for dialogue in dialogues}
    if len(set(dialogue_ids)) != len(dialogue_ids) or not set(dialogue_ids).issubset(available_ids):
        return None
    return ConfirmedStructureReference(
        workspace_id=workspace_id,
        episode_id=episode_id,
        script_version_id=script_version_id,
        scene_id=scene_id,
        dialogue_ids=tuple(dialogue_ids),
    )


async def resolve_confirmed_structures(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    episode_id: UUID,
    queries: list[ConfirmedStructureQuery],
) -> dict[ConfirmedStructureQuery, ConfirmedStructureReference | None]:
    scoped_queries = [
        EpisodeConfirmedStructureQuery(
            episode_id=episode_id,
            structure=query,
        )
        for query in dict.fromkeys(queries)
    ]
    results = await resolve_episode_confirmed_structures(
        session,
        workspace_id=workspace_id,
        queries=scoped_queries,
    )
    return {scoped.structure: results[scoped] for scoped in scoped_queries}


async def resolve_episode_confirmed_structures(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    queries: list[EpisodeConfirmedStructureQuery],
) -> dict[EpisodeConfirmedStructureQuery, ConfirmedStructureReference | None]:
    unique_queries = list(dict.fromkeys(queries))
    rows = await repository.find_structure_rows(
        session,
        [query.structure.script_version_id for query in unique_queries],
        [query.structure.scene_id for query in unique_queries],
    )
    by_pair = {(version.id, scene.id): (version, source, scene) for version, source, scene in rows}
    dialogues = await repository.list_dialogues(
        session,
        [scene.id for _version, _source, scene in rows],
    )
    dialogue_ids_by_scene: dict[UUID, set[UUID]] = {}
    for dialogue in dialogues:
        dialogue_ids_by_scene.setdefault(dialogue.scene_id, set()).add(dialogue.id)
    results: dict[
        EpisodeConfirmedStructureQuery,
        ConfirmedStructureReference | None,
    ] = {}
    for scoped_query in unique_queries:
        query = scoped_query.structure
        row = by_pair.get((query.script_version_id, query.scene_id))
        if row is None:
            results[scoped_query] = None
            continue
        version, source, scene = row
        valid = (
            version.workspace_id == workspace_id
            and version.status == "published"
            and bool(version.structure_summary.get("confirmation_batch_id"))
            and source.workspace_id == workspace_id
            and source.episode_id == scoped_query.episode_id
            and source.status == "active"
            and scene.workspace_id == workspace_id
            and len(set(query.dialogue_ids)) == len(query.dialogue_ids)
            and set(query.dialogue_ids).issubset(dialogue_ids_by_scene.get(scene.id, set()))
        )
        results[scoped_query] = (
            ConfirmedStructureReference(
                workspace_id=workspace_id,
                episode_id=scoped_query.episode_id,
                script_version_id=version.id,
                scene_id=scene.id,
                dialogue_ids=query.dialogue_ids,
            )
            if valid
            else None
        )
    return results


async def list_versions(
    session: AsyncSession,
    claims: AccessTokenClaims,
    source_id: UUID,
    *,
    limit: int,
    offset: int,
) -> PaginatedScriptVersions:
    source = await repository.find_source(session, source_id)
    if source is None:
        raise resource_not_found("Script source")
    await require_resource_access(session, claims, source.workspace_id, "Script source")
    versions, total = await repository.list_versions(session, source_id, limit=limit, offset=offset)
    return PaginatedScriptVersions(
        items=[_version_response(version) for version in versions],
        total=total,
        limit=limit,
        offset=offset,
    )


async def publish_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    source_id: UUID,
    request: ScriptVersionPublishRequest,
    impact_reader: ScriptVersionImpactReader,
    narrative_impact_recorder: NarrativeImpactRecorder,
    *,
    trace_id: str,
) -> ScriptVersionPublishResponse:
    content_hash = sha256(request.body.encode("utf-8")).hexdigest()
    now = datetime.now(UTC)
    async with session.begin():
        source = await repository.find_source(session, source_id, for_update=True)
        if source is None:
            raise resource_not_found("Script source")
        await require_resource_access(session, claims, source.workspace_id, "Script source")
        if source.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Script source is archived",
                status_code=409,
            )
        episode = await lock_active_episode_for_content_write(session, claims, source.episode_id)
        version = ScriptVersion(
            id=uuid7(),
            workspace_id=source.workspace_id,
            source_id=source.id,
            version_no=await repository.latest_version_number(session, source.id) + 1,
            status="published",
            body=request.body,
            content_hash=content_hash,
            structure_summary={},
            created_by=claims.sub,
            created_at=now,
        )
        current = await compare_and_set_current_script_version(
            session,
            claims,
            episode.episode_id,
            request.expected_current_version_id,
            version.id,
        )
        session.add(version)
        append_audit_event(
            session,
            workspace_id=source.workspace_id,
            actor_id=claims.sub,
            action="script.version_published",
            target_type="script_version",
            target_id=version.id,
            trace_id=trace_id,
            metadata={
                "source_id": str(source.id),
                "episode_id": str(current.episode_id),
                "version_no": version.version_no,
                "previous_version_id": (
                    str(request.expected_current_version_id)
                    if request.expected_current_version_id is not None
                    else None
                ),
                "current_version_id": str(version.id),
                "episode_revision": current.revision,
            },
            occurred_at=now,
        )
        await session.flush()
        affected_shot_ids = await impact_reader(
            episode_id=current.episode_id,
            current_script_version_id=version.id,
        )
        narrative_impact = await narrative_impact_recorder(
            workspace_id=source.workspace_id,
            episode_id=current.episode_id,
            episode_revision=current.revision,
            previous_script_version_id=request.expected_current_version_id,
            current_script_version_id=version.id,
            affected_shot_ids=affected_shot_ids,
            actor_id=claims.sub,
        )
    return ScriptVersionPublishResponse(
        version=_version_response(version),
        current=_current_response(
            current.episode_id,
            request.expected_current_version_id,
            version.id,
            current.revision,
            affected_shot_ids,
            narrative_impact,
        ),
    )


async def set_current_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: CurrentScriptVersionRequest,
    impact_reader: ScriptVersionImpactReader,
    narrative_impact_recorder: NarrativeImpactRecorder,
    *,
    trace_id: str,
) -> CurrentScriptVersionResponse:
    async with session.begin():
        episode = await lock_active_episode_for_content_write(session, claims, episode_id)
        version = await repository.find_version(session, request.version_id)
        if version is None:
            raise resource_not_found("Script version")
        await require_resource_access(session, claims, version.workspace_id, "Script version")
        source = await repository.find_source(session, version.source_id)
        if source is None:
            raise resource_not_found("Script source")
        await require_resource_access(session, claims, source.workspace_id, "Script source")
        if source.episode_id != episode.episode_id:
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Script version belongs to another episode",
                status_code=409,
            )
        if version.status != "published":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Only published script versions can be current",
                status_code=409,
            )
        current = await compare_and_set_current_script_version(
            session,
            claims,
            episode.episode_id,
            request.expected_current_version_id,
            version.id,
        )
        append_audit_event(
            session,
            workspace_id=source.workspace_id,
            actor_id=claims.sub,
            action="script.current_changed",
            target_type="episode",
            target_id=current.episode_id,
            trace_id=trace_id,
            metadata={
                "episode_revision": current.revision,
                "previous_version_id": (
                    str(request.expected_current_version_id)
                    if request.expected_current_version_id is not None
                    else None
                ),
                "current_version_id": str(version.id),
            },
        )
        await session.flush()
        affected_shot_ids = await impact_reader(
            episode_id=current.episode_id,
            current_script_version_id=version.id,
        )
        narrative_impact = await narrative_impact_recorder(
            workspace_id=source.workspace_id,
            episode_id=current.episode_id,
            episode_revision=current.revision,
            previous_script_version_id=request.expected_current_version_id,
            current_script_version_id=version.id,
            affected_shot_ids=affected_shot_ids,
            actor_id=claims.sub,
        )
    return _current_response(
        current.episode_id,
        request.expected_current_version_id,
        version.id,
        current.revision,
        affected_shot_ids,
        narrative_impact,
    )


async def set_source_archived(
    session: AsyncSession,
    claims: AccessTokenClaims,
    source_id: UUID,
    request: ScriptSourceStateRequest,
    *,
    archived: bool,
    trace_id: str,
) -> ScriptSourceResponse:
    expected_status = "active" if archived else "archived"
    async with session.begin():
        source = await repository.find_source(session, source_id, for_update=True)
        if source is None:
            raise resource_not_found("Script source")
        await require_resource_access(session, claims, source.workspace_id, "Script source")
        await lock_active_episode_for_content_write(session, claims, source.episode_id)
        _source_revision(source, request.expected_revision)
        if source.status != expected_status:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Script source state conflict",
                status_code=409,
            )
        previous_status = source.status
        now = datetime.now(UTC)
        source.status = "archived" if archived else "active"
        source.archived_at = now if archived else None
        source.archived_by = claims.sub if archived else None
        source.revision += 1
        append_audit_event(
            session,
            workspace_id=source.workspace_id,
            actor_id=claims.sub,
            action=("script.source_archived" if archived else "script.source_restored"),
            target_type="script_source",
            target_id=source.id,
            trace_id=trace_id,
            metadata={
                "revision": source.revision,
                "previous_status": previous_status,
                "status": source.status,
                "episode_id": str(source.episode_id),
            },
            occurred_at=now,
        )
        await session.flush()
    return _source_response(source)


def _delete_blockers(
    version: ScriptVersion,
    episode_id: UUID,
    current_script_version_id: UUID | None,
    batches: list[ExtractionBatch],
) -> list[ScriptVersionDeleteBlocker]:
    blockers: list[ScriptVersionDeleteBlocker] = []
    if version.status != "draft":
        blockers.append(
            ScriptVersionDeleteBlocker(
                code="VERSION_NOT_DRAFT",
                resource_type="script_version",
                resource_id=version.id,
                summary="只有未引用的草稿版本可以硬删除",
            )
        )
    if current_script_version_id == version.id:
        blockers.append(
            ScriptVersionDeleteBlocker(
                code="CURRENT_VERSION",
                resource_type="episode",
                resource_id=episode_id,
                summary="该版本是单集当前剧本",
            )
        )
    for batch in batches:
        if batch.script_version_id == version.id:
            blockers.append(
                ScriptVersionDeleteBlocker(
                    code="HAS_EXTRACTION_BATCH",
                    resource_type="extraction_batch",
                    resource_id=batch.id,
                    summary="该版本已用于结构提取",
                )
            )
        if batch.confirmed_script_version_id == version.id:
            blockers.append(
                ScriptVersionDeleteBlocker(
                    code="CONFIRMED_STRUCTURE_VERSION",
                    resource_type="extraction_batch",
                    resource_id=batch.id,
                    summary="该版本是结构确认结果",
                )
            )
    return blockers


async def delete_draft_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
    *,
    confirmed: bool,
    trace_id: str,
) -> ScriptVersionDeleteResponse:
    if not confirmed:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Draft version deletion must be explicitly confirmed",
            status_code=422,
        )
    async with session.begin():
        version = await repository.find_version(session, version_id)
        if version is None:
            raise resource_not_found("Script version")
        await require_resource_access(
            session,
            claims,
            version.workspace_id,
            "Script version",
        )
        source = await repository.find_source(
            session,
            version.source_id,
            for_update=True,
        )
        if source is None or source.workspace_id != version.workspace_id:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Script source is unavailable",
                status_code=500,
            )
        episode = await lock_active_episode_for_content_write(
            session,
            claims,
            source.episode_id,
        )
        locked_version = await repository.find_version(
            session,
            version_id,
            for_update=True,
        )
        if locked_version is None:
            raise resource_not_found("Script version")
        if (
            locked_version.source_id != source.id
            or locked_version.workspace_id != source.workspace_id
        ):
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Script version state is unavailable",
                status_code=500,
            )
        batches = await repository.list_extraction_batches_referencing_version(
            session,
            locked_version.id,
        )
        blockers = _delete_blockers(
            locked_version,
            episode.episode_id,
            episode.current_script_version_id,
            batches,
        )
        if blockers:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Script version cannot be deleted",
                status_code=409,
                next_action="review_script_version_delete_blockers",
                details={
                    "script_version_id": str(locked_version.id),
                    "blockers": [blocker.model_dump(mode="json") for blocker in blockers],
                },
            )
        append_audit_event(
            session,
            workspace_id=locked_version.workspace_id,
            actor_id=claims.sub,
            action="script.version_deleted",
            target_type="script_version",
            target_id=locked_version.id,
            trace_id=trace_id,
            metadata={
                "source_id": str(source.id),
                "episode_id": str(episode.episode_id),
                "version_no": locked_version.version_no,
                "status": locked_version.status,
            },
        )
        await session.delete(locked_version)
        await session.flush()
    return ScriptVersionDeleteResponse(script_version_id=version_id)


async def diff_versions(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
    other_version_id: UUID,
) -> ScriptVersionDiffResponse:
    base = await repository.find_version(session, version_id)
    if base is None:
        raise resource_not_found("Script version")
    await require_resource_access(session, claims, base.workspace_id, "Script version")
    target = await repository.find_version(session, other_version_id)
    if target is None:
        raise resource_not_found("Script version")
    await require_resource_access(session, claims, target.workspace_id, "Script version")
    if base.source_id != target.source_id:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Script versions belong to different sources",
            status_code=409,
        )
    diff_lines = list(
        unified_diff(
            base.body.splitlines(),
            target.body.splitlines(),
            fromfile=f"version-{base.version_no}",
            tofile=f"version-{target.version_no}",
            lineterm="",
        )
    )
    return ScriptVersionDiffResponse(
        base_version_id=base.id,
        target_version_id=target.id,
        added_lines=sum(line.startswith("+") and not line.startswith("+++") for line in diff_lines),
        removed_lines=sum(
            line.startswith("-") and not line.startswith("---") for line in diff_lines
        ),
        diff_lines=diff_lines,
    )
