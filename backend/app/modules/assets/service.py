import json
from datetime import UTC, datetime
from hashlib import sha256
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.assets import repository
from app.modules.assets.contracts import (
    AssetCandidateCommand,
    AssetCandidateDecisionCountReader,
    AssetCandidateResult,
    AssetOccurrenceNarrativeReader,
    AssetOccurrenceNarrativeSnapshot,
    AssetVersionReadinessReference,
    AssetVersionReference,
    ProjectAssetReferenceSummary,
    ProjectAssetSummary,
    StoryboardAssetInput,
)
from app.modules.assets.models import (
    Asset,
    AssetMediaReference,
    AssetNameRevision,
    AssetOccurrenceDecision,
    AssetState,
    AssetVersion,
)
from app.modules.assets.schemas import (
    AssetBibleAsset,
    AssetBibleResponse,
    AssetBibleState,
    AssetBibleSummary,
    AssetCreateRequest,
    AssetDeleteBlocker,
    AssetDeletePreflightResponse,
    AssetKind,
    AssetMediaReferenceResponse,
    AssetOccurrenceDecisionResponse,
    AssetOccurrenceRequest,
    AssetOccurrenceResponse,
    AssetReadinessBlocker,
    AssetReadinessDependencySnapshot,
    AssetReadinessResponse,
    AssetResponse,
    AssetSpec,
    AssetStateCreateRequest,
    AssetStateCreateResponse,
    AssetStateReadinessResponse,
    AssetStateReadinessSnapshot,
    AssetStateResponse,
    AssetStatusRequest,
    AssetUpdateRequest,
    AssetVersionCreateRequest,
    AssetVersionCreateResponse,
    AssetVersionResponse,
    PaginatedAssetOccurrences,
    PaginatedAssets,
    PaginatedAssetStates,
    PaginatedAssetVersions,
    ReadinessStatus,
    parse_asset_spec,
    spec_to_json,
)
from app.modules.governance import (
    RightsGateResult,
    RightsUsage,
    SubjectReference,
    SubjectType,
    check_rights,
    check_rights_for_resolved_subjects,
)
from app.modules.governance.audit import append_audit_event
from app.modules.identity import ActorContext
from app.modules.media import (
    MediaVersionReference,
    resolve_media_version_reference,
    resolve_media_version_references,
)
from app.modules.projects import (
    lock_active_project_for_content_write,
    project_for_content_read,
)

_ALLOWED_MEDIA_PURPOSES: dict[str, frozenset[str]] = {
    "character": frozenset({"portrait", "full_body", "expression", "turnaround"}),
    "location": frozenset({"environment"}),
    "prop": frozenset({"object"}),
    "costume": frozenset({"outfit"}),
    "visual_style": frozenset({"style_reference"}),
    "voice": frozenset({"voice_sample"}),
}
_MEDIA_KIND_BY_PURPOSE: dict[str, str] = {
    "portrait": "image",
    "full_body": "image",
    "expression": "image",
    "turnaround": "image",
    "environment": "image",
    "object": "image",
    "outfit": "image",
    "style_reference": "image",
    "voice_sample": "audio",
}
_REQUIRED_MEDIA_PURPOSE: dict[str, str | None] = {
    "character": "portrait",
    "location": "environment",
    "prop": "object",
    "costume": "outfit",
    "visual_style": None,
    "voice": "voice_sample",
}
_REQUIRED_SPEC_FIELDS: dict[str, tuple[str, ...]] = {
    "character": ("identity", "appearance", "age_impression", "temperament"),
    "location": (
        "spatial_description",
        "time_weather",
        "visual_elements",
        "lighting",
    ),
    "prop": ("appearance", "material", "usage_context"),
    "costume": ("appearance", "material", "usage_context", "wearer_character_id"),
    "visual_style": ("visual_language", "palette", "lighting_language"),
    "voice": ("source_kind", "language", "performance_traits", "allowed_usage"),
}


def normalize_name(value: str) -> str:
    return " ".join(value.strip().casefold().split())


def clean_values(values: list[str]) -> list[str]:
    cleaned: list[str] = []
    seen: set[str] = set()
    for value in values:
        item = value.strip()
        if item and item not in seen:
            seen.add(item)
            cleaned.append(item)
    return cleaned


def asset_response(
    asset: Asset,
    *,
    duplicate: bool = False,
) -> AssetResponse:
    return AssetResponse(
        id=asset.id,
        workspace_id=asset.workspace_id,
        project_id=asset.project_id,
        kind=cast(AssetKind, asset.kind),
        name=asset.name,
        aliases=asset.aliases,
        tags=asset.tags,
        status=cast(Literal["active", "archived"], asset.status),
        availability=cast(Literal["enabled", "disabled"], asset.availability),
        name_revision=asset.name_revision,
        revision=asset.revision,
        created_at=asset.created_at,
        updated_at=asset.updated_at,
        warnings=["duplicate_name"] if duplicate else [],
    )


def state_response(state: AssetState) -> AssetStateResponse:
    return AssetStateResponse(
        id=state.id,
        workspace_id=state.workspace_id,
        asset_id=state.asset_id,
        state_key=state.state_key,
        label=state.label,
        description=state.description,
        status=cast(Literal["active", "disabled"], state.status),
        current_version_id=state.current_version_id,
        revision=state.revision,
        created_by=state.created_by,
        created_at=state.created_at,
        updated_at=state.updated_at,
    )


def _version_response(
    version: AssetVersion,
    references: list[AssetMediaReference],
) -> AssetVersionResponse:
    return AssetVersionResponse(
        id=version.id,
        workspace_id=version.workspace_id,
        asset_id=version.asset_id,
        asset_state_id=version.asset_state_id,
        version_no=version.version_no,
        schema_version=version.schema_version,
        spec=parse_asset_spec(str(version.spec["kind"]), version.spec),
        prompt_description=version.prompt_description,
        source_type=cast(
            Literal["manual", "script_extraction_candidate"],
            version.source_type,
        ),
        source_id=version.source_id,
        content_hash=version.content_hash,
        media_references=[
            AssetMediaReferenceResponse(
                media_version_id=item.media_version_id,
                purpose=item.purpose,
                position=item.position,
            )
            for item in references
        ],
        created_by=version.created_by,
        created_at=version.created_at,
    )


def _version_hash(
    spec: AssetSpec,
    prompt_description: str,
    references: list[tuple[UUID, str, int]],
    source_type: str,
    source_id: UUID | None,
) -> str:
    canonical = json.dumps(
        {
            "schema_version": 1,
            "spec": spec_to_json(spec),
            "prompt_description": prompt_description.strip(),
            "media_references": [
                {
                    "media_version_id": str(media_id),
                    "purpose": purpose,
                    "position": position,
                }
                for media_id, purpose, position in sorted(
                    references, key=lambda item: (item[1], item[2], str(item[0]))
                )
            ],
            "source_type": source_type,
            "source_id": str(source_id) if source_id else None,
        },
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return sha256(canonical.encode()).hexdigest()


def asset_not_found(resource: str = "Asset") -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, f"{resource} not found", status_code=404)


def require_asset_revision(asset: Asset, expected_revision: int) -> None:
    if asset.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Asset has changed",
            status_code=409,
            details={"current_revision": asset.revision},
        )


def require_state_revision(state: AssetState, expected_revision: int) -> None:
    if state.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Asset state has changed",
            status_code=409,
            details={"current_revision": state.revision},
        )


def require_expected_current(state: AssetState, expected_current_version_id: UUID | None) -> None:
    if state.current_version_id != expected_current_version_id:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Current asset state version has changed",
            status_code=409,
            details={
                "current_version_id": (
                    str(state.current_version_id) if state.current_version_id is not None else None
                )
            },
        )


async def asset_for_read(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
) -> Asset:
    asset = await repository.find_asset(session, asset_id)
    if asset is None:
        raise asset_not_found()
    try:
        context = await project_for_content_read(session, claims, asset.project_id)
    except ApiError as error:
        if error.code == ErrorCode.NOT_FOUND:
            raise asset_not_found() from error
        raise
    if context.workspace_id != asset.workspace_id:
        raise asset_not_found()
    return asset


async def lock_asset_for_write(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
) -> Asset:
    asset_reference = await repository.find_asset(session, asset_id)
    if asset_reference is None:
        raise asset_not_found()
    try:
        context = await lock_active_project_for_content_write(
            session, claims, asset_reference.project_id
        )
    except ApiError as error:
        if error.code == ErrorCode.NOT_FOUND:
            raise asset_not_found() from error
        raise
    asset = await repository.find_asset(session, asset_id, for_update=True)
    if (
        asset is None
        or asset.project_id != context.project_id
        or context.workspace_id != asset.workspace_id
    ):
        raise asset_not_found()
    return asset


async def state_for_read(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
) -> tuple[AssetState, Asset]:
    state = await repository.find_state(session, state_id)
    if state is None:
        raise asset_not_found("Asset state")
    asset = await asset_for_read(session, claims, state.asset_id)
    if state.workspace_id != asset.workspace_id:
        raise asset_not_found("Asset state")
    return state, asset


async def lock_state_for_write(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
) -> tuple[AssetState, Asset]:
    state_reference = await repository.find_state(session, state_id)
    if state_reference is None:
        raise asset_not_found("Asset state")
    asset = await lock_asset_for_write(session, claims, state_reference.asset_id)
    state = await repository.find_state(session, state_id, for_update=True)
    if state is None or state.asset_id != asset.id or state.workspace_id != asset.workspace_id:
        raise asset_not_found("Asset state")
    return state, asset


async def create_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: AssetCreateRequest,
    *,
    trace_id: str,
) -> AssetResponse:
    name = request.name.strip()
    if not name:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Asset name is required", status_code=422)
    async with session.begin():
        project = await lock_active_project_for_content_write(session, claims, project_id)
        normalized_name = normalize_name(name)
        duplicate = (
            await repository.find_duplicate_name(session, project_id, request.kind, normalized_name)
            is not None
        )
        asset = Asset(
            id=uuid7(),
            workspace_id=project.workspace_id,
            project_id=project.project_id,
            kind=request.kind,
            name=name,
            normalized_name=normalized_name,
            aliases=clean_values(request.aliases),
            tags=clean_values(request.tags),
            status="active",
            availability="enabled",
            name_revision=1,
            revision=1,
            created_by=claims.sub,
        )
        session.add(asset)
        session.add(
            AssetNameRevision(
                asset_id=asset.id,
                revision_no=1,
                workspace_id=asset.workspace_id,
                name_snapshot=asset.name,
                normalized_name=asset.normalized_name,
                created_by=claims.sub,
            )
        )
        session.add(
            AssetState(
                id=uuid7(),
                workspace_id=asset.workspace_id,
                asset_id=asset.id,
                state_key="base",
                label="基础状态",
                description="",
                status="active",
                current_version_id=None,
                revision=1,
                creation_key="base",
                command_receipts={},
                created_by=claims.sub,
            )
        )
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.created",
            target_type="asset",
            target_id=asset.id,
            trace_id=trace_id,
            metadata={
                "revision": asset.revision,
                "kind": asset.kind,
                "project_id": str(asset.project_id),
            },
        )
        await session.flush()
    return asset_response(asset, duplicate=duplicate)


async def list_assets(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    *,
    kind: AssetKind | None,
    include_archived: bool,
    query: str | None,
    limit: int,
    offset: int,
) -> PaginatedAssets:
    await project_for_content_read(session, claims, project_id)
    rows, total = await repository.list_assets(
        session,
        project_id,
        kind=kind,
        include_archived=include_archived,
        query=query,
        limit=limit,
        offset=offset,
    )
    return PaginatedAssets(
        items=[asset_response(item) for item in rows],
        total=total,
        limit=limit,
        offset=offset,
    )


async def get_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
) -> AssetResponse:
    return asset_response(await asset_for_read(session, claims, asset_id))


async def create_state(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetStateCreateRequest,
    *,
    trace_id: str,
) -> AssetStateCreateResponse:
    state_key = request.state_key.strip()
    label = request.label.strip()
    description = request.description.strip()
    idempotency_key = request.idempotency_key.strip()
    async with session.begin():
        asset = await lock_asset_for_write(session, claims, asset_id)
        replay = await repository.find_state_by_creation_key(
            session,
            asset.id,
            idempotency_key,
        )
        if replay is not None:
            if (
                replay.state_key != state_key
                or replay.label != label
                or replay.description != description
            ):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Asset state idempotency key was reused with different input",
                    status_code=409,
                )
            return AssetStateCreateResponse(
                asset=asset_response(asset),
                state=state_response(replay),
            )
        if await repository.find_state_by_key(session, asset.id, state_key) is not None:
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Asset state key already exists",
                status_code=409,
            )
        require_asset_revision(asset, request.expected_asset_revision)
        now = datetime.now(UTC)
        state = AssetState(
            id=uuid7(),
            workspace_id=asset.workspace_id,
            asset_id=asset.id,
            state_key=state_key,
            label=label,
            description=description,
            status="active",
            current_version_id=None,
            revision=1,
            creation_key=idempotency_key,
            command_receipts={},
            created_by=claims.sub,
            created_at=now,
            updated_at=now,
        )
        session.add(state)
        asset.revision += 1
        asset.updated_at = now
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.state_created",
            target_type="asset_state",
            target_id=state.id,
            trace_id=trace_id,
            metadata={
                "asset_id": str(asset.id),
                "asset_revision": asset.revision,
                "state_key": state.state_key,
            },
            occurred_at=now,
        )
        await session.flush()
    return AssetStateCreateResponse(
        asset=asset_response(asset),
        state=state_response(state),
    )


async def list_states(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
) -> PaginatedAssetStates:
    asset = await asset_for_read(session, claims, asset_id)
    states = await repository.list_states(session, asset.id)
    return PaginatedAssetStates(
        items=[state_response(state) for state in states],
        total=len(states),
    )


def _current_occurrence_rows(
    rows: list[AssetOccurrenceDecision],
) -> list[AssetOccurrenceDecision]:
    latest: dict[tuple[UUID, UUID], AssetOccurrenceDecision] = {}
    for row in rows:
        latest[(row.asset_state_id, row.narrative_unit_id)] = row
    return [row for row in latest.values() if row.decision == "link"]


def _occurrence_response(
    row: AssetOccurrenceDecision,
    narratives: dict[UUID, AssetOccurrenceNarrativeSnapshot],
) -> AssetOccurrenceResponse:
    narrative = narratives.get(row.narrative_unit_version_id)
    freshness: Literal["current", "stale"] = (
        "current"
        if narrative is not None
        and narrative.narrative_unit_id == row.narrative_unit_id
        and narrative.is_current
        else "stale"
    )
    return AssetOccurrenceResponse(
        id=row.id,
        workspace_id=row.workspace_id,
        asset_state_id=row.asset_state_id,
        episode_id=row.episode_id,
        narrative_unit_id=row.narrative_unit_id,
        narrative_unit_version_id=row.narrative_unit_version_id,
        sequence=row.sequence,
        decision=cast(Literal["link", "unlink"], row.decision),
        origin=cast(Literal["manual", "script_candidate"], row.origin),
        evidence_hash=row.evidence_hash,
        idempotency_key=row.idempotency_key,
        freshness=freshness,
        created_by=row.created_by,
        created_at=row.created_at,
    )


async def decide_occurrence(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    request: AssetOccurrenceRequest,
    read_narratives: AssetOccurrenceNarrativeReader,
    *,
    trace_id: str,
) -> AssetOccurrenceDecisionResponse:
    async with session.begin():
        state, asset = await lock_state_for_write(session, claims, state_id)
        replay = await repository.find_occurrence_by_key(
            session,
            state.id,
            request.idempotency_key,
        )
        if replay is not None:
            if (
                replay.decision != request.decision
                or replay.narrative_unit_id != request.narrative_unit_id
                or replay.narrative_unit_version_id != request.narrative_unit_version_id
            ):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Occurrence idempotency key was reused with different input",
                    status_code=409,
                )
            replay_narratives = await read_narratives(
                workspace_id=state.workspace_id,
                unit_version_ids=[replay.narrative_unit_version_id],
            )
            return AssetOccurrenceDecisionResponse(
                state=state_response(state),
                decision=_occurrence_response(replay, replay_narratives),
            )
        require_state_revision(state, request.expected_revision)
        if state.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Disabled asset state cannot change occurrences",
                status_code=409,
                next_action="enable_asset_state",
            )
        narratives = await read_narratives(
            workspace_id=state.workspace_id,
            unit_version_ids=[request.narrative_unit_version_id],
        )
        narrative = narratives.get(request.narrative_unit_version_id)
        if narrative is None or narrative.narrative_unit_id != request.narrative_unit_id:
            raise asset_not_found("Narrative unit version")
        if narrative.project_id != asset.project_id:
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Narrative unit belongs to another project",
                status_code=409,
            )
        if not narrative.is_current:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Narrative unit version is not current",
                status_code=409,
                next_action="refresh_narrative_structure",
            )
        sequence = await repository.latest_occurrence_sequence(session, state.id) + 1
        evidence_hash = sha256(
            json.dumps(
                {
                    "asset_state_id": str(state.id),
                    "decision": request.decision,
                    "episode_id": str(narrative.episode_id),
                    "narrative_unit_id": str(narrative.narrative_unit_id),
                    "narrative_unit_version_id": str(narrative.narrative_unit_version_id),
                    "text_hash": narrative.text_hash,
                },
                sort_keys=True,
                separators=(",", ":"),
            ).encode()
        ).hexdigest()
        now = datetime.now(UTC)
        decision = AssetOccurrenceDecision(
            id=uuid7(),
            workspace_id=state.workspace_id,
            asset_state_id=state.id,
            episode_id=narrative.episode_id,
            narrative_unit_id=narrative.narrative_unit_id,
            narrative_unit_version_id=narrative.narrative_unit_version_id,
            sequence=sequence,
            decision=request.decision,
            origin="manual",
            evidence_hash=evidence_hash,
            idempotency_key=request.idempotency_key,
            created_by=claims.sub,
            created_at=now,
        )
        session.add(decision)
        state.revision += 1
        state.updated_at = now
        append_audit_event(
            session,
            workspace_id=state.workspace_id,
            actor_id=claims.sub,
            action="asset.occurrence_decided",
            target_type="asset_state",
            target_id=state.id,
            trace_id=trace_id,
            metadata={
                "asset_id": str(asset.id),
                "state_revision": state.revision,
                "decision_id": str(decision.id),
                "decision": decision.decision,
                "narrative_unit_id": str(decision.narrative_unit_id),
            },
            occurred_at=now,
        )
        await session.flush()
    return AssetOccurrenceDecisionResponse(
        state=state_response(state),
        decision=_occurrence_response(decision, narratives),
    )


async def list_occurrences(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    read_narratives: AssetOccurrenceNarrativeReader,
    *,
    include_history: bool,
) -> PaginatedAssetOccurrences:
    state, _asset = await state_for_read(session, claims, state_id)
    history = await repository.list_occurrence_decisions(session, [state.id])
    rows = history if include_history else _current_occurrence_rows(history)
    narratives = await read_narratives(
        workspace_id=state.workspace_id,
        unit_version_ids=[row.narrative_unit_version_id for row in rows],
    )
    return PaginatedAssetOccurrences(
        items=[_occurrence_response(row, narratives) for row in rows],
        total=len(rows),
    )


async def update_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetUpdateRequest,
    *,
    trace_id: str,
) -> AssetResponse:
    changes = request.model_dump(exclude={"expected_revision"}, exclude_unset=True)
    if not changes:
        raise ApiError(ErrorCode.INVALID_REQUEST, "No asset changes supplied", status_code=422)
    async with session.begin():
        asset = await lock_asset_for_write(session, claims, asset_id)
        require_asset_revision(asset, request.expected_revision)
        if request.aliases is not None:
            asset.aliases = clean_values(request.aliases)
        if request.tags is not None:
            asset.tags = clean_values(request.tags)
        now = datetime.now(UTC)
        asset.revision += 1
        asset.updated_at = now
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.updated",
            target_type="asset",
            target_id=asset.id,
            trace_id=trace_id,
            metadata={
                "revision": asset.revision,
                "changed_fields": sorted(changes),
            },
            occurred_at=now,
        )
        await session.flush()
    return asset_response(asset)


async def set_asset_archived(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetStatusRequest,
    *,
    archived: bool,
    trace_id: str,
) -> AssetResponse:
    expected_status = "active" if archived else "archived"
    async with session.begin():
        asset = await lock_asset_for_write(session, claims, asset_id)
        require_asset_revision(asset, request.expected_revision)
        if asset.status != expected_status:
            raise ApiError(ErrorCode.STATE_CONFLICT, "Asset state conflict", status_code=409)
        previous_status = asset.status
        now = datetime.now(UTC)
        asset.status = "archived" if archived else "active"
        asset.archived_at = now if archived else None
        asset.archived_by = claims.sub if archived else None
        asset.revision += 1
        asset.updated_at = now
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.archived" if archived else "asset.restored",
            target_type="asset",
            target_id=asset.id,
            trace_id=trace_id,
            metadata={
                "revision": asset.revision,
                "previous_status": previous_status,
                "status": asset.status,
            },
            occurred_at=now,
        )
        await session.flush()
    return asset_response(asset)


async def delete_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    read_candidate_decision_counts: AssetCandidateDecisionCountReader,
) -> AssetDeletePreflightResponse:
    asset = await asset_for_read(session, claims, asset_id)
    version_count = await repository.count_versions(session, asset.id)
    decision_count = (
        await read_candidate_decision_counts(
            workspace_id=asset.workspace_id,
            asset_ids=[asset.id],
        )
    ).get(asset.id, 0)
    related_version_count = (
        await repository.count_related_asset_versions(
            session,
            asset.workspace_id,
            [asset.id],
        )
    ).get(asset.id, 0)
    blockers: list[AssetDeleteBlocker] = []
    if version_count:
        blockers.append(
            AssetDeleteBlocker(
                code="asset_has_versions",
                summary=f"Asset has {version_count} immutable version(s)",
                version_count=version_count,
                decision_count=0,
                related_version_count=0,
            )
        )
    if related_version_count:
        blockers.append(
            AssetDeleteBlocker(
                code="asset_has_related_versions",
                summary=(
                    f"Asset is referenced by {related_version_count} related asset version(s)"
                ),
                version_count=0,
                decision_count=0,
                related_version_count=related_version_count,
            )
        )
    if decision_count:
        blockers.append(
            AssetDeleteBlocker(
                code="asset_has_candidate_decisions",
                summary=(f"Asset is linked from {decision_count} script candidate decision(s)"),
                version_count=0,
                decision_count=decision_count,
                related_version_count=0,
            )
        )
    return AssetDeletePreflightResponse(allowed=not blockers, blockers=blockers)


async def delete_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    expected_revision: int,
    read_candidate_decision_counts: AssetCandidateDecisionCountReader,
    *,
    trace_id: str,
) -> None:
    async with session.begin():
        asset = await lock_asset_for_write(session, claims, asset_id)
        require_asset_revision(asset, expected_revision)
        if await repository.count_versions(session, asset.id):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Asset has immutable versions",
                status_code=409,
                next_action="review_delete_blockers",
            )
        related_version_count = (
            await repository.count_related_asset_versions(
                session,
                asset.workspace_id,
                [asset.id],
            )
        ).get(asset.id, 0)
        if related_version_count:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Asset has related asset version references",
                status_code=409,
                next_action="review_delete_blockers",
            )
        decision_count = (
            await read_candidate_decision_counts(
                workspace_id=asset.workspace_id,
                asset_ids=[asset.id],
            )
        ).get(asset.id, 0)
        if decision_count:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Asset has candidate decision references",
                status_code=409,
                next_action="review_delete_blockers",
            )
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.deleted",
            target_type="asset",
            target_id=asset.id,
            trace_id=trace_id,
            metadata={
                "revision": asset.revision,
                "kind": asset.kind,
                "project_id": str(asset.project_id),
            },
        )
        await repository.delete_states(session, asset.id)
        await session.flush()
        await session.delete(asset)


async def _validate_related_assets(
    session: AsyncSession,
    asset: Asset,
    spec: AssetSpec,
) -> None:
    related_id = None
    if spec.kind == "prop":
        related_id = spec.holder_character_id
    elif spec.kind == "costume":
        related_id = spec.wearer_character_id
    if related_id is None:
        return
    related = await repository.find_asset(session, related_id, for_update=True)
    if (
        related is None
        or related.workspace_id != asset.workspace_id
        or related.project_id != asset.project_id
        or related.kind != "character"
    ):
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Related character must belong to the same project",
            status_code=422,
        )


async def _resolve_media_references(
    session: AsyncSession,
    asset: Asset,
    request: AssetVersionCreateRequest,
) -> dict[UUID, MediaVersionReference]:
    resolved: dict[UUID, MediaVersionReference] = {}
    allowed = _ALLOWED_MEDIA_PURPOSES[asset.kind]
    for reference in request.media_references:
        if reference.purpose not in allowed:
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Media purpose does not match asset kind",
                status_code=422,
                details={"purpose": reference.purpose, "asset_kind": asset.kind},
            )
        media = await resolve_media_version_reference(
            session, asset.workspace_id, reference.media_version_id
        )
        if media is None:
            raise asset_not_found("Media version")
        expected_kind = _MEDIA_KIND_BY_PURPOSE[reference.purpose]
        if media.kind != expected_kind:
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Media kind does not match reference purpose",
                status_code=422,
                details={"expected_kind": expected_kind, "actual_kind": media.kind},
            )
        if media.object_status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Archived media cannot be used by a new asset version",
                status_code=409,
                next_action="restore_media",
            )
        resolved[media.id] = media
    return resolved


def _missing_spec_blockers(spec: AssetSpec) -> list[AssetReadinessBlocker]:
    payload = spec.model_dump(mode="python")
    blockers: list[AssetReadinessBlocker] = []
    for field in _REQUIRED_SPEC_FIELDS[spec.kind]:
        value = payload.get(field)
        missing = value is None or value == "" or value == []
        if missing:
            blockers.append(
                AssetReadinessBlocker(
                    code="required_field_missing",
                    field_path=f"spec.{field}",
                    summary=f"Required asset field {field} is missing",
                    next_action="complete_asset_spec",
                )
            )
    return blockers


def _media_blockers(
    kind: str,
    references: list[AssetMediaReference],
    media: dict[UUID, MediaVersionReference],
) -> tuple[list[AssetReadinessBlocker], bool]:
    blockers: list[AssetReadinessBlocker] = []
    required = _REQUIRED_MEDIA_PURPOSE[kind]
    if required is not None and not any(item.purpose == required for item in references):
        blockers.append(
            AssetReadinessBlocker(
                code="required_media_missing",
                field_path="media_references",
                summary=f"Required media purpose {required} is missing",
                next_action="attach_media_reference",
            )
        )
    for reference in references:
        snapshot = media.get(reference.media_version_id)
        if snapshot is None:
            blockers.append(
                AssetReadinessBlocker(
                    code="media_unavailable",
                    dependency_type="MEDIA_VERSION",
                    dependency_id=reference.media_version_id,
                    summary="Referenced media is unavailable",
                    next_action="replace_media_reference",
                )
            )
            continue
        if snapshot.probe_status != "ready" or not snapshot.has_active_location:
            blockers.append(
                AssetReadinessBlocker(
                    code=f"media_{snapshot.probe_status}",
                    dependency_type="MEDIA_VERSION",
                    dependency_id=snapshot.id,
                    summary="Referenced media is not ready",
                    next_action="review_media_probe",
                )
            )
    return blockers, required is not None and not any(
        item.purpose == required for item in references
    )


def _readiness_prerequisites(
    asset: Asset,
    version: AssetVersion,
    references: list[AssetMediaReference],
    *,
    media: dict[UUID, MediaVersionReference],
) -> tuple[list[AssetReadinessBlocker], bool]:
    spec = parse_asset_spec(asset.kind, version.spec)
    blockers = _missing_spec_blockers(spec)
    draft = bool(blockers)
    media_blockers, required_media_missing = _media_blockers(asset.kind, references, media)
    blockers.extend(media_blockers)
    draft = draft or required_media_missing
    return blockers, draft


def _compose_readiness(
    asset: Asset,
    state: AssetState,
    version: AssetVersion,
    references: list[AssetMediaReference],
    *,
    blockers: list[AssetReadinessBlocker],
    draft: bool,
    rights: RightsGateResult | None,
    rights_unavailable: bool,
    at_time: datetime,
) -> AssetReadinessResponse:
    blockers = list(blockers)
    consent_ids: list[UUID] = []
    if state.status != "active":
        blockers.append(
            AssetReadinessBlocker(
                code="asset_state_disabled",
                dependency_type="ASSET_STATE",
                dependency_id=state.id,
                summary="Disabled asset state cannot be used by new work",
                next_action="enable_asset_state",
            )
        )
    if (
        not draft
        and asset.status == "active"
        and asset.availability == "enabled"
        and state.status == "active"
    ):
        if rights_unavailable:
            blockers.append(
                AssetReadinessBlocker(
                    code="rights_dependency_unavailable",
                    dependency_type="GOVERNANCE",
                    summary="Rights evaluation is unavailable",
                    next_action="retry_readiness",
                )
            )
        elif rights is not None:
            consent_ids = list(rights.consent_ids)
            blockers.extend(
                AssetReadinessBlocker(
                    code=item.code,
                    dependency_type="CONSENT",
                    dependency_id=item.consent_id,
                    summary="Asset rights do not cover this usage",
                    next_action="review_asset_consent",
                )
                for item in rights.blockers
            )
    elif asset.status != "active":
        blockers.append(
            AssetReadinessBlocker(
                code="asset_archived",
                dependency_type="ASSET",
                dependency_id=asset.id,
                summary="Archived asset cannot be used by new work",
                next_action="restore_asset",
            )
        )

    if asset.availability != "enabled":
        blockers.append(
            AssetReadinessBlocker(
                code="asset_disabled",
                dependency_type="ASSET",
                dependency_id=asset.id,
                summary="Disabled asset cannot be used by new work",
                next_action="enable_asset",
            )
        )

    hard_blocked = asset.availability != "enabled" or state.status != "active"
    status: ReadinessStatus = (
        "blocked" if hard_blocked else "draft" if draft else "blocked" if blockers else "ready"
    )
    next_actions = list(dict.fromkeys(item.next_action for item in blockers))
    return AssetReadinessResponse(
        status=status,
        blockers=blockers,
        warnings=[],
        next_actions=next_actions,
        dependency_snapshot=AssetReadinessDependencySnapshot(
            asset_version_id=version.id,
            asset_state_id=state.id,
            asset_state_revision=state.revision,
            media_version_ids=[item.media_version_id for item in references],
            consent_ids=consent_ids,
            evaluated_at=at_time,
        ),
    )


async def _evaluate_readiness(
    session: AsyncSession,
    asset: Asset,
    state: AssetState,
    version: AssetVersion,
    references: list[AssetMediaReference],
    *,
    purpose: str,
    channel: str,
    region: str,
    at_time: datetime,
    known_media: dict[UUID, MediaVersionReference] | None = None,
) -> AssetReadinessResponse:
    media: dict[UUID, MediaVersionReference] = known_media or {}
    for reference in references:
        if reference.media_version_id not in media:
            resolved = await resolve_media_version_reference(
                session, asset.workspace_id, reference.media_version_id
            )
            if resolved is not None:
                media[resolved.id] = resolved
    blockers, draft = _readiness_prerequisites(
        asset,
        version,
        references,
        media=media,
    )
    rights: RightsGateResult | None = None
    rights_unavailable = False
    if (
        not draft
        and asset.status == "active"
        and asset.availability == "enabled"
        and state.status == "active"
    ):
        try:
            rights = await check_rights(
                session,
                workspace_id=asset.workspace_id,
                subject=SubjectReference(
                    subject_type=SubjectType.ASSET_VERSION,
                    subject_id=version.id,
                ),
                usage=RightsUsage(
                    purpose=purpose,
                    channel=channel,
                    region=region,
                    at_time=at_time,
                ),
            )
        except (SQLAlchemyError, ApiError) as error:
            if isinstance(error, ApiError) and error.code != ErrorCode.DEPENDENCY_UNAVAILABLE:
                raise
            rights_unavailable = True
    return _compose_readiness(
        asset,
        state,
        version,
        references,
        blockers=blockers,
        draft=draft,
        rights=rights,
        rights_unavailable=rights_unavailable,
        at_time=at_time,
    )


async def append_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    request: AssetVersionCreateRequest,
    *,
    trace_id: str,
) -> AssetVersionCreateResponse:
    async with session.begin():
        state, asset = await lock_state_for_write(session, claims, state_id)
        if asset.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Archived asset cannot accept a version",
                status_code=409,
                next_action="restore_asset",
            )
        if asset.availability != "enabled":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Disabled asset cannot accept a version",
                status_code=409,
                next_action="enable_asset",
            )
        if state.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Disabled asset state cannot accept a version",
                status_code=409,
                next_action="enable_asset_state",
            )
        previous_version_id = state.current_version_id
        require_state_revision(state, request.expected_revision)
        require_expected_current(state, request.expected_current_version_id)
        if request.spec.kind != asset.kind:
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Asset spec kind does not match asset identity",
                status_code=422,
            )
        await _validate_related_assets(session, asset, request.spec)
        media = await _resolve_media_references(session, asset, request)
        now = datetime.now(UTC)
        references = [
            (item.media_version_id, item.purpose, item.position)
            for item in request.media_references
        ]
        version = AssetVersion(
            id=uuid7(),
            workspace_id=asset.workspace_id,
            asset_id=asset.id,
            asset_state_id=state.id,
            version_no=await repository.latest_version_number(session, asset.id) + 1,
            schema_version=1,
            spec=spec_to_json(request.spec),
            prompt_description=request.prompt_description.strip(),
            source_type=request.source_type,
            source_id=request.source_id,
            content_hash=_version_hash(
                request.spec,
                request.prompt_description,
                references,
                request.source_type,
                request.source_id,
            ),
            created_by=claims.sub,
            created_at=now,
        )
        session.add(version)
        await session.flush([version])
        stored_references = [
            AssetMediaReference(
                id=uuid7(),
                workspace_id=asset.workspace_id,
                asset_version_id=version.id,
                media_version_id=item.media_version_id,
                purpose=item.purpose,
                position=item.position,
                created_at=now,
            )
            for item in request.media_references
        ]
        session.add_all(stored_references)
        if request.set_as_current:
            state.current_version_id = version.id
            state.revision += 1
            state.updated_at = now
        await session.flush()
        readiness = await _evaluate_readiness(
            session,
            asset,
            state,
            version,
            stored_references,
            purpose="ai_short_drama_generation",
            channel="lanverse_preview",
            region="CN",
            at_time=now,
            known_media=media,
        )
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.version_created",
            target_type="asset_version",
            target_id=version.id,
            trace_id=trace_id,
            metadata={
                "asset_id": str(asset.id),
                "asset_state_id": str(state.id),
                "state_revision": state.revision,
                "version_no": version.version_no,
                "kind": asset.kind,
                "set_as_current": request.set_as_current,
                "previous_version_id": (
                    str(previous_version_id) if previous_version_id is not None else None
                ),
                "current_version_id": (
                    str(state.current_version_id) if state.current_version_id is not None else None
                ),
            },
            occurred_at=now,
        )
        await session.flush()
    return AssetVersionCreateResponse(
        state=state_response(state),
        version=_version_response(version, stored_references),
        readiness=readiness,
    )


async def list_versions(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    *,
    limit: int,
    offset: int,
) -> PaginatedAssetVersions:
    state, _asset = await state_for_read(session, claims, state_id)
    rows, total = await repository.list_versions(session, state.id, limit=limit, offset=offset)
    references = await repository.list_media_references(session, [item.id for item in rows])
    by_version: dict[UUID, list[AssetMediaReference]] = {}
    for item in references:
        by_version.setdefault(item.asset_version_id, []).append(item)
    return PaginatedAssetVersions(
        items=[_version_response(item, by_version.get(item.id, [])) for item in rows],
        total=total,
        limit=limit,
        offset=offset,
    )


async def get_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
) -> AssetVersionResponse:
    result = await repository.find_version(session, version_id)
    if result is None:
        raise asset_not_found("Asset version")
    version, _state, asset = result
    await asset_for_read(session, claims, asset.id)
    references = await repository.list_media_references(session, [version.id])
    return _version_response(version, references)


async def get_readiness(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
    *,
    purpose: str,
    channel: str,
    region: str,
) -> AssetReadinessResponse:
    result = await repository.find_version(session, version_id)
    if result is None:
        raise asset_not_found("Asset version")
    version, state, asset = result
    await asset_for_read(session, claims, asset.id)
    references = await repository.list_media_references(session, [version.id])
    return await _evaluate_readiness(
        session,
        asset,
        state,
        version,
        references,
        purpose=purpose,
        channel=channel,
        region=region,
        at_time=datetime.now(UTC),
    )


async def summarize_project_assets(
    session: AsyncSession,
    workspace_id: UUID,
    project_id: UUID,
    *,
    purpose: str,
    channel: str,
    region: str,
) -> ProjectAssetSummary:
    rows, total = await repository.list_active_states_with_current_version(session, project_id)
    if any(asset.workspace_id != workspace_id for asset, _state, _version in rows):
        raise ApiError(
            ErrorCode.DEPENDENCY_UNAVAILABLE,
            "Asset summary scope is inconsistent",
            status_code=503,
        )
    references = await repository.list_media_references(
        session, [version.id for _asset, _state, version in rows]
    )
    references_by_version: dict[UUID, list[AssetMediaReference]] = {}
    for reference in references:
        references_by_version.setdefault(reference.asset_version_id, []).append(reference)
    evaluated_at = datetime.now(UTC)
    evaluated = [
        (
            asset.id,
            asset.kind,
            await _evaluate_readiness(
                session,
                asset,
                state,
                version,
                references_by_version.get(version.id, []),
                purpose=purpose,
                channel=channel,
                region=region,
                at_time=evaluated_at,
            ),
        )
        for asset, state, version in rows
    ]
    status_by_asset: dict[UUID, str] = {}
    kind_by_asset: dict[UUID, str] = {}
    for asset_id, kind, readiness in evaluated:
        kind_by_asset[asset_id] = kind
        previous = status_by_asset.get(asset_id)
        if readiness.status == "ready" or previous is None:
            status_by_asset[asset_id] = readiness.status
        elif previous != "ready" and readiness.status == "blocked":
            status_by_asset[asset_id] = "blocked"
    ready_kinds = tuple(
        sorted(
            {
                kind_by_asset[asset_id]
                for asset_id, state_status in status_by_asset.items()
                if state_status == "ready"
            }
        )
    )
    ready = sum(state_status == "ready" for state_status in status_by_asset.values())
    blocked = sum(state_status == "blocked" for state_status in status_by_asset.values())
    draft = total - ready - blocked
    required_kinds = ("character", "location", "voice")
    if set(required_kinds).issubset(ready_kinds):
        status = "ready"
    elif blocked:
        status = "blocked"
    elif total:
        status = "draft"
    else:
        status = "not_started"
    return ProjectAssetSummary(
        status=cast(
            Literal["not_started", "draft", "blocked", "ready", "unavailable"],
            status,
        ),
        total=total,
        versioned=len(status_by_asset),
        ready=ready,
        draft=draft,
        blocked=blocked,
        ready_kinds=ready_kinds,
    )


async def summarize_project_asset_references(
    session: AsyncSession,
    workspace_id: UUID,
    project_ids: list[UUID],
) -> dict[UUID, ProjectAssetReferenceSummary]:
    summaries = {
        project_id: ProjectAssetReferenceSummary(asset_count=0, version_count=0)
        for project_id in project_ids
    }
    for (
        project_id,
        asset_count,
        version_count,
    ) in await repository.count_asset_references_by_project(
        session,
        workspace_id,
        project_ids,
    ):
        summaries[project_id] = ProjectAssetReferenceSummary(
            asset_count=asset_count,
            version_count=version_count,
        )
    return summaries


async def asset_version_exists(
    session: AsyncSession,
    workspace_id: UUID,
    version_id: UUID,
) -> bool:
    result = await repository.find_version(session, version_id)
    return result is not None and result[0].workspace_id == workspace_id


async def resolve_asset_version(
    session: AsyncSession,
    workspace_id: UUID,
    version_id: UUID,
) -> AssetVersionReference | None:
    result = await repository.find_version(session, version_id)
    if result is None:
        return None
    version, state, asset = result
    if version.workspace_id != workspace_id:
        return None
    return AssetVersionReference(
        id=version.id,
        workspace_id=version.workspace_id,
        project_id=asset.project_id,
        asset_id=asset.id,
        asset_state_id=state.id,
        kind=asset.kind,
        asset_status=asset.status,
        asset_availability=asset.availability,
        asset_state_status=state.status,
    )


async def asset_version_for_content_read(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
) -> AssetVersionReference:
    result = await repository.find_version(session, version_id)
    if result is None:
        raise asset_not_found("Asset version")
    version, state, asset = result
    await asset_for_read(session, claims, asset.id)
    return AssetVersionReference(
        id=version.id,
        workspace_id=version.workspace_id,
        project_id=asset.project_id,
        asset_id=asset.id,
        asset_state_id=state.id,
        kind=asset.kind,
        asset_status=asset.status,
        asset_availability=asset.availability,
        asset_state_status=state.status,
    )


def _readiness_reference(
    asset: Asset,
    state: AssetState,
    version: AssetVersion,
    readiness: AssetReadinessResponse,
) -> AssetVersionReadinessReference:
    blocker_codes = tuple(blocker.code for blocker in readiness.blockers)
    status: Literal["draft", "ready", "blocked", "unavailable"] = readiness.status
    if "rights_dependency_unavailable" in blocker_codes:
        status = "unavailable"
    stable_snapshot = {
        "asset_version_id": str(version.id),
        "asset_id": str(asset.id),
        "asset_state_id": str(state.id),
        "asset_status": asset.status,
        "asset_availability": asset.availability,
        "asset_state_status": state.status,
        "content_hash": version.content_hash,
        "status": status,
        "blockers": [
            blocker.model_dump(mode="json", exclude_none=True) for blocker in readiness.blockers
        ],
        "media_version_ids": [
            str(media_id) for media_id in readiness.dependency_snapshot.media_version_ids
        ],
        "consent_ids": [
            str(consent_id) for consent_id in readiness.dependency_snapshot.consent_ids
        ],
    }
    evaluation_hash = sha256(
        json.dumps(
            stable_snapshot,
            sort_keys=True,
            separators=(",", ":"),
            ensure_ascii=False,
        ).encode()
    ).hexdigest()
    return AssetVersionReadinessReference(
        id=version.id,
        asset_id=asset.id,
        asset_state_id=state.id,
        kind=asset.kind,
        asset_status=asset.status,
        asset_availability=asset.availability,
        asset_state_status=state.status,
        status=status,
        blocker_codes=blocker_codes,
        media_version_ids=tuple(readiness.dependency_snapshot.media_version_ids),
        consent_ids=tuple(readiness.dependency_snapshot.consent_ids),
        evaluation_hash=evaluation_hash,
    )


async def resolve_asset_versions_readiness(
    session: AsyncSession,
    workspace_id: UUID,
    project_id: UUID,
    version_ids: list[UUID],
    *,
    purpose: str,
    channel: str,
    region: str,
) -> dict[UUID, AssetVersionReadinessReference]:
    unique_ids = list(dict.fromkeys(version_ids))
    rows = await repository.find_versions(session, unique_ids)
    scoped = [
        (version, state, asset)
        for version, state, asset in rows
        if version.workspace_id == workspace_id and asset.project_id == project_id
    ]
    references = await repository.list_media_references(
        session,
        [version.id for version, _state, _asset in scoped],
    )
    by_version: dict[UUID, list[AssetMediaReference]] = {}
    for reference in references:
        by_version.setdefault(reference.asset_version_id, []).append(reference)
    media = await resolve_media_version_references(
        session,
        workspace_id,
        [reference.media_version_id for reference in references],
    )
    prerequisites: dict[
        UUID,
        tuple[list[AssetReadinessBlocker], bool],
    ] = {}
    rights_subjects: list[SubjectReference] = []
    for version, state, asset in scoped:
        version_references = by_version.get(version.id, [])
        blockers, draft = _readiness_prerequisites(
            asset,
            version,
            version_references,
            media=media,
        )
        prerequisites[version.id] = (blockers, draft)
        if (
            not draft
            and asset.status == "active"
            and asset.availability == "enabled"
            and state.status == "active"
        ):
            rights_subjects.append(
                SubjectReference(
                    subject_type=SubjectType.ASSET_VERSION,
                    subject_id=version.id,
                )
            )
    at_time = datetime.now(UTC)
    rights_by_subject: dict[SubjectReference, RightsGateResult] = {}
    rights_unavailable = False
    if rights_subjects:
        try:
            rights_by_subject = await check_rights_for_resolved_subjects(
                session,
                workspace_id=workspace_id,
                subjects=rights_subjects,
                usage=RightsUsage(
                    purpose=purpose,
                    channel=channel,
                    region=region,
                    at_time=at_time,
                ),
            )
        except (SQLAlchemyError, ApiError) as error:
            if isinstance(error, ApiError) and error.code != ErrorCode.DEPENDENCY_UNAVAILABLE:
                raise
            rights_unavailable = True
    results: dict[UUID, AssetVersionReadinessReference] = {}
    for version, state, asset in scoped:
        blockers, draft = prerequisites[version.id]
        subject = SubjectReference(
            subject_type=SubjectType.ASSET_VERSION,
            subject_id=version.id,
        )
        readiness = _compose_readiness(
            asset,
            state,
            version,
            by_version.get(version.id, []),
            blockers=blockers,
            draft=draft,
            rights=rights_by_subject.get(subject),
            rights_unavailable=rights_unavailable and subject in rights_subjects,
            at_time=at_time,
        )
        results[version.id] = _readiness_reference(asset, state, version, readiness)
    return results


async def resolve_asset_version_readiness(
    session: AsyncSession,
    workspace_id: UUID,
    project_id: UUID,
    version_id: UUID,
    *,
    purpose: str,
    channel: str,
    region: str,
) -> AssetVersionReadinessReference | None:
    results = await resolve_asset_versions_readiness(
        session,
        workspace_id,
        project_id,
        [version_id],
        purpose=purpose,
        channel=channel,
        region=region,
    )
    return results.get(version_id)


async def resolve_storyboard_assets(
    session: AsyncSession,
    workspace_id: UUID,
    project_id: UUID,
    state_ids: list[UUID],
    *,
    for_update: bool = False,
) -> tuple[StoryboardAssetInput, ...]:
    unique_ids = list(dict.fromkeys(state_ids))
    rows = await repository.find_state_scopes(
        session,
        unique_ids,
        for_update=for_update,
    )
    by_state = {
        state.id: (state, asset)
        for state, asset in rows
        if state.workspace_id == workspace_id
        and asset.workspace_id == workspace_id
        and asset.project_id == project_id
    }
    if len(by_state) != len(unique_ids):
        return ()
    version_ids = [
        state.current_version_id
        for state, _asset in by_state.values()
        if state.current_version_id is not None
    ]
    if len(version_ids) != len(unique_ids):
        return ()
    readiness = await resolve_asset_versions_readiness(
        session,
        workspace_id,
        project_id,
        version_ids,
        purpose="ai_short_drama_generation",
        channel="lanverse_preview",
        region="CN",
    )
    if len(readiness) != len(unique_ids):
        return ()
    inputs: list[StoryboardAssetInput] = []
    for state_id in unique_ids:
        state, asset = by_state[state_id]
        assert state.current_version_id is not None
        reference = readiness[state.current_version_id]
        if reference.status != "ready":
            return ()
        inputs.append(
            StoryboardAssetInput(
                workspace_id=workspace_id,
                project_id=project_id,
                asset_id=asset.id,
                asset_state_id=state.id,
                asset_version_id=state.current_version_id,
                kind=asset.kind,
                name=asset.name,
                state_label=state.label,
                state_revision=state.revision,
                readiness_hash=reference.evaluation_hash,
            )
        )
    return tuple(inputs)


_STATE_NEXT_ACTIONS = {
    "required_field_missing": "complete_asset_spec",
    "required_media_missing": "attach_media_reference",
    "media_unavailable": "replace_media_reference",
    "consent_missing": "review_asset_consent",
    "consent_revoked": "review_asset_consent",
    "rights_dependency_unavailable": "retry_readiness",
    "asset_archived": "restore_asset",
    "asset_disabled": "enable_asset",
    "asset_state_disabled": "enable_asset_state",
}


def _state_readiness(
    state: AssetState,
    current_occurrences: list[AssetOccurrenceDecision],
    readiness: AssetVersionReadinessReference | None,
    *,
    evaluated_at: datetime,
) -> AssetStateReadinessResponse:
    if state.current_version_id is None:
        blockers = [
            AssetReadinessBlocker(
                code="state_current_version_missing",
                dependency_type="ASSET_STATE",
                dependency_id=state.id,
                summary="Asset state has no current immutable version",
                next_action="create_asset_state_version",
            )
        ]
        status: Literal["draft", "ready", "blocked", "unavailable"] = (
            "blocked" if state.status != "active" else "draft"
        )
        if state.status != "active":
            blockers.append(
                AssetReadinessBlocker(
                    code="asset_state_disabled",
                    dependency_type="ASSET_STATE",
                    dependency_id=state.id,
                    summary="Asset state is disabled",
                    next_action="enable_asset_state",
                )
            )
        return AssetStateReadinessResponse(
            status=status,
            blockers=blockers,
            warnings=[],
            next_actions=list(dict.fromkeys(item.next_action for item in blockers)),
            dependency_snapshot=AssetStateReadinessSnapshot(
                asset_state_id=state.id,
                asset_state_revision=state.revision,
                current_version_id=None,
                occurrence_decision_ids=[item.id for item in current_occurrences],
                media_version_ids=[],
                consent_ids=[],
                evaluated_at=evaluated_at,
            ),
        )
    if readiness is None:
        blockers = [
            AssetReadinessBlocker(
                code="state_current_version_unavailable",
                dependency_type="ASSET_VERSION",
                dependency_id=state.current_version_id,
                summary="Current asset state version is unavailable",
                next_action="repair_asset_state_current",
            )
        ]
        return AssetStateReadinessResponse(
            status="unavailable",
            blockers=blockers,
            warnings=[],
            next_actions=["repair_asset_state_current"],
            dependency_snapshot=AssetStateReadinessSnapshot(
                asset_state_id=state.id,
                asset_state_revision=state.revision,
                current_version_id=state.current_version_id,
                occurrence_decision_ids=[item.id for item in current_occurrences],
                media_version_ids=[],
                consent_ids=[],
                evaluated_at=evaluated_at,
            ),
        )
    blockers = [
        AssetReadinessBlocker(
            code=code,
            dependency_type="ASSET_VERSION",
            dependency_id=readiness.id,
            summary=f"Asset state dependency is blocked: {code}",
            next_action=_STATE_NEXT_ACTIONS.get(code, "review_asset_state"),
        )
        for code in readiness.blocker_codes
    ]
    return AssetStateReadinessResponse(
        status=readiness.status,
        blockers=blockers,
        warnings=[],
        next_actions=list(dict.fromkeys(item.next_action for item in blockers)),
        dependency_snapshot=AssetStateReadinessSnapshot(
            asset_state_id=state.id,
            asset_state_revision=state.revision,
            current_version_id=state.current_version_id,
            occurrence_decision_ids=[item.id for item in current_occurrences],
            media_version_ids=list(readiness.media_version_ids),
            consent_ids=list(readiness.consent_ids),
            evaluated_at=evaluated_at,
        ),
    )


async def get_state_readiness(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    *,
    purpose: str,
    channel: str,
    region: str,
) -> AssetStateReadinessResponse:
    state, asset = await state_for_read(session, claims, state_id)
    occurrences = _current_occurrence_rows(
        await repository.list_occurrence_decisions(session, [state.id])
    )
    readiness_by_version = await resolve_asset_versions_readiness(
        session,
        asset.workspace_id,
        asset.project_id,
        [state.current_version_id] if state.current_version_id is not None else [],
        purpose=purpose,
        channel=channel,
        region=region,
    )
    return _state_readiness(
        state,
        occurrences,
        (
            readiness_by_version.get(state.current_version_id)
            if state.current_version_id is not None
            else None
        ),
        evaluated_at=datetime.now(UTC),
    )


async def get_asset_bible(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    read_narratives: AssetOccurrenceNarrativeReader,
    *,
    purpose: str,
    channel: str,
    region: str,
) -> AssetBibleResponse:
    project = await project_for_content_read(session, claims, project_id)
    assets = await repository.list_project_assets(session, project_id)
    states = await repository.list_states_for_assets(
        session,
        [asset.id for asset in assets],
    )
    occurrence_history = await repository.list_occurrence_decisions(
        session,
        [state.id for state in states],
    )
    current_occurrences = _current_occurrence_rows(occurrence_history)
    narratives = await read_narratives(
        workspace_id=project.workspace_id,
        unit_version_ids=[row.narrative_unit_version_id for row in current_occurrences],
    )
    current_version_ids = [
        state.current_version_id for state in states if state.current_version_id is not None
    ]
    version_rows = await repository.find_versions(session, current_version_ids)
    version_by_id = {version.id: version for version, _state, _asset in version_rows}
    media_references = await repository.list_media_references(
        session,
        current_version_ids,
    )
    media_by_version: dict[UUID, list[AssetMediaReference]] = {}
    for reference in media_references:
        media_by_version.setdefault(reference.asset_version_id, []).append(reference)
    readiness_by_version = await resolve_asset_versions_readiness(
        session,
        project.workspace_id,
        project_id,
        current_version_ids,
        purpose=purpose,
        channel=channel,
        region=region,
    )
    occurrences_by_state: dict[UUID, list[AssetOccurrenceDecision]] = {}
    for occurrence in current_occurrences:
        occurrences_by_state.setdefault(occurrence.asset_state_id, []).append(occurrence)
    states_by_asset: dict[UUID, list[AssetState]] = {}
    for state in states:
        states_by_asset.setdefault(state.asset_id, []).append(state)
    evaluated_at = datetime.now(UTC)
    items: list[AssetBibleAsset] = []
    state_statuses: list[str] = []
    for asset in assets:
        state_items: list[AssetBibleState] = []
        for state in states_by_asset.get(asset.id, []):
            state_occurrences = occurrences_by_state.get(state.id, [])
            version = (
                version_by_id.get(state.current_version_id)
                if state.current_version_id is not None
                else None
            )
            state_readiness = _state_readiness(
                state,
                state_occurrences,
                (
                    readiness_by_version.get(state.current_version_id)
                    if state.current_version_id is not None
                    else None
                ),
                evaluated_at=evaluated_at,
            )
            state_statuses.append(state_readiness.status)
            state_items.append(
                AssetBibleState(
                    state=state_response(state),
                    current_version=(
                        _version_response(
                            version,
                            media_by_version.get(version.id, []),
                        )
                        if version is not None
                        else None
                    ),
                    occurrences=[
                        _occurrence_response(row, narratives) for row in state_occurrences
                    ],
                    readiness=state_readiness,
                )
            )
        items.append(AssetBibleAsset(asset=asset_response(asset), states=state_items))
    return AssetBibleResponse(
        items=items,
        summary=AssetBibleSummary(
            asset_count=len(assets),
            state_count=len(states),
            ready=state_statuses.count("ready"),
            draft=state_statuses.count("draft"),
            blocked=state_statuses.count("blocked"),
            unavailable=state_statuses.count("unavailable"),
        ),
    )


def _candidate_spec(command: AssetCandidateCommand) -> AssetSpec:
    payloads: dict[str, dict[str, object]] = {
        "character": {
            "kind": "character",
            "identity": command.name,
            "appearance": command.description,
            "age_impression": "",
            "temperament": [],
        },
        "location": {
            "kind": "location",
            "spatial_description": command.description,
            "time_weather": "",
            "visual_elements": [],
            "lighting": "",
        },
        "prop": {
            "kind": "prop",
            "appearance": command.description,
            "material": "",
            "usage_context": "",
            "holder_character_id": None,
        },
        "costume": {
            "kind": "costume",
            "appearance": command.description,
            "material": "",
            "usage_context": "",
            "wearer_character_id": None,
        },
        "visual_style": {
            "kind": "visual_style",
            "visual_language": command.description,
            "palette": "",
            "lighting_language": "",
            "negative_constraints": [],
        },
        "voice": {
            "kind": "voice",
            "source_kind": None,
            "language": "",
            "performance_traits": [],
            "allowed_usage": [],
        },
    }
    return parse_asset_spec(command.kind, payloads[command.kind])


async def create_or_link_candidate(
    session: AsyncSession,
    actor: ActorContext,
    command: AssetCandidateCommand,
) -> AssetCandidateResult:
    existing_version = await repository.find_candidate_version(session, command.candidate_id)
    if existing_version is not None:
        return AssetCandidateResult(
            asset_id=existing_version.asset_id,
            asset_version_id=existing_version.id,
        )
    if command.action == "link_existing":
        if command.target_asset_id is None:
            raise ApiError(ErrorCode.INVALID_REQUEST, "Target asset is required", status_code=422)
        asset = await repository.find_asset(session, command.target_asset_id, for_update=True)
        if (
            asset is None
            or asset.workspace_id != command.workspace_id
            or asset.project_id != command.project_id
            or asset.kind != command.kind
        ):
            raise asset_not_found()
        base_state = await repository.find_state_by_key(session, asset.id, "base")
        if base_state is None:
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Asset base state is unavailable",
                status_code=503,
            )
        return AssetCandidateResult(
            asset_id=asset.id,
            asset_version_id=base_state.current_version_id,
        )

    normalized_name = normalize_name(command.name)
    asset = Asset(
        id=uuid7(),
        workspace_id=command.workspace_id,
        project_id=command.project_id,
        kind=command.kind,
        name=command.name.strip(),
        normalized_name=normalized_name,
        aliases=[],
        tags=["script_candidate"],
        status="active",
        availability="enabled",
        name_revision=1,
        revision=1,
        created_by=actor.user_id,
    )
    state = AssetState(
        id=uuid7(),
        workspace_id=command.workspace_id,
        asset_id=asset.id,
        state_key="base",
        label="基础状态",
        description="",
        status="active",
        current_version_id=None,
        revision=1,
        creation_key="base",
        command_receipts={},
        created_by=actor.user_id,
    )
    spec = _candidate_spec(command)
    version = AssetVersion(
        id=uuid7(),
        workspace_id=command.workspace_id,
        asset_id=asset.id,
        asset_state_id=state.id,
        version_no=1,
        schema_version=1,
        spec=spec_to_json(spec),
        prompt_description=command.description,
        source_type="script_extraction_candidate",
        source_id=command.candidate_id,
        content_hash=_version_hash(
            spec,
            command.description,
            [],
            "script_extraction_candidate",
            command.candidate_id,
        ),
        created_by=actor.user_id,
    )
    session.add(asset)
    session.add(
        AssetNameRevision(
            asset_id=asset.id,
            revision_no=1,
            workspace_id=asset.workspace_id,
            name_snapshot=asset.name,
            normalized_name=asset.normalized_name,
            created_by=actor.user_id,
        )
    )
    session.add(state)
    await session.flush([asset, state])
    session.add(version)
    await session.flush([version])
    state.current_version_id = version.id
    state.revision = 2
    await session.flush([state])
    return AssetCandidateResult(asset_id=asset.id, asset_version_id=version.id)
