import json
from datetime import UTC, datetime
from difflib import unified_diff
from hashlib import sha256
from typing import Literal, cast
from uuid import UUID

from pydantic import TypeAdapter
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import service as identity_service
from app.modules.identity.policy import Capability
from app.modules.production import repository as production_repository
from app.modules.production import service as production_service
from app.modules.production.schemas import (
    ScriptExtractionTaskCommand,
    TaskResponse,
    TaskStatus,
)
from app.modules.projects import service as projects_service
from app.modules.scripts import repository
from app.modules.scripts.models import (
    ExtractionBatch,
    ExtractionCandidate,
    ScriptSource,
    ScriptVersion,
)
from app.modules.scripts.schemas import (
    CandidateKind,
    CandidateProposal,
    CandidateSourceRange,
    CandidateStatus,
    CurrentScriptVersionRequest,
    CurrentScriptVersionResponse,
    ExtractionBatchResponse,
    ExtractionCandidateResponse,
    PaginatedExtractionCandidates,
    PaginatedScriptVersions,
    ScriptExtractionRequest,
    ScriptExtractionResult,
    ScriptImportRequest,
    ScriptImportResponse,
    ScriptSourceResponse,
    ScriptSourceStateRequest,
    ScriptVersionDiffResponse,
    ScriptVersionPublishRequest,
    ScriptVersionPublishResponse,
    ScriptVersionResponse,
)

SCRIPT_STRUCTURE_EXTRACTOR_VERSION = "script-structure-v1"
_CANDIDATE_PROPOSAL_ADAPTER: TypeAdapter[CandidateProposal] = TypeAdapter(
    CandidateProposal
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


def _not_found(resource: str) -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, f"{resource} not found", status_code=404)


def _current_response(
    episode_id: UUID,
    current_script_version_id: UUID,
    episode_revision: int,
) -> CurrentScriptVersionResponse:
    return CurrentScriptVersionResponse(
        episode_id=episode_id,
        current_script_version_id=current_script_version_id,
        episode_revision=episode_revision,
    )


def _source_revision(source: ScriptSource, expected_revision: int) -> None:
    if source.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Script source has changed",
            status_code=409,
            details={"current_revision": source.revision},
        )


def _batch_response(
    batch: ExtractionBatch,
    task: TaskResponse,
) -> ExtractionBatchResponse:
    if batch.task_id != task.id:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Extraction batch task is unavailable",
            status_code=500,
        )
    return ExtractionBatchResponse(
        id=batch.id,
        workspace_id=batch.workspace_id,
        script_version_id=batch.script_version_id,
        scope=cast(Literal["full"], batch.scope),
        extractor_version=batch.extractor_version,
        input_hash=batch.input_hash,
        status=task.status,
        confirmed_script_version_id=batch.confirmed_script_version_id,
        candidate_count=batch.candidate_count,
        task=task,
        created_at=batch.created_at,
    )


def _candidate_response(
    candidate: ExtractionCandidate,
) -> ExtractionCandidateResponse:
    return ExtractionCandidateResponse(
        id=candidate.id,
        batch_id=candidate.batch_id,
        candidate_key=candidate.candidate_key,
        kind=cast(CandidateKind, candidate.kind),
        source_range=CandidateSourceRange(
            start=candidate.source_start,
            end=candidate.source_end,
        ),
        proposal=_CANDIDATE_PROPOSAL_ADAPTER.validate_python(candidate.proposal),
        confidence_note=candidate.confidence_note,
        required=candidate.required,
        status=cast(CandidateStatus, candidate.status),
        revision=candidate.revision,
        created_at=candidate.created_at,
    )


def _extraction_result_hash(result: ScriptExtractionResult) -> str:
    candidates = sorted(
        (
            candidate.model_dump(mode="json")
            for candidate in result.candidates
        ),
        key=lambda candidate: candidate["candidate_key"],
    )
    canonical = json.dumps(
        {"candidates": candidates},
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return sha256(canonical.encode()).hexdigest()


def _same_extraction(
    batch: ExtractionBatch,
    version: ScriptVersion,
    request: ScriptExtractionRequest,
) -> bool:
    return (
        batch.script_version_id == version.id
        and batch.scope == request.scope
        and batch.extractor_version == SCRIPT_STRUCTURE_EXTRACTOR_VERSION
        and batch.input_hash == version.content_hash
    )


def _task_idempotency_key(version_id: UUID, idempotency_key: str) -> str:
    return sha256(f"{version_id}:{idempotency_key}".encode()).hexdigest()


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
) -> ScriptImportResponse:
    title = request.title.strip()
    rights_declaration = request.rights_declaration.strip()
    if not title or not rights_declaration:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Script metadata is required", status_code=422)
    content_hash = sha256(request.body.encode("utf-8")).hexdigest()
    now = datetime.now(UTC)
    async with session.begin():
        episode = await projects_service.lock_active_episode_for_content_write(
            session, claims, episode_id
        )
        source_id = uuid7()
        inserted_id = await session.scalar(
            insert(ScriptSource)
            .values(
                id=source_id,
                workspace_id=episode.workspace_id,
                episode_id=episode.id,
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
                session, episode.id, request.idempotency_key
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
            episode_id=episode.id,
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
    user = await identity_service.authenticated_user(session, claims)
    source = await repository.find_source_for_user(session, user.id, source_id)
    if source is None:
        raise _not_found("Script source")
    return _source_response(source)


async def get_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
) -> ScriptVersionResponse:
    user = await identity_service.authenticated_user(session, claims)
    version = await repository.find_version_for_user(session, user.id, version_id)
    if version is None:
        raise _not_found("Script version")
    return _version_response(version)


async def list_versions(
    session: AsyncSession,
    claims: AccessTokenClaims,
    source_id: UUID,
    *,
    limit: int,
    offset: int,
) -> PaginatedScriptVersions:
    user = await identity_service.authenticated_user(session, claims)
    result = await repository.list_versions_for_user(
        session, user.id, source_id, limit=limit, offset=offset
    )
    if result is None:
        raise _not_found("Script source")
    versions, total = result
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
) -> ScriptVersionPublishResponse:
    content_hash = sha256(request.body.encode("utf-8")).hexdigest()
    now = datetime.now(UTC)
    async with session.begin():
        user = await identity_service.authenticated_user(session, claims)
        source = await repository.find_source_for_user(
            session, user.id, source_id, for_update=True
        )
        if source is None:
            raise _not_found("Script source")
        if source.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Script source is archived",
                status_code=409,
            )
        episode = await projects_service.lock_active_episode_for_content_write(
            session, claims, source.episode_id
        )
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
        projects_service.compare_and_set_current_script_version(
            episode,
            request.expected_current_version_id,
            version.id,
        )
        session.add(version)
        await session.flush()
    return ScriptVersionPublishResponse(
        version=_version_response(version),
        current=_current_response(
            episode.id,
            version.id,
            episode.revision,
        ),
    )


async def set_current_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: CurrentScriptVersionRequest,
) -> CurrentScriptVersionResponse:
    async with session.begin():
        episode = await projects_service.lock_active_episode_for_content_write(
            session, claims, episode_id
        )
        version = await repository.find_version_for_user(
            session, claims.sub, request.version_id
        )
        if version is None:
            raise _not_found("Script version")
        source = await repository.find_source_for_user(
            session, claims.sub, version.source_id
        )
        if source is None:
            raise _not_found("Script source")
        if source.episode_id != episode.id:
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
        projects_service.compare_and_set_current_script_version(
            episode,
            request.expected_current_version_id,
            version.id,
        )
        await session.flush()
    return _current_response(episode.id, version.id, episode.revision)


async def set_source_archived(
    session: AsyncSession,
    claims: AccessTokenClaims,
    source_id: UUID,
    request: ScriptSourceStateRequest,
    *,
    archived: bool,
) -> ScriptSourceResponse:
    expected_status = "active" if archived else "archived"
    async with session.begin():
        user = await identity_service.authenticated_user(session, claims)
        source = await repository.find_source_for_user(
            session, user.id, source_id, for_update=True
        )
        if source is None:
            raise _not_found("Script source")
        await projects_service.lock_active_episode_for_content_write(
            session, claims, source.episode_id
        )
        _source_revision(source, request.expected_revision)
        if source.status != expected_status:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Script source state conflict",
                status_code=409,
            )
        source.status = "archived" if archived else "active"
        source.archived_at = datetime.now(UTC) if archived else None
        source.archived_by = claims.sub if archived else None
        source.revision += 1
        await session.flush()
    return _source_response(source)


async def diff_versions(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
    other_version_id: UUID,
) -> ScriptVersionDiffResponse:
    user = await identity_service.authenticated_user(session, claims)
    base = await repository.find_version_for_user(session, user.id, version_id)
    if base is None:
        raise _not_found("Script version")
    target = await repository.find_version_for_user(
        session, user.id, other_version_id
    )
    if target is None:
        raise _not_found("Script version")
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
        added_lines=sum(
            line.startswith("+") and not line.startswith("+++")
            for line in diff_lines
        ),
        removed_lines=sum(
            line.startswith("-") and not line.startswith("---")
            for line in diff_lines
        ),
        diff_lines=diff_lines,
    )


async def start_extraction(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
    request: ScriptExtractionRequest,
    *,
    trace_id: str,
) -> ExtractionBatchResponse:
    now = datetime.now(UTC)
    async with session.begin():
        user = await identity_service.authenticated_user(session, claims)
        version = await repository.find_version_for_user(session, user.id, version_id)
        if version is None:
            raise _not_found("Script version")
        if version.status != "published":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Only published script versions can be extracted",
                status_code=409,
            )
        source = await repository.find_source_for_user(
            session, user.id, version.source_id, for_update=True
        )
        if source is None:
            raise _not_found("Script source")
        if source.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Script source is archived",
                status_code=409,
            )
        episode = await projects_service.lock_active_episode_for_content_write(
            session, claims, source.episode_id
        )
        actor = await identity_service.actor_context(
            session,
            claims,
            source.workspace_id,
            Capability.CONTENT_WRITE,
        )
        batch_id = uuid7()
        inserted_id = await session.scalar(
            insert(ExtractionBatch)
            .values(
                id=batch_id,
                workspace_id=source.workspace_id,
                script_version_id=version.id,
                scope=request.scope,
                extractor_version=SCRIPT_STRUCTURE_EXTRACTOR_VERSION,
                input_hash=version.content_hash,
                status="queued",
                idempotency_key=request.idempotency_key,
                created_by=claims.sub,
                created_at=now,
                updated_at=now,
            )
            .on_conflict_do_nothing(
                constraint="uq_scr_batch_version_idempotency"
            )
            .returning(ExtractionBatch.id)
        )
        if inserted_id is None:
            batch = await repository.find_idempotent_extraction_batch(
                session, version.id, request.idempotency_key
            )
            if batch is None or batch.task_id is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Extraction batch state is unavailable",
                    status_code=500,
                )
            if not _same_extraction(batch, version, request):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            task = await production_service.get_task(session, claims, batch.task_id)
            return _batch_response(batch, task)

        task_model = await production_service.create_script_extraction_task(
            session,
            actor,
            ScriptExtractionTaskCommand(
                workspace_id=source.workspace_id,
                episode_id=episode.id,
                request_id=inserted_id,
                input_version_id=version.id,
                input_hash=version.content_hash,
                idempotency_key=_task_idempotency_key(
                    version.id, request.idempotency_key
                ),
            ),
            trace_id=trace_id,
        )
        batch = await repository.find_extraction_batch(
            session, inserted_id, for_update=True
        )
        if batch is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Extraction batch state is unavailable",
                status_code=500,
            )
        batch.task_id = task_model.id
        await session.flush()
    return _batch_response(batch, production_service.task_response(task_model))


async def get_extraction_batch(
    session: AsyncSession,
    claims: AccessTokenClaims,
    batch_id: UUID,
) -> ExtractionBatchResponse:
    user = await identity_service.authenticated_user(session, claims)
    batch = await repository.find_extraction_batch_for_user(
        session, user.id, batch_id
    )
    if batch is None or batch.task_id is None:
        raise _not_found("Extraction batch")
    task = await production_service.get_task(session, claims, batch.task_id)
    return _batch_response(batch, task)


async def synchronize_extraction_batch_status(
    session: AsyncSession,
    batch_id: UUID,
    status: TaskStatus,
    *,
    now: datetime,
) -> None:
    batch = await repository.find_extraction_batch(
        session, batch_id, for_update=True
    )
    if batch is None:
        return
    batch.status = status
    batch.updated_at = now
    await session.flush()


async def record_extraction_result(
    session: AsyncSession,
    batch_id: UUID,
    result: ScriptExtractionResult,
) -> None:
    snapshot = await repository.find_extraction_batch(session, batch_id)
    if snapshot is None or snapshot.task_id is None:
        raise ApiError(
            ErrorCode.NOT_FOUND,
            "Extraction batch not found",
            status_code=404,
        )
    task = await production_repository.find_task(
        session,
        snapshot.task_id,
        for_update=True,
    )
    batch = await repository.find_extraction_batch(
        session,
        batch_id,
        for_update=True,
    )
    if (
        task is None
        or batch is None
        or batch.task_id != task.id
        or task.request_id != batch.id
        or task.workspace_id != batch.workspace_id
    ):
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Extraction task state is unavailable",
            status_code=500,
        )

    result_hash = _extraction_result_hash(result)
    if task.status == "succeeded" or batch.status == "succeeded":
        if (
            task.status == "succeeded"
            and batch.status == "succeeded"
            and batch.result_hash == result_hash
        ):
            return
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Extraction result does not match the completed batch",
            status_code=409,
        )
    if task.status in {"failed", "cancelled"} or batch.status in {
        "failed",
        "cancelled",
    }:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Extraction batch cannot accept a result",
            status_code=409,
        )
    if batch.candidate_count != 0 or batch.result_hash is not None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Extraction result state is unavailable",
            status_code=500,
        )

    version = await repository.find_version(session, batch.script_version_id)
    if version is None or version.workspace_id != batch.workspace_id:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Extraction input version is unavailable",
            status_code=500,
        )
    for candidate in result.candidates:
        if candidate.source_range.end > len(version.body):
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Candidate source range exceeds the script body",
                status_code=422,
            )

    now = datetime.now(UTC)
    session.add_all(
        [
            ExtractionCandidate(
                id=uuid7(),
                workspace_id=batch.workspace_id,
                batch_id=batch.id,
                candidate_key=candidate.candidate_key,
                kind=candidate.proposal.kind,
                source_start=candidate.source_range.start,
                source_end=candidate.source_range.end,
                proposal=candidate.proposal.model_dump(mode="json"),
                confidence_note=candidate.confidence_note,
                required=candidate.proposal.kind in {"scene", "dialogue"},
                status="pending",
                revision=1,
                created_at=now,
                updated_at=now,
            )
            for candidate in result.candidates
        ]
    )
    production_service.complete_script_extraction_task(task, now=now)
    batch.status = "succeeded"
    batch.result_hash = result_hash
    batch.candidate_count = len(result.candidates)
    batch.updated_at = now
    await session.flush()


async def list_extraction_candidates(
    session: AsyncSession,
    claims: AccessTokenClaims,
    batch_id: UUID,
    *,
    kind: CandidateKind | None,
    status: CandidateStatus | None,
    limit: int,
    offset: int,
) -> PaginatedExtractionCandidates:
    user = await identity_service.authenticated_user(session, claims)
    result = await repository.list_extraction_candidates_for_user(
        session,
        user.id,
        batch_id,
        kind=kind,
        status=status,
        limit=limit,
        offset=offset,
    )
    if result is None:
        raise _not_found("Extraction batch")
    candidates, total = result
    return PaginatedExtractionCandidates(
        items=[_candidate_response(candidate) for candidate in candidates],
        total=total,
        limit=limit,
        offset=offset,
    )


async def get_extraction_candidate(
    session: AsyncSession,
    claims: AccessTokenClaims,
    candidate_id: UUID,
) -> ExtractionCandidateResponse:
    user = await identity_service.authenticated_user(session, claims)
    candidate = await repository.find_extraction_candidate_for_user(
        session,
        user.id,
        candidate_id,
    )
    if candidate is None:
        raise _not_found("Extraction candidate")
    return _candidate_response(candidate)
