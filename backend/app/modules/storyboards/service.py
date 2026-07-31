from collections import defaultdict
from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.assets import resolve_asset_version
from app.modules.projects import (
    episode_for_content_read,
    lock_active_episode_for_content_write,
)
from app.modules.scripts import (
    resolve_confirmed_shot_candidate,
    resolve_confirmed_structure,
)
from app.modules.storyboards import repository
from app.modules.storyboards.hashing import shot_order_hash, storyboard_content_hashes
from app.modules.storyboards.models import AssetReference, Shot, ShotSpecVersion
from app.modules.storyboards.schemas import (
    AssetReferenceRequest,
    AssetReferenceResponse,
    ShotCreateRequest,
    ShotCurrentSpecRequest,
    ShotDeleteBlocker,
    ShotDeletePreflightResponse,
    ShotDeleteResponse,
    ShotOrderResponse,
    ShotReorderRequest,
    ShotResponse,
    ShotSpec,
    ShotSpecCreateRequest,
    ShotSpecCreateResponse,
    ShotSpecVersionResponse,
    ShotStateRequest,
    ShotStateResponse,
    ShotUpdateRequest,
)

MAX_ACTIVE_SHOTS = 120


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
        active = await repository.list_active_shots(
            session, shot.episode_id, for_update=True
        )
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
) -> None:
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


async def append_spec_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    shot_id: UUID,
    request: ShotSpecCreateRequest,
) -> ShotSpecCreateResponse:
    async with session.begin():
        current = await repository.find_shot(session, shot_id)
        if current is None:
            raise _not_found("Shot")
        episode = await lock_active_episode_for_content_write(
            session, claims, current.episode_id
        )
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
        await _validate_asset_references(
            session,
            workspace_id=shot.workspace_id,
            project_id=episode.project_id,
            references=request.asset_references,
        )
        hashes = storyboard_content_hashes(request.spec, request.asset_references)
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
                subject_key=reference.subject_key,
            )
            for reference in request.asset_references
        ]
        session.add_all(references)
        shot.current_spec_version_id = version.id
        shot.revision += 1
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
        shot.current_spec_version_id = request.version_id
        shot.revision += 1
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
                details={
                    "blockers": [blocker.model_dump() for blocker in preflight.blockers]
                },
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
