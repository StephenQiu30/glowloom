import json
from dataclasses import asdict
from datetime import UTC, datetime
from hashlib import sha256
from typing import Any, Literal, cast
from uuid import UUID

from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.assets import repository, service
from app.modules.assets.contracts import (
    AssetProductionImpactReader,
    AssetPromptSnapshot,
    AssetShotUsageReader,
    AssetShotUsageSnapshot,
    AssetTaskSnapshot,
)
from app.modules.assets.models import (
    Asset,
    AssetNameRevision,
    AssetOccurrenceDecision,
    AssetState,
    AssetVersion,
)
from app.modules.assets.schemas import (
    AssetAvailabilityResponse,
    AssetDisablePreflightRequest,
    AssetDisableRequest,
    AssetEnableRequest,
    AssetEpisodeImpact,
    AssetImpactResponse,
    AssetImpactSummary,
    AssetPromptImpact,
    AssetRenamePreflightRequest,
    AssetRenameRequest,
    AssetRenameResponse,
    AssetResponse,
    AssetShotImpact,
    AssetStateAvailabilityResponse,
    AssetStateCurrentPreflightRequest,
    AssetStateCurrentRequest,
    AssetStateCurrentResponse,
    AssetStateEnableRequest,
    AssetStateResponse,
    AssetStateUpdateRequest,
    AssetTaskImpact,
)
from app.modules.governance.audit import append_audit_event

ImpactOperation = Literal["rename", "disable_asset", "disable_state", "set_current"]


def _request_hash(operation: str, payload: BaseModel) -> str:
    serialized = payload.model_dump(mode="json")
    return sha256(
        json.dumps(
            {"operation": operation, "payload": serialized},
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode()
    ).hexdigest()


def _receipt(
    receipts: dict[str, Any],
    idempotency_key: str,
    command_hash: str,
) -> dict[str, Any] | None:
    stored = receipts.get(idempotency_key)
    if stored is None:
        return None
    if not isinstance(stored, dict):
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Asset change command key was reused with different input",
            status_code=409,
        )
    receipt = cast(dict[str, Any], stored)
    if receipt.get("command_hash") != command_hash:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Asset change command key was reused with different input",
            status_code=409,
        )
    return receipt


def _current_occurrences(
    rows: list[AssetOccurrenceDecision],
) -> list[AssetOccurrenceDecision]:
    latest: dict[tuple[UUID, UUID], AssetOccurrenceDecision] = {}
    for row in rows:
        latest[(row.asset_state_id, row.narrative_unit_id)] = row
    return [row for row in latest.values() if row.decision == "link"]


def _shot_impacts(usages: list[AssetShotUsageSnapshot]) -> list[AssetShotImpact]:
    grouped: dict[UUID, dict[str, object]] = {}
    for usage in usages:
        item = grouped.setdefault(
            usage.shot_id,
            {
                "shot_title": usage.shot_title,
                "episode_id": usage.episode_id,
                "spec_version_ids": set(),
                "current_spec_version_id": usage.current_spec_version_id,
                "slot_keys": set(),
            },
        )
        cast(set[UUID], item["spec_version_ids"]).add(usage.spec_version_id)
        cast(set[str], item["slot_keys"]).update(usage.slot_keys)
    return [
        AssetShotImpact(
            shot_id=shot_id,
            shot_title=cast(str, item["shot_title"]),
            episode_id=cast(UUID, item["episode_id"]),
            spec_version_ids=sorted(
                cast(set[UUID], item["spec_version_ids"]),
                key=str,
            ),
            current_spec_version_id=cast(
                UUID | None,
                item["current_spec_version_id"],
            ),
            slot_keys=sorted(cast(set[str], item["slot_keys"])),
        )
        for shot_id, item in sorted(grouped.items(), key=lambda pair: str(pair[0]))
    ]


def _episode_impacts(
    episode_ids: set[UUID],
    shots: list[AssetShotImpact],
    prompts: list[AssetPromptSnapshot],
    tasks: list[AssetTaskSnapshot],
) -> list[AssetEpisodeImpact]:
    prompt_by_request = {prompt.generation_request_id: prompt for prompt in prompts}
    return [
        AssetEpisodeImpact(
            episode_id=episode_id,
            shot_count=len({item.shot_id for item in shots if item.episode_id == episode_id}),
            prompt_snapshot_count=sum(item.episode_id == episode_id for item in prompts),
            active_task_count=sum(
                prompt_by_request[item.generation_request_id].episode_id == episode_id
                for item in tasks
                if item.generation_request_id in prompt_by_request
            ),
        )
        for episode_id in sorted(episode_ids, key=str)
    ]


async def _impact_snapshot(
    session: AsyncSession,
    *,
    operation: ImpactOperation,
    asset: Asset,
    states: list[AssetState],
    versions: list[AssetVersion],
    state: AssetState | None,
    old_version_id: UUID | None,
    new_version_id: UUID | None,
    command: dict[str, object],
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
    for_update: bool,
) -> AssetImpactResponse:
    version_ids = [item.id for item in versions]
    usages = await read_shot_usages(
        workspace_id=asset.workspace_id,
        asset_version_ids=version_ids,
        for_update=for_update,
    )
    prompts, tasks = await read_production_impacts(
        workspace_id=asset.workspace_id,
        project_id=asset.project_id,
        asset_version_ids=version_ids,
        for_update=for_update,
    )
    occurrence_rows = await repository.list_occurrence_decisions(
        session,
        [item.id for item in states],
    )
    occurrences = _current_occurrences(occurrence_rows)
    shots = _shot_impacts(usages)
    episode_ids = {
        *(item.episode_id for item in shots),
        *(item.episode_id for item in prompts),
        *(item.episode_id for item in occurrences),
    }
    episodes = _episode_impacts(episode_ids, shots, prompts, tasks)
    prompt_impacts = [AssetPromptImpact(**asdict(item)) for item in prompts]
    task_impacts = [AssetTaskImpact(**asdict(item)) for item in tasks]
    response = AssetImpactResponse(
        operation=operation,
        asset_id=asset.id,
        state_id=state.id if state is not None else None,
        old_version_id=old_version_id,
        new_version_id=new_version_id,
        summary=AssetImpactSummary(
            episode_count=len(episodes),
            shot_count=len(shots),
            spec_version_count=len({item.spec_version_id for item in usages}),
            prompt_snapshot_count=len(prompt_impacts),
            active_task_count=len(task_impacts),
        ),
        episodes=episodes,
        shots=shots,
        prompt_snapshots=prompt_impacts,
        active_tasks=task_impacts,
        impact_hash="0" * 64,
    )
    facts = {
        "command": command,
        "asset": {
            "id": str(asset.id),
            "revision": asset.revision,
            "status": asset.status,
            "availability": asset.availability,
            "name_revision": asset.name_revision,
            "name": asset.name,
        },
        "states": [
            {
                "id": str(item.id),
                "revision": item.revision,
                "status": item.status,
                "current_version_id": (
                    str(item.current_version_id) if item.current_version_id is not None else None
                ),
            }
            for item in states
        ],
        "versions": [
            {
                "id": str(item.id),
                "state_id": str(item.asset_state_id),
                "version_no": item.version_no,
                "content_hash": item.content_hash,
            }
            for item in versions
        ],
        "occurrences": [
            {
                "id": str(item.id),
                "state_id": str(item.asset_state_id),
                "episode_id": str(item.episode_id),
                "unit_version_id": str(item.narrative_unit_version_id),
                "sequence": item.sequence,
            }
            for item in occurrences
        ],
        "usages": [asdict(item) for item in usages],
        "prompts": [asdict(item) for item in prompts],
        "tasks": [asdict(item) for item in tasks],
        "response": response.model_dump(mode="json", exclude={"impact_hash"}),
    }
    response.impact_hash = sha256(
        json.dumps(
            facts,
            default=str,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode()
    ).hexdigest()
    return response


async def _asset_impact(
    session: AsyncSession,
    asset: Asset,
    operation: Literal["rename", "disable_asset"],
    command: dict[str, object],
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
    *,
    for_update: bool,
) -> AssetImpactResponse:
    states = await repository.list_states(session, asset.id)
    versions = await repository.list_change_versions(session, asset_id=asset.id)
    return await _impact_snapshot(
        session,
        operation=operation,
        asset=asset,
        states=states,
        versions=versions,
        state=None,
        old_version_id=None,
        new_version_id=None,
        command=command,
        read_shot_usages=read_shot_usages,
        read_production_impacts=read_production_impacts,
        for_update=for_update,
    )


async def _state_impact(
    session: AsyncSession,
    asset: Asset,
    state: AssetState,
    operation: Literal["disable_state", "set_current"],
    command: dict[str, object],
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
    *,
    new_version_id: UUID | None,
    for_update: bool,
) -> AssetImpactResponse:
    versions = await repository.list_change_versions(
        session,
        asset_id=asset.id,
        state_id=state.id,
    )
    if operation == "set_current":
        versions = [item for item in versions if item.id == state.current_version_id]
    return await _impact_snapshot(
        session,
        operation=operation,
        asset=asset,
        states=[state],
        versions=versions,
        state=state,
        old_version_id=state.current_version_id,
        new_version_id=new_version_id,
        command=command,
        read_shot_usages=read_shot_usages,
        read_production_impacts=read_production_impacts,
        for_update=for_update,
    )


def _require_impact(expected: str, current: AssetImpactResponse) -> None:
    if expected != current.impact_hash:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Asset change impact has changed",
            status_code=409,
            next_action="repeat_asset_change_preflight",
            details={"current_impact": current.model_dump(mode="json")},
        )


async def rename_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetRenamePreflightRequest,
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
) -> AssetImpactResponse:
    asset = await service.asset_for_read(session, claims, asset_id)
    service.require_asset_revision(asset, request.expected_revision)
    name = request.new_name.strip()
    if not name or name == asset.name:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Asset rename requires a different name",
            status_code=422,
        )
    return await _asset_impact(
        session,
        asset,
        "rename",
        {"new_name": name},
        read_shot_usages,
        read_production_impacts,
        for_update=False,
    )


async def rename_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetRenameRequest,
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
    *,
    trace_id: str,
) -> AssetRenameResponse:
    command_hash = _request_hash("rename", request)
    async with session.begin():
        asset = await service.lock_asset_for_write(session, claims, asset_id)
        replay = _receipt(asset.command_receipts, request.idempotency_key, command_hash)
        if replay is not None:
            return AssetRenameResponse(
                asset=service.asset_response(asset),
                impact=AssetImpactResponse.model_validate(replay["impact"]),
            )
        service.require_asset_revision(asset, request.expected_revision)
        name = request.new_name.strip()
        if not name or name == asset.name:
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Asset rename requires a different name",
                status_code=422,
            )
        impact = await _asset_impact(
            session,
            asset,
            "rename",
            {"new_name": name},
            read_shot_usages,
            read_production_impacts,
            for_update=True,
        )
        _require_impact(request.impact_hash, impact)
        old_name = asset.name
        normalized_name = service.normalize_name(name)
        duplicate = await repository.find_duplicate_name(
            session,
            asset.project_id,
            asset.kind,
            normalized_name,
            excluding_id=asset.id,
        )
        now = datetime.now(UTC)
        asset.name = name
        asset.normalized_name = normalized_name
        asset.aliases = service.clean_values([*asset.aliases, old_name])
        asset.name_revision += 1
        asset.revision += 1
        asset.updated_at = now
        session.add(
            AssetNameRevision(
                asset_id=asset.id,
                revision_no=asset.name_revision,
                workspace_id=asset.workspace_id,
                name_snapshot=name,
                normalized_name=normalized_name,
                created_by=claims.sub,
                created_at=now,
            )
        )
        asset.command_receipts = {
            **asset.command_receipts,
            request.idempotency_key: {
                "command_hash": command_hash,
                "impact": impact.model_dump(mode="json"),
            },
        }
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.renamed",
            target_type="asset",
            target_id=asset.id,
            trace_id=trace_id,
            metadata={
                "revision": asset.revision,
                "name_revision": asset.name_revision,
                "impact_hash": impact.impact_hash,
            },
            occurred_at=now,
        )
        await session.flush()
    return AssetRenameResponse(
        asset=service.asset_response(asset, duplicate=duplicate is not None),
        impact=impact,
    )


async def asset_disable_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetDisablePreflightRequest,
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
) -> AssetImpactResponse:
    asset = await service.asset_for_read(session, claims, asset_id)
    service.require_asset_revision(asset, request.expected_revision)
    if asset.availability != "enabled":
        raise ApiError(ErrorCode.STATE_CONFLICT, "Asset is already disabled", status_code=409)
    return await _asset_impact(
        session,
        asset,
        "disable_asset",
        {},
        read_shot_usages,
        read_production_impacts,
        for_update=False,
    )


async def disable_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetDisableRequest,
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
    *,
    trace_id: str,
) -> AssetAvailabilityResponse:
    command_hash = _request_hash("disable_asset", request)
    async with session.begin():
        asset = await service.lock_asset_for_write(session, claims, asset_id)
        replay = _receipt(asset.command_receipts, request.idempotency_key, command_hash)
        if replay is not None:
            return AssetAvailabilityResponse(
                asset=service.asset_response(asset),
                impact=AssetImpactResponse.model_validate(replay["impact"]),
            )
        service.require_asset_revision(asset, request.expected_revision)
        if asset.availability != "enabled":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Asset is already disabled",
                status_code=409,
            )
        impact = await _asset_impact(
            session,
            asset,
            "disable_asset",
            {},
            read_shot_usages,
            read_production_impacts,
            for_update=True,
        )
        _require_impact(request.impact_hash, impact)
        now = datetime.now(UTC)
        asset.availability = "disabled"
        asset.revision += 1
        asset.updated_at = now
        asset.command_receipts = {
            **asset.command_receipts,
            request.idempotency_key: {
                "command_hash": command_hash,
                "impact": impact.model_dump(mode="json"),
            },
        }
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.disabled",
            target_type="asset",
            target_id=asset.id,
            trace_id=trace_id,
            metadata={"revision": asset.revision, "impact_hash": impact.impact_hash},
            occurred_at=now,
        )
        await session.flush()
    return AssetAvailabilityResponse(asset=service.asset_response(asset), impact=impact)


async def enable_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetEnableRequest,
    *,
    trace_id: str,
) -> AssetResponse:
    command_hash = _request_hash("enable_asset", request)
    async with session.begin():
        asset = await service.lock_asset_for_write(session, claims, asset_id)
        if _receipt(asset.command_receipts, request.idempotency_key, command_hash):
            return service.asset_response(asset)
        service.require_asset_revision(asset, request.expected_revision)
        if asset.availability != "disabled":
            raise ApiError(ErrorCode.STATE_CONFLICT, "Asset is already enabled", status_code=409)
        now = datetime.now(UTC)
        asset.availability = "enabled"
        asset.revision += 1
        asset.updated_at = now
        asset.command_receipts = {
            **asset.command_receipts,
            request.idempotency_key: {"command_hash": command_hash},
        }
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.enabled",
            target_type="asset",
            target_id=asset.id,
            trace_id=trace_id,
            metadata={"revision": asset.revision},
            occurred_at=now,
        )
        await session.flush()
    return service.asset_response(asset)


async def update_state(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    request: AssetStateUpdateRequest,
    *,
    trace_id: str,
) -> AssetStateResponse:
    command_hash = _request_hash("update_state", request)
    changes = request.model_dump(
        exclude={"expected_revision", "idempotency_key"},
        exclude_unset=True,
    )
    if not changes:
        raise ApiError(ErrorCode.INVALID_REQUEST, "No state changes supplied", status_code=422)
    async with session.begin():
        state, asset = await service.lock_state_for_write(session, claims, state_id)
        if _receipt(state.command_receipts, request.idempotency_key, command_hash):
            return service.state_response(state)
        service.require_state_revision(state, request.expected_revision)
        if request.label is not None:
            state.label = request.label.strip()
        if request.description is not None:
            state.description = request.description.strip()
        now = datetime.now(UTC)
        state.revision += 1
        state.updated_at = now
        state.command_receipts = {
            **state.command_receipts,
            request.idempotency_key: {"command_hash": command_hash},
        }
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.state_updated",
            target_type="asset_state",
            target_id=state.id,
            trace_id=trace_id,
            metadata={"revision": state.revision, "changed_fields": sorted(changes)},
            occurred_at=now,
        )
        await session.flush()
    return service.state_response(state)


async def state_disable_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    request: AssetDisablePreflightRequest,
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
) -> AssetImpactResponse:
    state, asset = await service.state_for_read(session, claims, state_id)
    service.require_state_revision(state, request.expected_revision)
    if state.status != "active":
        raise ApiError(ErrorCode.STATE_CONFLICT, "Asset state is already disabled", status_code=409)
    return await _state_impact(
        session,
        asset,
        state,
        "disable_state",
        {},
        read_shot_usages,
        read_production_impacts,
        new_version_id=None,
        for_update=False,
    )


async def disable_state(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    request: AssetDisableRequest,
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
    *,
    trace_id: str,
) -> AssetStateAvailabilityResponse:
    command_hash = _request_hash("disable_state", request)
    async with session.begin():
        state, asset = await service.lock_state_for_write(session, claims, state_id)
        replay = _receipt(state.command_receipts, request.idempotency_key, command_hash)
        if replay is not None:
            return AssetStateAvailabilityResponse(
                state=service.state_response(state),
                impact=AssetImpactResponse.model_validate(replay["impact"]),
            )
        service.require_state_revision(state, request.expected_revision)
        if state.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Asset state is already disabled",
                status_code=409,
            )
        impact = await _state_impact(
            session,
            asset,
            state,
            "disable_state",
            {},
            read_shot_usages,
            read_production_impacts,
            new_version_id=None,
            for_update=True,
        )
        _require_impact(request.impact_hash, impact)
        now = datetime.now(UTC)
        state.status = "disabled"
        state.revision += 1
        state.updated_at = now
        state.command_receipts = {
            **state.command_receipts,
            request.idempotency_key: {
                "command_hash": command_hash,
                "impact": impact.model_dump(mode="json"),
            },
        }
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.state_disabled",
            target_type="asset_state",
            target_id=state.id,
            trace_id=trace_id,
            metadata={"revision": state.revision, "impact_hash": impact.impact_hash},
            occurred_at=now,
        )
        await session.flush()
    return AssetStateAvailabilityResponse(state=service.state_response(state), impact=impact)


async def enable_state(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    request: AssetStateEnableRequest,
    *,
    trace_id: str,
) -> AssetStateResponse:
    command_hash = _request_hash("enable_state", request)
    async with session.begin():
        state, asset = await service.lock_state_for_write(session, claims, state_id)
        if _receipt(state.command_receipts, request.idempotency_key, command_hash):
            return service.state_response(state)
        service.require_state_revision(state, request.expected_revision)
        if state.status != "disabled":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Asset state is already active",
                status_code=409,
            )
        now = datetime.now(UTC)
        state.status = "active"
        state.revision += 1
        state.updated_at = now
        state.command_receipts = {
            **state.command_receipts,
            request.idempotency_key: {"command_hash": command_hash},
        }
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.state_enabled",
            target_type="asset_state",
            target_id=state.id,
            trace_id=trace_id,
            metadata={"revision": state.revision},
            occurred_at=now,
        )
        await session.flush()
    return service.state_response(state)


async def _current_target(
    session: AsyncSession,
    state: AssetState,
    asset: Asset,
    request: AssetStateCurrentPreflightRequest | AssetStateCurrentRequest,
) -> AssetVersion:
    service.require_state_revision(state, request.expected_revision)
    service.require_expected_current(state, request.expected_current_version_id)
    if request.version_id == state.current_version_id:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Current version change requires a different version",
            status_code=422,
        )
    result = await repository.find_version(session, request.version_id)
    if result is None:
        raise service.asset_not_found("Asset version")
    version, version_state, version_asset = result
    if version_asset.id != asset.id or version_state.id != state.id:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Asset version does not belong to this state",
            status_code=409,
        )
    return version


async def current_version_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    request: AssetStateCurrentPreflightRequest,
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
) -> AssetImpactResponse:
    state, asset = await service.state_for_read(session, claims, state_id)
    target = await _current_target(session, state, asset, request)
    return await _state_impact(
        session,
        asset,
        state,
        "set_current",
        {"version_id": str(target.id)},
        read_shot_usages,
        read_production_impacts,
        new_version_id=target.id,
        for_update=False,
    )


async def set_current_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    state_id: UUID,
    request: AssetStateCurrentRequest,
    read_shot_usages: AssetShotUsageReader,
    read_production_impacts: AssetProductionImpactReader,
    *,
    trace_id: str,
) -> AssetStateCurrentResponse:
    command_hash = _request_hash("set_current", request)
    async with session.begin():
        state, asset = await service.lock_state_for_write(session, claims, state_id)
        replay = _receipt(state.command_receipts, request.idempotency_key, command_hash)
        if replay is not None:
            return AssetStateCurrentResponse(
                state=service.state_response(state),
                impact=AssetImpactResponse.model_validate(replay["impact"]),
            )
        target = await _current_target(session, state, asset, request)
        impact = await _state_impact(
            session,
            asset,
            state,
            "set_current",
            {"version_id": str(target.id)},
            read_shot_usages,
            read_production_impacts,
            new_version_id=target.id,
            for_update=True,
        )
        _require_impact(request.impact_hash, impact)
        previous_version_id = state.current_version_id
        now = datetime.now(UTC)
        state.current_version_id = target.id
        state.revision += 1
        state.updated_at = now
        state.command_receipts = {
            **state.command_receipts,
            request.idempotency_key: {
                "command_hash": command_hash,
                "impact": impact.model_dump(mode="json"),
            },
        }
        append_audit_event(
            session,
            workspace_id=asset.workspace_id,
            actor_id=claims.sub,
            action="asset.state_current_changed",
            target_type="asset_state",
            target_id=state.id,
            trace_id=trace_id,
            metadata={
                "asset_id": str(asset.id),
                "revision": state.revision,
                "previous_version_id": (
                    str(previous_version_id) if previous_version_id is not None else None
                ),
                "current_version_id": str(target.id),
                "impact_hash": impact.impact_hash,
            },
            occurred_at=now,
        )
        await session.flush()
    return AssetStateCurrentResponse(state=service.state_response(state), impact=impact)
