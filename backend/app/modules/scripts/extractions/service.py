import json
from datetime import UTC, datetime
from hashlib import sha256
from typing import Literal, cast
from uuid import UUID

from pydantic import TypeAdapter
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.assets import AssetCandidateCommand, create_or_link_candidate
from app.modules.identity import Capability, actor_context
from app.modules.production import (
    ScriptExtractionTaskCommand,
    TaskResponse,
    TaskStatus,
    complete_script_extraction_task,
    create_script_extraction_task,
    get_task,
    lock_task,
)
from app.modules.projects import lock_active_episode_for_content_write
from app.modules.scripts import repository
from app.modules.scripts.authorization import (
    require_resource_access,
    resource_not_found,
)
from app.modules.scripts.extractions.schemas import (
    AcceptWithChangesDecision,
    AssetCandidateProposal,
    CandidateDecisionCommand,
    CandidateDecisionEvidenceResponse,
    CandidateDecisionRequest,
    CandidateDecisionResultResponse,
    CandidateKind,
    CandidateProposal,
    CandidateSourceRange,
    CandidateStatus,
    ExtractionBatchResponse,
    ExtractionCandidateResponse,
    LinkExistingDecision,
    MergeIntoDecision,
    PaginatedCandidateDecisions,
    PaginatedExtractionCandidates,
    ScriptExtractionRequest,
    ScriptExtractionResult,
)
from app.modules.scripts.models import (
    CandidateDecision,
    ExtractionBatch,
    ExtractionCandidate,
    ScriptVersion,
)

SCRIPT_STRUCTURE_EXTRACTOR_VERSION = "script-structure-v1"
_CANDIDATE_PROPOSAL_ADAPTER: TypeAdapter[CandidateProposal] = TypeAdapter(
    CandidateProposal
)
_CANDIDATE_DECISION_ADAPTER: TypeAdapter[CandidateDecisionCommand] = TypeAdapter(
    CandidateDecisionCommand
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


def _decision_payload(decision: CandidateDecisionCommand) -> dict[str, object]:
    return decision.model_dump(mode="json", exclude={"action"})


def _decision_evidence(
    decision: CandidateDecision,
) -> CandidateDecisionEvidenceResponse:
    command = _CANDIDATE_DECISION_ADAPTER.validate_python(
        {"action": decision.action, **decision.payload}
    )
    return CandidateDecisionEvidenceResponse(
        id=decision.id,
        candidate_id=decision.candidate_id,
        sequence=decision.sequence,
        decision_key=decision.decision_key,
        decision=command,
        downstream_type=cast(Literal["ASSET"] | None, decision.downstream_type),
        downstream_id=decision.downstream_id,
        actor_id=decision.actor_id,
        created_at=decision.created_at,
    )


def _same_decision(
    existing: CandidateDecision,
    requested: CandidateDecisionCommand,
) -> bool:
    return (
        existing.action == requested.action
        and existing.payload == _decision_payload(requested)
    )


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
        version = await repository.find_version(session, version_id)
        if version is None:
            raise resource_not_found("Script version")
        await require_resource_access(
            session, claims, version.workspace_id, "Script version"
        )
        if version.status != "published":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Only published script versions can be extracted",
                status_code=409,
            )
        source = await repository.find_source(
            session, version.source_id, for_update=True
        )
        if source is None:
            raise resource_not_found("Script source")
        await require_resource_access(
            session, claims, source.workspace_id, "Script source"
        )
        if source.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Script source is archived",
                status_code=409,
            )
        episode = await lock_active_episode_for_content_write(
            session, claims, source.episode_id
        )
        actor = await actor_context(
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
            task = await get_task(session, claims, batch.task_id)
            return _batch_response(batch, task)

        task = await create_script_extraction_task(
            session,
            actor,
            ScriptExtractionTaskCommand(
                workspace_id=source.workspace_id,
                episode_id=episode.episode_id,
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
        batch.task_id = task.id
        await session.flush()
    return _batch_response(batch, task)


async def get_extraction_batch(
    session: AsyncSession,
    claims: AccessTokenClaims,
    batch_id: UUID,
) -> ExtractionBatchResponse:
    batch = await repository.find_extraction_batch(session, batch_id)
    if batch is None or batch.task_id is None:
        raise resource_not_found("Extraction batch")
    await require_resource_access(
        session, claims, batch.workspace_id, "Extraction batch"
    )
    task = await get_task(session, claims, batch.task_id)
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
    task = await lock_task(session, snapshot.task_id)
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
    await complete_script_extraction_task(session, task.id, now=now)
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
    batch = await repository.find_extraction_batch(session, batch_id)
    if batch is None:
        raise resource_not_found("Extraction batch")
    await require_resource_access(
        session, claims, batch.workspace_id, "Extraction batch"
    )
    candidates, total = await repository.list_extraction_candidates(
        session,
        batch_id,
        kind=kind,
        status=status,
        limit=limit,
        offset=offset,
    )
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
    candidate = await repository.find_extraction_candidate(session, candidate_id)
    if candidate is None:
        raise resource_not_found("Extraction candidate")
    await require_resource_access(
        session, claims, candidate.workspace_id, "Extraction candidate"
    )
    return _candidate_response(candidate)


async def decide_extraction_candidate(
    session: AsyncSession,
    claims: AccessTokenClaims,
    candidate_id: UUID,
    request: CandidateDecisionRequest,
) -> CandidateDecisionResultResponse:
    async with session.begin():
        candidate = await repository.find_extraction_candidate(
            session,
            candidate_id,
            for_update=True,
        )
        if candidate is None:
            raise resource_not_found("Extraction candidate")
        await require_resource_access(
            session, claims, candidate.workspace_id, "Extraction candidate"
        )
        actor = await actor_context(
            session,
            claims,
            candidate.workspace_id,
            Capability.CONTENT_WRITE,
        )
        existing = await repository.find_candidate_decision_by_key(
            session,
            candidate.id,
            request.decision_key,
        )
        if existing is not None:
            if not _same_decision(existing, request.decision):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Decision key was used with different input",
                    status_code=409,
                )
            return CandidateDecisionResultResponse(
                candidate=_candidate_response(candidate),
                evidence=_decision_evidence(existing),
            )
        if candidate.revision != request.expected_revision:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Extraction candidate has changed",
                status_code=409,
                details={"current_revision": candidate.revision},
            )
        if candidate.status != "pending":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Extraction candidate has already been decided",
                status_code=409,
            )
        batch = await repository.find_extraction_batch(session, candidate.batch_id)
        if batch is None or batch.status != "succeeded":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Extraction batch is not ready for decisions",
                status_code=409,
            )

        decision = request.decision
        if (
            isinstance(decision, AcceptWithChangesDecision)
            and decision.proposal.kind != candidate.kind
        ):
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Changed proposal must match the candidate kind",
                status_code=422,
            )
        if isinstance(decision, MergeIntoDecision):
            target = await repository.find_extraction_candidate(
                session,
                decision.target_candidate_id,
            )
            if (
                target is None
                or target.id == candidate.id
                or target.workspace_id != candidate.workspace_id
                or target.batch_id != candidate.batch_id
                or target.kind != candidate.kind
                or target.status in {"merged", "ignored"}
            ):
                raise ApiError(
                    ErrorCode.INVALID_REQUEST,
                    "Merge target must be an active candidate of the same kind and batch",
                    status_code=422,
                )

        if isinstance(decision, LinkExistingDecision) and candidate.kind != "asset":
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Only asset candidates can link an existing asset",
                status_code=422,
            )

        downstream_type: str | None = None
        downstream_id: UUID | None = None
        if candidate.kind == "asset" and decision.action in {
            "accept_new",
            "accept_with_changes",
            "link_existing",
        }:
            if batch.confirmed_script_version_id is None:
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Script structure must be confirmed before asset handoff",
                    status_code=409,
                    next_action="confirm_structure",
                )
            proposal_raw = (
                decision.proposal
                if isinstance(decision, AcceptWithChangesDecision)
                else _CANDIDATE_PROPOSAL_ADAPTER.validate_python(candidate.proposal)
            )
            if not isinstance(proposal_raw, AssetCandidateProposal):
                raise ApiError(
                    ErrorCode.INVALID_REQUEST,
                    "Asset candidate proposal is invalid",
                    status_code=422,
                )
            input_version = await repository.find_version(
                session, batch.script_version_id
            )
            source = (
                None
                if input_version is None
                else await repository.find_source(session, input_version.source_id)
            )
            if source is None or source.workspace_id != candidate.workspace_id:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Asset candidate source is unavailable",
                    status_code=500,
                )
            episode = await lock_active_episode_for_content_write(
                session, claims, source.episode_id
            )
            result = await create_or_link_candidate(
                session,
                actor,
                AssetCandidateCommand(
                    workspace_id=candidate.workspace_id,
                    project_id=episode.project_id,
                    candidate_id=candidate.id,
                    decision_key=request.decision_key,
                    actor_id=actor.user_id,
                    action=decision.action,
                    kind=proposal_raw.asset_kind,
                    name=proposal_raw.name,
                    description=proposal_raw.description,
                    target_asset_id=(
                        decision.downstream_id
                        if isinstance(decision, LinkExistingDecision)
                        else None
                    ),
                ),
            )
            downstream_type = "ASSET"
            downstream_id = result.asset_id

        status_by_action: dict[str, CandidateStatus] = {
            "accept_new": "accepted",
            "accept_with_changes": "accepted",
            "link_existing": "linked",
            "merge_into": "merged",
            "ignore": "ignored",
        }
        now = datetime.now(UTC)
        evidence = CandidateDecision(
            id=uuid7(),
            workspace_id=candidate.workspace_id,
            candidate_id=candidate.id,
            sequence=candidate.revision,
            decision_key=request.decision_key,
            action=decision.action,
            payload=_decision_payload(decision),
            downstream_type=downstream_type,
            downstream_id=downstream_id,
            actor_id=actor.user_id,
            created_at=now,
        )
        session.add(evidence)
        candidate.status = status_by_action[decision.action]
        candidate.revision += 1
        candidate.updated_at = now
        await session.flush()
    return CandidateDecisionResultResponse(
        candidate=_candidate_response(candidate),
        evidence=_decision_evidence(evidence),
    )


async def list_candidate_decisions(
    session: AsyncSession,
    claims: AccessTokenClaims,
    candidate_id: UUID,
    *,
    limit: int,
    offset: int,
) -> PaginatedCandidateDecisions:
    candidate = await repository.find_extraction_candidate(session, candidate_id)
    if candidate is None:
        raise resource_not_found("Extraction candidate")
    await require_resource_access(
        session, claims, candidate.workspace_id, "Extraction candidate"
    )
    decisions, total = await repository.list_candidate_decisions(
        session,
        candidate_id,
        limit=limit,
        offset=offset,
    )
    return PaginatedCandidateDecisions(
        items=[_decision_evidence(decision) for decision in decisions],
        total=total,
        limit=limit,
        offset=offset,
    )
