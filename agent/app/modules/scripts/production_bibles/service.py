import hashlib
import json
from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from pydantic import ValidationError
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import Capability, actor_context
from app.modules.production import (
    ProductionBibleTaskCommand,
    create_production_bible_task,
    lock_task,
)
from app.modules.projects import (
    lock_active_project_for_content_write,
    project_for_content_read,
)
from app.modules.scripts.documents import repository as document_repository
from app.modules.scripts.production_bibles import repository
from app.modules.scripts.production_bibles.harness import (
    ProductionBibleCheckpoint,
    build_evidence_chunks,
)
from app.modules.scripts.production_bibles.models import (
    ProductionBible,
    ProductionBibleEntity,
    ProductionBibleEntityState,
    ProductionBibleWorldEntry,
)
from app.modules.scripts.production_bibles.ports import (
    PRODUCTION_BIBLE_ENGINE_VERSION,
    PRODUCTION_BIBLE_HARNESS_VERSION,
    PRODUCTION_BIBLE_PROMPT_VERSION,
    PRODUCTION_BIBLE_SCHEMA_VERSION,
    ProductionBibleInput,
)
from app.modules.scripts.production_bibles.schemas import (
    BibleEntityCandidate,
    BibleEntityKind,
    BibleEvidence,
    BibleReviewIssue,
    ProductionBibleCreateRequest,
    ProductionBibleEntityResponse,
    ProductionBibleEntityStateResponse,
    ProductionBibleProviderResult,
    ProductionBibleResponse,
    ProductionBibleResumeRequest,
    ProductionBibleWorldEntryResponse,
)

PRODUCTION_BIBLE_MODEL_NAME = "codex-local"


class ProductionBibleInputChanged(RuntimeError):
    pass


class ProductionBibleLeaseActive(RuntimeError):
    pass


def _result_hash(result: ProductionBibleProviderResult) -> str:
    payload = json.dumps(
        result.model_dump(mode="json"),
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def _resume_receipt_key(bible_id: UUID, idempotency_key: str) -> str:
    return hashlib.sha256(f"{bible_id}:{idempotency_key}".encode()).hexdigest()


def _resume_command_hash(
    bible_id: UUID,
    request: ProductionBibleResumeRequest,
) -> str:
    payload = json.dumps(
        {
            "bible_id": str(bible_id),
            "expected_revision": request.expected_revision,
        },
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def _find_resume_receipt(
    bible: ProductionBible,
    *,
    receipt_key: str,
    command_hash: str,
) -> UUID | None:
    raw_receipt: object = bible.resume_receipts.get(receipt_key)
    if raw_receipt is None:
        return None
    if not isinstance(raw_receipt, dict):
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Production Bible resume receipt is invalid",
            status_code=409,
            next_action="review_production_bible_failure",
        )
    receipt = cast(dict[str, object], raw_receipt)
    if receipt.get("command_hash") != command_hash:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Production Bible resume key was used with different input",
            status_code=409,
        )
    try:
        return UUID(str(receipt["task_id"]))
    except (KeyError, TypeError, ValueError) as error:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Production Bible resume receipt is invalid",
            status_code=409,
            next_action="review_production_bible_failure",
        ) from error


def _checkpoint_evidence(checkpoint: ProductionBibleCheckpoint) -> list[BibleEvidence]:
    evidence = [
        item
        for chunk in checkpoint.evidence_chunks
        for observation in chunk.observations
        for item in observation.evidence
    ]
    if checkpoint.candidate is not None:
        for entity in checkpoint.candidate.entities:
            evidence.extend(entity.evidence)
            for state in entity.states:
                evidence.extend(state.evidence)
        for entry in checkpoint.candidate.world_entries:
            evidence.extend(entry.evidence)
        for issue in checkpoint.candidate.review_issues:
            evidence.extend(issue.evidence)
    if checkpoint.review is not None:
        for issue in checkpoint.review.review_issues:
            evidence.extend(issue.evidence)
    return evidence


def _checkpoint_matches_source(
    checkpoint: ProductionBibleCheckpoint,
    source: str,
) -> bool:
    chunks = build_evidence_chunks(source)
    expected_chunks = {chunk.key: (chunk.source_start, chunk.source_end) for chunk in chunks}
    if any(
        expected_chunks.get(chunk.chunk_key) != (chunk.source_start, chunk.source_end)
        for chunk in checkpoint.evidence_chunks
    ):
        return False
    if checkpoint.stage in {"reconciled", "reviewed"} and set(
        checkpoint.completed_chunk_keys
    ) != set(expected_chunks):
        return False
    return all(
        item.source_end <= len(source)
        and source[item.source_start : item.source_end] == item.exact_anchor
        for item in _checkpoint_evidence(checkpoint)
    )


async def _response(
    session: AsyncSession,
    bible: ProductionBible,
) -> ProductionBibleResponse:
    entities = await repository.list_entities(session, bible.id)
    states = await repository.list_entity_states(session, bible.id)
    world_entries = await repository.list_world_entries(session, bible.id)
    states_by_entity: dict[UUID, list[ProductionBibleEntityStateResponse]] = {}
    for state in states:
        states_by_entity.setdefault(state.entity_id, []).append(
            ProductionBibleEntityStateResponse(
                id=state.id,
                entity_id=state.entity_id,
                state_key=state.state_key,
                label=state.label,
                state_spec=state.state_spec,
                episode_numbers=list(state.episode_numbers),
                evidence=[BibleEvidence.model_validate(item) for item in state.evidence],
                asset_state_id=state.asset_state_id,
                asset_version_id=state.asset_version_id,
                created_at=state.created_at,
                updated_at=state.updated_at,
            )
        )
    return ProductionBibleResponse(
        id=bible.id,
        workspace_id=bible.workspace_id,
        project_id=bible.project_id,
        document_revision_id=bible.document_revision_id,
        task_id=bible.task_id,
        status=cast(
            Literal[
                "queued",
                "running",
                "needs_review",
                "confirmed",
                "failed",
                "unknown",
                "superseded",
                "cancelled",
            ],
            bible.status,
        ),
        input_hash=bible.input_hash,
        result_hash=bible.result_hash,
        engine_version=bible.engine_version,
        model_name=bible.model_name,
        prompt_version=bible.prompt_version,
        schema_version=bible.schema_version,
        harness_version=bible.harness_version,
        checkpoint_stage=(
            cast(str | None, bible.checkpoint.get("stage"))
            if bible.checkpoint is not None
            else None
        ),
        checkpoint_revision=bible.checkpoint_revision,
        checkpoint_updated_at=bible.checkpoint_updated_at,
        review_issues=[BibleReviewIssue.model_validate(item) for item in bible.review_issues],
        revision=bible.revision,
        confirmed_at=bible.confirmed_at,
        confirmed_by=bible.confirmed_by,
        entities=[
            ProductionBibleEntityResponse(
                id=entity.id,
                entity_key=entity.entity_key,
                kind=cast(BibleEntityKind, entity.kind),
                canonical_name=entity.canonical_name,
                normalized_name=entity.normalized_name,
                aliases=list(entity.aliases),
                stable_spec=entity.stable_spec,
                episode_numbers=list(entity.episode_numbers),
                evidence=[BibleEvidence.model_validate(item) for item in entity.evidence],
                asset_id=entity.asset_id,
                states=states_by_entity.get(entity.id, []),
                created_at=entity.created_at,
                updated_at=entity.updated_at,
            )
            for entity in entities
        ],
        world_entries=[
            ProductionBibleWorldEntryResponse(
                id=entry.id,
                entry_key=entry.entry_key,
                category=entry.category,
                title=entry.title,
                facts=list(entry.facts),
                rules=list(entry.rules),
                entity_keys=list(entry.entity_keys),
                episode_numbers=list(entry.episode_numbers),
                evidence=[BibleEvidence.model_validate(item) for item in entry.evidence],
                created_at=entry.created_at,
                updated_at=entry.updated_at,
            )
            for entry in world_entries
        ],
        created_at=bible.created_at,
        updated_at=bible.updated_at,
    )


async def create_bible(
    session: AsyncSession,
    claims: AccessTokenClaims,
    revision_id: UUID,
    request: ProductionBibleCreateRequest,
    *,
    trace_id: str,
) -> ProductionBibleResponse:
    now = datetime.now(UTC)
    async with session.begin():
        found = await document_repository.find_revision_with_document(session, revision_id)
        if found is None:
            raise ApiError(ErrorCode.NOT_FOUND, "Document revision not found", status_code=404)
        revision, document = found
        project = await lock_active_project_for_content_write(
            session,
            claims,
            document.project_id,
        )
        existing = await repository.find_bible_by_idempotency(
            session,
            revision.id,
            request.idempotency_key,
        )
        if existing is not None:
            return await _response(session, existing)

        bible_id = uuid7()
        inserted_id = await session.scalar(
            insert(ProductionBible)
            .values(
                id=bible_id,
                workspace_id=project.workspace_id,
                project_id=project.project_id,
                document_revision_id=revision.id,
                task_id=None,
                status="queued",
                input_hash=revision.normalized_hash,
                result_hash=None,
                engine_version=PRODUCTION_BIBLE_ENGINE_VERSION,
                model_name=PRODUCTION_BIBLE_MODEL_NAME,
                prompt_version=PRODUCTION_BIBLE_PROMPT_VERSION,
                schema_version=PRODUCTION_BIBLE_SCHEMA_VERSION,
                harness_version=PRODUCTION_BIBLE_HARNESS_VERSION,
                checkpoint=None,
                checkpoint_revision=0,
                checkpoint_updated_at=None,
                run_token=None,
                lease_expires_at=None,
                review_issues=[],
                resume_receipts={},
                revision=1,
                idempotency_key=request.idempotency_key,
                confirm_result={},
                created_by=claims.sub,
                created_at=now,
                updated_at=now,
            )
            .on_conflict_do_nothing(constraint="uq_scr_prod_bible_revision_idempotency")
            .returning(ProductionBible.id)
        )
        if inserted_id is None:
            existing = await repository.find_bible_by_idempotency(
                session,
                revision.id,
                request.idempotency_key,
            )
            if existing is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Production Bible state is unavailable",
                    status_code=500,
                )
            return await _response(session, existing)

        actor = await actor_context(
            session,
            claims,
            project.workspace_id,
            Capability.CONTENT_WRITE,
        )
        task = await create_production_bible_task(
            session,
            actor,
            ProductionBibleTaskCommand(
                workspace_id=project.workspace_id,
                bible_id=inserted_id,
                document_revision_id=revision.id,
                input_hash=revision.normalized_hash,
                idempotency_key=f"production-bible:{inserted_id}",
            ),
            trace_id=trace_id,
        )
        bible = await repository.find_bible(session, inserted_id, for_update=True)
        if bible is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Production Bible state is unavailable",
                status_code=500,
            )
        bible.task_id = task.id
        append_audit_event(
            session,
            workspace_id=bible.workspace_id,
            actor_id=claims.sub,
            action="script.production_bible_created",
            target_type="production_bible",
            target_id=bible.id,
            trace_id=trace_id,
            metadata={
                "document_revision_id": str(revision.id),
                "project_id": str(project.project_id),
                "input_hash": bible.input_hash,
            },
            occurred_at=now,
        )
        await session.flush()
        return await _response(session, bible)


async def get_bible(
    session: AsyncSession,
    claims: AccessTokenClaims,
    bible_id: UUID,
) -> ProductionBibleResponse:
    bible = await repository.find_bible(session, bible_id)
    if bible is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Production Bible not found", status_code=404)
    try:
        await project_for_content_read(session, claims, bible.project_id)
    except ApiError as error:
        if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
            raise ApiError(
                ErrorCode.NOT_FOUND,
                "Production Bible not found",
                status_code=404,
            ) from error
        raise
    return await _response(session, bible)


async def get_current_bible(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
) -> ProductionBibleResponse:
    await project_for_content_read(session, claims, project_id)
    bible = await repository.find_current_confirmed_bible(session, project_id)
    if bible is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Production Bible not found", status_code=404)
    return await _response(session, bible)


async def resume_bible(
    session: AsyncSession,
    claims: AccessTokenClaims,
    bible_id: UUID,
    request: ProductionBibleResumeRequest,
    *,
    trace_id: str,
) -> ProductionBibleResponse:
    receipt_key = _resume_receipt_key(bible_id, request.idempotency_key)
    command_hash = _resume_command_hash(bible_id, request)
    now = datetime.now(UTC)
    async with session.begin():
        bible = await repository.find_bible(session, bible_id, for_update=True)
        if bible is None:
            raise ApiError(ErrorCode.NOT_FOUND, "Production Bible not found", status_code=404)
        project = await lock_active_project_for_content_write(
            session,
            claims,
            bible.project_id,
        )
        if project.workspace_id != bible.workspace_id:
            raise ApiError(ErrorCode.NOT_FOUND, "Production Bible not found", status_code=404)

        replayed_task_id = _find_resume_receipt(
            bible,
            receipt_key=receipt_key,
            command_hash=command_hash,
        )
        if replayed_task_id is not None:
            return await _response(session, bible)

        if bible.revision != request.expected_revision:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Production Bible has changed",
                status_code=409,
                details={"current_revision": bible.revision},
            )
        if bible.status not in {"failed", "unknown"}:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible cannot be resumed from its current state",
                status_code=409,
                next_action="review_production_bible_failure",
            )
        if bible.run_token is not None or bible.lease_expires_at is not None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible still has an active run lease",
                status_code=409,
                next_action="review_production_bible_failure",
            )
        if (
            bible.task_id is None
            or bible.checkpoint is None
            or bible.checkpoint_revision < 1
            or bible.checkpoint_updated_at is None
        ):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible has no recoverable checkpoint",
                status_code=409,
                next_action="review_production_bible_failure",
            )

        previous_task = await lock_task(session, bible.task_id)
        if (
            previous_task is None
            or previous_task.workspace_id != bible.workspace_id
            or previous_task.request_id != bible.id
            or previous_task.task_type != "production_bible"
            or previous_task.request_type != "production_bible"
            or previous_task.usage_type != "document_revision"
            or previous_task.usage_id != bible.document_revision_id
            or previous_task.input_hash != bible.input_hash
            or previous_task.status != bible.status
        ):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible execution history is inconsistent",
                status_code=409,
                next_action="review_production_bible_failure",
            )

        found = await document_repository.find_revision_with_document(
            session,
            bible.document_revision_id,
        )
        if found is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible source revision is unavailable",
                status_code=409,
                next_action="review_production_bible_failure",
            )
        revision, document = found
        if (
            document.project_id != bible.project_id
            or revision.workspace_id != bible.workspace_id
            or revision.normalized_hash != bible.input_hash
        ):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible source revision has changed",
                status_code=409,
                next_action="review_production_bible_failure",
            )
        try:
            checkpoint = ProductionBibleCheckpoint.model_validate(bible.checkpoint)
        except (TypeError, ValidationError, ValueError) as error:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible checkpoint is invalid",
                status_code=409,
                next_action="review_production_bible_failure",
            ) from error
        if (
            checkpoint.bible_id != bible.id
            or checkpoint.task_id != previous_task.id
            or checkpoint.input_hash != bible.input_hash
            or checkpoint.harness_version != bible.harness_version
            or checkpoint.harness_version != PRODUCTION_BIBLE_HARNESS_VERSION
            or not _checkpoint_matches_source(checkpoint, revision.normalized_text)
        ):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible checkpoint cannot be resumed safely",
                status_code=409,
                next_action="review_production_bible_failure",
            )

        actor = await actor_context(
            session,
            claims,
            bible.workspace_id,
            Capability.CONTENT_WRITE,
        )
        task = await create_production_bible_task(
            session,
            actor,
            ProductionBibleTaskCommand(
                workspace_id=bible.workspace_id,
                bible_id=bible.id,
                document_revision_id=bible.document_revision_id,
                input_hash=bible.input_hash,
                idempotency_key=f"production-bible-resume:{receipt_key}",
            ),
            trace_id=trace_id,
        )
        previous_status = bible.status
        previous_task_id = bible.task_id
        bible.task_id = task.id
        bible.status = "queued"
        bible.run_token = None
        bible.lease_expires_at = None
        bible.revision += 1
        bible.updated_at = now
        receipts = dict(bible.resume_receipts)
        receipts[receipt_key] = {
            "command_hash": command_hash,
            "task_id": str(task.id),
            "result_revision": bible.revision,
        }
        bible.resume_receipts = receipts
        append_audit_event(
            session,
            workspace_id=bible.workspace_id,
            actor_id=claims.sub,
            action="script.production_bible_resumed",
            target_type="production_bible",
            target_id=bible.id,
            trace_id=trace_id,
            metadata={
                "previous_status": previous_status,
                "previous_task_id": str(previous_task_id),
                "task_id": str(task.id),
                "revision": bible.revision,
                "checkpoint_revision": bible.checkpoint_revision,
            },
            occurred_at=now,
        )
        await session.flush()
        return await _response(session, bible)


async def prepare_bible_input(
    session: AsyncSession,
    *,
    bible_id: UUID,
    task_id: UUID,
    run_token: UUID,
    lease_expires_at: datetime,
    now: datetime,
) -> ProductionBibleInput:
    if lease_expires_at <= now:
        raise ValueError("Production Bible lease must expire in the future")
    bible = await repository.find_bible(session, bible_id, for_update=True)
    if bible is None or bible.task_id != task_id:
        raise ProductionBibleInputChanged("Production Bible task input is unavailable")
    if bible.status not in {"queued", "running"}:
        raise ProductionBibleInputChanged("Production Bible is no longer runnable")
    if (
        bible.run_token is not None
        and bible.run_token != run_token
        and bible.lease_expires_at is not None
        and bible.lease_expires_at > now
    ):
        raise ProductionBibleLeaseActive("Production Bible run lease is active")
    found = await document_repository.find_revision_with_document(
        session,
        bible.document_revision_id,
    )
    if found is None:
        raise ProductionBibleInputChanged("Document revision is unavailable")
    revision, document = found
    if (
        document.project_id != bible.project_id
        or revision.workspace_id != bible.workspace_id
        or revision.normalized_hash != bible.input_hash
    ):
        raise ProductionBibleInputChanged("Production Bible input has changed")
    bible.status = "running"
    bible.run_token = run_token
    bible.lease_expires_at = lease_expires_at
    bible.revision += 1
    bible.updated_at = now
    await session.flush()
    return ProductionBibleInput(
        bible_id=bible.id,
        task_id=task_id,
        workspace_id=bible.workspace_id,
        project_id=bible.project_id,
        document_revision_id=revision.id,
        input_hash=bible.input_hash,
        normalized_text=revision.normalized_text,
        run_token=run_token,
    )


async def renew_bible_lease(
    session: AsyncSession,
    *,
    bible_id: UUID,
    task_id: UUID,
    run_token: UUID,
    lease_expires_at: datetime,
) -> bool:
    bible = await repository.find_bible(session, bible_id, for_update=True)
    now = datetime.now(UTC)
    if lease_expires_at <= now:
        raise ValueError("Production Bible lease must expire in the future")
    if (
        bible is None
        or bible.task_id != task_id
        or bible.status != "running"
        or bible.run_token != run_token
        or bible.lease_expires_at is None
        or bible.lease_expires_at <= now
    ):
        return False
    bible.lease_expires_at = lease_expires_at
    bible.updated_at = now
    await session.flush()
    return True


async def fence_bible_run(
    session: AsyncSession,
    *,
    bible_id: UUID,
    task_id: UUID,
    run_token: UUID,
) -> bool:
    bible = await repository.find_bible(session, bible_id, for_update=True)
    return bool(
        bible is not None
        and bible.task_id == task_id
        and bible.status == "running"
        and bible.run_token == run_token
        and bible.lease_expires_at is not None
        and bible.lease_expires_at > datetime.now(UTC)
    )


async def record_bible_result(
    session: AsyncSession,
    *,
    bible_id: UUID,
    task_id: UUID,
    run_token: UUID,
    result: ProductionBibleProviderResult | dict[str, object],
) -> str:
    try:
        result = ProductionBibleProviderResult.model_validate(result)
    except ValidationError as error:
        raise ValueError("Production Bible provider result is invalid") from error
    bible = await repository.find_bible(session, bible_id, for_update=True)
    if bible is None or bible.task_id != task_id:
        raise ProductionBibleInputChanged("Production Bible task is unavailable")
    result_hash = _result_hash(result)
    if bible.status == "needs_review" and bible.result_hash == result_hash:
        return result_hash
    if not await fence_bible_run(
        session,
        bible_id=bible_id,
        task_id=task_id,
        run_token=run_token,
    ):
        raise ProductionBibleInputChanged("Production Bible lease was lost")
    if bible.status != "running":
        raise ProductionBibleInputChanged("Production Bible cannot accept provider output")
    now = datetime.now(UTC)
    entity_rows: list[tuple[ProductionBibleEntity, BibleEntityCandidate]] = []
    for candidate in result.entities:
        row = ProductionBibleEntity(
            id=uuid7(),
            workspace_id=bible.workspace_id,
            project_id=bible.project_id,
            bible_id=bible.id,
            entity_key=candidate.entity_key,
            kind=candidate.kind,
            canonical_name=candidate.canonical_name,
            normalized_name=candidate.normalized_name,
            aliases=list(candidate.aliases),
            stable_spec=candidate.stable_spec.to_payload(),
            episode_numbers=list(candidate.episode_numbers),
            evidence=[item.model_dump(mode="json") for item in candidate.evidence],
            asset_id=None,
            created_at=now,
            updated_at=now,
        )
        session.add(row)
        entity_rows.append((row, candidate))
    await session.flush()
    for entity, candidate in entity_rows:
        for state in candidate.states:
            session.add(
                ProductionBibleEntityState(
                    id=uuid7(),
                    workspace_id=bible.workspace_id,
                    project_id=bible.project_id,
                    bible_id=bible.id,
                    entity_id=entity.id,
                    state_key=state.state_key,
                    label=state.label,
                    state_spec=state.state_spec.to_payload(),
                    episode_numbers=list(state.episode_numbers),
                    evidence=[item.model_dump(mode="json") for item in state.evidence],
                    asset_state_id=None,
                    asset_version_id=None,
                    created_at=now,
                    updated_at=now,
                )
            )
    for entry in result.world_entries:
        session.add(
            ProductionBibleWorldEntry(
                id=uuid7(),
                workspace_id=bible.workspace_id,
                project_id=bible.project_id,
                bible_id=bible.id,
                entry_key=entry.entry_key,
                category=entry.category,
                title=entry.title,
                facts=list(entry.facts),
                rules=list(entry.rules),
                entity_keys=list(entry.entity_keys),
                episode_numbers=list(entry.episode_numbers),
                evidence=[item.model_dump(mode="json") for item in entry.evidence],
                created_at=now,
                updated_at=now,
            )
        )
    bible.result_hash = result_hash
    bible.review_issues = [issue.model_dump(mode="json") for issue in result.review_issues]
    bible.status = "needs_review"
    bible.run_token = None
    bible.lease_expires_at = None
    bible.revision += 1
    bible.updated_at = now
    await session.flush()
    return result_hash


async def record_bible_error(
    session: AsyncSession,
    *,
    bible_id: UUID,
    task_id: UUID,
    error_code: str,
    unknown: bool,
) -> None:
    bible = await repository.find_bible(session, bible_id, for_update=True)
    if bible is None or bible.task_id != task_id:
        raise ProductionBibleInputChanged("Production Bible task is unavailable")
    if bible.status in {"needs_review", "confirmed", "cancelled", "superseded"}:
        return
    bible.status = "unknown" if unknown else "failed"
    bible.review_issues = [
        BibleReviewIssue(
            issue_key=f"provider:{error_code}",
            code=error_code,
            severity="blocking",
            scope="global",
            subject_key=None,
            summary="Production Bible provider did not produce a confirmable result.",
            repair_hint="Resume from the latest valid checkpoint or start a new run.",
            evidence=[],
        ).model_dump(mode="json")
    ]
    bible.run_token = None
    bible.lease_expires_at = None
    bible.revision += 1
    bible.updated_at = datetime.now(UTC)
    await session.flush()
