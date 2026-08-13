from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.assets import StoryboardAssetInput, resolve_storyboard_assets
from app.modules.governance.audit import append_audit_event
from app.modules.identity import Capability, actor_context
from app.modules.production import (
    StoryboardDraftTaskCommand,
    complete_storyboard_draft_task,
    create_storyboard_draft_task,
    start_storyboard_draft_task,
)
from app.modules.projects import (
    episode_for_content_read,
    lock_active_episode_for_content_write,
    resolve_storyboard_episode,
)
from app.modules.scripts import StoryboardNarrativeSnapshot, resolve_storyboard_narrative
from app.modules.storyboards import repository as storyboard_repository
from app.modules.storyboards.contracts import (
    StoryboardDraftAsset,
    StoryboardDraftInput,
    StoryboardDraftInputChanged,
    StoryboardDraftUnit,
)
from app.modules.storyboards.drafts import repository
from app.modules.storyboards.drafts.models import (
    DraftAssetReference,
    DraftDecision,
    DraftInputAsset,
    DraftInputUnit,
    DraftShot,
    DraftShotUnit,
    StoryboardDraftBatch,
)
from app.modules.storyboards.drafts.schemas import (
    DraftApplyDiff,
    DraftApplyPreflightRequest,
    DraftApplyPreflightResponse,
    DraftApplyRequest,
    DraftApplyResponse,
    DraftApproveRequest,
    DraftAssetReferenceResponse,
    DraftBatchCreateRequest,
    DraftBatchResponse,
    DraftDecisionRequest,
    DraftDecisionResponse,
    DraftDecisionResult,
    DraftDecisionSummary,
    DraftInputSummary,
    DraftProviderResult,
    DraftShotResponse,
    DraftTarget,
)
from app.modules.storyboards.hashing import (
    canonical_payload_hash,
    shot_order_hash,
    storyboard_content_hashes,
)
from app.modules.storyboards.models import AssetReference, Shot, ShotSpecVersion

ENGINE_VERSION = "storyboard-draft-v1"
MODEL_NAME = "deepseek-chat"
PROMPT_VERSION = "storyboard-draft-prompt-v1"
SCHEMA_VERSION = "storyboard-draft-schema-v1"
MAX_ACTIVE_SHOTS = 120
MIN_AI_SHOT_DURATION_MS = 4_000
DURATION_TOLERANCE = 0.25


def _not_found(resource: str) -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, f"{resource} not found", status_code=404)


def _version_conflict(message: str) -> ApiError:
    return ApiError(
        ErrorCode.VERSION_CONFLICT,
        message,
        status_code=409,
        next_action="create_new_storyboard_draft_batch",
    )


def _command_hash(payload: object) -> str:
    return canonical_payload_hash(payload)


def _shot_baseline(shots: list[Shot]) -> list[dict[str, object]]:
    return [
        {
            "shot_id": str(shot.id),
            "position": shot.position,
            "revision": shot.revision,
            "current_spec_version_id": (
                str(shot.current_spec_version_id)
                if shot.current_spec_version_id is not None
                else None
            ),
        }
        for shot in shots
    ]


def _batch_input_hash(
    *,
    narrative: StoryboardNarrativeSnapshot,
    assets: tuple[StoryboardAssetInput, ...],
    target_duration_ms: int,
    aspect_ratio: str,
    visual_style: str | None,
    base_shot_hash: str,
) -> str:
    return canonical_payload_hash(
        {
            "script_version_id": str(narrative.script_version_id),
            "narrative_structure_id": str(narrative.structure_id),
            "narrative_revision": narrative.structure_revision,
            "narrative_dependency_hash": narrative.dependency_hash,
            "units": [
                {
                    "unit_version_id": str(unit.unit_version_id),
                    "text_hash": unit.text_hash,
                    "position": unit.position,
                }
                for unit in narrative.units
            ],
            "assets": [
                {
                    "asset_state_id": str(asset.asset_state_id),
                    "asset_version_id": str(asset.asset_version_id),
                    "state_revision": asset.state_revision,
                    "readiness_hash": asset.readiness_hash,
                }
                for asset in assets
            ],
            "target_duration_ms": target_duration_ms,
            "aspect_ratio": aspect_ratio,
            "visual_style": visual_style,
            "base_shot_hash": base_shot_hash,
            "engine_version": ENGINE_VERSION,
            "model_name": MODEL_NAME,
            "prompt_version": PROMPT_VERSION,
            "schema_version": SCHEMA_VERSION,
        }
    )


def _same_batch_command(batch: StoryboardDraftBatch, input_hash: str) -> bool:
    return batch.input_hash == input_hash


def _decision_command_hash(draft_id: UUID, request: DraftDecisionRequest) -> str:
    return _command_hash(
        {
            "draft_id": str(draft_id),
            "action": request.action,
            "target": (
                request.target.model_dump(mode="json") if request.target is not None else None
            ),
        }
    )


def _decision_response(decision: DraftDecision) -> DraftDecisionResponse:
    return DraftDecisionResponse(
        id=decision.id,
        sequence=decision.sequence,
        action=cast(Literal["accepted", "modified", "ignored"], decision.action),
        target=(
            DraftTarget.model_validate(decision.target) if decision.target is not None else None
        ),
        created_by=decision.actor_id,
        created_at=decision.created_at,
    )


def _latest_decisions(decisions: list[DraftDecision]) -> dict[UUID, DraftDecision]:
    latest: dict[UUID, DraftDecision] = {}
    for decision in decisions:
        latest[decision.draft_shot_id] = decision
    return latest


async def _batch_response(
    session: AsyncSession,
    batch: StoryboardDraftBatch,
) -> DraftBatchResponse:
    units = await repository.list_input_units(session, batch.id)
    assets = await repository.list_input_assets(session, batch.id)
    drafts = await repository.list_drafts(session, batch.id)
    draft_ids = [draft.id for draft in drafts]
    draft_units = await repository.list_draft_units(session, draft_ids)
    draft_assets = await repository.list_draft_assets(session, draft_ids)
    decisions = await repository.list_decisions(session, batch.id)

    units_by_draft: dict[UUID, list[DraftShotUnit]] = {}
    for unit in draft_units:
        units_by_draft.setdefault(unit.draft_shot_id, []).append(unit)
    assets_by_draft: dict[UUID, list[DraftAssetReference]] = {}
    for reference in draft_assets:
        assets_by_draft.setdefault(reference.draft_shot_id, []).append(reference)
    decisions_by_draft: dict[UUID, list[DraftDecision]] = {}
    for decision in decisions:
        decisions_by_draft.setdefault(decision.draft_shot_id, []).append(decision)

    latest = _latest_decisions(decisions)
    counts = {"accepted": 0, "modified": 0, "ignored": 0}
    for decision in latest.values():
        counts[decision.action] += 1
    draft_responses = [
        DraftShotResponse(
            id=draft.id,
            proposal_key=draft.proposal_key,
            position=draft.position,
            title=draft.title,
            narrative_unit_version_ids=[
                unit.unit_version_id for unit in units_by_draft.get(draft.id, [])
            ],
            spec=DraftTarget.model_validate(
                {
                    "title": draft.title,
                    "narrative_unit_version_ids": [
                        unit.unit_version_id for unit in units_by_draft.get(draft.id, [])
                    ],
                    "spec": draft.spec,
                    "asset_references": [],
                }
            ).spec,
            asset_references=[
                DraftAssetReferenceResponse(
                    slot_key=reference.slot_key,
                    role=cast(
                        Literal[
                            "location",
                            "character",
                            "prop",
                            "costume",
                            "visual_style",
                            "voice",
                        ],
                        reference.role,
                    ),
                    asset_version_id=reference.asset_version_id,
                    subject_key=reference.subject_key,
                )
                for reference in assets_by_draft.get(draft.id, [])
            ],
            risk_codes=draft.risk_codes,
            decision_history=[
                _decision_response(decision) for decision in decisions_by_draft.get(draft.id, [])
            ],
        )
        for draft in drafts
    ]
    return DraftBatchResponse(
        id=batch.id,
        workspace_id=batch.workspace_id,
        project_id=batch.project_id,
        episode_id=batch.episode_id,
        status=cast(
            Literal[
                "queued",
                "running",
                "needs_review",
                "approved",
                "applied",
                "failed",
                "unknown",
                "cancelled",
            ],
            batch.status,
        ),
        revision=batch.revision,
        task_id=batch.task_id,
        input=DraftInputSummary(
            script_version_id=batch.input_script_version_id,
            narrative_structure_id=batch.narrative_structure_id,
            narrative_revision=batch.narrative_revision,
            narrative_dependency_hash=batch.narrative_dependency_hash,
            narrative_unit_version_ids=[unit.unit_version_id for unit in units],
            asset_state_ids=[asset.asset_state_id for asset in assets],
            asset_version_ids=[asset.asset_version_id for asset in assets],
            target_duration_ms=batch.target_duration_ms,
            aspect_ratio=cast(Literal["9:16", "16:9", "1:1"], batch.aspect_ratio),
            visual_style=batch.visual_style,
            input_hash=batch.input_hash,
        ),
        drafts=draft_responses,
        decision_summary=DraftDecisionSummary(
            pending=len(drafts) - len(latest),
            accepted=counts["accepted"],
            modified=counts["modified"],
            ignored=counts["ignored"],
        ),
        error_code=batch.error_code,
        created_at=batch.created_at,
        updated_at=batch.updated_at,
    )


async def create_batch(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: DraftBatchCreateRequest,
    *,
    trace_id: str,
) -> DraftBatchResponse:
    async with session.begin():
        content = await lock_active_episode_for_content_write(session, claims, episode_id)
        episode = await resolve_storyboard_episode(
            session,
            content.workspace_id,
            episode_id,
            for_update=False,
        )
        if episode is None:
            raise _not_found("Episode")
        if episode.current_script_version_id != request.input_script_version_id:
            raise _version_conflict("Current script version has changed")
        narrative = await resolve_storyboard_narrative(
            session,
            content.workspace_id,
            request.input_script_version_id,
        )
        if narrative is None or narrative.episode_id != episode_id:
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Current narrative structure is unavailable",
                status_code=503,
                next_action="review_narrative_structure",
            )
        if not narrative.units or any(
            unit.source_scene_id is None
            or (unit.kind == "dialogue" and unit.source_dialogue_id is None)
            for unit in narrative.units
        ):
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Confirmed script structure is unavailable",
                status_code=503,
                next_action="review_script_structure",
            )
        assets = await resolve_storyboard_assets(
            session,
            content.workspace_id,
            episode.project_id,
            request.asset_state_ids,
        )
        if len(assets) != len(request.asset_state_ids):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Every selected asset state must have a ready current version",
                status_code=409,
                next_action="review_asset_readiness",
            )
        shots = await storyboard_repository.list_active_shots(
            session,
            episode_id,
            for_update=True,
        )
        baseline = _shot_baseline(shots)
        base_order_hash = shot_order_hash([shot.id for shot in shots])
        base_shot_hash = canonical_payload_hash(baseline)
        input_hash = _batch_input_hash(
            narrative=narrative,
            assets=assets,
            target_duration_ms=episode.target_duration_ms,
            aspect_ratio=episode.aspect_ratio,
            visual_style=episode.visual_style,
            base_shot_hash=base_shot_hash,
        )
        existing = await repository.find_batch_by_key(
            session,
            episode_id,
            request.idempotency_key,
        )
        if existing is not None:
            if not _same_batch_command(existing, input_hash):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            return await _batch_response(session, existing)

        now = datetime.now(UTC)
        batch = StoryboardDraftBatch(
            id=uuid7(),
            workspace_id=content.workspace_id,
            project_id=episode.project_id,
            episode_id=episode_id,
            input_script_version_id=request.input_script_version_id,
            narrative_structure_id=narrative.structure_id,
            narrative_revision=narrative.structure_revision,
            narrative_dependency_hash=narrative.dependency_hash,
            input_hash=input_hash,
            target_duration_ms=episode.target_duration_ms,
            aspect_ratio=episode.aspect_ratio,
            visual_style=episode.visual_style,
            engine_version=ENGINE_VERSION,
            model_name=MODEL_NAME,
            prompt_version=PROMPT_VERSION,
            schema_version=SCHEMA_VERSION,
            base_order_hash=base_order_hash,
            base_shot_hash=base_shot_hash,
            base_shots=baseline,
            status="queued",
            revision=1,
            idempotency_key=request.idempotency_key,
            created_by=claims.sub,
            created_at=now,
            updated_at=now,
        )
        session.add(batch)
        await session.flush()
        session.add_all(
            DraftInputUnit(
                id=uuid7(),
                workspace_id=content.workspace_id,
                episode_id=episode_id,
                batch_id=batch.id,
                narrative_unit_id=unit.narrative_unit_id,
                unit_version_id=unit.unit_version_id,
                position=unit.position,
                kind=unit.kind,
                exact_text=unit.exact_text,
                text_hash=unit.text_hash,
                required_for_coverage=unit.required_for_coverage,
                source_scene_id=unit.source_scene_id,
                source_dialogue_id=unit.source_dialogue_id,
            )
            for unit in narrative.units
        )
        session.add_all(
            DraftInputAsset(
                id=uuid7(),
                workspace_id=content.workspace_id,
                batch_id=batch.id,
                asset_id=asset.asset_id,
                asset_state_id=asset.asset_state_id,
                asset_version_id=asset.asset_version_id,
                position=position,
                kind=asset.kind,
                name=asset.name,
                state_label=asset.state_label,
                state_revision=asset.state_revision,
                readiness_hash=asset.readiness_hash,
            )
            for position, asset in enumerate(assets, start=1)
        )
        actor = await actor_context(
            session,
            claims,
            content.workspace_id,
            Capability.CONTENT_WRITE,
        )
        task = await create_storyboard_draft_task(
            session,
            actor,
            StoryboardDraftTaskCommand(
                workspace_id=content.workspace_id,
                episode_id=episode_id,
                batch_id=batch.id,
                input_version_id=request.input_script_version_id,
                input_hash=input_hash,
                idempotency_key=f"storyboard-draft:{batch.id}",
            ),
            trace_id=trace_id,
        )
        batch.task_id = task.id
        append_audit_event(
            session,
            workspace_id=batch.workspace_id,
            actor_id=claims.sub,
            action="storyboard.draft_batch_created",
            target_type="storyboard_draft_batch",
            target_id=batch.id,
            trace_id=trace_id,
            metadata={
                "episode_id": str(batch.episode_id),
                "input_hash": batch.input_hash,
                "task_id": str(task.id),
                "revision": batch.revision,
            },
            occurred_at=now,
        )
        await session.flush()
        return await _batch_response(session, batch)


def _validate_target(
    target: DraftTarget,
    batch: StoryboardDraftBatch,
    units: list[DraftInputUnit],
    assets: list[DraftInputAsset],
) -> None:
    allowed_units = {unit.unit_version_id for unit in units}
    allowed_assets = {asset.asset_version_id for asset in assets}
    if not set(target.narrative_unit_version_ids).issubset(allowed_units):
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Draft references narrative units outside the fixed input",
            status_code=422,
        )
    if target.spec.script_reference.confirmed_script_version_id != batch.input_script_version_id:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Draft spec references a different script version",
            status_code=422,
        )
    allowed_scenes = {unit.source_scene_id for unit in units if unit.source_scene_id is not None}
    allowed_dialogues = {
        unit.source_dialogue_id for unit in units if unit.source_dialogue_id is not None
    }
    if target.spec.script_reference.scene_id not in allowed_scenes or not set(
        target.spec.script_reference.dialogue_ids
    ).issubset(allowed_dialogues):
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Draft spec references structure outside the fixed narrative input",
            status_code=422,
        )
    dialogue_scenes = {
        unit.source_dialogue_id: unit.source_scene_id
        for unit in units
        if unit.source_dialogue_id is not None
    }
    if any(
        dialogue_scenes[dialogue_id] != target.spec.script_reference.scene_id
        for dialogue_id in target.spec.script_reference.dialogue_ids
    ):
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Draft dialogue references must belong to the selected scene",
            status_code=422,
        )
    if not {reference.asset_version_id for reference in target.asset_references}.issubset(
        allowed_assets
    ):
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Draft references assets outside the fixed input",
            status_code=422,
        )


def _validate_provider_result(
    result: DraftProviderResult,
    batch: StoryboardDraftBatch,
    units: list[DraftInputUnit],
) -> None:
    covered_units = {
        unit_version_id
        for shot in result.shots
        for unit_version_id in shot.narrative_unit_version_ids
    }
    required_units = {unit.unit_version_id for unit in units if unit.required_for_coverage}
    if not required_units.issubset(covered_units):
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Storyboard drafts do not cover every required narrative unit",
            status_code=422,
        )
    if any(shot.spec.duration_ms < MIN_AI_SHOT_DURATION_MS for shot in result.shots):
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "AI storyboard shot duration must be between 4 and 15 seconds",
            status_code=422,
        )
    total_duration_ms = sum(shot.spec.duration_ms for shot in result.shots)
    duration_lower_ms = round(batch.target_duration_ms * (1 - DURATION_TOLERANCE))
    duration_upper_ms = round(batch.target_duration_ms * (1 + DURATION_TOLERANCE))
    if not duration_lower_ms <= total_duration_ms <= duration_upper_ms:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Storyboard draft duration is outside the accepted target range",
            status_code=422,
        )
    if 60_000 <= batch.target_duration_ms <= 120_000 and not 12 <= len(result.shots) <= 24:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "A 60-120 second episode requires 12-24 storyboard drafts",
            status_code=422,
        )


async def record_draft_result(
    session: AsyncSession,
    *,
    batch_id: UUID,
    result: DraftProviderResult | dict[str, object],
) -> None:
    result = DraftProviderResult.model_validate(result)
    batch = await repository.find_batch(session, batch_id, for_update=True)
    if batch is None:
        raise _not_found("Storyboard draft batch")
    result_hash = canonical_payload_hash(result.model_dump(mode="json"))
    if batch.status == "needs_review" and batch.provider_result_hash == result_hash:
        return
    if batch.status not in {"queued", "running"}:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Storyboard draft batch cannot accept provider output",
            status_code=409,
        )
    if batch.task_id is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Draft task is unavailable", status_code=500)
    units = await repository.list_input_units(session, batch.id)
    assets = await repository.list_input_assets(session, batch.id)
    _validate_provider_result(result, batch, units)
    for proposal in result.shots:
        _validate_target(
            DraftTarget(
                title=proposal.title,
                narrative_unit_version_ids=proposal.narrative_unit_version_ids,
                spec=proposal.spec,
                asset_references=proposal.asset_references,
            ),
            batch,
            units,
            assets,
        )
    now = datetime.now(UTC)
    if batch.status == "queued":
        await start_storyboard_draft_task(
            session,
            batch.task_id,
            now=now,
            trace_id=f"draft-{batch.id}",
        )
        batch.status = "running"
        batch.revision += 1
    for proposal in result.shots:
        hashes = storyboard_content_hashes(proposal.spec, proposal.asset_references)
        draft = DraftShot(
            id=uuid7(),
            workspace_id=batch.workspace_id,
            batch_id=batch.id,
            proposal_key=proposal.proposal_key,
            position=proposal.position,
            title=proposal.title,
            spec=proposal.spec.model_dump(mode="json"),
            content_hash=hashes.content_hash,
            risk_codes=proposal.risk_codes,
            created_at=now,
        )
        session.add(draft)
        await session.flush()
        session.add_all(
            DraftShotUnit(
                id=uuid7(),
                workspace_id=batch.workspace_id,
                batch_id=batch.id,
                draft_shot_id=draft.id,
                unit_version_id=unit_version_id,
                position=position,
            )
            for position, unit_version_id in enumerate(
                proposal.narrative_unit_version_ids,
                start=1,
            )
        )
        session.add_all(
            DraftAssetReference(
                id=uuid7(),
                workspace_id=batch.workspace_id,
                batch_id=batch.id,
                draft_shot_id=draft.id,
                slot_key=reference.slot_key,
                role=reference.role,
                asset_version_id=reference.asset_version_id,
                subject_key=reference.subject_key,
            )
            for reference in proposal.asset_references
        )
    batch.provider_result_hash = result_hash
    batch.status = "needs_review"
    batch.revision += 1
    batch.updated_at = now
    await complete_storyboard_draft_task(
        session,
        batch.task_id,
        now=now,
        trace_id=f"draft-{batch.id}",
    )
    await session.flush()


async def record_draft_error(
    session: AsyncSession,
    *,
    batch_id: UUID,
    task_id: UUID,
    error_code: str,
    unknown: bool = False,
) -> None:
    batch = await repository.find_batch(session, batch_id, for_update=True)
    if batch is None or batch.task_id != task_id:
        raise _not_found("Storyboard draft batch")
    if batch.status in {"needs_review", "approved", "applied", "cancelled"}:
        return
    batch.status = "unknown" if unknown else "failed"
    batch.error_code = error_code
    batch.revision += 1
    batch.updated_at = datetime.now(UTC)
    await session.flush()


async def get_batch(
    session: AsyncSession,
    claims: AccessTokenClaims,
    batch_id: UUID,
) -> DraftBatchResponse:
    batch = await repository.find_batch(session, batch_id)
    if batch is None:
        raise _not_found("Storyboard draft batch")
    await episode_for_content_read(session, claims, batch.episode_id)
    return await _batch_response(session, batch)


async def decide_draft(
    session: AsyncSession,
    claims: AccessTokenClaims,
    draft_id: UUID,
    request: DraftDecisionRequest,
    *,
    trace_id: str,
) -> DraftDecisionResult:
    async with session.begin():
        draft = await repository.find_draft(session, draft_id, for_update=True)
        if draft is None:
            raise _not_found("Storyboard draft")
        batch = await repository.find_batch(session, draft.batch_id, for_update=True)
        if batch is None:
            raise _not_found("Storyboard draft batch")
        await lock_active_episode_for_content_write(session, claims, batch.episode_id)
        command_hash = _decision_command_hash(draft_id, request)
        existing = await repository.find_decision_by_key(
            session,
            batch.workspace_id,
            request.idempotency_key,
        )
        if existing is not None:
            if existing.draft_shot_id != draft_id or existing.command_hash != command_hash:
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            return DraftDecisionResult(
                batch=await _batch_response(session, batch),
                draft=next(
                    item
                    for item in (await _batch_response(session, batch)).drafts
                    if item.id == draft_id
                ),
            )
        if batch.status != "needs_review":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Storyboard drafts are not open for review",
                status_code=409,
            )
        if batch.revision != request.expected_batch_revision:
            raise _version_conflict("Storyboard draft batch has changed")
        units = await repository.list_input_units(session, batch.id)
        assets = await repository.list_input_assets(session, batch.id)
        if request.target is not None:
            _validate_target(request.target, batch, units, assets)
        now = datetime.now(UTC)
        decision = DraftDecision(
            id=uuid7(),
            workspace_id=batch.workspace_id,
            batch_id=batch.id,
            draft_shot_id=draft.id,
            sequence=await repository.next_decision_sequence(session, batch.id),
            action=request.action,
            target=(request.target.model_dump(mode="json") if request.target is not None else None),
            command_hash=command_hash,
            idempotency_key=request.idempotency_key,
            actor_id=claims.sub,
            created_at=now,
        )
        session.add(decision)
        batch.revision += 1
        batch.updated_at = now
        append_audit_event(
            session,
            workspace_id=batch.workspace_id,
            actor_id=claims.sub,
            action="storyboard.draft_decided",
            target_type="storyboard_draft",
            target_id=draft.id,
            trace_id=trace_id,
            metadata={
                "batch_id": str(batch.id),
                "decision": request.action,
                "revision": batch.revision,
            },
            occurred_at=now,
        )
        await session.flush()
        response = await _batch_response(session, batch)
        return DraftDecisionResult(
            batch=response,
            draft=next(item for item in response.drafts if item.id == draft.id),
        )


async def approve_batch(
    session: AsyncSession,
    claims: AccessTokenClaims,
    batch_id: UUID,
    request: DraftApproveRequest,
    *,
    trace_id: str,
) -> DraftBatchResponse:
    async with session.begin():
        batch = await repository.find_batch(session, batch_id, for_update=True)
        if batch is None:
            raise _not_found("Storyboard draft batch")
        await lock_active_episode_for_content_write(session, claims, batch.episode_id)
        command_hash = _command_hash({"batch_id": str(batch_id)})
        if batch.approve_idempotency_key is not None:
            if (
                batch.approve_idempotency_key != request.idempotency_key
                or batch.approve_command_hash != command_hash
            ):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Storyboard draft batch was approved by a different command",
                    status_code=409,
                )
            return await _batch_response(session, batch)
        if batch.status != "needs_review":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Storyboard draft batch cannot be approved",
                status_code=409,
            )
        if batch.revision != request.expected_revision:
            raise _version_conflict("Storyboard draft batch has changed")
        drafts = await repository.list_drafts(session, batch.id)
        latest = _latest_decisions(await repository.list_decisions(session, batch.id))
        if len(latest) != len(drafts):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Every storyboard draft requires a decision before approval",
                status_code=409,
                next_action="review_pending_storyboard_drafts",
            )
        now = datetime.now(UTC)
        batch.status = "approved"
        batch.approve_idempotency_key = request.idempotency_key
        batch.approve_command_hash = command_hash
        batch.revision += 1
        batch.updated_at = now
        append_audit_event(
            session,
            workspace_id=batch.workspace_id,
            actor_id=claims.sub,
            action="storyboard.draft_batch_approved",
            target_type="storyboard_draft_batch",
            target_id=batch.id,
            trace_id=trace_id,
            metadata={"revision": batch.revision},
            occurred_at=now,
        )
        await session.flush()
        return await _batch_response(session, batch)


async def _assert_current_inputs(
    session: AsyncSession,
    batch: StoryboardDraftBatch,
    *,
    for_update: bool,
) -> tuple[tuple[StoryboardAssetInput, ...], list[Shot]]:
    episode = await resolve_storyboard_episode(
        session,
        batch.workspace_id,
        batch.episode_id,
        for_update=for_update,
    )
    if episode is None:
        raise _version_conflict("Episode is no longer available")
    if episode.current_script_version_id != batch.input_script_version_id:
        raise _version_conflict("Current script version has changed")
    narrative = await resolve_storyboard_narrative(
        session,
        batch.workspace_id,
        batch.input_script_version_id,
    )
    if (
        narrative is None
        or narrative.structure_id != batch.narrative_structure_id
        or narrative.structure_revision != batch.narrative_revision
        or narrative.dependency_hash != batch.narrative_dependency_hash
    ):
        raise _version_conflict("Narrative structure has changed")
    inputs = await repository.list_input_assets(session, batch.id)
    current_assets = await resolve_storyboard_assets(
        session,
        batch.workspace_id,
        batch.project_id,
        [asset.asset_state_id for asset in inputs],
        for_update=for_update,
    )
    current_by_state = {asset.asset_state_id: asset for asset in current_assets}
    if len(current_assets) != len(inputs) or any(
        (current := current_by_state.get(asset.asset_state_id)) is None
        or current.asset_version_id != asset.asset_version_id
        or current.state_revision != asset.state_revision
        or current.readiness_hash != asset.readiness_hash
        for asset in inputs
    ):
        raise _version_conflict("Storyboard asset inputs have changed")
    if (
        episode.project_id != batch.project_id
        or episode.target_duration_ms != batch.target_duration_ms
        or episode.aspect_ratio != batch.aspect_ratio
        or episode.visual_style != batch.visual_style
    ):
        raise _version_conflict("Storyboard project settings have changed")
    shots = await storyboard_repository.list_active_shots(
        session,
        batch.episode_id,
        for_update=for_update,
    )
    baseline = _shot_baseline(shots)
    order_hash = shot_order_hash([shot.id for shot in shots])
    if (
        order_hash != batch.base_order_hash
        or canonical_payload_hash(baseline) != batch.base_shot_hash
        or baseline != batch.base_shots
    ):
        raise _version_conflict("Storyboard shot baseline has changed")
    return current_assets, shots


async def prepare_draft_input(
    session: AsyncSession,
    *,
    batch_id: UUID,
    task_id: UUID,
) -> StoryboardDraftInput:
    batch = await repository.find_batch(session, batch_id, for_update=True)
    if batch is None or batch.task_id != task_id:
        raise StoryboardDraftInputChanged("Storyboard draft task input is unavailable")
    if batch.status not in {"queued", "running"}:
        raise StoryboardDraftInputChanged("Storyboard draft batch is no longer runnable")
    try:
        await _assert_current_inputs(session, batch, for_update=False)
    except ApiError as error:
        raise StoryboardDraftInputChanged("Storyboard draft input has changed") from error
    units = await repository.list_input_units(session, batch.id)
    assets = await repository.list_input_assets(session, batch.id)
    if not units:
        raise StoryboardDraftInputChanged("Storyboard narrative input is unavailable")
    if batch.status == "queued":
        batch.status = "running"
        batch.revision += 1
        batch.updated_at = datetime.now(UTC)
    await session.flush()
    return StoryboardDraftInput(
        batch_id=batch.id,
        task_id=task_id,
        input_hash=batch.input_hash,
        script_version_id=batch.input_script_version_id,
        target_duration_ms=batch.target_duration_ms,
        aspect_ratio=cast(Literal["9:16", "16:9", "1:1"], batch.aspect_ratio),
        visual_style=batch.visual_style,
        units=tuple(
            StoryboardDraftUnit(
                unit_version_id=unit.unit_version_id,
                position=unit.position,
                kind=cast(
                    Literal["scene_heading", "action", "dialogue", "narration"],
                    unit.kind,
                ),
                exact_text=unit.exact_text,
                required_for_coverage=unit.required_for_coverage,
                source_scene_id=unit.source_scene_id,
                source_dialogue_id=unit.source_dialogue_id,
            )
            for unit in units
        ),
        assets=tuple(
            StoryboardDraftAsset(
                asset_version_id=asset.asset_version_id,
                position=asset.position,
                kind=asset.kind,
                name=asset.name,
                state_label=asset.state_label,
            )
            for asset in assets
        ),
    )


async def _current_preflight(
    session: AsyncSession,
    batch: StoryboardDraftBatch,
    *,
    for_update: bool,
) -> DraftApplyPreflightResponse:
    current_assets, shots = await _assert_current_inputs(
        session,
        batch,
        for_update=for_update,
    )
    order_hash = shot_order_hash([shot.id for shot in shots])
    drafts = await repository.list_drafts(session, batch.id)
    latest = _latest_decisions(await repository.list_decisions(session, batch.id))
    if len(latest) != len(drafts):
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Storyboard draft review is incomplete",
            status_code=409,
        )
    created = sum(decision.action != "ignored" for decision in latest.values())
    diff = DraftApplyDiff(kept=len(shots), created=created)
    impact_hash = canonical_payload_hash(
        {
            "batch_id": str(batch.id),
            "batch_revision": batch.revision,
            "order_hash": order_hash,
            "base_shot_hash": batch.base_shot_hash,
            "narrative_dependency_hash": batch.narrative_dependency_hash,
            "asset_readiness_hashes": [asset.readiness_hash for asset in current_assets],
            "decisions": [
                {
                    "draft_shot_id": str(draft_id),
                    "decision_id": str(decision.id),
                    "action": decision.action,
                    "command_hash": decision.command_hash,
                }
                for draft_id, decision in sorted(latest.items(), key=lambda item: str(item[0]))
            ],
            "diff": diff.model_dump(mode="json"),
        }
    )
    return DraftApplyPreflightResponse(
        batch_id=batch.id,
        batch_revision=batch.revision,
        order_hash=order_hash,
        impact_hash=impact_hash,
        diff=diff,
    )


async def preflight_apply(
    session: AsyncSession,
    claims: AccessTokenClaims,
    batch_id: UUID,
    request: DraftApplyPreflightRequest,
) -> DraftApplyPreflightResponse:
    batch = await repository.find_batch(session, batch_id)
    if batch is None:
        raise _not_found("Storyboard draft batch")
    await episode_for_content_read(session, claims, batch.episode_id)
    if batch.status != "approved":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Storyboard draft batch is not approved",
            status_code=409,
        )
    if batch.revision != request.expected_revision:
        raise _version_conflict("Storyboard draft batch has changed")
    return await _current_preflight(session, batch, for_update=False)


async def apply_batch(
    session: AsyncSession,
    claims: AccessTokenClaims,
    batch_id: UUID,
    request: DraftApplyRequest,
    *,
    trace_id: str,
) -> DraftApplyResponse:
    async with session.begin():
        batch = await repository.find_batch(session, batch_id, for_update=True)
        if batch is None:
            raise _not_found("Storyboard draft batch")
        await lock_active_episode_for_content_write(session, claims, batch.episode_id)
        command_hash = _command_hash(
            {
                "batch_id": str(batch_id),
                "expected_revision": request.expected_revision,
                "expected_order_hash": request.expected_order_hash,
                "impact_hash": request.impact_hash,
            }
        )
        if batch.apply_idempotency_key is not None:
            if (
                batch.apply_idempotency_key != request.idempotency_key
                or batch.apply_command_hash != command_hash
            ):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Storyboard draft batch was applied by a different command",
                    status_code=409,
                )
            stored_value = batch.apply_result.get("created_shot_ids")
            if not isinstance(stored_value, list):
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Storyboard apply receipt is invalid",
                    status_code=500,
                )
            stored_ids = cast(list[object], stored_value)
            if not all(isinstance(value, str) for value in stored_ids):
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Storyboard apply receipt is invalid",
                    status_code=500,
                )
            return DraftApplyResponse(
                batch=await _batch_response(session, batch),
                created_shot_ids=[UUID(cast(str, value)) for value in stored_ids],
            )
        if batch.status != "approved":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Storyboard draft batch is not approved",
                status_code=409,
            )
        if batch.revision != request.expected_revision:
            raise _version_conflict("Storyboard draft batch has changed")
        preflight = await _current_preflight(session, batch, for_update=True)
        if preflight.order_hash != request.expected_order_hash:
            raise _version_conflict("Storyboard shot order has changed")
        if preflight.impact_hash != request.impact_hash:
            raise _version_conflict("Storyboard draft impact has changed")

        drafts = await repository.list_drafts(session, batch.id)
        draft_ids = [draft.id for draft in drafts]
        relations = await repository.list_draft_units(session, draft_ids)
        references = await repository.list_draft_assets(session, draft_ids)
        decisions = _latest_decisions(await repository.list_decisions(session, batch.id))
        input_assets = {
            asset.asset_version_id: asset
            for asset in await repository.list_input_assets(session, batch.id)
        }
        units_by_draft: dict[UUID, list[UUID]] = {}
        for relation in relations:
            units_by_draft.setdefault(relation.draft_shot_id, []).append(relation.unit_version_id)
        refs_by_draft: dict[UUID, list[DraftAssetReference]] = {}
        for reference in references:
            refs_by_draft.setdefault(reference.draft_shot_id, []).append(reference)
        current_shots = await storyboard_repository.list_active_shots(
            session,
            batch.episode_id,
            for_update=True,
        )
        accepted = [draft for draft in drafts if decisions[draft.id].action != "ignored"]
        if len(current_shots) + len(accepted) > MAX_ACTIVE_SHOTS:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Storyboard would exceed the active shot limit",
                status_code=409,
            )

        created_ids: list[UUID] = []
        now = datetime.now(UTC)
        for offset, draft in enumerate(accepted, start=1):
            decision = decisions[draft.id]
            if decision.action == "modified":
                assert decision.target is not None
                target = DraftTarget.model_validate(decision.target)
            else:
                target = DraftTarget.model_validate(
                    {
                        "title": draft.title,
                        "narrative_unit_version_ids": units_by_draft[draft.id],
                        "spec": draft.spec,
                        "asset_references": [
                            {
                                "slot_key": reference.slot_key,
                                "role": reference.role,
                                "asset_version_id": reference.asset_version_id,
                                "subject_key": reference.subject_key,
                            }
                            for reference in refs_by_draft.get(draft.id, [])
                        ],
                    }
                )
            spec_id = uuid7()
            shot = Shot(
                id=uuid7(),
                workspace_id=batch.workspace_id,
                episode_id=batch.episode_id,
                position=len(current_shots) + offset,
                title=target.title,
                source_script_version_id=batch.input_script_version_id,
                source_scene_id=target.spec.script_reference.scene_id,
                source_candidate_id=None,
                source_draft_shot_id=draft.id,
                creation_key=f"storyboard-draft:{draft.id}",
                status="active",
                current_spec_version_id=spec_id,
                revision=1,
                created_by=claims.sub,
                created_at=now,
                updated_at=now,
            )
            hashes = storyboard_content_hashes(target.spec, target.asset_references)
            spec = ShotSpecVersion(
                id=spec_id,
                workspace_id=batch.workspace_id,
                shot_id=shot.id,
                version_no=1,
                schema_version=1,
                spec=target.spec.model_dump(mode="json"),
                content_hash=hashes.content_hash,
                input_hash=hashes.input_hash,
                created_by=claims.sub,
                created_at=now,
            )
            session.add(shot)
            await session.flush()
            session.add(spec)
            await session.flush()
            for reference in target.asset_references:
                asset = input_assets[reference.asset_version_id]
                session.add(
                    AssetReference(
                        id=uuid7(),
                        workspace_id=batch.workspace_id,
                        shot_spec_version_id=spec.id,
                        slot_key=reference.slot_key,
                        role=reference.role,
                        asset_version_id=reference.asset_version_id,
                        asset_state_id=asset.asset_state_id,
                        asset_id=asset.asset_id,
                        binding_source="manual" if decision.action == "modified" else "ai",
                        subject_key=reference.subject_key,
                        created_at=now,
                    )
                )
            created_ids.append(shot.id)
        batch.status = "applied"
        batch.apply_idempotency_key = request.idempotency_key
        batch.apply_command_hash = command_hash
        batch.apply_result = {"created_shot_ids": [str(shot_id) for shot_id in created_ids]}
        batch.revision += 1
        batch.updated_at = now
        append_audit_event(
            session,
            workspace_id=batch.workspace_id,
            actor_id=claims.sub,
            action="storyboard.draft_batch_applied",
            target_type="storyboard_draft_batch",
            target_id=batch.id,
            trace_id=trace_id,
            metadata={
                "revision": batch.revision,
                "created_shot_ids": [str(shot_id) for shot_id in created_ids],
            },
            occurred_at=now,
        )
        await session.flush()
        return DraftApplyResponse(
            batch=await _batch_response(session, batch),
            created_shot_ids=created_ids,
        )
