from collections import defaultdict
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.assets import (
    AssetVersionReadinessReference,
    AssetVersionReference,
    asset_version_for_content_read,
    resolve_asset_version,
    resolve_asset_versions_readiness,
)
from app.modules.governance.audit import append_audit_event
from app.modules.projects import (
    EpisodeContentContext,
    episode_for_content_read,
    lock_active_episode_for_content_write,
    resolve_episode_content_context,
)
from app.modules.scripts import (
    ConfirmedStructureQuery,
    EpisodeConfirmedStructureQuery,
    NarrativeDependencySnapshot,
    resolve_confirmed_shot_candidate,
    resolve_confirmed_structure,
    resolve_episode_confirmed_structures,
    resolve_narrative_dependencies,
    resolve_storyboard_narrative,
)
from app.modules.storyboards import repository
from app.modules.storyboards.conservation import (
    TransformConservationError,
    validate_merge_content,
    validate_merge_narrative,
    validate_split_content,
    validate_split_narrative,
)
from app.modules.storyboards.contracts import (
    AssetShotUsageSnapshot,
    EpisodeStoryboardSummary,
    ShotAssetReferenceSnapshot,
    ShotProductionSnapshot,
    ShotSpecRef,
    StoryboardReferenceSummary,
)
from app.modules.storyboards.coverage import repository as coverage_repository
from app.modules.storyboards.coverage.models import ShotNarrativeReference
from app.modules.storyboards.coverage.schemas import CoverageReportResponse
from app.modules.storyboards.coverage.service import (
    resolve_report as resolve_coverage_report,
)
from app.modules.storyboards.coverage.service import (
    unavailable_report as unavailable_coverage_report,
)
from app.modules.storyboards.coverage.service import validate_reference_inputs
from app.modules.storyboards.hashing import (
    canonical_payload_hash,
    shot_order_hash,
    storyboard_content_hashes,
)
from app.modules.storyboards.models import (
    AssetReference,
    Shot,
    ShotSpecVersion,
    ShotTransform,
)
from app.modules.storyboards.schemas import (
    AssetReferenceRequest,
    AssetReferenceResponse,
    AssetShotUsageResponse,
    AssetUpgradeApplyRequest,
    AssetUpgradeApplyResponse,
    AssetUpgradePreflightRequest,
    AssetUpgradePreflightResponse,
    AssetUpgradeTargetRequest,
    CopyShotRequest,
    DownstreamEvidenceResponse,
    MergePreflightRequest,
    MergeShotRequest,
    NarrativeReferenceInput,
    PaginatedAssetShotUsages,
    ShotCreateRequest,
    ShotCurrentSpecRequest,
    ShotDeleteBlocker,
    ShotDeletePreflightResponse,
    ShotDeleteResponse,
    ShotOrderResponse,
    ShotReadinessBatchResponse,
    ShotReadinessDependencies,
    ShotReadinessIssue,
    ShotReadinessResponse,
    ShotReadinessSummary,
    ShotReadinessWarning,
    ShotReorderRequest,
    ShotResponse,
    ShotSpec,
    ShotSpecCreateRequest,
    ShotSpecCreateResponse,
    ShotSpecVersionResponse,
    ShotStateRequest,
    ShotStateResponse,
    ShotTransformEvidenceResponse,
    ShotTransformPreflightResponse,
    ShotTransformResponse,
    ShotUpdateRequest,
    SplitPreflightRequest,
    SplitShotRequest,
    TargetShotSpecRequest,
)

MAX_ACTIVE_SHOTS = 120


@dataclass(frozen=True, slots=True)
class ScriptReadinessState:
    confirmed_structure_available: bool | None
    current_script_version_id: UUID | None
    narrative: NarrativeDependencySnapshot | None
    narrative_unavailable: bool


async def summarize_episode_storyboard_references(
    session: AsyncSession,
    workspace_id: UUID,
    episode_ids: list[UUID],
) -> dict[UUID, StoryboardReferenceSummary]:
    summaries = {
        episode_id: StoryboardReferenceSummary(shot_count=0, spec_version_count=0)
        for episode_id in episode_ids
    }
    for (
        episode_id,
        shot_count,
        spec_version_count,
    ) in await repository.count_storyboard_references_by_episode(
        session,
        workspace_id,
        episode_ids,
    ):
        summaries[episode_id] = StoryboardReferenceSummary(
            shot_count=shot_count,
            spec_version_count=spec_version_count,
        )
    return summaries


async def list_script_version_affected_shot_ids(
    session: AsyncSession,
    *,
    episode_id: UUID,
    current_script_version_id: UUID,
) -> list[UUID]:
    """Return active shots whose immutable source does not match the new current script."""
    return await repository.list_active_shot_ids_not_using_script_version(
        session,
        episode_id,
        current_script_version_id,
    )


def _not_found(resource: str) -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, f"{resource} not found", status_code=404)


def _shot_response(shot: Shot) -> ShotResponse:
    return ShotResponse(
        id=shot.id,
        workspace_id=shot.workspace_id,
        episode_id=shot.episode_id,
        position=shot.position,
        title=shot.title,
        source_script_version_id=shot.source_script_version_id,
        source_scene_id=shot.source_scene_id,
        source_candidate_id=shot.source_candidate_id,
        source_draft_shot_id=shot.source_draft_shot_id,
        status=cast(Literal["active", "archived"], shot.status),
        current_spec_version_id=shot.current_spec_version_id,
        revision=shot.revision,
        created_at=shot.created_at,
        updated_at=shot.updated_at,
    )


def _order_response(shots: list[Shot]) -> ShotOrderResponse:
    return ShotOrderResponse(
        items=[_shot_response(shot) for shot in shots],
        order_hash=shot_order_hash([shot.id for shot in shots]),
    )


def _reference_response(reference: AssetReference) -> AssetReferenceResponse:
    return AssetReferenceResponse(
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
        asset_state_id=reference.asset_state_id,
        asset_id=reference.asset_id,
        binding_source=cast(Literal["manual", "ai"], reference.binding_source),
        subject_key=reference.subject_key,
    )


def _spec_response(
    version: ShotSpecVersion,
    references: list[AssetReference],
) -> ShotSpecVersionResponse:
    return ShotSpecVersionResponse(
        id=version.id,
        workspace_id=version.workspace_id,
        shot_id=version.shot_id,
        version_no=version.version_no,
        schema_version=cast(Literal[1], version.schema_version),
        spec=ShotSpec.model_validate(version.spec),
        content_hash=version.content_hash,
        input_hash=version.input_hash,
        asset_references=[_reference_response(reference) for reference in references],
        created_by=version.created_by,
        created_at=version.created_at,
    )


def _transform_evidence_response(
    transform: ShotTransform,
) -> ShotTransformEvidenceResponse:
    return ShotTransformEvidenceResponse(
        id=transform.id,
        operation=cast(Literal["copy", "split", "merge"], transform.operation),
        source_shot_ids=transform.source_shot_ids,
        source_spec_version_ids=transform.source_spec_version_ids,
        result_shot_ids=transform.result_shot_ids,
        impact_hash=transform.impact_hash,
        input_hash=transform.input_hash,
        idempotency_key=transform.idempotency_key,
        actor_id=transform.actor_id,
        created_at=transform.created_at,
    )


def _transform_input_hash(
    operation: Literal["copy", "split", "merge"],
    payload: object,
) -> str:
    return canonical_payload_hash({"operation": operation, "payload": payload})


def _impact_hash(
    operation: Literal["copy", "split", "merge"],
    shots: list[Shot],
    specs: list[ShotSpecVersion],
    order_hash: str,
) -> str:
    return canonical_payload_hash(
        {
            "operation": operation,
            "sources": [
                {
                    "shot_id": str(shot.id),
                    "revision": shot.revision,
                    "status": shot.status,
                    "current_spec_version_id": (
                        str(shot.current_spec_version_id)
                        if shot.current_spec_version_id is not None
                        else None
                    ),
                }
                for shot in shots
            ],
            "source_spec_version_ids": [str(spec.id) for spec in specs],
            "order_hash": order_hash,
            "downstream_evidence": DownstreamEvidenceResponse().model_dump(mode="json"),
        }
    )


async def _current_spec(
    session: AsyncSession,
    shot: Shot,
    expected_spec_version_id: UUID,
) -> ShotSpecVersion:
    if shot.current_spec_version_id != expected_spec_version_id:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Shot spec version has changed",
            status_code=409,
            details={
                "current_spec_version_id": (
                    str(shot.current_spec_version_id)
                    if shot.current_spec_version_id is not None
                    else None
                )
            },
        )
    result = await repository.find_spec_version(session, expected_spec_version_id)
    if result is None or result[0].shot_id != shot.id:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Shot spec version is unavailable",
            status_code=409,
            next_action="save_shot_spec",
        )
    return result[0]


async def _references_for_spec(
    session: AsyncSession,
    version_id: UUID,
) -> list[AssetReference]:
    return await repository.list_asset_references(session, [version_id])


async def _transform_result(
    session: AsyncSession,
    transform: ShotTransform,
) -> ShotTransformResponse:
    shots = await repository.find_shots(session, transform.result_shot_ids)
    if len(shots) != len(transform.result_shot_ids):
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Storyboard transform result is unavailable",
            status_code=500,
        )
    versions: list[ShotSpecVersion] = []
    for shot in shots:
        if shot.current_spec_version_id is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Storyboard transform spec is unavailable",
                status_code=500,
            )
        result = await repository.find_spec_version(session, shot.current_spec_version_id)
        if result is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Storyboard transform spec is unavailable",
                status_code=500,
            )
        versions.append(result[0])
    references = await repository.list_asset_references(
        session,
        [version.id for version in versions],
    )
    by_version: dict[UUID, list[AssetReference]] = defaultdict(list)
    for reference in references:
        by_version[reference.shot_spec_version_id].append(reference)
    order = await repository.list_active_shots(session, shots[0].episode_id)
    return ShotTransformResponse(
        transform=_transform_evidence_response(transform),
        shots=[_shot_response(shot) for shot in shots],
        spec_versions=[_spec_response(version, by_version[version.id]) for version in versions],
        order=_order_response(order),
    )


async def _idempotent_transform(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    idempotency_key: str,
    input_hash: str,
) -> ShotTransformResponse | None:
    existing = await repository.find_transform_by_idempotency(
        session,
        workspace_id,
        idempotency_key,
    )
    if existing is None:
        return None
    if existing.input_hash != input_hash:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Idempotency key was used with different transform input",
            status_code=409,
        )
    return await _transform_result(session, existing)


def _require_revision(shot: Shot, expected_revision: int) -> None:
    if shot.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Shot has changed",
            status_code=409,
            details={"current_revision": shot.revision},
        )


def _require_current_spec(shot: Shot, expected_version_id: UUID | None) -> None:
    if shot.current_spec_version_id != expected_version_id:
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


def _require_order(shots: list[Shot], expected_order_hash: str) -> None:
    current_hash = shot_order_hash([shot.id for shot in shots])
    if current_hash != expected_order_hash:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Shot order has changed",
            status_code=409,
            details={
                "current_order_hash": current_hash,
                "current_shot_ids": [str(shot.id) for shot in shots],
            },
        )


def _require_capacity(shots: list[Shot], *, additional: int = 1) -> None:
    if len(shots) + additional > MAX_ACTIVE_SHOTS:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Episode has reached the active shot limit",
            status_code=409,
            next_action="archive_unused_shots",
            details={"maximum_active_shots": MAX_ACTIVE_SHOTS},
        )


async def _require_confirmed_structure(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    episode_id: UUID,
    script_version_id: UUID,
    scene_id: UUID,
    dialogue_ids: list[UUID],
) -> None:
    reference = await resolve_confirmed_structure(
        session,
        workspace_id=workspace_id,
        episode_id=episode_id,
        script_version_id=script_version_id,
        scene_id=scene_id,
        dialogue_ids=dialogue_ids,
    )
    if reference is None:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Confirmed script structure reference is invalid",
            status_code=422,
            next_action="select_confirmed_script_structure",
            details={
                "confirmed_script_version_id": str(script_version_id),
                "scene_id": str(scene_id),
                "dialogue_ids": [str(dialogue_id) for dialogue_id in dialogue_ids],
            },
        )


async def _locked_shot_for_write(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
) -> Shot:
    current = await repository.find_shot(session, shot_id)
    if current is None:
        raise _not_found("Shot")
    await lock_active_episode_for_content_write(session, claims, current.episode_id)
    shot = await repository.find_shot(session, shot_id, for_update=True)
    if shot is None:
        raise _not_found("Shot")
    return shot


async def create_manual_shot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: ShotCreateRequest,
) -> ShotResponse:
    title = request.title.strip()
    async with session.begin():
        episode = await lock_active_episode_for_content_write(session, claims, episode_id)
        existing = await repository.find_shot_by_creation_key(
            session,
            episode.workspace_id,
            request.creation_key,
        )
        if existing is not None:
            if (
                existing.episode_id != episode_id
                or existing.title != title
                or existing.source_script_version_id != request.source_script_version_id
                or existing.source_scene_id != request.source_scene_id
                or existing.source_candidate_id is not None
            ):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Creation key was used with different input",
                    status_code=409,
                )
            return _shot_response(existing)
        shots = await repository.list_active_shots(session, episode_id, for_update=True)
        _require_capacity(shots)
        await _require_confirmed_structure(
            session,
            workspace_id=episode.workspace_id,
            episode_id=episode_id,
            script_version_id=request.source_script_version_id,
            scene_id=request.source_scene_id,
            dialogue_ids=[],
        )
        now = datetime.now(UTC)
        shot = Shot(
            id=uuid7(),
            workspace_id=episode.workspace_id,
            episode_id=episode_id,
            position=len(shots) + 1,
            title=title,
            source_script_version_id=request.source_script_version_id,
            source_scene_id=request.source_scene_id,
            creation_key=request.creation_key,
            status="active",
            revision=1,
            created_by=claims.sub,
            created_at=now,
            updated_at=now,
        )
        session.add(shot)
        await session.flush()
    return _shot_response(shot)


async def create_from_confirmed_candidate(
    session: AsyncSession,
    claims: AccessTokenClaims,
    candidate_id: UUID,
) -> ShotResponse:
    async with session.begin():
        reference = await resolve_confirmed_shot_candidate(session, candidate_id)
        if reference is None:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Shot candidate is not accepted against a confirmed structure",
                status_code=422,
                next_action="confirm_script_structure",
            )
        episode = await lock_active_episode_for_content_write(
            session,
            claims,
            reference.episode_id,
        )
        confirmed = await resolve_confirmed_shot_candidate(session, candidate_id)
        if (
            confirmed is None
            or confirmed.workspace_id != episode.workspace_id
            or confirmed.episode_id != episode.episode_id
        ):
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Shot candidate confirmation has changed",
                status_code=409,
                next_action="reload_script_candidates",
            )
        existing = await repository.find_shot_by_candidate(
            session,
            episode.workspace_id,
            candidate_id,
        )
        if existing is not None:
            return _shot_response(existing)
        shots = await repository.list_active_shots(
            session,
            episode.episode_id,
            for_update=True,
        )
        _require_capacity(shots)
        now = datetime.now(UTC)
        shot = Shot(
            id=uuid7(),
            workspace_id=episode.workspace_id,
            episode_id=episode.episode_id,
            position=len(shots) + 1,
            title=confirmed.title.strip(),
            source_script_version_id=confirmed.script_version_id,
            source_scene_id=confirmed.scene_id,
            source_candidate_id=confirmed.candidate_id,
            creation_key=None,
            status="active",
            revision=1,
            created_by=claims.sub,
            created_at=now,
            updated_at=now,
        )
        session.add(shot)
        await session.flush()
    return _shot_response(shot)


async def list_shots(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> ShotOrderResponse:
    await episode_for_content_read(session, claims, episode_id)
    return _order_response(await repository.list_active_shots(session, episode_id))


async def list_archived_shots(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> list[ShotResponse]:
    await episode_for_content_read(session, claims, episode_id)
    return [
        _shot_response(shot) for shot in await repository.list_archived_shots(session, episode_id)
    ]


async def get_shot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
) -> ShotResponse:
    shot = await repository.find_shot(session, shot_id)
    if shot is None:
        raise _not_found("Shot")
    await episode_for_content_read(session, claims, shot.episode_id)
    return _shot_response(shot)


async def update_shot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    request: ShotUpdateRequest,
) -> ShotResponse:
    async with session.begin():
        shot = await _locked_shot_for_write(session, claims, shot_id)
        if shot.status != "active":
            raise ApiError(ErrorCode.STATE_CONFLICT, "Shot is archived", status_code=409)
        _require_revision(shot, request.expected_revision)
        shot.title = request.title.strip()
        shot.revision += 1
        await session.flush()
    return _shot_response(shot)


async def reorder_shots(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: ShotReorderRequest,
) -> ShotOrderResponse:
    async with session.begin():
        await lock_active_episode_for_content_write(session, claims, episode_id)
        shots = await repository.list_active_shots(session, episode_id, for_update=True)
        _require_order(shots, request.expected_order_hash)
        by_id = {shot.id: shot for shot in shots}
        if set(request.shot_ids) != set(by_id):
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Shot order must contain every active shot exactly once",
                status_code=422,
                details={"current_shot_ids": [str(shot.id) for shot in shots]},
            )
        temporary_start = len(shots) * 2 + 1
        for offset, shot in enumerate(shots):
            shot.position = temporary_start + offset
        await session.flush()
        ordered = [by_id[shot_id] for shot_id in request.shot_ids]
        for position, shot in enumerate(ordered, start=1):
            shot.position = position
        await session.flush()
    return _order_response(ordered)


async def set_shot_archived(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    request: ShotStateRequest,
    *,
    archived: bool,
) -> ShotStateResponse:
    async with session.begin():
        shot = await _locked_shot_for_write(session, claims, shot_id)
        _require_revision(shot, request.expected_revision)
        expected_status = "active" if archived else "archived"
        if shot.status != expected_status:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                f"Shot is already {'archived' if archived else 'active'}",
                status_code=409,
            )
        active = await repository.list_active_shots(session, shot.episode_id, for_update=True)
        _require_order(active, request.expected_order_hash)
        now = datetime.now(UTC)
        if archived:
            temporary_start = len(active) * 2 + 1
            for offset, active_shot in enumerate(active):
                active_shot.position = temporary_start + offset
            await session.flush()
            shot.status = "archived"
            shot.archived_at = now
            shot.archived_by = claims.sub
            remaining = [active_shot for active_shot in active if active_shot.id != shot.id]
            for position, active_shot in enumerate(remaining, start=1):
                active_shot.position = position
        else:
            _require_capacity(active)
            shot.position = len(active) + 1
            shot.status = "active"
            shot.archived_at = None
            shot.archived_by = None
            remaining = [*active, shot]
        shot.revision += 1
        await session.flush()
    return ShotStateResponse(shot=_shot_response(shot), order=_order_response(remaining))


async def _validate_asset_references(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    project_id: UUID,
    references: list[AssetReferenceRequest],
) -> dict[UUID, AssetVersionReference]:
    resolved_by_version: dict[UUID, AssetVersionReference] = {}
    for reference in references:
        resolved = await resolve_asset_version(
            session,
            workspace_id,
            reference.asset_version_id,
        )
        if (
            resolved is None
            or resolved.project_id != project_id
            or resolved.kind != reference.role
            or resolved.asset_status != "active"
            or resolved.asset_availability != "enabled"
            or resolved.asset_state_status != "active"
        ):
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Asset version reference is invalid",
                status_code=422,
                next_action="select_project_asset_version",
                details={
                    "slot_key": reference.slot_key,
                    "asset_version_id": str(reference.asset_version_id),
                    "role": reference.role,
                },
            )
        resolved_by_version[reference.asset_version_id] = resolved
    return resolved_by_version


async def _validate_target(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    episode_id: UUID,
    project_id: UUID,
    target: TargetShotSpecRequest,
) -> None:
    script = target.spec.script_reference
    await _require_confirmed_structure(
        session,
        workspace_id=workspace_id,
        episode_id=episode_id,
        script_version_id=script.confirmed_script_version_id,
        scene_id=script.scene_id,
        dialogue_ids=script.dialogue_ids,
    )
    await _validate_asset_references(
        session,
        workspace_id=workspace_id,
        project_id=project_id,
        references=target.asset_references,
    )
    episode = await resolve_episode_content_context(session, workspace_id, episode_id)
    narrative = (
        await resolve_storyboard_narrative(
            session,
            workspace_id,
            episode.current_script_version_id,
        )
        if episode is not None and episode.current_script_version_id is not None
        else None
    )
    if narrative is None:
        raise ApiError(
            ErrorCode.DEPENDENCY_UNAVAILABLE,
            "Current narrative structure is unavailable",
            status_code=503,
            next_action="retry_coverage",
        )
    units = {unit.unit_version_id: unit for unit in narrative.units}
    for reference in target.narrative_references:
        unit = units.get(reference.unit_version_id)
        if unit is None:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Narrative reference is not current for the target episode",
                status_code=422,
                details={"reason": "unit_version_outside_episode"},
            )
        if (
            reference.segment_end is not None
            and reference.segment_end > len(unit.exact_text)
        ):
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Narrative segment exceeds the fixed unit text",
                status_code=422,
                details={"reason": "segment_out_of_range"},
            )


def _narrative_input(value: ShotNarrativeReference) -> NarrativeReferenceInput:
    return NarrativeReferenceInput(
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
    )


async def _store_narrative_references(
    session: AsyncSession,
    *,
    shot: Shot,
    version: ShotSpecVersion,
    inputs: list[NarrativeReferenceInput],
    actor_id: UUID,
    now: datetime,
    origin: Literal["ai", "human", "migrated"] = "human",
) -> list[ShotNarrativeReference]:
    if not inputs:
        return []
    episode = await resolve_episode_content_context(
        session,
        shot.workspace_id,
        shot.episode_id,
    )
    narrative = (
        await resolve_storyboard_narrative(
            session,
            shot.workspace_id,
            episode.current_script_version_id,
        )
        if episode is not None and episode.current_script_version_id is not None
        else None
    )
    if narrative is None:
        raise ApiError(
            ErrorCode.DEPENDENCY_UNAVAILABLE,
            "Current narrative structure is unavailable",
            status_code=503,
            next_action="retry_coverage",
        )
    units = {unit.unit_version_id: unit for unit in narrative.units}
    missing = [
        value.unit_version_id for value in inputs if value.unit_version_id not in units
    ]
    if missing:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Narrative reference is not current for the target episode",
            status_code=422,
            details={
                "reason": "unit_version_outside_episode",
                "unit_version_ids": [str(value) for value in missing],
            },
        )
    stored = [
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
            segment_key=(
                "full"
                if value.coverage_mode == "full"
                else f"{value.segment_start}:{value.segment_end}"
            ),
            contribution=value.contribution,
            origin=origin,
            created_by=actor_id,
            created_at=now,
        )
        for value in inputs
    ]
    session.add_all(stored)
    return stored


async def _create_target(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    episode_id: UUID,
    position: int,
    target: TargetShotSpecRequest,
    actor_id: UUID,
    now: datetime,
    trace_id: str,
    audit_source: Literal["copy", "split", "merge"],
) -> tuple[Shot, ShotSpecVersion, list[AssetReference]]:
    script = target.spec.script_reference
    shot = Shot(
        id=uuid7(),
        workspace_id=workspace_id,
        episode_id=episode_id,
        position=position,
        title=target.title.strip(),
        source_script_version_id=script.confirmed_script_version_id,
        source_scene_id=script.scene_id,
        source_candidate_id=None,
        creation_key=None,
        status="active",
        revision=1,
        created_by=actor_id,
        created_at=now,
        updated_at=now,
    )
    session.add(shot)
    await session.flush()
    hashes = storyboard_content_hashes(target.spec, target.asset_references)
    version = ShotSpecVersion(
        id=uuid7(),
        workspace_id=workspace_id,
        shot_id=shot.id,
        version_no=1,
        schema_version=target.spec.schema_version,
        spec=target.spec.model_dump(mode="json"),
        content_hash=hashes.content_hash,
        input_hash=hashes.input_hash,
        created_by=actor_id,
        created_at=now,
    )
    session.add(version)
    await session.flush()
    episode_context = await resolve_episode_content_context(
        session,
        workspace_id,
        episode_id,
    )
    if episode_context is None:
        raise ApiError(
            ErrorCode.DEPENDENCY_UNAVAILABLE,
            "Episode context is unavailable",
            status_code=503,
        )
    resolved_by_version = await _validate_asset_references(
        session,
        workspace_id=workspace_id,
        project_id=episode_context.project_id,
        references=target.asset_references,
    )
    references = [
        AssetReference(
            id=uuid7(),
            workspace_id=workspace_id,
            shot_spec_version_id=version.id,
            slot_key=reference.slot_key,
            role=reference.role,
            asset_version_id=reference.asset_version_id,
            asset_state_id=resolved_by_version[reference.asset_version_id].asset_state_id,
            asset_id=resolved_by_version[reference.asset_version_id].asset_id,
            binding_source="manual",
            subject_key=reference.subject_key,
            created_at=now,
        )
        for reference in target.asset_references
    ]
    session.add_all(references)
    await _store_narrative_references(
        session,
        shot=shot,
        version=version,
        inputs=target.narrative_references,
        actor_id=actor_id,
        now=now,
    )
    shot.current_spec_version_id = version.id
    _append_spec_version_audit(
        session,
        shot=shot,
        version=version,
        actor_id=actor_id,
        trace_id=trace_id,
        source=audit_source,
        previous_version_id=None,
        occurred_at=now,
    )
    await session.flush()
    return shot, version, references


def _append_spec_version_audit(
    session: AsyncSession,
    *,
    shot: Shot,
    version: ShotSpecVersion,
    actor_id: UUID,
    trace_id: str,
    source: Literal["manual_save", "copy", "split", "merge", "asset_upgrade"],
    previous_version_id: UUID | None,
    occurred_at: datetime,
) -> None:
    append_audit_event(
        session,
        workspace_id=shot.workspace_id,
        actor_id=actor_id,
        action="shot.spec_version_created",
        target_type="shot_spec_version",
        target_id=version.id,
        trace_id=trace_id,
        metadata={
            "shot_id": str(shot.id),
            "episode_id": str(shot.episode_id),
            "version_no": version.version_no,
            "shot_revision": shot.revision,
            "source": source,
            "previous_version_id": (
                str(previous_version_id) if previous_version_id is not None else None
            ),
            "current_version_id": str(version.id),
        },
        occurred_at=occurred_at,
    )


async def append_spec_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    request: ShotSpecCreateRequest,
    *,
    trace_id: str,
) -> ShotSpecCreateResponse:
    async with session.begin():
        current = await repository.find_shot(session, shot_id)
        if current is None:
            raise _not_found("Shot")
        episode = await lock_active_episode_for_content_write(session, claims, current.episode_id)
        shot = await repository.find_shot(session, shot_id, for_update=True)
        if shot is None:
            raise _not_found("Shot")
        if shot.status != "active":
            raise ApiError(ErrorCode.STATE_CONFLICT, "Shot is archived", status_code=409)
        _require_current_spec(shot, request.expected_current_spec_version_id)
        script = request.spec.script_reference
        if (
            script.confirmed_script_version_id != shot.source_script_version_id
            or script.scene_id != shot.source_scene_id
        ):
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Shot spec source must match the shot source",
                status_code=422,
                next_action="use_shot_script_source",
            )
        await _require_confirmed_structure(
            session,
            workspace_id=shot.workspace_id,
            episode_id=shot.episode_id,
            script_version_id=script.confirmed_script_version_id,
            scene_id=script.scene_id,
            dialogue_ids=script.dialogue_ids,
        )
        resolved_by_version = await _validate_asset_references(
            session,
            workspace_id=shot.workspace_id,
            project_id=episode.project_id,
            references=request.asset_references,
        )
        coverage_report = await resolve_coverage_report(session, episode)
        if request.narrative_references and coverage_report.status == "unavailable":
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Coverage dependencies are unavailable",
                status_code=503,
                next_action="retry_coverage",
            )
        validate_reference_inputs(
            request.narrative_references,
            coverage_report,
            shot_id=shot.id,
        )
        hashes = storyboard_content_hashes(request.spec, request.asset_references)
        previous_version_id = shot.current_spec_version_id
        now = datetime.now(UTC)
        version = ShotSpecVersion(
            id=uuid7(),
            workspace_id=shot.workspace_id,
            shot_id=shot.id,
            version_no=await repository.latest_spec_version_number(session, shot.id) + 1,
            schema_version=request.spec.schema_version,
            spec=request.spec.model_dump(mode="json"),
            content_hash=hashes.content_hash,
            input_hash=hashes.input_hash,
            created_by=claims.sub,
            created_at=now,
        )
        session.add(version)
        await session.flush()
        references = [
            AssetReference(
                id=uuid7(),
                workspace_id=shot.workspace_id,
                shot_spec_version_id=version.id,
                slot_key=reference.slot_key,
                role=reference.role,
                asset_version_id=reference.asset_version_id,
                asset_state_id=resolved_by_version[reference.asset_version_id].asset_state_id,
                asset_id=resolved_by_version[reference.asset_version_id].asset_id,
                binding_source="manual",
                subject_key=reference.subject_key,
            )
            for reference in request.asset_references
        ]
        session.add_all(references)
        await _store_narrative_references(
            session,
            shot=shot,
            version=version,
            inputs=request.narrative_references,
            actor_id=claims.sub,
            now=now,
        )
        shot.current_spec_version_id = version.id
        shot.revision += 1
        _append_spec_version_audit(
            session,
            shot=shot,
            version=version,
            actor_id=claims.sub,
            trace_id=trace_id,
            source="manual_save",
            previous_version_id=previous_version_id,
            occurred_at=now,
        )
        await session.flush()
    return ShotSpecCreateResponse(
        shot=_shot_response(shot),
        version=_spec_response(version, references),
    )


async def list_spec_versions(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
) -> list[ShotSpecVersionResponse]:
    shot = await repository.find_shot(session, shot_id)
    if shot is None:
        raise _not_found("Shot")
    await episode_for_content_read(session, claims, shot.episode_id)
    versions = await repository.list_spec_versions(session, shot_id)
    references = await repository.list_asset_references(
        session, [version.id for version in versions]
    )
    by_version: dict[UUID, list[AssetReference]] = defaultdict(list)
    for reference in references:
        by_version[reference.shot_spec_version_id].append(reference)
    return [_spec_response(version, by_version[version.id]) for version in versions]


async def get_spec_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
) -> ShotSpecVersionResponse:
    result = await repository.find_spec_version(session, version_id)
    if result is None:
        raise _not_found("Shot spec version")
    version, shot = result
    await episode_for_content_read(session, claims, shot.episode_id)
    references = await repository.list_asset_references(session, [version.id])
    return _spec_response(version, references)


async def set_current_spec_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    request: ShotCurrentSpecRequest,
    *,
    trace_id: str,
) -> ShotResponse:
    async with session.begin():
        shot = await _locked_shot_for_write(session, claims, shot_id)
        if shot.status != "active":
            raise ApiError(ErrorCode.STATE_CONFLICT, "Shot is archived", status_code=409)
        _require_revision(shot, request.expected_revision)
        _require_current_spec(shot, request.expected_current_spec_version_id)
        result = await repository.find_spec_version(session, request.version_id)
        if result is None or result[0].shot_id != shot.id:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Shot spec version belongs to another shot",
                status_code=422,
            )
        previous_version_id = shot.current_spec_version_id
        shot.current_spec_version_id = request.version_id
        shot.revision += 1
        append_audit_event(
            session,
            workspace_id=shot.workspace_id,
            actor_id=claims.sub,
            action="shot.current_spec_changed",
            target_type="shot",
            target_id=shot.id,
            trace_id=trace_id,
            metadata={
                "episode_id": str(shot.episode_id),
                "revision": shot.revision,
                "previous_version_id": (
                    str(previous_version_id) if previous_version_id is not None else None
                ),
                "current_version_id": str(request.version_id),
            },
        )
        await session.flush()
    return _shot_response(shot)


async def delete_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
) -> ShotDeletePreflightResponse:
    shot = await repository.find_shot(session, shot_id)
    if shot is None:
        raise _not_found("Shot")
    await episode_for_content_read(session, claims, shot.episode_id)
    blockers: list[ShotDeleteBlocker] = []
    if shot.source_candidate_id is not None:
        blockers.append(
            ShotDeleteBlocker(
                code="SOURCE_CANDIDATE_EVIDENCE",
                summary="Shot created from a confirmed candidate must retain source evidence",
            )
        )
    if await repository.count_spec_versions(session, shot.id):
        blockers.append(
            ShotDeleteBlocker(
                code="SPEC_VERSION_EVIDENCE",
                summary="Shot with immutable spec versions cannot be deleted",
            )
        )
    return ShotDeletePreflightResponse(allowed=not blockers, blockers=blockers)


async def delete_shot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    *,
    expected_revision: int,
    expected_order_hash: str,
) -> ShotDeleteResponse:
    async with session.begin():
        shot = await _locked_shot_for_write(session, claims, shot_id)
        if shot.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Only an active empty shot can be deleted",
                status_code=409,
                next_action="restore_shot",
            )
        _require_revision(shot, expected_revision)
        active = await repository.list_active_shots(
            session,
            shot.episode_id,
            for_update=True,
        )
        _require_order(active, expected_order_hash)
        preflight = await delete_preflight(session, claims, shot.id)
        if not preflight.allowed:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Shot evidence prevents deletion",
                status_code=409,
                next_action="archive_shot",
                details={"blockers": [blocker.model_dump() for blocker in preflight.blockers]},
            )
        temporary_start = len(active) * 2 + 1
        for offset, active_shot in enumerate(active):
            active_shot.position = temporary_start + offset
        await session.flush()
        remaining = [active_shot for active_shot in active if active_shot.id != shot.id]
        await session.delete(shot)
        await session.flush()
        for position, active_shot in enumerate(remaining, start=1):
            active_shot.position = position
        await session.flush()
    return ShotDeleteResponse(order=_order_response(remaining))


async def copy_shot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    request: CopyShotRequest,
    *,
    trace_id: str,
) -> ShotTransformResponse:
    input_hash = _transform_input_hash(
        "copy",
        {"source_shot_id": str(shot_id), **request.model_dump(mode="json")},
    )
    async with session.begin():
        source = await _locked_shot_for_write(session, claims, shot_id)
        repeated = await _idempotent_transform(
            session,
            workspace_id=source.workspace_id,
            idempotency_key=request.idempotency_key,
            input_hash=input_hash,
        )
        if repeated is not None:
            return repeated
        if source.status != "active":
            raise ApiError(ErrorCode.STATE_CONFLICT, "Shot is archived", status_code=409)
        active = await repository.list_active_shots(
            session,
            source.episode_id,
            for_update=True,
        )
        _require_order(active, request.expected_order_hash)
        _require_capacity(active)
        source_spec = await _current_spec(
            session,
            source,
            request.expected_source_spec_version_id,
        )
        source_references = await _references_for_spec(session, source_spec.id)
        source_narrative_references = await coverage_repository.list_references(
            session,
            [source_spec.id],
        )
        reference_requests = [
            AssetReferenceRequest(
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
            for reference in source_references
        ]
        target = TargetShotSpecRequest(
            title=request.title,
            spec=ShotSpec.model_validate(source_spec.spec),
            asset_references=reference_requests,
            narrative_references=[
                _narrative_input(reference).model_copy(
                    update={"role": "supporting", "contribution": "supporting"}
                )
                for reference in source_narrative_references
            ],
        )
        source_index = next(
            index for index, active_shot in enumerate(active) if active_shot.id == source.id
        )
        temporary_start = len(active) * 2 + 1
        for offset, active_shot in enumerate(active):
            active_shot.position = temporary_start + offset
        await session.flush()
        now = datetime.now(UTC)
        result_shot, result_spec, _ = await _create_target(
            session,
            workspace_id=source.workspace_id,
            episode_id=source.episode_id,
            position=source_index + 2,
            target=target,
            actor_id=claims.sub,
            now=now,
            trace_id=trace_id,
            audit_source="copy",
        )
        ordered = [*active[: source_index + 1], result_shot, *active[source_index + 1 :]]
        for position, active_shot in enumerate(ordered, start=1):
            active_shot.position = position
        impact_hash = _impact_hash(
            "copy",
            [source],
            [source_spec],
            request.expected_order_hash,
        )
        transform = ShotTransform(
            id=uuid7(),
            workspace_id=source.workspace_id,
            episode_id=source.episode_id,
            operation="copy",
            source_shot_ids=[source.id],
            source_spec_version_ids=[source_spec.id],
            result_shot_ids=[result_shot.id],
            impact_hash=impact_hash,
            input_hash=input_hash,
            idempotency_key=request.idempotency_key,
            actor_id=claims.sub,
            created_at=now,
        )
        session.add(transform)
        await session.flush()
        result = ShotTransformResponse(
            transform=_transform_evidence_response(transform),
            shots=[_shot_response(result_shot)],
            spec_versions=[_spec_response(result_spec, source_references)],
            order=_order_response(ordered),
        )
    return result


async def split_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    request: SplitPreflightRequest,
) -> ShotTransformPreflightResponse:
    source = await repository.find_shot(session, shot_id)
    if source is None:
        raise _not_found("Shot")
    await episode_for_content_read(session, claims, source.episode_id)
    if source.status != "active":
        raise ApiError(ErrorCode.STATE_CONFLICT, "Shot is archived", status_code=409)
    active = await repository.list_active_shots(session, source.episode_id)
    _require_order(active, request.expected_order_hash)
    source_spec = await _current_spec(
        session,
        source,
        request.expected_source_spec_version_id,
    )
    impact_hash = _impact_hash(
        "split",
        [source],
        [source_spec],
        request.expected_order_hash,
    )
    return ShotTransformPreflightResponse(
        operation="split",
        source_shot_ids=[source.id],
        source_spec_version_ids=[source_spec.id],
        order_hash=request.expected_order_hash,
        downstream_evidence=DownstreamEvidenceResponse(),
        impact_hash=impact_hash,
    )


async def split_shot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    request: SplitShotRequest,
    *,
    trace_id: str,
) -> ShotTransformResponse:
    input_hash = _transform_input_hash(
        "split",
        {"source_shot_id": str(shot_id), **request.model_dump(mode="json")},
    )
    async with session.begin():
        source = await _locked_shot_for_write(session, claims, shot_id)
        repeated = await _idempotent_transform(
            session,
            workspace_id=source.workspace_id,
            idempotency_key=request.idempotency_key,
            input_hash=input_hash,
        )
        if repeated is not None:
            return repeated
        if source.status != "active":
            raise ApiError(ErrorCode.STATE_CONFLICT, "Shot is archived", status_code=409)
        episode = await episode_for_content_read(session, claims, source.episode_id)
        active = await repository.list_active_shots(
            session,
            source.episode_id,
            for_update=True,
        )
        _require_order(active, request.expected_order_hash)
        _require_capacity(active, additional=1)
        source_spec = await _current_spec(
            session,
            source,
            request.expected_source_spec_version_id,
        )
        expected_impact_hash = _impact_hash(
            "split",
            [source],
            [source_spec],
            request.expected_order_hash,
        )
        if request.impact_hash != expected_impact_hash:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Split impact has changed",
                status_code=409,
                next_action="repeat_split_preflight",
                details={"current_impact_hash": expected_impact_hash},
            )
        source_model = ShotSpec.model_validate(source_spec.spec)
        source_narrative_references = await coverage_repository.list_references(
            session,
            [source_spec.id],
        )
        try:
            validate_split_content(source_model, request.targets)
            validate_split_narrative(
                [_narrative_input(value) for value in source_narrative_references],
                request.targets,
            )
        except TransformConservationError as error:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                error.summary,
                status_code=422,
                next_action="review_split_content",
                details={"reason": error.code},
            ) from error
        for target in request.targets:
            await _validate_target(
                session,
                workspace_id=source.workspace_id,
                episode_id=source.episode_id,
                project_id=episode.project_id,
                target=target,
            )
        source_index = next(
            index for index, active_shot in enumerate(active) if active_shot.id == source.id
        )
        temporary_start = len(active) * 2 + 2
        for offset, active_shot in enumerate(active):
            active_shot.position = temporary_start + offset
        await session.flush()
        now = datetime.now(UTC)
        results: list[Shot] = []
        versions: list[ShotSpecVersion] = []
        references: list[list[AssetReference]] = []
        for offset, target in enumerate(request.targets):
            result_shot, version, target_references = await _create_target(
                session,
                workspace_id=source.workspace_id,
                episode_id=source.episode_id,
                position=source_index + offset + 1,
                target=target,
                actor_id=claims.sub,
                now=now,
                trace_id=trace_id,
                audit_source="split",
            )
            results.append(result_shot)
            versions.append(version)
            references.append(target_references)
        source.status = "archived"
        source.archived_at = now
        source.archived_by = claims.sub
        source.revision += 1
        ordered = [*active[:source_index], *results, *active[source_index + 1 :]]
        for position, active_shot in enumerate(ordered, start=1):
            active_shot.position = position
        transform = ShotTransform(
            id=uuid7(),
            workspace_id=source.workspace_id,
            episode_id=source.episode_id,
            operation="split",
            source_shot_ids=[source.id],
            source_spec_version_ids=[source_spec.id],
            result_shot_ids=[shot.id for shot in results],
            impact_hash=expected_impact_hash,
            input_hash=input_hash,
            idempotency_key=request.idempotency_key,
            actor_id=claims.sub,
            created_at=now,
        )
        session.add(transform)
        await session.flush()
        result = ShotTransformResponse(
            transform=_transform_evidence_response(transform),
            shots=[_shot_response(shot) for shot in results],
            spec_versions=[
                _spec_response(version, target_references)
                for version, target_references in zip(
                    versions,
                    references,
                    strict=True,
                )
            ],
            order=_order_response(ordered),
        )
    return result


async def _merge_sources(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: MergePreflightRequest,
    *,
    for_update: bool,
) -> tuple[list[Shot], list[ShotSpecVersion], list[Shot]]:
    shots = await repository.find_shots(
        session,
        request.shot_ids,
        for_update=for_update,
    )
    if len(shots) != 2:
        raise _not_found("Shot")
    if shots[0].episode_id != shots[1].episode_id:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Merge sources must belong to one episode",
            status_code=422,
        )
    if for_update:
        await lock_active_episode_for_content_write(session, claims, shots[0].episode_id)
        shots = await repository.find_shots(session, request.shot_ids, for_update=True)
    else:
        await episode_for_content_read(session, claims, shots[0].episode_id)
    if any(shot.status != "active" for shot in shots):
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Merge sources must be active",
            status_code=409,
        )
    active = await repository.list_active_shots(
        session,
        shots[0].episode_id,
        for_update=for_update,
    )
    _require_order(active, request.expected_order_hash)
    positions = [shot.position for shot in shots]
    if positions[1] != positions[0] + 1:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Merge sources must be adjacent and ordered",
            status_code=422,
        )
    specs = [
        await _current_spec(session, shot, expected_id)
        for shot, expected_id in zip(
            shots,
            request.expected_spec_version_ids,
            strict=True,
        )
    ]
    try:
        validate_merge_content(
            [ShotSpec.model_validate(spec.spec) for spec in specs],
            None,
        )
    except TransformConservationError as error:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            error.summary,
            status_code=422,
            next_action="review_merge_sources",
            details={"reason": error.code},
        ) from error
    return shots, specs, active


async def merge_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: MergePreflightRequest,
) -> ShotTransformPreflightResponse:
    shots, specs, _ = await _merge_sources(
        session,
        claims,
        request,
        for_update=False,
    )
    impact_hash = _impact_hash(
        "merge",
        shots,
        specs,
        request.expected_order_hash,
    )
    return ShotTransformPreflightResponse(
        operation="merge",
        source_shot_ids=[shot.id for shot in shots],
        source_spec_version_ids=[spec.id for spec in specs],
        order_hash=request.expected_order_hash,
        downstream_evidence=DownstreamEvidenceResponse(),
        impact_hash=impact_hash,
    )


async def merge_shots(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: MergeShotRequest,
    *,
    trace_id: str,
) -> ShotTransformResponse:
    input_hash = _transform_input_hash("merge", request.model_dump(mode="json"))
    async with session.begin():
        initial = await repository.find_shots(session, request.shot_ids)
        if len(initial) != 2:
            raise _not_found("Shot")
        episode = await lock_active_episode_for_content_write(
            session,
            claims,
            initial[0].episode_id,
        )
        repeated = await _idempotent_transform(
            session,
            workspace_id=episode.workspace_id,
            idempotency_key=request.idempotency_key,
            input_hash=input_hash,
        )
        if repeated is not None:
            return repeated
        shots, specs, active = await _merge_sources(
            session,
            claims,
            request,
            for_update=True,
        )
        expected_impact_hash = _impact_hash(
            "merge",
            shots,
            specs,
            request.expected_order_hash,
        )
        if request.impact_hash != expected_impact_hash:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Merge impact has changed",
                status_code=409,
                next_action="repeat_merge_preflight",
                details={"current_impact_hash": expected_impact_hash},
            )
        source_models = [ShotSpec.model_validate(spec.spec) for spec in specs]
        source_narrative_references = await coverage_repository.list_references(
            session,
            [spec.id for spec in specs],
        )
        narrative_by_spec: dict[UUID, list[NarrativeReferenceInput]] = defaultdict(list)
        for reference in source_narrative_references:
            narrative_by_spec[reference.shot_spec_version_id].append(
                _narrative_input(reference)
            )
        try:
            validate_merge_content(source_models, request.target)
            validate_merge_narrative(
                [narrative_by_spec[spec.id] for spec in specs],
                request.target,
            )
        except TransformConservationError as error:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                error.summary,
                status_code=422,
                next_action="review_merge_content",
                details={"reason": error.code},
            ) from error
        await _validate_target(
            session,
            workspace_id=episode.workspace_id,
            episode_id=episode.episode_id,
            project_id=episode.project_id,
            target=request.target,
        )
        first_index = next(
            index for index, active_shot in enumerate(active) if active_shot.id == shots[0].id
        )
        temporary_start = len(active) * 2 + 1
        for offset, active_shot in enumerate(active):
            active_shot.position = temporary_start + offset
        await session.flush()
        now = datetime.now(UTC)
        result_shot, result_spec, result_references = await _create_target(
            session,
            workspace_id=episode.workspace_id,
            episode_id=episode.episode_id,
            position=first_index + 1,
            target=request.target,
            actor_id=claims.sub,
            now=now,
            trace_id=trace_id,
            audit_source="merge",
        )
        source_ids = {shot.id for shot in shots}
        for shot in shots:
            shot.status = "archived"
            shot.archived_at = now
            shot.archived_by = claims.sub
            shot.revision += 1
        ordered = [
            *active[:first_index],
            result_shot,
            *(shot for shot in active[first_index:] if shot.id not in source_ids),
        ]
        for position, active_shot in enumerate(ordered, start=1):
            active_shot.position = position
        transform = ShotTransform(
            id=uuid7(),
            workspace_id=episode.workspace_id,
            episode_id=episode.episode_id,
            operation="merge",
            source_shot_ids=[shot.id for shot in shots],
            source_spec_version_ids=[spec.id for spec in specs],
            result_shot_ids=[result_shot.id],
            impact_hash=expected_impact_hash,
            input_hash=input_hash,
            idempotency_key=request.idempotency_key,
            actor_id=claims.sub,
            created_at=now,
        )
        session.add(transform)
        await session.flush()
        result = ShotTransformResponse(
            transform=_transform_evidence_response(transform),
            shots=[_shot_response(result_shot)],
            spec_versions=[_spec_response(result_spec, result_references)],
            order=_order_response(ordered),
        )
    return result


def _unique_readiness_issues(
    issues: list[ShotReadinessIssue],
) -> list[ShotReadinessIssue]:
    unique: list[ShotReadinessIssue] = []
    seen: set[tuple[str, str | None, UUID | None]] = set()
    for issue in issues:
        key = (issue.code, issue.field_path, issue.dependency_id)
        if key not in seen:
            seen.add(key)
            unique.append(issue)
    return unique


def _asset_readiness_issues(
    reference: AssetReference,
    readiness: AssetVersionReadinessReference,
) -> list[ShotReadinessIssue]:
    issues: list[ShotReadinessIssue] = []
    if readiness.kind != reference.role:
        issues.append(
            ShotReadinessIssue(
                code="ASSET_KIND_MISMATCH",
                field_path=f"asset_references.{reference.slot_key}",
                dependency_type="ASSET_VERSION",
                dependency_id=reference.asset_version_id,
                summary="Asset kind does not match the fixed reference role",
                next_action="replace_asset_reference",
            )
        )
        return issues
    if readiness.status == "unavailable":
        issues.append(
            ShotReadinessIssue(
                code="DEPENDENCY_UNAVAILABLE",
                field_path=f"asset_references.{reference.slot_key}",
                dependency_type="ASSET_READINESS",
                dependency_id=reference.asset_version_id,
                summary="Asset readiness dependency is unavailable",
                next_action="retry_readiness",
            )
        )
        return issues
    for blocker_code in readiness.blocker_codes:
        if blocker_code == "asset_archived":
            code = "ASSET_VERSION_UNAVAILABLE"
            summary = "The fixed asset version belongs to an archived asset"
            next_action = "replace_or_restore_asset"
        elif blocker_code == "asset_disabled":
            code = "ASSET_DISABLED"
            summary = "The fixed asset version belongs to a disabled asset"
            next_action = "enable_asset"
        elif blocker_code.startswith("media_") or blocker_code == "required_media_missing":
            code = "MEDIA_REFERENCE_UNAVAILABLE"
            summary = "The fixed asset version has unavailable required media"
            next_action = "review_asset_media"
        elif blocker_code in {
            "consent_missing",
            "consent_revoked",
            "consent_not_yet_valid",
            "consent_expired",
            "purpose_not_covered",
            "channel_not_covered",
            "region_not_covered",
            "proof_unavailable",
            "minor_not_supported",
        }:
            code = "RIGHTS_BLOCKED"
            summary = "The fixed asset version is not authorized for generation"
            next_action = "review_asset_consent"
        else:
            code = "ASSET_NOT_READY"
            summary = "The fixed asset version is not ready"
            next_action = "complete_asset_version"
        issues.append(
            ShotReadinessIssue(
                code=code,
                field_path=f"asset_references.{reference.slot_key}",
                dependency_type="ASSET_VERSION",
                dependency_id=reference.asset_version_id,
                summary=summary,
                next_action=next_action,
            )
        )
    if readiness.status != "ready" and not readiness.blocker_codes:
        issues.append(
            ShotReadinessIssue(
                code="ASSET_NOT_READY",
                field_path=f"asset_references.{reference.slot_key}",
                dependency_type="ASSET_VERSION",
                dependency_id=reference.asset_version_id,
                summary="The fixed asset version is not ready",
                next_action="complete_asset_version",
            )
        )
    return issues


def _finalize_readiness(
    *,
    shot_id: UUID,
    issues: list[ShotReadinessIssue],
    warnings: list[ShotReadinessWarning],
    dependencies: ShotReadinessDependencies,
) -> ShotReadinessResponse:
    blocking_reasons = _unique_readiness_issues(issues)
    status: Literal["ready", "blocked", "unavailable"]
    if any(
        issue.code in {"DEPENDENCY_UNAVAILABLE", "COVERAGE_DEPENDENCY_UNAVAILABLE"}
        for issue in blocking_reasons
    ):
        status = "unavailable"
    elif blocking_reasons:
        status = "blocked"
    else:
        status = "ready"
    next_actions = list(
        dict.fromkeys(
            [issue.next_action for issue in blocking_reasons]
            + [warning.next_action for warning in warnings]
        )
    )
    evaluation_hash = canonical_payload_hash(
        {
            "shot_id": str(shot_id),
            "status": status,
            "blocking_reasons": [
                issue.model_dump(mode="json", exclude_none=True) for issue in blocking_reasons
            ],
            "warnings": [
                warning.model_dump(mode="json", exclude_none=True) for warning in warnings
            ],
            "evaluated_dependencies": dependencies.model_dump(mode="json"),
        }
    )
    return ShotReadinessResponse(
        shot_id=shot_id,
        status=status,
        ready=status == "ready",
        blocking_reasons=blocking_reasons,
        warnings=warnings,
        next_actions=next_actions,
        evaluated_dependencies=dependencies,
        evaluation_hash=evaluation_hash,
    )


def _evaluate_loaded_readiness(
    shot: Shot,
    version: ShotSpecVersion | None,
    references: list[AssetReference],
    *,
    script_state: ScriptReadinessState,
    asset_snapshots: dict[UUID, AssetVersionReadinessReference],
    assets_unavailable: bool,
    coverage: CoverageReportResponse,
) -> ShotReadinessResponse:
    if version is None:
        return _finalize_readiness(
            shot_id=shot.id,
            issues=[
                ShotReadinessIssue(
                    code="CURRENT_SPEC_MISSING",
                    field_path="current_spec_version_id",
                    summary="Shot has no selected specification version",
                    next_action="save_shot_spec",
                )
            ],
            warnings=[],
            dependencies=ShotReadinessDependencies(
                shot_spec_version_id=None,
                confirmed_script_version_id=shot.source_script_version_id,
                current_script_version_id=script_state.current_script_version_id,
                narrative_structure_id=(
                    script_state.narrative.structure_id
                    if script_state.narrative is not None
                    else None
                ),
                narrative_structure_revision=(
                    script_state.narrative.structure_revision
                    if script_state.narrative is not None
                    else None
                ),
                narrative_dependency_hash=(
                    script_state.narrative.dependency_hash
                    if script_state.narrative is not None
                    else None
                ),
                coverage_basis_hash=coverage.basis_hash,
                coverage_evaluation_hash=coverage.evaluation_hash,
                scene_id=shot.source_scene_id,
                dialogue_ids=[],
                asset_version_ids=[],
                media_version_ids=[],
                consent_ids=[],
                asset_evaluation_hashes={},
            ),
        )

    spec = ShotSpec.model_validate(version.spec)
    issues: list[ShotReadinessIssue] = []
    warnings: list[ShotReadinessWarning] = []
    if script_state.confirmed_structure_available is None:
        issues.append(
            ShotReadinessIssue(
                code="DEPENDENCY_UNAVAILABLE",
                dependency_type="SCRIPTS",
                summary="Confirmed script dependency is unavailable",
                next_action="retry_readiness",
            )
        )
    elif not script_state.confirmed_structure_available:
        issues.append(
            ShotReadinessIssue(
                code="SCRIPT_VERSION_UNAVAILABLE",
                field_path="spec.script_reference",
                dependency_type="SCRIPT_VERSION",
                dependency_id=spec.script_reference.confirmed_script_version_id,
                summary="The fixed confirmed script structure is unavailable",
                next_action="select_confirmed_script_structure",
            )
        )

    if script_state.current_script_version_id != spec.script_reference.confirmed_script_version_id:
        issues.append(
            ShotReadinessIssue(
                code="SCRIPT_REVISION_NOT_CURRENT",
                field_path="spec.script_reference.confirmed_script_version_id",
                dependency_type="SCRIPT_VERSION",
                dependency_id=script_state.current_script_version_id,
                summary="The shot references a script revision that is no longer current",
                next_action="revise_storyboard_from_current_script",
            )
        )
    if script_state.narrative_unavailable or (
        script_state.current_script_version_id is not None and script_state.narrative is None
    ):
        issues.append(
            ShotReadinessIssue(
                code="DEPENDENCY_UNAVAILABLE",
                field_path="evaluated_dependencies.narrative_dependency_hash",
                dependency_type="NARRATIVE_STRUCTURE",
                dependency_id=script_state.current_script_version_id,
                summary="Current narrative structure dependency is unavailable",
                next_action="retry_readiness",
            )
        )

    if coverage.status == "unavailable":
        issues.append(
            ShotReadinessIssue(
                code="COVERAGE_DEPENDENCY_UNAVAILABLE",
                field_path="evaluated_dependencies.coverage_evaluation_hash",
                dependency_type="COVERAGE",
                summary="Narrative coverage dependencies are unavailable",
                next_action="retry_coverage",
            )
        )
    else:
        shot_coverage = next(
            (item for item in coverage.shots if item.shot_id == shot.id),
            None,
        )
        stale_reference_ids = set(coverage.stale_reference_ids)
        if any(
            reference.id in stale_reference_ids
            and reference.shot_id == shot.id
            for reference in coverage.references
        ) or coverage.stale_decision_ids:
            issues.append(
                ShotReadinessIssue(
                    code="NARRATIVE_REFERENCE_INVALID",
                    field_path="narrative_references",
                    dependency_type="COVERAGE",
                    summary="Narrative coverage references or decisions are stale",
                    next_action="review_stale_coverage",
                )
            )
        if coverage.summary.uncovered:
            issues.append(
                ShotReadinessIssue(
                    code="COVERAGE_UNACCOUNTED",
                    field_path="narrative_references",
                    dependency_type="COVERAGE",
                    summary="Required narrative units remain unaccounted",
                    next_action="map_or_omit_narrative_units",
                )
            )
        if shot_coverage is None or shot_coverage.status == "orphan":
            issues.append(
                ShotReadinessIssue(
                    code="SHOT_SOURCE_ORPHAN",
                    field_path="narrative_references",
                    dependency_type="COVERAGE",
                    dependency_id=version.id,
                    summary="Shot has no current narrative source or creative approval",
                    next_action="map_or_approve_invented_shot",
                )
            )

    if sum(reference.role == "location" for reference in references) != 1:
        issues.append(
            ShotReadinessIssue(
                code="LOCATION_REFERENCE_MISSING",
                field_path="asset_references",
                summary="Exactly one location asset reference is required",
                next_action="select_location_asset",
            )
        )
    character_subjects = {
        reference.subject_key
        for reference in references
        if reference.role == "character" and reference.subject_key is not None
    }
    for index, placement in enumerate(spec.visual.subject_placements):
        if placement.subject_key not in character_subjects:
            issues.append(
                ShotReadinessIssue(
                    code="CHARACTER_REFERENCE_MISSING",
                    field_path=f"spec.visual.subject_placements.{index}",
                    summary="A placed subject requires a matching character asset",
                    next_action="select_character_asset",
                )
            )
    voice_subjects = {
        reference.subject_key
        for reference in references
        if reference.role == "voice" and reference.subject_key is not None
    }
    for index, dialogue in enumerate(spec.dialogue_or_narration):
        if dialogue.render_as_audio and dialogue.speaker_subject_key not in voice_subjects:
            issues.append(
                ShotReadinessIssue(
                    code="VOICE_REFERENCE_MISSING",
                    field_path=f"spec.dialogue_or_narration.{index}",
                    summary="An audible dialogue requires a matching voice asset",
                    next_action="select_voice_asset",
                )
            )
    if assets_unavailable:
        issues.append(
            ShotReadinessIssue(
                code="DEPENDENCY_UNAVAILABLE",
                dependency_type="ASSETS",
                summary="Asset readiness dependency is unavailable",
                next_action="retry_readiness",
            )
        )
    else:
        for reference in references:
            readiness = asset_snapshots.get(reference.asset_version_id)
            if readiness is None:
                issues.append(
                    ShotReadinessIssue(
                        code="ASSET_VERSION_UNAVAILABLE",
                        field_path=f"asset_references.{reference.slot_key}",
                        dependency_type="ASSET_VERSION",
                        dependency_id=reference.asset_version_id,
                        summary="The fixed asset version is unavailable",
                        next_action="replace_asset_reference",
                    )
                )
            else:
                issues.extend(_asset_readiness_issues(reference, readiness))

    if spec.duration_ms > 8000:
        warnings.append(
            ShotReadinessWarning(
                code="DURATION_ABOVE_RECOMMENDED",
                field_path="spec.duration_ms",
                summary="Shot duration is above the recommended eight seconds",
                next_action="confirm_long_shot",
            )
        )
    beats_per_second = len(spec.action_beats) / (spec.duration_ms / 1000)
    if len(spec.action_beats) > 4 or beats_per_second > 1:
        warnings.append(
            ShotReadinessWarning(
                code="ACTION_DENSITY_HIGH",
                field_path="spec.action_beats",
                summary="Shot action density may reduce generation consistency",
                next_action="confirm_action_density",
            )
        )
    if not any(reference.role == "visual_style" for reference in references):
        warnings.append(
            ShotReadinessWarning(
                code="STYLE_REFERENCE_MISSING",
                field_path="asset_references",
                summary="An optional visual style reference is not fixed",
                next_action="add_optional_style_reference",
            )
        )

    used_snapshots = {
        reference.asset_version_id: asset_snapshots[reference.asset_version_id]
        for reference in references
        if reference.asset_version_id in asset_snapshots
    }
    dependencies = ShotReadinessDependencies(
        shot_spec_version_id=version.id,
        confirmed_script_version_id=spec.script_reference.confirmed_script_version_id,
        current_script_version_id=script_state.current_script_version_id,
        narrative_structure_id=(
            script_state.narrative.structure_id if script_state.narrative is not None else None
        ),
        narrative_structure_revision=(
            script_state.narrative.structure_revision
            if script_state.narrative is not None
            else None
        ),
        narrative_dependency_hash=(
            script_state.narrative.dependency_hash if script_state.narrative is not None else None
        ),
        coverage_basis_hash=coverage.basis_hash,
        coverage_evaluation_hash=coverage.evaluation_hash,
        scene_id=spec.script_reference.scene_id,
        dialogue_ids=spec.script_reference.dialogue_ids,
        asset_version_ids=[reference.asset_version_id for reference in references],
        media_version_ids=sorted(
            {
                media_id
                for snapshot in used_snapshots.values()
                for media_id in snapshot.media_version_ids
            },
            key=str,
        ),
        consent_ids=sorted(
            {
                consent_id
                for snapshot in used_snapshots.values()
                for consent_id in snapshot.consent_ids
            },
            key=str,
        ),
        asset_evaluation_hashes={
            asset_id: used_snapshots[asset_id].evaluation_hash
            for asset_id in sorted(used_snapshots, key=str)
        },
    )
    return _finalize_readiness(
        shot_id=shot.id,
        issues=issues,
        warnings=warnings,
        dependencies=dependencies,
    )


async def _resolve_project_readiness_dependencies(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    project_id: UUID,
    episode_id_by_version: dict[UUID, UUID],
    versions: list[ShotSpecVersion],
    references: list[AssetReference],
) -> tuple[
    dict[UUID, ScriptReadinessState],
    dict[UUID, AssetVersionReadinessReference],
    bool,
]:
    queries_by_version: dict[UUID, EpisodeConfirmedStructureQuery] = {}
    for version in versions:
        spec = ShotSpec.model_validate(version.spec)
        queries_by_version[version.id] = EpisodeConfirmedStructureQuery(
            episode_id=episode_id_by_version[version.id],
            structure=ConfirmedStructureQuery(
                script_version_id=spec.script_reference.confirmed_script_version_id,
                scene_id=spec.script_reference.scene_id,
                dialogue_ids=tuple(spec.script_reference.dialogue_ids),
            ),
        )
    structure_states: dict[UUID, bool | None]
    try:
        structures = await resolve_episode_confirmed_structures(
            session,
            workspace_id=workspace_id,
            queries=list(queries_by_version.values()),
        )
    except (SQLAlchemyError, ApiError) as error:
        if isinstance(error, ApiError) and error.code != ErrorCode.DEPENDENCY_UNAVAILABLE:
            raise
        structure_states = {version.id: None for version in versions}
    else:
        structure_states = {
            version_id: structures.get(query) is not None
            for version_id, query in queries_by_version.items()
        }

    episode_ids = list(dict.fromkeys(episode_id_by_version.values()))
    contexts: dict[UUID, EpisodeContentContext | None] = {}
    narrative_resolution_unavailable = False
    try:
        for episode_id in episode_ids:
            contexts[episode_id] = await resolve_episode_content_context(
                session,
                workspace_id,
                episode_id,
            )
        current_script_ids = [
            context.current_script_version_id
            for context in contexts.values()
            if context is not None and context.current_script_version_id is not None
        ]
        narrative_snapshots = await resolve_narrative_dependencies(
            session,
            workspace_id,
            current_script_ids,
        )
    except (SQLAlchemyError, ApiError) as error:
        if isinstance(error, ApiError) and error.code != ErrorCode.DEPENDENCY_UNAVAILABLE:
            raise
        narrative_snapshots = {}
        narrative_resolution_unavailable = True

    script_states: dict[UUID, ScriptReadinessState] = {}
    for version in versions:
        episode_id = episode_id_by_version[version.id]
        context = contexts.get(episode_id)
        current_script_version_id = (
            context.current_script_version_id if context is not None else None
        )
        script_states[version.id] = ScriptReadinessState(
            confirmed_structure_available=structure_states.get(version.id),
            current_script_version_id=current_script_version_id,
            narrative=(
                narrative_snapshots.get(current_script_version_id)
                if current_script_version_id is not None
                else None
            ),
            narrative_unavailable=(narrative_resolution_unavailable or context is None),
        )

    asset_ids = list(dict.fromkeys(reference.asset_version_id for reference in references))
    assets_unavailable = False
    try:
        asset_snapshots = await resolve_asset_versions_readiness(
            session,
            workspace_id,
            project_id,
            asset_ids,
            purpose="ai_short_drama_generation",
            channel="lanverse_preview",
            region="CN",
        )
    except (SQLAlchemyError, ApiError) as error:
        if isinstance(error, ApiError) and error.code != ErrorCode.DEPENDENCY_UNAVAILABLE:
            raise
        asset_snapshots = {}
        assets_unavailable = True
    return script_states, asset_snapshots, assets_unavailable


async def _resolve_readiness_dependencies(
    session: AsyncSession,
    episode: EpisodeContentContext,
    versions: list[ShotSpecVersion],
    references: list[AssetReference],
) -> tuple[
    dict[UUID, ScriptReadinessState],
    dict[UUID, AssetVersionReadinessReference],
    bool,
]:
    return await _resolve_project_readiness_dependencies(
        session,
        workspace_id=episode.workspace_id,
        project_id=episode.project_id,
        episode_id_by_version={version.id: episode.episode_id for version in versions},
        versions=versions,
        references=references,
    )


async def get_readiness(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    *,
    version_id: UUID | None = None,
) -> ShotReadinessResponse:
    shot = await repository.find_shot(session, shot_id)
    if shot is None:
        raise _not_found("Shot")
    episode = await episode_for_content_read(session, claims, shot.episode_id)
    selected_version_id = version_id or shot.current_spec_version_id
    version: ShotSpecVersion | None = None
    references: list[AssetReference] = []
    if selected_version_id is not None:
        version_result = await repository.find_spec_version(session, selected_version_id)
        if version_result is None or version_result[0].shot_id != shot.id:
            raise _not_found("Shot spec version")
        version = version_result[0]
        references = await repository.list_asset_references(session, [version.id])
    script_states, asset_snapshots, assets_unavailable = await _resolve_readiness_dependencies(
        session,
        episode,
        [version] if version is not None else [],
        references,
    )
    coverage = await resolve_coverage_report(session, episode)
    return _evaluate_loaded_readiness(
        shot,
        version,
        references,
        script_state=(
            script_states[version.id]
            if version is not None
            else ScriptReadinessState(
                confirmed_structure_available=False,
                current_script_version_id=episode.current_script_version_id,
                narrative=None,
                narrative_unavailable=False,
            )
        ),
        asset_snapshots=asset_snapshots,
        assets_unavailable=assets_unavailable,
        coverage=coverage,
    )


async def get_episode_readiness(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> ShotReadinessBatchResponse:
    episode = await episode_for_content_read(session, claims, episode_id)
    rows = await repository.list_active_shots_with_current_specs(session, episode_id)
    versions = [version for _shot, version in rows if version is not None]
    references = await repository.list_asset_references(
        session,
        [version.id for version in versions],
    )
    references_by_version: dict[UUID, list[AssetReference]] = defaultdict(list)
    for reference in references:
        references_by_version[reference.shot_spec_version_id].append(reference)
    script_states, asset_snapshots, assets_unavailable = await _resolve_readiness_dependencies(
        session,
        episode,
        versions,
        references,
    )
    coverage = await resolve_coverage_report(session, episode)
    items = [
        _evaluate_loaded_readiness(
            shot,
            version,
            references_by_version.get(version.id, []) if version is not None else [],
            script_state=(
                script_states[version.id]
                if version is not None
                else ScriptReadinessState(
                    confirmed_structure_available=False,
                    current_script_version_id=episode.current_script_version_id,
                    narrative=None,
                    narrative_unavailable=False,
                )
            ),
            asset_snapshots=asset_snapshots,
            assets_unavailable=assets_unavailable,
            coverage=coverage,
        )
        for shot, version in rows
    ]
    summary = ShotReadinessSummary(
        total=len(items),
        ready=sum(item.status == "ready" for item in items),
        blocked=sum(item.status == "blocked" for item in items),
        unavailable=sum(item.status == "unavailable" for item in items),
    )
    evaluation_hash = canonical_payload_hash(
        {
            "episode_id": str(episode_id),
            "items": [
                {"shot_id": str(item.shot_id), "evaluation_hash": item.evaluation_hash}
                for item in items
            ],
            "summary": summary.model_dump(mode="json"),
        }
    )
    return ShotReadinessBatchResponse(
        episode_id=episode_id,
        items=items,
        summary=summary,
        evaluation_hash=evaluation_hash,
    )


async def summarize_episode_storyboards(
    session: AsyncSession,
    workspace_id: UUID,
    project_id: UUID,
    episode_ids: list[UUID],
) -> dict[UUID, EpisodeStoryboardSummary]:
    unique_episode_ids = list(dict.fromkeys(episode_ids))
    summaries = {
        episode_id: EpisodeStoryboardSummary(
            status="not_started",
            total=0,
            ready=0,
            blocked=0,
            unavailable=0,
        )
        for episode_id in unique_episode_ids
    }
    rows = await repository.list_active_shots_with_current_specs_for_episodes(
        session,
        workspace_id=workspace_id,
        episode_ids=unique_episode_ids,
    )
    if not rows:
        return summaries

    versions = [version for _shot, version in rows if version is not None]
    references = await repository.list_asset_references(
        session,
        [version.id for version in versions],
    )
    references_by_version: dict[UUID, list[AssetReference]] = defaultdict(list)
    for reference in references:
        references_by_version[reference.shot_spec_version_id].append(reference)
    (
        script_states,
        asset_snapshots,
        assets_unavailable,
    ) = await _resolve_project_readiness_dependencies(
        session,
        workspace_id=workspace_id,
        project_id=project_id,
        episode_id_by_version={
            version.id: shot.episode_id for shot, version in rows if version is not None
        },
        versions=versions,
        references=references,
    )
    coverage_by_episode: dict[UUID, CoverageReportResponse] = {}
    for episode_id in unique_episode_ids:
        context = await resolve_episode_content_context(
            session,
            workspace_id,
            episode_id,
        )
        coverage_by_episode[episode_id] = (
            await resolve_coverage_report(session, context)
            if context is not None
            else unavailable_coverage_report(episode_id)
        )
    items_by_episode: dict[UUID, list[ShotReadinessResponse]] = defaultdict(list)
    for shot, version in rows:
        items_by_episode[shot.episode_id].append(
            _evaluate_loaded_readiness(
                shot,
                version,
                (references_by_version.get(version.id, []) if version is not None else []),
                script_state=(
                    script_states[version.id]
                    if version is not None
                    else ScriptReadinessState(
                        confirmed_structure_available=False,
                        current_script_version_id=None,
                        narrative=None,
                        narrative_unavailable=True,
                    )
                ),
                asset_snapshots=asset_snapshots,
                assets_unavailable=assets_unavailable,
                coverage=coverage_by_episode[shot.episode_id],
            )
        )

    for episode_id, items in items_by_episode.items():
        ready = sum(item.status == "ready" for item in items)
        blocked = sum(item.status == "blocked" for item in items)
        unavailable = sum(item.status == "unavailable" for item in items)
        status: Literal["not_started", "blocked", "ready", "unavailable"]
        if unavailable:
            status = "unavailable"
        elif ready == len(items):
            status = "ready"
        else:
            status = "blocked"
        summaries[episode_id] = EpisodeStoryboardSummary(
            status=status,
            total=len(items),
            ready=ready,
            blocked=blocked,
            unavailable=unavailable,
        )
    return summaries


async def list_asset_shot_usages(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_version_id: UUID,
    *,
    limit: int,
    offset: int,
) -> PaginatedAssetShotUsages:
    await asset_version_for_content_read(session, claims, asset_version_id)
    rows, total = await repository.list_asset_version_usages(
        session,
        asset_version_id,
        limit=limit,
        offset=offset,
    )
    return PaginatedAssetShotUsages(
        items=[
            AssetShotUsageResponse(
                shot_id=shot.id,
                shot_title=shot.title,
                episode_id=shot.episode_id,
                spec_version_id=version.id,
                spec_version_no=version.version_no,
                slot_keys=slot_keys,
                is_current=(shot.status == "active" and shot.current_spec_version_id == version.id),
            )
            for version, shot, slot_keys in rows
        ],
        total=total,
        limit=limit,
        offset=offset,
    )


async def read_asset_usages(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    asset_version_ids: list[UUID],
    for_update: bool,
) -> list[AssetShotUsageSnapshot]:
    rows = await repository.list_asset_usages(
        session,
        workspace_id,
        asset_version_ids,
        for_update=for_update,
    )
    return [
        AssetShotUsageSnapshot(
            shot_id=shot.id,
            shot_title=shot.title,
            episode_id=shot.episode_id,
            spec_version_id=version.id,
            spec_version_no=version.version_no,
            current_spec_version_id=shot.current_spec_version_id,
            shot_status=shot.status,
            slot_keys=tuple(slot_keys),
        )
        for version, shot, slot_keys in rows
    ]


def _replacement_reference_requests(
    references: list[AssetReference],
    *,
    old_asset_version_id: UUID,
    new_asset_version_id: UUID,
) -> list[AssetReferenceRequest]:
    return [
        AssetReferenceRequest(
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
            asset_version_id=(
                new_asset_version_id
                if reference.asset_version_id == old_asset_version_id
                else reference.asset_version_id
            ),
            subject_key=reference.subject_key,
        )
        for reference in references
    ]


async def _asset_upgrade_snapshot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    *,
    old_asset_version_id: UUID,
    new_asset_version_id: UUID,
    shot_ids: list[UUID],
    for_update: bool,
) -> tuple[
    AssetUpgradePreflightResponse,
    list[tuple[Shot, ShotSpecVersion]],
    dict[UUID, list[AssetReference]],
]:
    old_asset = await asset_version_for_content_read(
        session,
        claims,
        old_asset_version_id,
    )
    new_asset = await asset_version_for_content_read(
        session,
        claims,
        new_asset_version_id,
    )
    if old_asset_version_id == new_asset_version_id:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Asset upgrade requires a different target version",
            status_code=422,
            next_action="select_new_asset_version",
        )
    if old_asset.asset_id != new_asset.asset_id:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Asset upgrade versions must belong to the same asset",
            status_code=422,
            next_action="select_version_of_same_asset",
        )
    initial = await repository.find_shots_with_current_specs(session, shot_ids)
    if len(initial) != len(shot_ids):
        raise _not_found("Shot")
    episode_ids = sorted({shot.episode_id for shot, _version in initial}, key=str)
    contexts: dict[UUID, EpisodeContentContext] = {}
    for episode_id in episode_ids:
        context = (
            await lock_active_episode_for_content_write(session, claims, episode_id)
            if for_update
            else await episode_for_content_read(session, claims, episode_id)
        )
        contexts[episode_id] = context
    if for_update:
        initial = await repository.find_shots_with_current_specs(
            session,
            shot_ids,
            for_update=True,
        )
    typed_rows: list[tuple[Shot, ShotSpecVersion]] = []
    for shot, version in initial:
        context = contexts[shot.episode_id]
        if (
            context.workspace_id != old_asset.workspace_id
            or context.project_id != old_asset.project_id
        ):
            raise _not_found("Shot")
        if shot.status != "active" or version is None:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT if for_update else ErrorCode.STATE_CONFLICT,
                "Asset upgrade target has no active current specification",
                status_code=409,
                next_action="repeat_asset_upgrade_preflight",
            )
        typed_rows.append((shot, version))

    readiness = await resolve_asset_versions_readiness(
        session,
        old_asset.workspace_id,
        old_asset.project_id,
        [new_asset_version_id],
        purpose="ai_short_drama_generation",
        channel="lanverse_preview",
        region="CN",
    )
    new_readiness = readiness.get(new_asset_version_id)
    if new_readiness is None or new_readiness.status != "ready":
        raise ApiError(
            ErrorCode.VERSION_CONFLICT if for_update else ErrorCode.STATE_CONFLICT,
            "New asset version is not ready for generation",
            status_code=409,
            next_action=(
                "repeat_asset_upgrade_preflight" if for_update else "complete_new_asset_version"
            ),
            details={
                "asset_version_id": str(new_asset_version_id),
                "blocker_codes": (
                    list(new_readiness.blocker_codes)
                    if new_readiness is not None
                    else ["asset_version_unavailable"]
                ),
            },
        )
    references = await repository.list_asset_references(
        session,
        [version.id for _shot, version in typed_rows],
    )
    references_by_version: dict[UUID, list[AssetReference]] = defaultdict(list)
    for reference in references:
        references_by_version[reference.shot_spec_version_id].append(reference)
    targets: list[AssetUpgradeTargetRequest] = []
    for shot, version in typed_rows:
        version_references = references_by_version[version.id]
        slot_keys = [
            reference.slot_key
            for reference in version_references
            if reference.asset_version_id == old_asset_version_id
        ]
        if not slot_keys:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT if for_update else ErrorCode.VALIDATION_FAILED,
                "Asset upgrade target does not currently use the old version",
                status_code=409 if for_update else 422,
                next_action="repeat_asset_upgrade_preflight",
            )
        replacement_references = _replacement_reference_requests(
            version_references,
            old_asset_version_id=old_asset_version_id,
            new_asset_version_id=new_asset_version_id,
        )
        hashes = storyboard_content_hashes(
            ShotSpec.model_validate(version.spec),
            replacement_references,
        )
        targets.append(
            AssetUpgradeTargetRequest(
                shot_id=shot.id,
                expected_spec_version_id=version.id,
                expected_shot_revision=shot.revision,
                slot_keys=slot_keys,
                new_input_hash=hashes.input_hash,
            )
        )
    preflight_hash = canonical_payload_hash(
        {
            "old_asset_version_id": str(old_asset_version_id),
            "new_asset_version_id": str(new_asset_version_id),
            "new_asset_evaluation_hash": new_readiness.evaluation_hash,
            "targets": [target.model_dump(mode="json") for target in targets],
        }
    )
    return (
        AssetUpgradePreflightResponse(
            old_asset_version_id=old_asset_version_id,
            new_asset_version_id=new_asset_version_id,
            targets=targets,
            preflight_hash=preflight_hash,
        ),
        typed_rows,
        references_by_version,
    )


async def preflight_asset_upgrade(
    session: AsyncSession,
    claims: AccessTokenClaims,
    old_asset_version_id: UUID,
    request: AssetUpgradePreflightRequest,
) -> AssetUpgradePreflightResponse:
    preflight, _rows, _references = await _asset_upgrade_snapshot(
        session,
        claims,
        old_asset_version_id=old_asset_version_id,
        new_asset_version_id=request.new_asset_version_id,
        shot_ids=request.shot_ids,
        for_update=False,
    )
    return preflight


async def apply_asset_upgrade(
    session: AsyncSession,
    claims: AccessTokenClaims,
    old_asset_version_id: UUID,
    request: AssetUpgradeApplyRequest,
    *,
    trace_id: str,
) -> AssetUpgradeApplyResponse:
    async with session.begin():
        current, rows, references_by_version = await _asset_upgrade_snapshot(
            session,
            claims,
            old_asset_version_id=old_asset_version_id,
            new_asset_version_id=request.new_asset_version_id,
            shot_ids=[target.shot_id for target in request.targets],
            for_update=True,
        )
        if current.preflight_hash != request.preflight_hash or (current.targets != request.targets):
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Asset upgrade preflight has changed",
                status_code=409,
                next_action="repeat_asset_upgrade_preflight",
                details={"current_preflight": current.model_dump(mode="json")},
            )
        latest_numbers = await repository.latest_spec_version_numbers(
            session,
            [shot.id for shot, _version in rows],
        )
        source_narrative_references = await coverage_repository.list_references(
            session,
            [version.id for _shot, version in rows],
        )
        narrative_by_version: dict[UUID, list[ShotNarrativeReference]] = defaultdict(list)
        for reference in source_narrative_references:
            narrative_by_version[reference.shot_spec_version_id].append(reference)
        now = datetime.now(UTC)
        versions: list[ShotSpecVersion] = []
        stored_references: list[list[AssetReference]] = []
        replacement_requests_by_version: list[list[AssetReferenceRequest]] = []
        for (shot, source_version), target in zip(
            rows,
            request.targets,
            strict=True,
        ):
            source_references = references_by_version[source_version.id]
            replacement_requests = _replacement_reference_requests(
                source_references,
                old_asset_version_id=old_asset_version_id,
                new_asset_version_id=request.new_asset_version_id,
            )
            spec = ShotSpec.model_validate(source_version.spec)
            hashes = storyboard_content_hashes(spec, replacement_requests)
            if hashes.input_hash != target.new_input_hash:
                raise ApiError(
                    ErrorCode.VERSION_CONFLICT,
                    "Asset upgrade input has changed",
                    status_code=409,
                    next_action="repeat_asset_upgrade_preflight",
                )
            version = ShotSpecVersion(
                id=uuid7(),
                workspace_id=shot.workspace_id,
                shot_id=shot.id,
                version_no=latest_numbers[shot.id] + 1,
                schema_version=source_version.schema_version,
                spec=spec.model_dump(mode="json"),
                content_hash=hashes.content_hash,
                input_hash=hashes.input_hash,
                created_by=claims.sub,
                created_at=now,
            )
            versions.append(version)
            replacement_requests_by_version.append(replacement_requests)
            session.add(version)
        await session.flush()
        resolved_by_version: dict[UUID, AssetVersionReference] = {}
        for reference in (
            item for requests in replacement_requests_by_version for item in requests
        ):
            if reference.asset_version_id in resolved_by_version:
                continue
            resolved = await resolve_asset_version(
                session,
                rows[0][0].workspace_id,
                reference.asset_version_id,
            )
            if resolved is None:
                raise ApiError(
                    ErrorCode.DEPENDENCY_UNAVAILABLE,
                    "Asset version became unavailable during upgrade",
                    status_code=503,
                )
            resolved_by_version[reference.asset_version_id] = resolved
        for (shot, source_version), version, replacement_requests in zip(
            rows,
            versions,
            replacement_requests_by_version,
            strict=True,
        ):
            references = [
                AssetReference(
                    id=uuid7(),
                    workspace_id=shot.workspace_id,
                    shot_spec_version_id=version.id,
                    slot_key=reference.slot_key,
                    role=reference.role,
                    asset_version_id=reference.asset_version_id,
                    asset_state_id=resolved_by_version[reference.asset_version_id].asset_state_id,
                    asset_id=resolved_by_version[reference.asset_version_id].asset_id,
                    binding_source="manual",
                    subject_key=reference.subject_key,
                    created_at=now,
                )
                for reference in replacement_requests
            ]
            session.add_all(references)
            await _store_narrative_references(
                session,
                shot=shot,
                version=version,
                inputs=[
                    _narrative_input(reference)
                    for reference in narrative_by_version[source_version.id]
                ],
                actor_id=claims.sub,
                now=now,
                origin="migrated",
            )
            stored_references.append(references)
            shot.current_spec_version_id = version.id
            shot.revision += 1
            shot.updated_at = now
            _append_spec_version_audit(
                session,
                shot=shot,
                version=version,
                actor_id=claims.sub,
                trace_id=trace_id,
                source="asset_upgrade",
                previous_version_id=source_version.id,
                occurred_at=now,
            )
        await session.flush()
        result = AssetUpgradeApplyResponse(
            shots=[_shot_response(shot) for shot, _version in rows],
            spec_versions=[
                _spec_response(version, references)
                for version, references in zip(
                    versions,
                    stored_references,
                    strict=True,
                )
            ],
        )
    return result


async def get_production_snapshot(
    session: AsyncSession,
    workspace_id: UUID,
    version_id: UUID,
    *,
    for_update: bool = False,
) -> ShotProductionSnapshot | None:
    version_result = await repository.find_spec_version(
        session,
        version_id,
        for_update=for_update,
    )
    if version_result is None:
        return None
    version, shot = version_result
    if version.workspace_id != workspace_id or shot.workspace_id != workspace_id:
        return None
    episode = await resolve_episode_content_context(
        session,
        workspace_id,
        shot.episode_id,
    )
    if episode is None:
        return None
    references = await repository.list_asset_references(session, [version.id])
    script_states, asset_snapshots, assets_unavailable = await _resolve_readiness_dependencies(
        session,
        episode,
        [version],
        references,
    )
    coverage = await resolve_coverage_report(session, episode)
    readiness = _evaluate_loaded_readiness(
        shot,
        version,
        references,
        script_state=script_states[version.id],
        asset_snapshots=asset_snapshots,
        assets_unavailable=assets_unavailable,
        coverage=coverage,
    )
    spec = ShotSpec.model_validate(version.spec)
    return ShotProductionSnapshot(
        spec_ref=ShotSpecRef(
            workspace_id=workspace_id,
            episode_id=shot.episode_id,
            shot_id=shot.id,
            shot_spec_version_id=version.id,
            schema_version=version.schema_version,
            content_hash=version.content_hash,
            input_hash=version.input_hash,
        ),
        shot_status=cast(Literal["active", "archived"], shot.status),
        current_spec_version_id=shot.current_spec_version_id,
        shot_revision=shot.revision,
        spec=spec.model_dump(mode="json"),
        asset_references=tuple(
            ShotAssetReferenceSnapshot(
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
                asset_state_id=reference.asset_state_id,
                asset_id=reference.asset_id,
                binding_source=cast(Literal["manual", "ai"], reference.binding_source),
                subject_key=reference.subject_key,
            )
            for reference in references
        ),
        readiness_status=readiness.status,
        ready=readiness.ready,
        blocking_codes=tuple(issue.code for issue in readiness.blocking_reasons),
        warning_codes=tuple(warning.code for warning in readiness.warnings),
        evaluation_hash=readiness.evaluation_hash,
    )
