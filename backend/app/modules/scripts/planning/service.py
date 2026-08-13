import hashlib
import json
import re
from datetime import UTC, datetime
from decimal import Decimal
from typing import Literal, cast
from uuid import UUID

from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import Capability, actor_context
from app.modules.production import (
    EpisodePlanningTaskCommand,
    complete_episode_planning_task,
    create_episode_planning_task,
)
from app.modules.projects import (
    EpisodeBatchItem,
    EpisodeBatchMaterializeCommand,
    EpisodeScriptPublishBatchCommand,
    EpisodeScriptPublishItem,
    lock_active_project_for_content_write,
    materialize_episode_batch,
    project_episode_order_snapshot,
    project_for_content_read,
    publish_episode_script_version_batch,
)
from app.modules.projects.contracts import MaterializedEpisodeReference
from app.modules.scripts.documents.schemas import NarrativeBlockResponse
from app.modules.scripts.models import (
    DocumentRevision,
    EpisodePlan,
    EpisodeProposal,
    EpisodeSegmentOrigin,
    ImportCommit,
    NarrativeBlock,
    ScriptDocument,
    ScriptSource,
    ScriptVersion,
)
from app.modules.scripts.planning import repository
from app.modules.scripts.planning.ports import (
    EPISODE_PLANNER_PROMPT_VERSION,
    EPISODE_PLANNER_SCHEMA_VERSION,
    EpisodePlanningInput,
)
from app.modules.scripts.planning.schemas import (
    ConfirmEpisodePlanRequest,
    EpisodePlanCreateRequest,
    EpisodePlanDetailResponse,
    EpisodePlanImpactBlocker,
    EpisodePlanImpactResponse,
    EpisodePlanningProviderResult,
    EpisodePlanResponse,
    EpisodePlanSourceResponse,
    EpisodeProposalResponse,
    EpisodeSegmentOriginResponse,
    ImportCommitDetailResponse,
    ImportCommitResponse,
    MaterializeEpisodePlanRequest,
    MergeEpisodeProposalRequest,
    MoveEpisodeBoundaryRequest,
    PublishImportCommitRequest,
    RenameEpisodeProposalRequest,
    SplitEpisodeProposalRequest,
)

PLANNING_ENGINE_VERSION = "episode-planning-v1"
DEEPSEEK_EPISODE_MODEL = "deepseek-v4-pro"
MAX_EPISODES = 10
MAX_EPISODE_CODEPOINTS = 20_000
_TITLE_LINE = re.compile(r"^《(.{1,118})》$")


def _sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _canonical_hash(value: object) -> str:
    return _sha256(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")))


def _plan_not_found() -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, "Episode plan not found", status_code=404)


def _commit_not_found() -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, "Import commit not found", status_code=404)


def _plan_input_hash(
    revision: DocumentRevision,
    request: EpisodePlanCreateRequest,
) -> str:
    return _canonical_hash(
        {
            "document_revision_id": str(revision.id),
            "normalized_hash": revision.normalized_hash,
            "strategy": request.strategy,
            "target_duration_ms": request.target_duration_ms,
            "requested_episode_count": request.requested_episode_count,
            "engine": PLANNING_ENGINE_VERSION,
            "schema": EPISODE_PLANNER_SCHEMA_VERSION,
        }
    )


def _plan_response(plan: EpisodePlan) -> EpisodePlanResponse:
    return EpisodePlanResponse(
        id=plan.id,
        workspace_id=plan.workspace_id,
        project_id=plan.project_id,
        document_revision_id=plan.document_revision_id,
        strategy=cast(Literal["explicit_markers", "target_duration_ai"], plan.strategy),
        status=cast(
            Literal[
                "draft",
                "review_ready",
                "confirmed",
                "materialized",
                "superseded",
            ],
            plan.status,
        ),
        target_duration_ms=plan.target_duration_ms,
        requested_episode_count=plan.requested_episode_count,
        total_estimated_duration_ms=plan.total_estimated_duration_ms,
        input_hash=plan.input_hash,
        planning_engine_version=plan.planning_engine_version,
        model_name=plan.model_name,
        prompt_version=plan.prompt_version,
        schema_version=plan.schema_version,
        planning_task_id=plan.planning_task_id,
        planning_error_code=plan.planning_error_code,
        revision=plan.revision,
        confirmed_by=plan.confirmed_by,
        confirmed_at=plan.confirmed_at,
        created_by=plan.created_by,
        created_at=plan.created_at,
        updated_at=plan.updated_at,
    )


def _proposal_response(proposal: EpisodeProposal) -> EpisodeProposalResponse:
    return EpisodeProposalResponse(
        id=proposal.id,
        plan_id=proposal.plan_id,
        position=proposal.position,
        title=proposal.title,
        start_block_id=proposal.start_block_id,
        end_block_id=proposal.end_block_id,
        start_block_position=proposal.start_block_position,
        end_block_position=proposal.end_block_position,
        source_start=proposal.source_start,
        source_end=proposal.source_end,
        content_hash=proposal.content_hash,
        estimated_duration_ms=proposal.estimated_duration_ms,
        reason=proposal.reason,
        confidence=float(proposal.confidence),
        boundary_evidence=proposal.boundary_evidence,
        is_locked=proposal.is_locked,
    )


def _block_response(block: NarrativeBlock) -> NarrativeBlockResponse:
    return NarrativeBlockResponse(
        id=block.id,
        document_revision_id=block.document_revision_id,
        position=block.position,
        kind=cast(
            Literal[
                "preamble",
                "episode_marker",
                "scene_heading",
                "dialogue",
                "narration",
                "action",
                "separator",
            ],
            block.kind,
        ),
        source_start=block.source_start,
        source_end=block.source_end,
        text_hash=block.text_hash,
        metadata=block.block_metadata,
    )


def _estimated_duration_ms(body: str) -> int:
    effective = sum(1 for character in body if not character.isspace())
    return max(1_000, round(effective / 4.5 * 1_000))


def _title_for_segment(
    text: str,
    blocks: list[NarrativeBlock],
    marker: NarrativeBlock,
    fallback_number: int,
) -> str:
    for block in blocks:
        if block.position <= marker.position or block.kind == "separator":
            continue
        value = text[block.source_start : block.source_end].strip()
        matched = _TITLE_LINE.fullmatch(value)
        if matched is not None:
            return matched.group(1)
        break
    return f"第{fallback_number}集"


def _new_proposal(
    *,
    plan: EpisodePlan,
    position: int,
    title: str,
    start_block: NarrativeBlock,
    end_block: NarrativeBlock,
    source_text: str,
    reason: str,
    confidence: Decimal,
    evidence: dict[str, object],
    now: datetime,
) -> EpisodeProposal:
    body = source_text[start_block.source_start : end_block.source_end]
    return EpisodeProposal(
        id=uuid7(),
        workspace_id=plan.workspace_id,
        plan_id=plan.id,
        position=position,
        title=title.strip(),
        start_block_id=start_block.id,
        end_block_id=end_block.id,
        start_block_position=start_block.position,
        end_block_position=end_block.position,
        source_start=start_block.source_start,
        source_end=end_block.source_end,
        content_hash=_sha256(body),
        estimated_duration_ms=_estimated_duration_ms(body),
        reason=reason,
        confidence=confidence,
        boundary_evidence=evidence,
        is_locked=False,
        created_at=now,
    )


def _explicit_proposals(
    plan: EpisodePlan,
    revision: DocumentRevision,
    blocks: list[NarrativeBlock],
    now: datetime,
) -> list[EpisodeProposal]:
    markers = [block for block in blocks if block.kind == "episode_marker"]
    if not markers:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Document has no explicit episode markers",
            status_code=409,
            next_action="choose_ai_episode_plan",
        )
    proposals: list[EpisodeProposal] = []
    index_by_position = {block.position: index for index, block in enumerate(blocks)}
    for index, marker in enumerate(markers):
        start_index = 0 if index == 0 else index_by_position[marker.position]
        end_index = (
            index_by_position[markers[index + 1].position] - 1
            if index + 1 < len(markers)
            else len(blocks) - 1
        )
        episode_number = int(marker.block_metadata.get("episode_number", index + 1))
        marker_text = revision.normalized_text[marker.source_start : marker.source_end].strip()
        proposals.append(
            _new_proposal(
                plan=plan,
                position=index + 1,
                title=_title_for_segment(
                    revision.normalized_text,
                    blocks[start_index : end_index + 1],
                    marker,
                    episode_number,
                ),
                start_block=blocks[start_index],
                end_block=blocks[end_index],
                source_text=revision.normalized_text,
                reason=f"依据独占一行的第 {episode_number} 集标记确定边界",
                confidence=Decimal("1.0"),
                evidence={
                    "kind": "explicit_marker",
                    "episode_number": episode_number,
                    "marker_text": marker_text,
                    "marker_block_id": str(marker.id),
                    "source_start": marker.source_start,
                    "source_end": marker.source_end,
                },
                now=now,
            )
        )
    _require_conservation(revision, blocks, proposals)
    return proposals


def _require_conservation(
    revision: DocumentRevision,
    blocks: list[NarrativeBlock],
    proposals: list[EpisodeProposal],
) -> None:
    if not blocks or not proposals:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Episode plan does not cover the source",
            status_code=422,
        )
    if proposals[0].source_start != 0 or proposals[-1].source_end != len(revision.normalized_text):
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Episode plan source coverage is incomplete",
            status_code=422,
        )
    for index, proposal in enumerate(proposals):
        if proposal.position != index + 1:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Episode proposal order is invalid",
                status_code=422,
            )
        body = revision.normalized_text[proposal.source_start : proposal.source_end]
        if not body or proposal.content_hash != _sha256(body):
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Episode proposal source hash is invalid",
                status_code=422,
            )
        if index > 0 and proposals[index - 1].source_end != proposal.source_start:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Episode plan has a gap or overlap",
                status_code=422,
            )
    reconstructed = "".join(
        revision.normalized_text[item.source_start : item.source_end] for item in proposals
    )
    if reconstructed != revision.normalized_text:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Episode plan did not preserve the source",
            status_code=422,
        )


def _provider_proposals(
    plan: EpisodePlan,
    revision: DocumentRevision,
    blocks: list[NarrativeBlock],
    result: EpisodePlanningProviderResult,
    now: datetime,
) -> list[EpisodeProposal]:
    if len(result.proposals) > MAX_EPISODES:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "AI episode plan exceeds the episode limit",
            status_code=422,
        )
    previous_end_position = 0
    proposals: list[EpisodeProposal] = []
    block_by_position = {block.position: block for block in blocks}
    for position, candidate in enumerate(result.proposals, start=1):
        if candidate.end_block_position <= previous_end_position:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "AI episode boundaries are not monotonic",
                status_code=422,
            )
        end_block = block_by_position.get(candidate.end_block_position)
        start_block = block_by_position.get(previous_end_position + 1)
        if end_block is None or start_block is None:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "AI episode boundary does not reference a narrative block",
                status_code=422,
            )
        anchor = candidate.exact_end_anchor
        if revision.normalized_text.count(anchor) != 1:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "AI episode boundary anchor is not unique",
                status_code=422,
                next_action="edit_episode_boundary",
            )
        block_body = revision.normalized_text[end_block.source_start : end_block.source_end]
        logical_block_body = block_body.rstrip("\r\n")
        if block_body.endswith(anchor):
            anchor_end = end_block.source_end
        elif logical_block_body.endswith(anchor):
            anchor_end = end_block.source_start + len(logical_block_body)
        else:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "AI episode anchor does not end at the declared block",
                status_code=422,
                next_action="edit_episode_boundary",
            )
        anchor_start = anchor_end - len(anchor)
        proposal = _new_proposal(
            plan=plan,
            position=position,
            title=candidate.title,
            start_block=start_block,
            end_block=end_block,
            source_text=revision.normalized_text,
            reason=candidate.reason,
            confidence=Decimal(str(candidate.confidence)),
            evidence={
                "kind": "ai_anchor",
                "exact_end_anchor": anchor,
                "anchor_source_start": anchor_start,
                "anchor_source_end": anchor_end,
                "prefix": revision.normalized_text[max(0, anchor_start - 32) : anchor_start],
                "suffix": revision.normalized_text[anchor_end : anchor_end + 32],
                "end_block_id": str(end_block.id),
                "reason": candidate.reason,
            },
            now=now,
        )
        proposal.estimated_duration_ms = candidate.estimated_duration_ms
        proposals.append(proposal)
        previous_end_position = candidate.end_block_position
    if previous_end_position != blocks[-1].position:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "AI episode plan does not cover the final narrative block",
            status_code=422,
        )
    _require_conservation(revision, blocks, proposals)
    return proposals


async def _detail_response(
    session: AsyncSession,
    claims: AccessTokenClaims,
    plan: EpisodePlan,
) -> EpisodePlanDetailResponse:
    found = await repository.find_revision_document(session, plan.document_revision_id)
    if found is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR, "Document revision is unavailable", status_code=500
        )
    revision, _ = found
    proposals = await repository.list_proposals(session, plan.id)
    blocks = await repository.list_blocks(session, plan.document_revision_id)
    snapshot = await project_episode_order_snapshot(session, claims, plan.project_id)
    projected = snapshot.active_episode_count + len(proposals)
    blockers: list[EpisodePlanImpactBlocker] = []
    if projected > MAX_EPISODES:
        blockers.append(
            EpisodePlanImpactBlocker(
                code="EPISODE_LIMIT_EXCEEDED",
                summary=f"当前 {snapshot.active_episode_count} 集，加上计划后将达到 {projected} 集",
                next_action="reduce_episode_count",
            )
        )
    for proposal in proposals:
        if proposal.source_end - proposal.source_start > MAX_EPISODE_CODEPOINTS:
            blockers.append(
                EpisodePlanImpactBlocker(
                    code="EPISODE_TEXT_TOO_LONG",
                    summary=f"第 {proposal.position} 集超过 20,000 字符",
                    next_action="move_episode_boundary",
                )
            )
    return EpisodePlanDetailResponse(
        plan=_plan_response(plan),
        proposals=[_proposal_response(item) for item in proposals],
        impact=EpisodePlanImpactResponse(
            project_revision=snapshot.project_revision,
            active_episode_count=snapshot.active_episode_count,
            active_order_hash=snapshot.active_order_hash,
            projected_episode_count=projected,
            allowed=not blockers and bool(proposals),
            blockers=blockers,
        ),
        source=EpisodePlanSourceResponse(
            document_revision_id=revision.id,
            normalized_text=revision.normalized_text,
            normalized_hash=revision.normalized_hash,
            codepoint_count=revision.codepoint_count,
            blocks=[_block_response(block) for block in blocks],
        ),
    )


async def create_plan(
    session: AsyncSession,
    claims: AccessTokenClaims,
    revision_id: UUID,
    request: EpisodePlanCreateRequest,
    *,
    trace_id: str,
) -> EpisodePlanDetailResponse:
    created_plan: EpisodePlan | None = None
    proposals: list[EpisodeProposal] = []
    async with session.begin():
        found = await repository.find_revision_document(session, revision_id)
        if found is None:
            raise ApiError(ErrorCode.NOT_FOUND, "Document revision not found", status_code=404)
        revision, document = found
        project = await lock_active_project_for_content_write(session, claims, document.project_id)
        if project.workspace_id != revision.workspace_id:
            raise ApiError(ErrorCode.NOT_FOUND, "Document revision not found", status_code=404)
        input_hash = _plan_input_hash(revision, request)
        existing = await repository.find_plan_by_idempotency(
            session, project.project_id, request.idempotency_key
        )
        if existing is not None:
            if existing.input_hash != input_hash:
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            created_plan = existing
        else:
            blocking = await repository.list_blocking_issues(session, revision.id)
            if blocking:
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Document has blocking format issues",
                    status_code=409,
                    next_action=blocking[0].next_action,
                    details={"issue_codes": [item.code for item in blocking]},
                )
            if (
                request.strategy == "explicit_markers"
                and revision.analysis_status != "deterministic"
            ):
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Explicit markers are not available",
                    status_code=409,
                    next_action="choose_ai_episode_plan",
                )
            if (
                request.strategy == "target_duration_ai"
                and revision.analysis_status != "ai_candidate_required"
            ):
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "AI planning is only available for unmarked documents",
                    status_code=409,
                    next_action="use_explicit_markers",
                )
            now = datetime.now(UTC)
            created_plan = EpisodePlan(
                id=uuid7(),
                workspace_id=revision.workspace_id,
                project_id=project.project_id,
                document_revision_id=revision.id,
                strategy=request.strategy,
                status="draft" if request.strategy == "target_duration_ai" else "review_ready",
                target_duration_ms=request.target_duration_ms,
                requested_episode_count=request.requested_episode_count,
                total_estimated_duration_ms=0,
                input_hash=input_hash,
                planning_engine_version=PLANNING_ENGINE_VERSION,
                model_name=(
                    DEEPSEEK_EPISODE_MODEL if request.strategy == "target_duration_ai" else None
                ),
                prompt_version=(
                    EPISODE_PLANNER_PROMPT_VERSION
                    if request.strategy == "target_duration_ai"
                    else None
                ),
                schema_version=EPISODE_PLANNER_SCHEMA_VERSION,
                command_receipts={},
                revision=1,
                idempotency_key=request.idempotency_key,
                created_by=claims.sub,
                created_at=now,
                updated_at=now,
            )
            session.add(created_plan)
            await session.flush()
            blocks = await repository.list_blocks(session, revision.id)
            if request.strategy == "explicit_markers":
                proposals = _explicit_proposals(created_plan, revision, blocks, now)
                created_plan.total_estimated_duration_ms = sum(
                    item.estimated_duration_ms for item in proposals
                )
                session.add_all(proposals)
            else:
                actor = await actor_context(
                    session,
                    claims,
                    revision.workspace_id,
                    Capability.CONTENT_WRITE,
                )
                task = await create_episode_planning_task(
                    session,
                    actor,
                    EpisodePlanningTaskCommand(
                        workspace_id=revision.workspace_id,
                        plan_id=created_plan.id,
                        document_revision_id=revision.id,
                        input_hash=input_hash,
                        idempotency_key=_sha256(
                            f"episode-plan:{created_plan.id}:{request.idempotency_key}"
                        ),
                    ),
                    trace_id=trace_id,
                )
                created_plan.planning_task_id = task.id
            append_audit_event(
                session,
                workspace_id=revision.workspace_id,
                actor_id=claims.sub,
                action="script.episode_plan_created",
                target_type="episode_plan",
                target_id=created_plan.id,
                trace_id=trace_id,
                metadata={
                    "project_id": str(project.project_id),
                    "document_revision_id": str(revision.id),
                    "strategy": created_plan.strategy,
                    "status": created_plan.status,
                    "proposal_count": len(proposals)
                    if request.strategy == "explicit_markers"
                    else 0,
                },
                occurred_at=now,
            )
            await session.flush()
    assert created_plan is not None
    return await _detail_response(session, claims, created_plan)


async def get_plan(
    session: AsyncSession,
    claims: AccessTokenClaims,
    plan_id: UUID,
) -> EpisodePlanDetailResponse:
    plan = await repository.find_plan(session, plan_id)
    if plan is None:
        raise _plan_not_found()
    try:
        await project_for_content_read(session, claims, plan.project_id)
    except ApiError as error:
        if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
            raise _plan_not_found() from error
        raise
    return await _detail_response(session, claims, plan)


def _command_input_hash(command_type: str, request: BaseModel) -> str:
    return _canonical_hash(
        {
            "command_type": command_type,
            "request": request.model_dump(mode="json", exclude={"idempotency_key"}),
        }
    )


def _check_command_receipt(
    plan: EpisodePlan,
    *,
    idempotency_key: str,
    input_hash: str,
) -> bool:
    key_hash = _sha256(idempotency_key)
    existing = plan.command_receipts.get(key_hash)
    if existing is None:
        return False
    if not isinstance(existing, dict):
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Idempotency receipt is invalid",
            status_code=409,
        )
    receipt = cast(dict[str, object], existing)
    if receipt.get("input_hash") != input_hash:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Idempotency key was used with different input",
            status_code=409,
        )
    return True


def _record_command_receipt(
    plan: EpisodePlan,
    *,
    idempotency_key: str,
    input_hash: str,
) -> None:
    receipts = dict(plan.command_receipts)
    receipts[_sha256(idempotency_key)] = {
        "input_hash": input_hash,
        "result_revision": plan.revision,
    }
    plan.command_receipts = receipts


def _require_editable(plan: EpisodePlan, expected_revision: int) -> None:
    if plan.status != "review_ready":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Episode plan is not editable",
            status_code=409,
            next_action="create_new_episode_plan",
        )
    if plan.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Episode plan has changed",
            status_code=409,
            details={"current_revision": plan.revision},
        )


async def _locked_plan_for_write(
    session: AsyncSession,
    claims: AccessTokenClaims,
    plan_id: UUID,
) -> EpisodePlan:
    initial = await repository.find_plan(session, plan_id)
    if initial is None:
        raise _plan_not_found()
    try:
        await lock_active_project_for_content_write(session, claims, initial.project_id)
    except ApiError as error:
        if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
            raise _plan_not_found() from error
        raise
    plan = await repository.find_plan(session, plan_id, for_update=True)
    if plan is None or plan.workspace_id != initial.workspace_id:
        raise _plan_not_found()
    return plan


def _block_start_at(
    blocks: list[NarrativeBlock],
    source_offset: int,
) -> tuple[int, NarrativeBlock]:
    for index, block in enumerate(blocks):
        if block.source_start == source_offset:
            return index, block
    raise ApiError(
        ErrorCode.VALIDATION_FAILED,
        "Boundary must align with a narrative block",
        status_code=422,
        next_action="choose_block_boundary",
    )


def _set_proposal_range(
    proposal: EpisodeProposal,
    start_block: NarrativeBlock,
    end_block: NarrativeBlock,
    revision: DocumentRevision,
    *,
    evidence_kind: str,
) -> None:
    body = revision.normalized_text[start_block.source_start : end_block.source_end]
    proposal.start_block_id = start_block.id
    proposal.end_block_id = end_block.id
    proposal.start_block_position = start_block.position
    proposal.end_block_position = end_block.position
    proposal.source_start = start_block.source_start
    proposal.source_end = end_block.source_end
    proposal.content_hash = _sha256(body)
    proposal.estimated_duration_ms = _estimated_duration_ms(body)
    proposal.confidence = Decimal("1.0")
    proposal.boundary_evidence = {
        "kind": evidence_kind,
        "source_start": start_block.source_start,
        "source_end": end_block.source_end,
        "start_block_id": str(start_block.id),
        "end_block_id": str(end_block.id),
    }


async def move_boundary(
    session: AsyncSession,
    claims: AccessTokenClaims,
    plan_id: UUID,
    request: MoveEpisodeBoundaryRequest,
    *,
    trace_id: str,
) -> EpisodePlanDetailResponse:
    async with session.begin():
        plan = await _locked_plan_for_write(session, claims, plan_id)
        input_hash = _command_input_hash("move_boundary", request)
        if not _check_command_receipt(
            plan, idempotency_key=request.idempotency_key, input_hash=input_hash
        ):
            _require_editable(plan, request.expected_revision)
            proposals = await repository.list_proposals(session, plan.id, for_update=True)
            left_index = next(
                (
                    index
                    for index, item in enumerate(proposals)
                    if item.id == request.left_proposal_id
                ),
                -1,
            )
            if left_index < 0 or left_index + 1 >= len(proposals):
                raise ApiError(
                    ErrorCode.INVALID_REQUEST,
                    "Boundary does not have two adjacent proposals",
                    status_code=422,
                )
            found = await repository.find_revision_document(session, plan.document_revision_id)
            if found is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR, "Document revision is unavailable", status_code=500
                )
            revision, _ = found
            blocks = await repository.list_blocks(session, revision.id)
            block_index, right_start = _block_start_at(blocks, request.source_offset)
            left = proposals[left_index]
            right = proposals[left_index + 1]
            if not (left.source_start < request.source_offset < right.source_end):
                raise ApiError(
                    ErrorCode.VALIDATION_FAILED,
                    "Boundary would create an empty episode",
                    status_code=422,
                )
            _set_proposal_range(
                left,
                blocks[left.start_block_position - 1],
                blocks[block_index - 1],
                revision,
                evidence_kind="manual_boundary",
            )
            _set_proposal_range(
                right,
                right_start,
                blocks[right.end_block_position - 1],
                revision,
                evidence_kind="manual_boundary",
            )
            _require_conservation(revision, blocks, proposals)
            plan.total_estimated_duration_ms = sum(item.estimated_duration_ms for item in proposals)
            plan.revision += 1
            plan.updated_at = datetime.now(UTC)
            _record_command_receipt(
                plan, idempotency_key=request.idempotency_key, input_hash=input_hash
            )
            _append_plan_command_audit(
                session, plan, claims, "script.episode_boundary_moved", trace_id
            )
            await session.flush()
    return await _detail_response(session, claims, plan)


async def _resequence_proposals(
    session: AsyncSession,
    proposals: list[EpisodeProposal],
) -> None:
    for index, proposal in enumerate(proposals, start=1):
        proposal.position = 100 + index
    await session.flush()
    for index, proposal in enumerate(proposals, start=1):
        proposal.position = index


async def split_proposal(
    session: AsyncSession,
    claims: AccessTokenClaims,
    plan_id: UUID,
    request: SplitEpisodeProposalRequest,
    *,
    trace_id: str,
) -> EpisodePlanDetailResponse:
    async with session.begin():
        plan = await _locked_plan_for_write(session, claims, plan_id)
        input_hash = _command_input_hash("split", request)
        if not _check_command_receipt(
            plan, idempotency_key=request.idempotency_key, input_hash=input_hash
        ):
            _require_editable(plan, request.expected_revision)
            proposals = await repository.list_proposals(session, plan.id, for_update=True)
            if len(proposals) >= MAX_EPISODES:
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Episode limit would be exceeded",
                    status_code=409,
                    next_action="merge_episodes",
                )
            target_index = next(
                (index for index, item in enumerate(proposals) if item.id == request.proposal_id),
                -1,
            )
            if target_index < 0:
                raise ApiError(
                    ErrorCode.INVALID_REQUEST, "Episode proposal not found", status_code=422
                )
            found = await repository.find_revision_document(session, plan.document_revision_id)
            if found is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR, "Document revision is unavailable", status_code=500
                )
            revision, _ = found
            blocks = await repository.list_blocks(session, revision.id)
            block_index, right_start = _block_start_at(blocks, request.source_offset)
            target = proposals[target_index]
            if not (target.source_start < request.source_offset < target.source_end):
                raise ApiError(
                    ErrorCode.VALIDATION_FAILED,
                    "Split would create an empty episode",
                    status_code=422,
                )
            original_end = blocks[target.end_block_position - 1]
            new_right = _new_proposal(
                plan=plan,
                position=target.position + 1,
                title=request.new_title,
                start_block=right_start,
                end_block=original_end,
                source_text=revision.normalized_text,
                reason="人工拆分分集边界",
                confidence=Decimal("1.0"),
                evidence={"kind": "manual_split", "source_offset": request.source_offset},
                now=datetime.now(UTC),
            )
            _set_proposal_range(
                target,
                blocks[target.start_block_position - 1],
                blocks[block_index - 1],
                revision,
                evidence_kind="manual_split",
            )
            proposals.insert(target_index + 1, new_right)
            session.add(new_right)
            await _resequence_proposals(session, proposals)
            _require_conservation(revision, blocks, proposals)
            plan.total_estimated_duration_ms = sum(item.estimated_duration_ms for item in proposals)
            plan.revision += 1
            plan.updated_at = datetime.now(UTC)
            _record_command_receipt(
                plan, idempotency_key=request.idempotency_key, input_hash=input_hash
            )
            _append_plan_command_audit(session, plan, claims, "script.episode_split", trace_id)
            await session.flush()
    return await _detail_response(session, claims, plan)


async def merge_proposals(
    session: AsyncSession,
    claims: AccessTokenClaims,
    plan_id: UUID,
    request: MergeEpisodeProposalRequest,
    *,
    trace_id: str,
) -> EpisodePlanDetailResponse:
    async with session.begin():
        plan = await _locked_plan_for_write(session, claims, plan_id)
        input_hash = _command_input_hash("merge", request)
        if not _check_command_receipt(
            plan, idempotency_key=request.idempotency_key, input_hash=input_hash
        ):
            _require_editable(plan, request.expected_revision)
            proposals = await repository.list_proposals(session, plan.id, for_update=True)
            left_index = next(
                (
                    index
                    for index, item in enumerate(proposals)
                    if item.id == request.left_proposal_id
                ),
                -1,
            )
            if left_index < 0 or left_index + 1 >= len(proposals):
                raise ApiError(
                    ErrorCode.INVALID_REQUEST,
                    "Merge requires two adjacent proposals",
                    status_code=422,
                )
            found = await repository.find_revision_document(session, plan.document_revision_id)
            if found is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR, "Document revision is unavailable", status_code=500
                )
            revision, _ = found
            blocks = await repository.list_blocks(session, revision.id)
            left = proposals[left_index]
            right = proposals.pop(left_index + 1)
            _set_proposal_range(
                left,
                blocks[left.start_block_position - 1],
                blocks[right.end_block_position - 1],
                revision,
                evidence_kind="manual_merge",
            )
            await session.delete(right)
            await session.flush()
            await _resequence_proposals(session, proposals)
            _require_conservation(revision, blocks, proposals)
            plan.total_estimated_duration_ms = sum(item.estimated_duration_ms for item in proposals)
            plan.revision += 1
            plan.updated_at = datetime.now(UTC)
            _record_command_receipt(
                plan, idempotency_key=request.idempotency_key, input_hash=input_hash
            )
            _append_plan_command_audit(session, plan, claims, "script.episode_merged", trace_id)
            await session.flush()
    return await _detail_response(session, claims, plan)


async def rename_proposal(
    session: AsyncSession,
    claims: AccessTokenClaims,
    plan_id: UUID,
    request: RenameEpisodeProposalRequest,
    *,
    trace_id: str,
) -> EpisodePlanDetailResponse:
    async with session.begin():
        plan = await _locked_plan_for_write(session, claims, plan_id)
        input_hash = _command_input_hash("rename", request)
        if not _check_command_receipt(
            plan, idempotency_key=request.idempotency_key, input_hash=input_hash
        ):
            _require_editable(plan, request.expected_revision)
            proposals = await repository.list_proposals(session, plan.id, for_update=True)
            proposal = next((item for item in proposals if item.id == request.proposal_id), None)
            if proposal is None:
                raise ApiError(
                    ErrorCode.INVALID_REQUEST, "Episode proposal not found", status_code=422
                )
            proposal.title = request.title
            plan.revision += 1
            plan.updated_at = datetime.now(UTC)
            _record_command_receipt(
                plan, idempotency_key=request.idempotency_key, input_hash=input_hash
            )
            _append_plan_command_audit(session, plan, claims, "script.episode_renamed", trace_id)
            await session.flush()
    return await _detail_response(session, claims, plan)


def _append_plan_command_audit(
    session: AsyncSession,
    plan: EpisodePlan,
    claims: AccessTokenClaims,
    action: str,
    trace_id: str,
) -> None:
    append_audit_event(
        session,
        workspace_id=plan.workspace_id,
        actor_id=claims.sub,
        action=action,
        target_type="episode_plan",
        target_id=plan.id,
        trace_id=trace_id,
        metadata={"revision": plan.revision, "status": plan.status},
        occurred_at=datetime.now(UTC),
    )


async def confirm_plan(
    session: AsyncSession,
    claims: AccessTokenClaims,
    plan_id: UUID,
    request: ConfirmEpisodePlanRequest,
    *,
    trace_id: str,
) -> EpisodePlanDetailResponse:
    async with session.begin():
        plan = await _locked_plan_for_write(session, claims, plan_id)
        input_hash = _command_input_hash("confirm", request)
        if not _check_command_receipt(
            plan, idempotency_key=request.idempotency_key, input_hash=input_hash
        ):
            _require_editable(plan, request.expected_revision)
            found = await repository.find_revision_document(session, plan.document_revision_id)
            if found is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR, "Document revision is unavailable", status_code=500
                )
            revision, _ = found
            blocks = await repository.list_blocks(session, revision.id)
            proposals = await repository.list_proposals(session, plan.id, for_update=True)
            _require_conservation(revision, blocks, proposals)
            if any(
                proposal.source_end - proposal.source_start > MAX_EPISODE_CODEPOINTS
                for proposal in proposals
            ):
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "An episode exceeds the text limit",
                    status_code=409,
                    next_action="move_episode_boundary",
                )
            plan.status = "confirmed"
            plan.confirmed_by = claims.sub
            confirmed_at = datetime.now(UTC)
            plan.confirmed_at = confirmed_at
            plan.revision += 1
            plan.updated_at = confirmed_at
            _record_command_receipt(
                plan, idempotency_key=request.idempotency_key, input_hash=input_hash
            )
            _append_plan_command_audit(
                session, plan, claims, "script.episode_plan_confirmed", trace_id
            )
            await session.flush()
    return await _detail_response(session, claims, plan)


def _materialize_input_hash(
    plan: EpisodePlan,
    request: MaterializeEpisodePlanRequest,
) -> str:
    return _canonical_hash(
        {
            "plan_id": str(plan.id),
            "mode": request.mode,
            "expected_plan_revision": request.expected_plan_revision,
            "expected_project_revision": request.expected_project_revision,
            "expected_active_order_hash": request.expected_active_order_hash,
        }
    )


async def _write_materialized_segment(
    session: AsyncSession,
    *,
    plan: EpisodePlan,
    proposal: EpisodeProposal,
    episode: MaterializedEpisodeReference,
    commit: ImportCommit,
    document: ScriptDocument,
    revision: DocumentRevision,
    now: datetime,
) -> EpisodeSegmentOrigin:
    body = revision.normalized_text[proposal.source_start : proposal.source_end]
    source = ScriptSource(
        id=uuid7(),
        workspace_id=plan.workspace_id,
        episode_id=episode.episode_id,
        input_type="text",
        title=proposal.title,
        source_media_version_id=None,
        rights_declaration=document.rights_declaration,
        status="active",
        revision=1,
        idempotency_key=f"episode-plan:{commit.id}:{proposal.id}",
        created_at=now,
        updated_at=now,
    )
    version = ScriptVersion(
        id=uuid7(),
        workspace_id=plan.workspace_id,
        source_id=source.id,
        version_no=1,
        status="draft",
        body=body,
        content_hash=_sha256(body),
        structure_summary={
            "origin": "episode_plan",
            "document_revision_id": str(revision.id),
            "proposal_id": str(proposal.id),
            "source_start": proposal.source_start,
            "source_end": proposal.source_end,
        },
        created_by=commit.created_by,
        created_at=now,
    )
    origin = EpisodeSegmentOrigin(
        id=uuid7(),
        workspace_id=plan.workspace_id,
        import_commit_id=commit.id,
        proposal_id=proposal.id,
        document_revision_id=revision.id,
        episode_id=episode.episode_id,
        source_id=source.id,
        draft_version_id=version.id,
        published_version_id=None,
        position=proposal.position,
        source_start=proposal.source_start,
        source_end=proposal.source_end,
        source_hash=version.content_hash,
        created_at=now,
    )
    session.add(source)
    await session.flush()
    session.add(version)
    await session.flush()
    session.add(origin)
    await session.flush()
    return origin


def _commit_response(commit: ImportCommit) -> ImportCommitResponse:
    return ImportCommitResponse(
        id=commit.id,
        workspace_id=commit.workspace_id,
        project_id=commit.project_id,
        plan_id=commit.plan_id,
        mode="append_new",
        status=cast(
            Literal[
                "pending",
                "materializing",
                "materialized",
                "publishing",
                "published",
                "conflict",
                "failed",
            ],
            commit.status,
        ),
        input_hash=commit.input_hash,
        expected_project_revision=commit.expected_project_revision,
        expected_active_order_hash=commit.expected_active_order_hash,
        error_code=commit.error_code,
        revision=commit.revision,
        created_by=commit.created_by,
        created_at=commit.created_at,
        updated_at=commit.updated_at,
    )


def _origin_response(origin: EpisodeSegmentOrigin) -> EpisodeSegmentOriginResponse:
    return EpisodeSegmentOriginResponse(
        id=origin.id,
        import_commit_id=origin.import_commit_id,
        proposal_id=origin.proposal_id,
        document_revision_id=origin.document_revision_id,
        episode_id=origin.episode_id,
        source_id=origin.source_id,
        draft_version_id=origin.draft_version_id,
        published_version_id=origin.published_version_id,
        position=origin.position,
        source_start=origin.source_start,
        source_end=origin.source_end,
        source_hash=origin.source_hash,
    )


async def _commit_detail(
    session: AsyncSession,
    commit: ImportCommit,
) -> ImportCommitDetailResponse:
    origins = await repository.list_segment_origins(session, commit.id)
    return ImportCommitDetailResponse(
        commit=_commit_response(commit),
        segments=[_origin_response(item) for item in origins],
    )


async def materialize_plan(
    session: AsyncSession,
    claims: AccessTokenClaims,
    plan_id: UUID,
    request: MaterializeEpisodePlanRequest,
    *,
    trace_id: str,
) -> ImportCommitDetailResponse:
    failure: ApiError | None = None
    result: ImportCommitDetailResponse | None = None
    async with session.begin():
        plan = await _locked_plan_for_write(session, claims, plan_id)
        input_hash = _materialize_input_hash(plan, request)
        existing = await repository.find_import_commit_by_idempotency(
            session,
            plan.workspace_id,
            request.idempotency_key,
            for_update=True,
        )
        if existing is not None:
            if existing.plan_id != plan.id or existing.input_hash != input_hash:
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            result = await _commit_detail(session, existing)
        else:
            if plan.status != "confirmed":
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Episode plan must be confirmed before materialization",
                    status_code=409,
                    next_action="confirm_episode_plan",
                )
            if plan.revision != request.expected_plan_revision:
                raise ApiError(
                    ErrorCode.VERSION_CONFLICT,
                    "Episode plan has changed",
                    status_code=409,
                    details={"current_revision": plan.revision},
                )
            found = await repository.find_revision_document(session, plan.document_revision_id)
            if found is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR, "Document revision is unavailable", status_code=500
                )
            revision, document = found
            proposals = await repository.list_proposals(session, plan.id, for_update=True)
            blocks = await repository.list_blocks(session, revision.id)
            _require_conservation(revision, blocks, proposals)
            if len(proposals) > MAX_EPISODES or any(
                item.source_end - item.source_start > MAX_EPISODE_CODEPOINTS for item in proposals
            ):
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Episode plan exceeds MVP limits",
                    status_code=409,
                    next_action="edit_episode_plan",
                )
            now = datetime.now(UTC)
            commit = ImportCommit(
                id=uuid7(),
                workspace_id=plan.workspace_id,
                project_id=plan.project_id,
                plan_id=plan.id,
                mode=request.mode,
                status="pending",
                input_hash=input_hash,
                expected_project_revision=request.expected_project_revision,
                expected_active_order_hash=request.expected_active_order_hash,
                result_snapshot={},
                revision=1,
                idempotency_key=request.idempotency_key,
                created_by=claims.sub,
                created_at=now,
                updated_at=now,
            )
            session.add(commit)
            await session.flush()
            try:
                async with session.begin_nested():
                    commit.status = "materializing"
                    batch = await materialize_episode_batch(
                        session,
                        claims,
                        EpisodeBatchMaterializeCommand(
                            project_id=plan.project_id,
                            expected_project_revision=request.expected_project_revision,
                            expected_active_order_hash=request.expected_active_order_hash,
                            items=[
                                EpisodeBatchItem(
                                    client_reference_id=item.id,
                                    name=item.title,
                                    target_duration_ms=plan.target_duration_ms,
                                )
                                for item in proposals
                            ],
                        ),
                        trace_id=trace_id,
                    )
                    episode_by_proposal = {item.client_reference_id: item for item in batch.items}
                    for proposal in proposals:
                        await _write_materialized_segment(
                            session,
                            plan=plan,
                            proposal=proposal,
                            episode=episode_by_proposal[proposal.id],
                            commit=commit,
                            document=document,
                            revision=revision,
                            now=now,
                        )
                    commit.result_snapshot = {
                        "project_revision": batch.project_revision,
                        "active_order_hash": batch.active_order_hash,
                        "episodes": [
                            {
                                "proposal_id": str(item.client_reference_id),
                                "episode_id": str(item.episode_id),
                                "revision": item.revision,
                                "position": item.position,
                                "current_script_version_id": None,
                            }
                            for item in batch.items
                        ],
                    }
            except ApiError as error:
                await session.refresh(commit)
                commit.status = "conflict"
                commit.error_code = str(error.code)
                failure = error
            except Exception:
                await session.refresh(commit)
                commit.status = "failed"
                commit.error_code = "materialization_failed"
                failure = ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Episode materialization failed",
                    status_code=500,
                    next_action="retry_with_new_idempotency_key",
                )
            else:
                commit.status = "materialized"
                commit.error_code = None
                plan.status = "materialized"
                plan.revision += 1
                plan.updated_at = now
            commit.revision += 1
            commit.updated_at = now
            append_audit_event(
                session,
                workspace_id=plan.workspace_id,
                actor_id=claims.sub,
                action=(
                    "script.episode_plan_materialized"
                    if failure is None
                    else "script.episode_plan_materialization_failed"
                ),
                target_type="import_commit",
                target_id=commit.id,
                trace_id=trace_id,
                metadata={
                    "plan_id": str(plan.id),
                    "status": commit.status,
                    "revision": commit.revision,
                    "episode_count": len(proposals),
                    "error_code": commit.error_code,
                },
                occurred_at=now,
            )
            await session.flush()
            result = await _commit_detail(session, commit)
    if failure is not None:
        raise failure
    assert result is not None
    return result


def _publish_input_hash(
    commit: ImportCommit,
    request: PublishImportCommitRequest,
) -> str:
    return _canonical_hash(
        {
            "import_commit_id": str(commit.id),
            "expected_revision": request.expected_revision,
        }
    )


def _snapshot_int(value: object, *, field: str) -> int:
    if not isinstance(value, int):
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Import commit snapshot is invalid",
            status_code=500,
            details={"field": field},
        )
    return value


async def publish_import_commit(
    session: AsyncSession,
    claims: AccessTokenClaims,
    commit_id: UUID,
    request: PublishImportCommitRequest,
    *,
    trace_id: str,
) -> ImportCommitDetailResponse:
    failure: ApiError | None = None
    result: ImportCommitDetailResponse | None = None
    async with session.begin():
        initial = await repository.find_import_commit(session, commit_id)
        if initial is None:
            raise _commit_not_found()
        try:
            await lock_active_project_for_content_write(session, claims, initial.project_id)
        except ApiError as error:
            if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
                raise _commit_not_found() from error
            raise
        commit = await repository.find_import_commit(session, commit_id, for_update=True)
        if commit is None:
            raise _commit_not_found()
        input_hash = _publish_input_hash(commit, request)
        if commit.publish_idempotency_key is not None:
            if (
                commit.publish_idempotency_key != request.idempotency_key
                or commit.publish_input_hash != input_hash
            ):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Publish idempotency key was used with different input",
                    status_code=409,
                )
            result = await _commit_detail(session, commit)
        else:
            if commit.status != "materialized":
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Import commit is not ready to publish",
                    status_code=409,
                    next_action="review_import_commit",
                )
            if commit.revision != request.expected_revision:
                raise ApiError(
                    ErrorCode.VERSION_CONFLICT,
                    "Import commit has changed",
                    status_code=409,
                    details={"current_revision": commit.revision},
                )
            now = datetime.now(UTC)
            origins = await repository.list_segment_origins(session, commit.id, for_update=True)
            episode_snapshots = {
                UUID(str(item["episode_id"])): item
                for item in cast(list[dict[str, object]], commit.result_snapshot["episodes"])
            }
            commit.publish_idempotency_key = request.idempotency_key
            commit.publish_input_hash = input_hash
            try:
                async with session.begin_nested():
                    commit.status = "publishing"
                    publish_items: list[EpisodeScriptPublishItem] = []
                    versions_by_origin: dict[UUID, ScriptVersion] = {}
                    for origin in origins:
                        draft = await session.get(ScriptVersion, origin.draft_version_id)
                        if draft is None or draft.status != "draft":
                            raise ApiError(
                                ErrorCode.VERSION_CONFLICT,
                                "Draft script version has changed",
                                status_code=409,
                                next_action="review_import_commit",
                            )
                        published = ScriptVersion(
                            id=uuid7(),
                            workspace_id=commit.workspace_id,
                            source_id=draft.source_id,
                            version_no=2,
                            status="published",
                            body=draft.body,
                            content_hash=draft.content_hash,
                            structure_summary=draft.structure_summary,
                            created_by=claims.sub,
                            created_at=now,
                        )
                        session.add(published)
                        versions_by_origin[origin.id] = published
                        expected = episode_snapshots[origin.episode_id]
                        publish_items.append(
                            EpisodeScriptPublishItem(
                                episode_id=origin.episode_id,
                                expected_revision=_snapshot_int(
                                    expected["revision"], field="episodes[].revision"
                                ),
                                expected_current_script_version_id=None,
                                script_version_id=published.id,
                            )
                        )
                    await session.flush()
                    batch = await publish_episode_script_version_batch(
                        session,
                        claims,
                        EpisodeScriptPublishBatchCommand(
                            project_id=commit.project_id,
                            expected_project_revision=_snapshot_int(
                                commit.result_snapshot["project_revision"],
                                field="project_revision",
                            ),
                            items=publish_items,
                        ),
                        trace_id=trace_id,
                    )
                    published_by_episode = {item.episode_id: item for item in batch.items}
                    for origin in origins:
                        origin.published_version_id = versions_by_origin[origin.id].id
                    commit.result_snapshot = {
                        **commit.result_snapshot,
                        "published": [
                            {
                                "episode_id": str(item.episode_id),
                                "revision": item.revision,
                                "current_script_version_id": str(item.current_script_version_id),
                            }
                            for item in published_by_episode.values()
                        ],
                    }
            except ApiError as error:
                await session.refresh(commit)
                commit.status = "conflict"
                commit.error_code = str(error.code)
                failure = error
            except Exception:
                await session.refresh(commit)
                commit.status = "failed"
                commit.error_code = "publish_failed"
                failure = ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Episode publish failed",
                    status_code=500,
                    next_action="review_import_commit",
                )
            else:
                commit.status = "published"
                commit.error_code = None
            commit.revision += 1
            commit.updated_at = now
            append_audit_event(
                session,
                workspace_id=commit.workspace_id,
                actor_id=claims.sub,
                action=(
                    "script.import_commit_published"
                    if failure is None
                    else "script.import_commit_publish_failed"
                ),
                target_type="import_commit",
                target_id=commit.id,
                trace_id=trace_id,
                metadata={
                    "plan_id": str(commit.plan_id),
                    "status": commit.status,
                    "revision": commit.revision,
                    "episode_count": len(origins),
                    "error_code": commit.error_code,
                },
                occurred_at=now,
            )
            await session.flush()
            result = await _commit_detail(session, commit)
    if failure is not None:
        raise failure
    assert result is not None
    return result


async def get_episode_planning_input(
    session: AsyncSession,
    plan_id: UUID,
    task_id: UUID,
) -> EpisodePlanningInput:
    plan = await repository.find_plan(session, plan_id)
    if plan is None or plan.planning_task_id != task_id or plan.status != "draft":
        raise ApiError(
            ErrorCode.STATE_CONFLICT, "Episode plan input is unavailable", status_code=409
        )
    found = await repository.find_revision_document(session, plan.document_revision_id)
    if found is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR, "Document revision is unavailable", status_code=500
        )
    revision, _ = found
    return EpisodePlanningInput(
        plan_id=plan.id,
        task_id=task_id,
        workspace_id=plan.workspace_id,
        document_revision_id=revision.id,
        input_hash=plan.input_hash,
        normalized_text=revision.normalized_text,
        target_duration_ms=plan.target_duration_ms,
        maximum_episode_count=MAX_EPISODES,
    )


async def record_episode_planning_result(
    session: AsyncSession,
    planning_input: EpisodePlanningInput,
    result: EpisodePlanningProviderResult,
    *,
    trace_id: str,
) -> None:
    plan = await repository.find_plan(session, planning_input.plan_id, for_update=True)
    if plan is None or plan.planning_task_id != planning_input.task_id:
        raise ApiError(ErrorCode.STATE_CONFLICT, "Episode plan is unavailable", status_code=409)
    if plan.status != "draft" or plan.input_hash != planning_input.input_hash:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT, "Episode plan input has changed", status_code=409
        )
    found = await repository.find_revision_document(session, plan.document_revision_id)
    if found is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR, "Document revision is unavailable", status_code=500
        )
    revision, _ = found
    blocks = await repository.list_blocks(session, revision.id)
    now = datetime.now(UTC)
    proposals = _provider_proposals(plan, revision, blocks, result, now)
    session.add_all(proposals)
    plan.status = "review_ready"
    plan.total_estimated_duration_ms = sum(item.estimated_duration_ms for item in proposals)
    plan.planning_error_code = None
    plan.revision += 1
    plan.updated_at = now
    await complete_episode_planning_task(
        session,
        planning_input.task_id,
        now=now,
        trace_id=trace_id,
    )
    append_audit_event(
        session,
        workspace_id=plan.workspace_id,
        actor_id=plan.created_by,
        action="script.episode_plan_generated",
        target_type="episode_plan",
        target_id=plan.id,
        trace_id=trace_id,
        metadata={
            "revision": plan.revision,
            "status": plan.status,
            "proposal_count": len(proposals),
        },
        occurred_at=now,
    )
    await session.flush()


async def record_episode_planning_error(
    session: AsyncSession,
    plan_id: UUID,
    *,
    error_code: str,
) -> None:
    plan = await repository.find_plan(session, plan_id, for_update=True)
    if plan is None or plan.status != "draft":
        return
    plan.planning_error_code = error_code
    plan.updated_at = datetime.now(UTC)
    await session.flush()
