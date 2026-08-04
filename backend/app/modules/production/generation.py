import hashlib
import hmac
import json
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from decimal import Decimal
from typing import Any, Literal, cast
from uuid import UUID

from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.caching import (
    CacheUnavailableError,
    HighCostGuardPort,
    HighCostGuardRequest,
)
from app.modules.governance.audit import append_audit_event
from app.modules.identity import ActorContext, Capability, actor_context
from app.modules.messaging import (
    OutboxEventCommand,
    enqueue_outbox_event,
    find_outbox_event_id,
)
from app.modules.production import generation_repository as repository
from app.modules.production.capabilities import (
    CapabilityDefinition,
    builtin_capability,
    builtin_unavailable_capabilities,
    estimate_fixed_request_cost,
    validate_capability_parameters,
)
from app.modules.production.generation_presenters import (
    cost_entry_response,
    decimal_amount,
    generation_request_response,
    reservation_response,
)
from app.modules.production.generation_schemas import (
    CapabilityPricingResponse,
    CostQueryResponse,
    CostSummaryResponse,
    EstimatedCostResponse,
    GenerationBlocker,
    GenerationConfirmationRequirement,
    GenerationPreflightRequest,
    GenerationPreflightResponse,
    GenerationSubmissionRequest,
    GenerationSubmissionResponse,
    ModelCapabilityResponse,
)
from app.modules.production.models import (
    CostEntry,
    GenerationRequest,
    GenerationRequestAsset,
    ModelCapability,
    Reservation,
    Task,
)
from app.modules.production.service import task_response
from app.modules.projects import (
    GenerationProjectContext,
    resolve_episode_generation_context,
    resolve_project_generation_context,
)
from app.modules.storyboards import ShotProductionSnapshot, get_production_snapshot

PREFLIGHT_TTL = timedelta(minutes=5)


@dataclass(frozen=True, slots=True)
class _ResolvedCapability:
    id: UUID
    provider: str
    model: str
    kind: Literal["image", "video"]
    config_version: int
    input_types: list[str]
    parameter_schema: dict[str, Any]
    limits: dict[str, Any]
    pricing: dict[str, Any] | None
    status: Literal["active", "inactive", "unavailable"]
    unavailable_reason: str | None
    persisted: bool


@dataclass(frozen=True, slots=True)
class _PreflightEvaluation:
    actor: ActorContext
    snapshot: ShotProductionSnapshot
    project: GenerationProjectContext
    capability: _ResolvedCapability
    normalized_parameters: dict[str, Any]
    estimated_amount: Decimal | None
    currency: str | None
    high_cost_required: bool
    facts: dict[str, Any]
    response: GenerationPreflightResponse


def generation_preflight_signature(
    settings: Settings,
    facts: dict[str, Any],
    expires_at: datetime,
) -> str:
    payload = _canonical_json(
        {
            "expires_at": _utc_datetime(expires_at).isoformat(),
            "facts": facts,
        }
    )
    return hmac.new(
        settings.jwt_secret_key.get_secret_value().encode("utf-8"),
        payload.encode("utf-8"),
        hashlib.sha256,
    ).hexdigest()


def verify_generation_preflight_signature(
    settings: Settings,
    facts: dict[str, Any],
    expires_at: datetime,
    signature: str,
    *,
    now: datetime,
) -> bool:
    try:
        normalized_expiry = _utc_datetime(expires_at)
        normalized_now = _utc_datetime(now)
    except ValueError:
        return False
    if normalized_expiry < normalized_now:
        return False
    expected = generation_preflight_signature(settings, facts, normalized_expiry)
    return hmac.compare_digest(expected, signature)


async def list_model_capabilities(
    session: AsyncSession,
    claims: AccessTokenClaims,
    settings: Settings,
    workspace_id: UUID,
    *,
    kind: Literal["image", "video"] | None,
    model: str | None,
) -> list[ModelCapabilityResponse]:
    await actor_context(session, claims, workspace_id, Capability.CONTENT_READ)
    persisted = await repository.list_capabilities(session, kind=kind, model=model)
    by_id = {item.id: _resolved_persisted_capability(item) for item in persisted}
    for definition in builtin_unavailable_capabilities(settings):
        if kind is not None and definition.kind != kind:
            continue
        if model is not None and definition.model != model:
            continue
        by_id.setdefault(definition.id, _resolved_builtin_capability(definition))
    return [
        _capability_response(item)
        for item in sorted(
            by_id.values(),
            key=lambda item: (item.kind, item.model, -item.config_version),
        )
    ]


async def preflight_generation(
    session: AsyncSession,
    claims: AccessTokenClaims,
    settings: Settings,
    shot_id: UUID,
    request: GenerationPreflightRequest,
) -> GenerationPreflightResponse:
    evaluation = await _evaluate_preflight(
        session,
        claims,
        settings,
        shot_id,
        request,
        expires_at=datetime.now(UTC) + PREFLIGHT_TTL,
        for_update=False,
        authorization=Capability.CONTENT_READ,
    )
    return evaluation.response


async def submit_generation(
    session: AsyncSession,
    claims: AccessTokenClaims,
    settings: Settings,
    shot_id: UUID,
    request: GenerationSubmissionRequest,
    *,
    trace_id: str,
    high_cost_guard: HighCostGuardPort,
) -> GenerationSubmissionResponse:
    request_hash = _generation_request_hash(shot_id, request)
    async with session.begin():
        actor = await actor_context(
            session,
            claims,
            request.workspace_id,
            Capability.GENERATION_SUBMIT,
        )
        existing = await repository.find_idempotent_generation_request(
            session,
            request.workspace_id,
            request.idempotency_key,
        )
        if existing is not None:
            return await _replay_or_conflict(session, existing, request_hash)

        evaluation = await _evaluate_preflight(
            session,
            claims,
            settings,
            shot_id,
            request,
            expires_at=request.preflight_expires_at,
            for_update=True,
            authorization=Capability.GENERATION_SUBMIT,
            known_actor=actor,
        )
        _require_submittable_preflight(settings, request, evaluation)
        if not evaluation.capability.persisted:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Capability is not activated",
                status_code=409,
                next_action="select_available_capability",
            )
        if evaluation.estimated_amount is None or evaluation.currency is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Generation cost is unavailable",
                status_code=409,
                next_action="retry_generation_preflight",
            )
        await _authorize_high_cost_submission(
            high_cost_guard,
            settings,
            request,
            request_hash,
            evaluation,
        )

        request_id = uuid7()
        task_id = uuid7()
        reservation_id = uuid7()
        cost_entry_id = uuid7()
        now = datetime.now(UTC)
        inserted_id = await session.scalar(
            insert(GenerationRequest)
            .values(
                id=request_id,
                workspace_id=request.workspace_id,
                project_id=evaluation.project.project_id,
                episode_id=evaluation.snapshot.spec_ref.episode_id,
                shot_id=shot_id,
                shot_spec_version_id=request.shot_spec_version_id,
                capability_id=evaluation.capability.id,
                capability_config_version=evaluation.capability.config_version,
                parameter_snapshot=evaluation.normalized_parameters,
                warning_acknowledgements=request.warning_acknowledgements,
                shot_spec_input_hash=evaluation.snapshot.spec_ref.input_hash,
                input_hash=request_hash,
                preflight_hash=request.preflight_hash,
                preflight_expires_at=request.preflight_expires_at,
                high_cost_confirmed=request.high_cost_confirmed,
                idempotency_key=request.idempotency_key,
                requested_by=actor.user_id,
                created_at=now,
            )
            .on_conflict_do_nothing(constraint="uq_prod_request_workspace_idempotency")
            .returning(GenerationRequest.id)
        )
        if inserted_id is None:
            concurrent = await repository.find_idempotent_generation_request(
                session,
                request.workspace_id,
                request.idempotency_key,
            )
            if concurrent is None:
                raise RuntimeError("idempotent generation request is unavailable")
            return await _replay_or_conflict(session, concurrent, request_hash)

        generation_request = GenerationRequest(
            id=request_id,
            workspace_id=request.workspace_id,
            project_id=evaluation.project.project_id,
            episode_id=evaluation.snapshot.spec_ref.episode_id,
            shot_id=shot_id,
            shot_spec_version_id=request.shot_spec_version_id,
            capability_id=evaluation.capability.id,
            capability_config_version=evaluation.capability.config_version,
            parameter_snapshot=evaluation.normalized_parameters,
            warning_acknowledgements=request.warning_acknowledgements,
            shot_spec_input_hash=evaluation.snapshot.spec_ref.input_hash,
            input_hash=request_hash,
            preflight_hash=request.preflight_hash,
            preflight_expires_at=request.preflight_expires_at,
            high_cost_confirmed=request.high_cost_confirmed,
            idempotency_key=request.idempotency_key,
            requested_by=actor.user_id,
            created_at=now,
        )
        task = Task(
            id=task_id,
            workspace_id=request.workspace_id,
            task_type=f"{evaluation.capability.kind}_generation",
            request_type="generation_request",
            request_id=request_id,
            episode_id=evaluation.snapshot.spec_ref.episode_id,
            usage_type="shot",
            usage_id=shot_id,
            input_version_id=request.shot_spec_version_id,
            input_hash=request_hash,
            status="queued",
            progress_stage="queued",
            next_action="poll_task",
            cancel_status="none",
            idempotency_key=f"generation:{request.idempotency_key}",
            requested_by=actor.user_id,
            revision=1,
            created_at=now,
            updated_at=now,
        )
        reservation = Reservation(
            id=reservation_id,
            workspace_id=request.workspace_id,
            request_id=request_id,
            currency=evaluation.currency,
            estimated_amount=evaluation.estimated_amount,
            reserved_amount=evaluation.estimated_amount,
            status="active",
            revision=1,
            created_at=now,
            updated_at=now,
        )
        cost_entry = CostEntry(
            id=cost_entry_id,
            workspace_id=request.workspace_id,
            reservation_id=reservation_id,
            attempt_id=None,
            entry_type="reserve",
            amount=evaluation.estimated_amount,
            currency=evaluation.currency,
            provider_bill_ref=None,
            idempotency_key="initial-reserve",
            created_at=now,
        )
        session.add_all(
            [
                GenerationRequestAsset(
                    id=uuid7(),
                    workspace_id=request.workspace_id,
                    request_id=request_id,
                    asset_version_id=reference.asset_version_id,
                    slot_key=reference.slot_key,
                    created_at=now,
                )
                for reference in evaluation.snapshot.asset_references
            ]
        )
        session.add_all([task, reservation])
        await session.flush()
        session.add(cost_entry)
        await session.flush()
        outbox_event_id = await enqueue_outbox_event(
            session,
            OutboxEventCommand(
                workspace_id=request.workspace_id,
                event_type="generation.requested",
                schema_version=1,
                aggregate_type="task",
                aggregate_id=task.id,
                routing_key="io.provider.submit",
                payload={"task_id": str(task.id)},
                trace_id=trace_id,
                available_at=now,
                occurred_at=now,
            ),
        )
        append_audit_event(
            session,
            workspace_id=request.workspace_id,
            actor_id=actor.user_id,
            action="task.created",
            target_type="task",
            target_id=task.id,
            trace_id=trace_id,
            metadata={
                "revision": 1,
                "task_type": task.task_type,
                "request_type": task.request_type,
                "request_id": str(generation_request.id),
            },
            occurred_at=now,
        )
        await session.flush()
        return _submission_response(
            generation_request,
            task,
            reservation,
            cost_entry,
            outbox_event_id,
            replayed=False,
        )


async def get_costs(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    project_id: UUID,
    *,
    limit: int,
    offset: int,
) -> CostQueryResponse:
    await actor_context(session, claims, workspace_id, Capability.CONTENT_READ)
    project = await resolve_project_generation_context(
        session,
        workspace_id,
        project_id,
    )
    if project is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Project not found", status_code=404)
    rows, total = await repository.list_project_cost_entries(
        session,
        workspace_id,
        project_id,
        limit=limit,
        offset=offset,
    )
    totals = await repository.project_cost_totals(session, workspace_id, project_id)
    reserved = decimal_amount(totals.get("reserve", Decimal("0")))
    settled = decimal_amount(totals.get("settle", Decimal("0")))
    released = decimal_amount(totals.get("release", Decimal("0")))
    adjustments = decimal_amount(totals.get("adjust", Decimal("0")))
    remaining = decimal_amount(reserved + adjustments - settled - released)
    return CostQueryResponse(
        currency=project.currency,
        summary=CostSummaryResponse(
            reserved=reserved,
            settled=settled,
            released=released,
            adjustments=adjustments,
            remaining_reserved=remaining,
        ),
        items=[
            cost_entry_response(entry, reservation, request, task)
            for entry, reservation, request, task in rows
        ],
        total=total,
        limit=limit,
        offset=offset,
    )


async def _authorize_high_cost_submission(
    guard: HighCostGuardPort,
    settings: Settings,
    request: GenerationSubmissionRequest,
    request_hash: str,
    evaluation: _PreflightEvaluation,
) -> None:
    if not evaluation.high_cost_required:
        return
    try:
        decision = await guard.authorize_high_cost(
            HighCostGuardRequest(
                workspace_id=request.workspace_id,
                idempotency_digest=hashlib.sha256(
                    request.idempotency_key.encode("utf-8")
                ).hexdigest(),
                request_hash=request_hash,
                workspace_limit=settings.generation_high_cost_workspace_limit,
                global_limit=settings.generation_high_cost_global_limit,
                window_seconds=settings.generation_high_cost_window_seconds,
                idempotency_ttl_seconds=(
                    settings.generation_high_cost_idempotency_ttl_seconds
                ),
            )
        )
    except CacheUnavailableError as error:
        raise ApiError(
            ErrorCode.DEPENDENCY_UNAVAILABLE,
            "High cost protection is unavailable",
            status_code=503,
            next_action="retry_when_high_cost_protection_recovers",
        ) from error
    if decision.allowed:
        return
    if decision.outcome == "idempotency_conflict":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Idempotency key is already bound to different generation input",
            status_code=409,
            next_action="use_new_idempotency_key",
        )
    if decision.outcome in {"workspace_limit", "global_limit"}:
        raise ApiError(
            ErrorCode.QUOTA_INSUFFICIENT,
            "High cost generation limit is temporarily exhausted",
            status_code=422,
            next_action="retry_after_high_cost_window",
            details={
                "scope": (
                    "workspace" if decision.outcome == "workspace_limit" else "global"
                ),
                "retry_after_seconds": decision.retry_after_seconds,
            },
        )
    raise ApiError(
        ErrorCode.DEPENDENCY_UNAVAILABLE,
        "High cost protection returned an invalid decision",
        status_code=503,
        next_action="retry_when_high_cost_protection_recovers",
    )


async def _evaluate_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    settings: Settings,
    shot_id: UUID,
    request: GenerationPreflightRequest | GenerationSubmissionRequest,
    *,
    expires_at: datetime,
    for_update: bool,
    authorization: Capability,
    known_actor: ActorContext | None = None,
) -> _PreflightEvaluation:
    actor = known_actor or await actor_context(
        session,
        claims,
        request.workspace_id,
        authorization,
    )
    snapshot = await get_production_snapshot(
        session,
        request.workspace_id,
        request.shot_spec_version_id,
    )
    if snapshot is None or snapshot.spec_ref.shot_id != shot_id:
        raise ApiError(ErrorCode.NOT_FOUND, "Shot specification not found", status_code=404)
    project = await resolve_episode_generation_context(
        session,
        request.workspace_id,
        snapshot.spec_ref.episode_id,
        for_update=for_update,
    )
    if project is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Shot specification not found", status_code=404)
    if for_update:
        locked_snapshot = await get_production_snapshot(
            session,
            request.workspace_id,
            request.shot_spec_version_id,
            for_update=True,
        )
        if locked_snapshot is None or locked_snapshot.spec_ref.shot_id != shot_id:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Shot specification changed during submission",
                status_code=409,
                next_action="rerun_generation_preflight",
            )
        snapshot = locked_snapshot

    capability = await _resolve_capability(
        session,
        request.capability_id,
        for_update=for_update,
    )
    if capability is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Model capability not found", status_code=404)
    try:
        normalized_parameters = validate_capability_parameters(
            capability.parameter_schema,
            request.parameters,
        )
    except ValueError as error:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Generation parameters are invalid",
            status_code=422,
            details={"reason": str(error)},
        ) from error

    blockers: list[GenerationBlocker] = []
    if snapshot.shot_status != "active":
        blockers.append(
            GenerationBlocker(
                code="SHOT_ARCHIVED",
                summary="The shot is archived",
                next_action="restore_shot",
            )
        )
    if snapshot.current_spec_version_id != request.shot_spec_version_id:
        blockers.append(
            GenerationBlocker(
                code="SHOT_SPEC_NOT_CURRENT",
                summary="The selected shot specification is no longer current",
                next_action="reload_shot_specification",
            )
        )
    for code in snapshot.blocking_codes:
        blockers.append(
            GenerationBlocker(
                code=code,
                summary="Shot production readiness is blocked",
                next_action="resolve_shot_readiness",
            )
        )
    if project.project_status != "active" or project.episode_status != "active":
        blockers.append(
            GenerationBlocker(
                code="PRODUCTION_SCOPE_ARCHIVED",
                summary="The project or episode is archived",
                next_action="restore_production_scope",
            )
        )
    if capability.status != "active":
        blockers.append(
            GenerationBlocker(
                code="CAPABILITY_UNAVAILABLE",
                summary="The selected model capability is unavailable",
                next_action="select_available_capability",
            )
        )

    amount: Decimal | None = None
    currency: str | None = None
    threshold: Decimal | None = None
    if capability.status == "active":
        try:
            if capability.pricing is None:
                raise ValueError("pricing is unavailable")
            amount, currency, threshold = estimate_fixed_request_cost(capability.pricing)
        except ValueError:
            blockers.append(
                GenerationBlocker(
                    code="CAPABILITY_CONFIGURATION_INVALID",
                    summary="The capability pricing contract is not supported",
                    next_action="select_available_capability",
                )
            )
            amount = None
            currency = None
            threshold = None
    active_reserved = await repository.active_reserved_amount(
        session,
        request.workspace_id,
        project.project_id,
    )
    if amount is not None and currency is not None:
        if currency != project.currency:
            blockers.append(
                GenerationBlocker(
                    code="COST_CURRENCY_MISMATCH",
                    summary="Capability and project currencies do not match",
                    next_action="select_available_capability",
                )
            )
        elif amount > max(project.budget_limit - active_reserved, Decimal("0")):
            blockers.append(
                GenerationBlocker(
                    code="BUDGET_INSUFFICIENT",
                    summary="The project budget is insufficient",
                    next_action="update_project_budget",
                )
            )

    warning_codes = sorted(set(snapshot.warning_codes))
    high_cost_required = amount is not None and threshold is not None and amount >= threshold
    confirmations: list[GenerationConfirmationRequirement] = []
    if warning_codes:
        confirmations.append(
            GenerationConfirmationRequirement(
                code="ACKNOWLEDGE_WARNINGS",
                warning_codes=warning_codes,
            )
        )
    if high_cost_required:
        confirmations.append(
            GenerationConfirmationRequirement(
                code="CONFIRM_HIGH_COST",
                warning_codes=[],
            )
        )
    estimated_cost = (
        EstimatedCostResponse(
            amount=amount,
            currency=currency,
            pricing_version=capability.config_version,
        )
        if amount is not None and currency is not None
        else None
    )
    status: Literal["ready", "blocked", "unavailable"] = (
        "ready"
        if not blockers
        else "unavailable"
        if snapshot.readiness_status == "unavailable"
        else "blocked"
    )
    facts = {
        "workspace_id": str(request.workspace_id),
        "project_id": str(project.project_id),
        "project_revision": project.project_revision,
        "episode_id": str(snapshot.spec_ref.episode_id),
        "episode_revision": project.episode_revision,
        "shot_id": str(shot_id),
        "shot_revision": snapshot.shot_revision,
        "shot_spec_version_id": str(request.shot_spec_version_id),
        "shot_spec_input_hash": snapshot.spec_ref.input_hash,
        "current_spec_version_id": (
            str(snapshot.current_spec_version_id)
            if snapshot.current_spec_version_id is not None
            else None
        ),
        "readiness_evaluation_hash": snapshot.evaluation_hash,
        "capability_id": str(capability.id),
        "capability_config_version": capability.config_version,
        "capability_status": capability.status,
        "parameters": normalized_parameters,
        "warning_codes": warning_codes,
        "estimated_amount": str(amount) if amount is not None else None,
        "currency": currency,
        "project_budget_limit": str(decimal_amount(project.budget_limit)),
        "active_reserved": str(decimal_amount(active_reserved)),
        "blocker_codes": [item.code for item in blockers],
    }
    normalized_expiry = _utc_datetime(expires_at)
    signature = generation_preflight_signature(settings, facts, normalized_expiry)
    return _PreflightEvaluation(
        actor=actor,
        snapshot=snapshot,
        project=project,
        capability=capability,
        normalized_parameters=normalized_parameters,
        estimated_amount=amount,
        currency=currency,
        high_cost_required=high_cost_required,
        facts=facts,
        response=GenerationPreflightResponse(
            shot_id=shot_id,
            shot_spec_version_id=request.shot_spec_version_id,
            capability_id=capability.id,
            status=status,
            ready=not blockers,
            blocking_reasons=blockers,
            warning_codes=warning_codes,
            confirmation_requirements=confirmations,
            estimated_cost=estimated_cost,
            preflight_hash=signature,
            expires_at=normalized_expiry,
        ),
    )


def _require_submittable_preflight(
    settings: Settings,
    request: GenerationSubmissionRequest,
    evaluation: _PreflightEvaluation,
) -> None:
    if not verify_generation_preflight_signature(
        settings,
        evaluation.facts,
        request.preflight_expires_at,
        request.preflight_hash,
        now=datetime.now(UTC),
    ):
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Generation preflight is expired or no longer matches current facts",
            status_code=409,
            next_action="rerun_generation_preflight",
        )
    if not evaluation.response.ready:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Generation preflight is blocked",
            status_code=409,
            next_action="resolve_generation_blockers",
            details={
                "blocking_reasons": [
                    item.model_dump(mode="json") for item in evaluation.response.blocking_reasons
                ]
            },
        )
    if set(request.warning_acknowledgements) != set(evaluation.response.warning_codes):
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Generation warnings must be acknowledged",
            status_code=409,
            next_action="acknowledge_generation_warnings",
        )
    if evaluation.high_cost_required and not request.high_cost_confirmed:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "High generation cost must be confirmed",
            status_code=409,
            next_action="confirm_generation_cost",
        )


async def _resolve_capability(
    session: AsyncSession,
    capability_id: UUID,
    *,
    for_update: bool,
) -> _ResolvedCapability | None:
    persisted = await repository.find_capability(
        session,
        capability_id,
        for_update=for_update,
    )
    if persisted is not None:
        return _resolved_persisted_capability(persisted)
    definition = builtin_capability(capability_id)
    return None if definition is None else _resolved_builtin_capability(definition)


def _resolved_persisted_capability(capability: ModelCapability) -> _ResolvedCapability:
    return _ResolvedCapability(
        id=capability.id,
        provider=capability.provider,
        model=capability.model,
        kind=cast(Literal["image", "video"], capability.kind),
        config_version=capability.config_version,
        input_types=list(capability.input_types),
        parameter_schema=capability.parameter_schema,
        limits=capability.limits,
        pricing=capability.pricing,
        status=cast(Literal["active", "inactive", "unavailable"], capability.status),
        unavailable_reason=capability.unavailable_reason,
        persisted=True,
    )


def _resolved_builtin_capability(definition: CapabilityDefinition) -> _ResolvedCapability:
    return _ResolvedCapability(
        id=definition.id,
        provider=definition.provider,
        model=definition.model,
        kind=definition.kind,
        config_version=definition.config_version,
        input_types=list(definition.input_types),
        parameter_schema=definition.parameter_schema,
        limits=definition.limits,
        pricing=definition.pricing,
        status=definition.status,
        unavailable_reason=definition.unavailable_reason,
        persisted=False,
    )


def _capability_response(capability: _ResolvedCapability) -> ModelCapabilityResponse:
    pricing: CapabilityPricingResponse | None = None
    if capability.pricing is not None:
        try:
            amount, currency, threshold = estimate_fixed_request_cost(capability.pricing)
        except ValueError:
            pass
        else:
            pricing = CapabilityPricingResponse(
                unit="per_request",
                amount=amount,
                currency=currency,
                high_cost_threshold=threshold,
            )
    return ModelCapabilityResponse(
        id=capability.id,
        provider=capability.provider,
        model=capability.model,
        kind=capability.kind,
        config_version=capability.config_version,
        input_types=capability.input_types,
        parameter_schema=capability.parameter_schema,
        limits=capability.limits,
        pricing=pricing,
        status=capability.status,
        unavailable_reason=capability.unavailable_reason,
    )


def _generation_request_hash(shot_id: UUID, request: GenerationSubmissionRequest) -> str:
    return hashlib.sha256(
        _canonical_json(
            {
                "workspace_id": str(request.workspace_id),
                "shot_id": str(shot_id),
                "shot_spec_version_id": str(request.shot_spec_version_id),
                "capability_id": str(request.capability_id),
                "parameters": request.parameters,
                "warning_acknowledgements": request.warning_acknowledgements,
                "high_cost_confirmed": request.high_cost_confirmed,
            }
        ).encode("utf-8")
    ).hexdigest()


async def _replay_or_conflict(
    session: AsyncSession,
    request: GenerationRequest,
    request_hash: str,
) -> GenerationSubmissionResponse:
    if request.input_hash != request_hash:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Idempotency key is already used for different generation input",
            status_code=409,
            next_action="use_new_idempotency_key",
        )
    facts = await repository.generation_submission_facts(session, request.id)
    if facts is None:
        raise RuntimeError("generation submission facts are unavailable")
    generation_request, task, reservation, cost_entry = facts
    outbox_event_id = await find_outbox_event_id(
        session,
        aggregate_id=task.id,
        event_type="generation.requested",
    )
    if outbox_event_id is None:
        raise RuntimeError("generation outbox fact is unavailable")
    return _submission_response(
        generation_request,
        task,
        reservation,
        cost_entry,
        outbox_event_id,
        replayed=True,
    )


def _submission_response(
    request: GenerationRequest,
    task: Task,
    reservation: Reservation,
    cost_entry: CostEntry,
    outbox_event_id: UUID,
    *,
    replayed: bool,
) -> GenerationSubmissionResponse:
    return GenerationSubmissionResponse(
        request=generation_request_response(request),
        task=task_response(task),
        reservation=reservation_response(reservation),
        initial_cost_entry=cost_entry_response(cost_entry, reservation, request, task),
        outbox_event_id=outbox_event_id,
        replayed=replayed,
    )


def _canonical_json(value: object) -> str:
    return json.dumps(
        value,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )


def _utc_datetime(value: datetime) -> datetime:
    if value.tzinfo is None:
        raise ValueError("datetime must include a timezone")
    return value.astimezone(UTC)
