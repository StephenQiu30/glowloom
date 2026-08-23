from collections import defaultdict
from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.projects import (
    EpisodeContentContext,
    episode_for_content_read,
    lock_active_episode_for_content_write,
)
from app.modules.scripts import resolve_storyboard_narrative
from app.modules.scripts.contracts import StoryboardNarrativeSnapshot
from app.modules.storyboards import repository as storyboard_repository
from app.modules.storyboards.coverage import repository
from app.modules.storyboards.coverage.models import (
    CoverageDecision,
    ShotNarrativeReference,
)
from app.modules.storyboards.coverage.rules import required_channel
from app.modules.storyboards.coverage.schemas import (
    CoverageDecisionApplyResponse,
    CoverageDecisionRequest,
    CoverageDecisionResponse,
    CoverageReportResponse,
    CoverageSummaryResponse,
    NarrativeReferenceInput,
    NarrativeReferenceReplaceRequest,
    NarrativeReferenceReplaceResponse,
    NarrativeReferenceResponse,
    ShotCoverageResponse,
    UnitCoverageResponse,
)
from app.modules.storyboards.hashing import canonical_payload_hash
from app.modules.storyboards.models import AssetReference, Shot, ShotSpecVersion


def _reference_response(value: ShotNarrativeReference) -> NarrativeReferenceResponse:
    return NarrativeReferenceResponse(
        id=value.id,
        shot_id=value.shot_id,
        shot_spec_version_id=value.shot_spec_version_id,
        narrative_unit_id=value.narrative_unit_id,
        unit_version_id=value.unit_version_id,
        channel=cast(Literal["visual", "audio", "both"], value.channel),
        role=cast(
            Literal[
                "primary",
                "dialogue",
                "reaction",
                "insert",
                "setup",
                "payoff",
                "transition",
                "supporting",
            ],
            value.role,
        ),
        coverage_mode=cast(Literal["full", "partial"], value.coverage_mode),
        segment_start=value.segment_start,
        segment_end=value.segment_end,
        contribution=cast(Literal["required", "supporting"], value.contribution),
        origin=cast(Literal["ai", "human", "migrated"], value.origin),
        created_by=value.created_by,
        created_at=value.created_at,
    )


def _decision_response(value: CoverageDecision) -> CoverageDecisionResponse:
    return CoverageDecisionResponse(
        id=value.id,
        episode_id=value.episode_id,
        sequence=value.sequence,
        action=cast(
            Literal[
                "approve_omission",
                "revoke_omission",
                "approve_invented",
                "revoke_invented",
            ],
            value.action,
        ),
        unit_version_id=value.unit_version_id,
        shot_spec_version_id=value.shot_spec_version_id,
        basis_hash=value.basis_hash,
        reason=value.reason,
        evidence=value.evidence,
        idempotency_key=value.idempotency_key,
        actor_id=value.actor_id,
        created_at=value.created_at,
    )


def _segment_key(value: NarrativeReferenceInput) -> str:
    if value.coverage_mode == "full":
        return "full"
    return f"{value.segment_start}:{value.segment_end}"


def _channel_matches(actual: str, required: str) -> bool:
    return actual == required or actual == "both"


def _intervals_cover(length: int, references: list[ShotNarrativeReference]) -> bool:
    if any(reference.coverage_mode == "full" for reference in references):
        return True
    intervals = sorted(
        (
            reference.segment_start,
            reference.segment_end,
        )
        for reference in references
        if reference.segment_start is not None and reference.segment_end is not None
    )
    cursor = 0
    for start, end in intervals:
        if start > cursor:
            return False
        cursor = max(cursor, end)
        if cursor >= length:
            return True
    return cursor >= length


def _basis_hash(
    episode: EpisodeContentContext,
    narrative: StoryboardNarrativeSnapshot,
    rows: list[tuple[Shot, ShotSpecVersion | None]],
    references: list[ShotNarrativeReference],
) -> str:
    return canonical_payload_hash(
        {
            "episode_id": str(episode.episode_id),
            "script_version_id": str(narrative.script_version_id),
            "narrative_dependency_hash": narrative.dependency_hash,
            "units": [
                {
                    "id": str(unit.unit_version_id),
                    "text_hash": unit.text_hash,
                    "kind": unit.kind,
                    "required": unit.required_for_coverage,
                }
                for unit in narrative.units
            ],
            "shots": [
                {
                    "id": str(shot.id),
                    "revision": shot.revision,
                    "spec_version_id": str(version.id) if version is not None else None,
                }
                for shot, version in rows
            ],
            "references": [
                {
                    "id": str(reference.id),
                    "shot_id": str(reference.shot_id),
                    "spec_version_id": str(reference.shot_spec_version_id),
                    "unit_version_id": str(reference.unit_version_id),
                    "channel": reference.channel,
                    "role": reference.role,
                    "coverage_mode": reference.coverage_mode,
                    "segment_key": reference.segment_key,
                    "contribution": reference.contribution,
                    "origin": reference.origin,
                }
                for reference in references
            ],
        }
    )


def _unavailable_report(episode_id: UUID) -> CoverageReportResponse:
    basis_hash = canonical_payload_hash({"episode_id": str(episode_id), "status": "unavailable"})
    summary = CoverageSummaryResponse(
        required_total=0,
        covered=0,
        approved_omitted=0,
        uncovered=0,
        shots_total=0,
        linked=0,
        approved_invented=0,
        orphan=0,
        stale=0,
    )
    evaluation_hash = canonical_payload_hash({"basis_hash": basis_hash, "status": "unavailable"})
    return CoverageReportResponse(
        episode_id=episode_id,
        status="unavailable",
        ready=False,
        basis_hash=basis_hash,
        evaluation_hash=evaluation_hash,
        summary=summary,
        units=[],
        shots=[],
        references=[],
        stale_reference_ids=[],
        stale_decision_ids=[],
        next_actions=["retry_coverage"],
    )


def unavailable_report(episode_id: UUID) -> CoverageReportResponse:
    return _unavailable_report(episode_id)


def _latest_decisions(
    decisions: list[CoverageDecision],
) -> dict[tuple[str, UUID], CoverageDecision]:
    latest: dict[tuple[str, UUID], CoverageDecision] = {}
    for decision in decisions:
        if decision.narrative_unit_id is not None:
            latest[("unit", decision.narrative_unit_id)] = decision
        elif decision.shot_id is not None:
            latest[("shot", decision.shot_id)] = decision
    return latest


def _evaluate_report(
    episode: EpisodeContentContext,
    narrative: StoryboardNarrativeSnapshot,
    rows: list[tuple[Shot, ShotSpecVersion | None]],
    references: list[ShotNarrativeReference],
    decisions: list[CoverageDecision],
) -> CoverageReportResponse:
    basis_hash = _basis_hash(episode, narrative, rows, references)
    current_units = {unit.unit_version_id: unit for unit in narrative.units}
    current_specs = {version.id for _shot, version in rows if version is not None}
    spec_to_shot = {version.id: shot.id for shot, version in rows if version is not None}
    shot_position = {shot.id: shot.position for shot, _version in rows}

    stale_reference_ids: list[UUID] = []
    valid_references: list[ShotNarrativeReference] = []
    primary_keys: dict[tuple[UUID, str, str], UUID] = {}
    for reference in references:
        key = (reference.unit_version_id, reference.channel, reference.segment_key)
        invalid = (
            reference.unit_version_id not in current_units
            or reference.shot_spec_version_id not in current_specs
            or spec_to_shot.get(reference.shot_spec_version_id) != reference.shot_id
        )
        if reference.role == "primary":
            if key in primary_keys:
                invalid = True
                stale_reference_ids.append(primary_keys[key])
            else:
                primary_keys[key] = reference.id
        if invalid:
            stale_reference_ids.append(reference.id)
        else:
            valid_references.append(reference)
    stale_reference_ids = list(dict.fromkeys(stale_reference_ids))
    invalid_ids = set(stale_reference_ids)
    valid_references = [
        reference for reference in valid_references if reference.id not in invalid_ids
    ]

    latest = _latest_decisions(decisions)
    stale_decision_ids = [
        decision.id
        for decision in latest.values()
        if decision.basis_hash != basis_hash
        or (decision.unit_version_id is not None and decision.unit_version_id not in current_units)
        or (
            decision.shot_spec_version_id is not None
            and decision.shot_spec_version_id not in current_specs
        )
    ]

    refs_by_unit: dict[UUID, list[ShotNarrativeReference]] = defaultdict(list)
    refs_by_shot: dict[UUID, list[ShotNarrativeReference]] = defaultdict(list)
    for reference in valid_references:
        refs_by_unit[reference.unit_version_id].append(reference)
        refs_by_shot[reference.shot_id].append(reference)

    unit_items: list[UnitCoverageResponse] = []
    for unit in narrative.units:
        unit_required_channel = required_channel(unit.kind)
        required_references = [
            reference
            for reference in refs_by_unit[unit.unit_version_id]
            if reference.contribution == "required"
            and _channel_matches(reference.channel, unit_required_channel)
        ]
        covered = _intervals_cover(len(unit.exact_text), required_references)
        decision = latest.get(("unit", unit.narrative_unit_id))
        omission_approved = (
            decision is not None
            and decision.basis_hash == basis_hash
            and decision.action == "approve_omission"
        )
        unit_status: Literal["covered", "approved_omitted", "uncovered"]
        if covered:
            unit_status = "covered"
        elif omission_approved:
            unit_status = "approved_omitted"
        else:
            unit_status = "uncovered"
        unit_items.append(
            UnitCoverageResponse(
                narrative_unit_id=unit.narrative_unit_id,
                unit_version_id=unit.unit_version_id,
                position=unit.position,
                kind=unit.kind,
                exact_text=unit.exact_text,
                required_for_coverage=unit.required_for_coverage,
                required_channel=unit_required_channel,
                status=unit_status,
                shot_ids=sorted(
                    {reference.shot_id for reference in refs_by_unit[unit.unit_version_id]},
                    key=lambda shot_id: (shot_position.get(shot_id, 0), str(shot_id)),
                ),
            )
        )

    shot_items: list[ShotCoverageResponse] = []
    for shot, version in rows:
        shot_references = refs_by_shot[shot.id]
        decision = latest.get(("shot", shot.id))
        invented_approved = (
            decision is not None
            and decision.basis_hash == basis_hash
            and decision.action == "approve_invented"
        )
        shot_status: Literal["linked", "approved_invented", "orphan"]
        if shot_references:
            shot_status = "linked"
        elif invented_approved:
            shot_status = "approved_invented"
        else:
            shot_status = "orphan"
        shot_items.append(
            ShotCoverageResponse(
                shot_id=shot.id,
                spec_version_id=version.id if version is not None else None,
                position=shot.position,
                title=shot.title,
                status=shot_status,
                unit_version_ids=list(
                    dict.fromkeys(reference.unit_version_id for reference in shot_references)
                ),
            )
        )

    required_items = [item for item in unit_items if item.required_for_coverage]
    summary = CoverageSummaryResponse(
        required_total=len(required_items),
        covered=sum(item.status == "covered" for item in required_items),
        approved_omitted=sum(item.status == "approved_omitted" for item in required_items),
        uncovered=sum(item.status == "uncovered" for item in required_items),
        shots_total=len(shot_items),
        linked=sum(item.status == "linked" for item in shot_items),
        approved_invented=sum(item.status == "approved_invented" for item in shot_items),
        orphan=sum(item.status == "orphan" for item in shot_items),
        stale=len(stale_reference_ids) + len(stale_decision_ids),
    )
    ready = summary.uncovered == 0 and summary.orphan == 0 and summary.stale == 0
    next_actions: list[str] = []
    if summary.uncovered:
        next_actions.append("map_or_omit_narrative_units")
    if summary.orphan:
        next_actions.append("map_or_approve_invented_shots")
    if summary.stale:
        next_actions.append("review_stale_coverage")
    effective_decisions = sorted(
        (decision for decision in latest.values() if decision.basis_hash == basis_hash),
        key=lambda decision: (decision.sequence, str(decision.id)),
    )
    evaluation_hash = canonical_payload_hash(
        {
            "basis_hash": basis_hash,
            "decisions": [
                {
                    "id": str(decision.id),
                    "action": decision.action,
                    "sequence": decision.sequence,
                }
                for decision in effective_decisions
            ],
            "stale_reference_ids": [str(value) for value in stale_reference_ids],
            "stale_decision_ids": [str(value) for value in stale_decision_ids],
        }
    )
    return CoverageReportResponse(
        episode_id=episode.episode_id,
        status="ready" if ready else "blocked",
        ready=ready,
        basis_hash=basis_hash,
        evaluation_hash=evaluation_hash,
        summary=summary,
        units=unit_items,
        shots=shot_items,
        references=[_reference_response(reference) for reference in references],
        stale_reference_ids=stale_reference_ids,
        stale_decision_ids=stale_decision_ids,
        next_actions=next_actions,
    )


async def _report_for_context(
    session: AsyncSession,
    episode: EpisodeContentContext,
) -> CoverageReportResponse:
    if episode.current_script_version_id is None:
        return _unavailable_report(episode.episode_id)
    try:
        narrative = await resolve_storyboard_narrative(
            session,
            episode.workspace_id,
            episode.current_script_version_id,
        )
        if narrative is None:
            return _unavailable_report(episode.episode_id)
        rows = await storyboard_repository.list_active_shots_with_current_specs(
            session,
            episode.episode_id,
        )
        references = await repository.list_references(
            session,
            [version.id for _shot, version in rows if version is not None],
        )
        decisions = await repository.list_decisions(session, episode.episode_id)
    except SQLAlchemyError:
        return _unavailable_report(episode.episode_id)
    return _evaluate_report(episode, narrative, rows, references, decisions)


async def get_report(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> CoverageReportResponse:
    episode = await episode_for_content_read(session, claims, episode_id)
    return await _report_for_context(session, episode)


async def resolve_report(
    session: AsyncSession,
    episode: EpisodeContentContext,
) -> CoverageReportResponse:
    """Resolve coverage for an already authorized episode context."""
    return await _report_for_context(session, episode)


def validate_reference_inputs(
    references: list[NarrativeReferenceInput],
    report: CoverageReportResponse,
    *,
    shot_id: UUID,
) -> dict[UUID, UnitCoverageResponse]:
    units = {unit.unit_version_id: unit for unit in report.units}
    for reference in references:
        unit = units.get(reference.unit_version_id)
        if unit is None:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Narrative unit version does not belong to the current episode structure",
                status_code=422,
                details={"reason": "unit_version_outside_episode"},
            )
        if (
            reference.coverage_mode == "partial"
            and reference.segment_end is not None
            and reference.segment_end > len(unit.exact_text)
        ):
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Narrative segment exceeds the fixed unit text",
                status_code=422,
                details={"reason": "segment_out_of_range"},
            )
    existing = {
        (
            reference.unit_version_id,
            reference.channel,
            reference.segment_start,
            reference.segment_end,
        )
        for reference in report.references
        if reference.shot_id != shot_id
        and reference.role == "primary"
        and reference.id not in set(report.stale_reference_ids)
    }
    duplicate = next(
        (
            reference
            for reference in references
            if reference.role == "primary"
            and (
                reference.unit_version_id,
                reference.channel,
                reference.segment_start,
                reference.segment_end,
            )
            in existing
        ),
        None,
    )
    if duplicate is not None:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "A primary narrative segment is already assigned to another shot",
            status_code=409,
            details={"reason": "duplicate_primary_reference"},
        )
    return units


async def replace_references(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    request: NarrativeReferenceReplaceRequest,
    *,
    trace_id: str,
) -> NarrativeReferenceReplaceResponse:
    async with session.begin():
        initial = await storyboard_repository.find_shot(session, shot_id)
        if initial is None:
            raise ApiError(ErrorCode.NOT_FOUND, "Shot not found", status_code=404)
        episode = await lock_active_episode_for_content_write(
            session,
            claims,
            initial.episode_id,
        )
        shot = await storyboard_repository.find_shot(session, shot_id, for_update=True)
        if shot is None:
            raise ApiError(ErrorCode.NOT_FOUND, "Shot not found", status_code=404)
        if shot.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Narrative references require an active shot",
                status_code=409,
            )
        if shot.revision != request.expected_shot_revision:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Shot has changed",
                status_code=409,
                details={"current_revision": shot.revision},
            )
        if shot.current_spec_version_id != request.expected_current_spec_version_id:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Current shot spec version has changed",
                status_code=409,
                details={
                    "current_spec_version_id": (
                        str(shot.current_spec_version_id)
                        if shot.current_spec_version_id is not None
                        else None
                    )
                },
            )
        report = await _report_for_context(session, episode)
        if report.evaluation_hash != request.expected_evaluation_hash:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Coverage evaluation has changed",
                status_code=409,
                details={"current_evaluation_hash": report.evaluation_hash},
            )
        if report.status == "unavailable":
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Coverage dependencies are unavailable",
                status_code=503,
                next_action="retry_coverage",
            )
        units = validate_reference_inputs(
            request.references,
            report,
            shot_id=shot.id,
        )
        previous_result = await storyboard_repository.find_spec_version(
            session,
            request.expected_current_spec_version_id,
            for_update=True,
        )
        if previous_result is None or previous_result[0].shot_id != shot.id:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Current shot spec version is unavailable",
                status_code=409,
            )
        previous = previous_result[0]
        asset_references = await storyboard_repository.list_asset_references(
            session,
            [previous.id],
        )
        now = datetime.now(UTC)
        version = ShotSpecVersion(
            id=uuid7(),
            workspace_id=shot.workspace_id,
            shot_id=shot.id,
            version_no=(await storyboard_repository.latest_spec_version_number(session, shot.id))
            + 1,
            schema_version=previous.schema_version,
            spec=previous.spec,
            content_hash=previous.content_hash,
            input_hash=previous.input_hash,
            created_by=claims.sub,
            created_at=now,
        )
        session.add(version)
        await session.flush()
        cloned_assets = [
            AssetReference(
                id=uuid7(),
                workspace_id=value.workspace_id,
                shot_spec_version_id=version.id,
                slot_key=value.slot_key,
                role=value.role,
                asset_version_id=value.asset_version_id,
                asset_state_id=value.asset_state_id,
                asset_id=value.asset_id,
                binding_source=value.binding_source,
                subject_key=value.subject_key,
                created_at=now,
            )
            for value in asset_references
        ]
        narrative_references = [
            ShotNarrativeReference(
                id=uuid7(),
                workspace_id=shot.workspace_id,
                episode_id=shot.episode_id,
                shot_id=shot.id,
                shot_spec_version_id=version.id,
                narrative_unit_id=units[value.unit_version_id].narrative_unit_id,
                unit_version_id=value.unit_version_id,
                channel=value.channel,
                role=value.role,
                coverage_mode=value.coverage_mode,
                segment_start=value.segment_start,
                segment_end=value.segment_end,
                segment_key=_segment_key(value),
                contribution=value.contribution,
                origin="human",
                created_by=claims.sub,
                created_at=now,
            )
            for value in request.references
        ]
        session.add_all([*cloned_assets, *narrative_references])
        shot.current_spec_version_id = version.id
        shot.revision += 1
        append_audit_event(
            session,
            workspace_id=shot.workspace_id,
            actor_id=claims.sub,
            action="shot.narrative_references_replaced",
            target_type="shot_spec_version",
            target_id=version.id,
            trace_id=trace_id,
            metadata={
                "shot_id": str(shot.id),
                "episode_id": str(shot.episode_id),
                "previous_spec_version_id": str(previous.id),
                "reference_count": len(narrative_references),
                "shot_revision": shot.revision,
            },
            occurred_at=now,
        )
        await session.flush()
        updated_report = await _report_for_context(session, episode)
        result = NarrativeReferenceReplaceResponse(
            shot_id=shot.id,
            previous_spec_version_id=previous.id,
            current_spec_version_id=version.id,
            shot_revision=shot.revision,
            references=[_reference_response(value) for value in narrative_references],
            report=updated_report,
        )
    return result


async def decide_coverage(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: CoverageDecisionRequest,
    *,
    trace_id: str,
) -> CoverageDecisionApplyResponse:
    command_hash = canonical_payload_hash(request.model_dump(mode="json"))
    async with session.begin():
        episode = await lock_active_episode_for_content_write(
            session,
            claims,
            episode_id,
        )
        existing = await repository.find_decision_by_key(
            session,
            episode.workspace_id,
            request.idempotency_key,
        )
        if existing is not None:
            if existing.command_hash != command_hash:
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different coverage input",
                    status_code=409,
                )
            return CoverageDecisionApplyResponse(
                decision=_decision_response(existing),
                report=await _report_for_context(session, episode),
            )
        report = await _report_for_context(session, episode)
        if report.evaluation_hash != request.expected_evaluation_hash:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Coverage evaluation has changed",
                status_code=409,
                details={"current_evaluation_hash": report.evaluation_hash},
            )
        if report.status == "unavailable":
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Coverage dependencies are unavailable",
                status_code=503,
                next_action="retry_coverage",
            )

        unit_by_version = {unit.unit_version_id: unit for unit in report.units}
        shot_by_spec = {
            shot.spec_version_id: shot for shot in report.shots if shot.spec_version_id is not None
        }
        narrative_unit_id: UUID | None = None
        shot_id: UUID | None = None
        if request.unit_version_id is not None:
            unit = unit_by_version.get(request.unit_version_id)
            if unit is None:
                raise ApiError(
                    ErrorCode.VALIDATION_FAILED,
                    "Coverage decision unit is not current",
                    status_code=422,
                    details={"reason": "unit_version_outside_episode"},
                )
            narrative_unit_id = unit.narrative_unit_id
        if request.shot_spec_version_id is not None:
            shot = shot_by_spec.get(request.shot_spec_version_id)
            if shot is None:
                raise ApiError(
                    ErrorCode.VALIDATION_FAILED,
                    "Coverage decision shot spec is not current",
                    status_code=422,
                    details={"reason": "shot_spec_outside_episode"},
                )
            shot_id = shot.shot_id
        now = datetime.now(UTC)
        decision = CoverageDecision(
            id=uuid7(),
            workspace_id=episode.workspace_id,
            episode_id=episode.episode_id,
            sequence=await repository.next_decision_sequence(
                session,
                episode.episode_id,
            ),
            action=request.action,
            narrative_unit_id=narrative_unit_id,
            unit_version_id=request.unit_version_id,
            shot_id=shot_id,
            shot_spec_version_id=request.shot_spec_version_id,
            basis_hash=report.basis_hash,
            reason=request.reason,
            evidence=request.evidence,
            command_hash=command_hash,
            idempotency_key=request.idempotency_key,
            actor_id=claims.sub,
            created_at=now,
        )
        session.add(decision)
        append_audit_event(
            session,
            workspace_id=episode.workspace_id,
            actor_id=claims.sub,
            action="storyboard.coverage_decided",
            target_type="coverage_decision",
            target_id=decision.id,
            trace_id=trace_id,
            metadata={
                "episode_id": str(episode.episode_id),
                "sequence": decision.sequence,
                "decision_action": decision.action,
                "unit_version_id": (
                    str(decision.unit_version_id) if decision.unit_version_id is not None else None
                ),
                "shot_spec_version_id": (
                    str(decision.shot_spec_version_id)
                    if decision.shot_spec_version_id is not None
                    else None
                ),
                "basis_hash": decision.basis_hash,
            },
            occurred_at=now,
        )
        await session.flush()
        result = CoverageDecisionApplyResponse(
            decision=_decision_response(decision),
            report=await _report_for_context(session, episode),
        )
    return result
