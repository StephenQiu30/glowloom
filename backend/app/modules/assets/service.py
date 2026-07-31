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
    AssetCandidateResult,
    AssetVersionReadinessReference,
    AssetVersionReference,
    ProjectAssetSummary,
)
from app.modules.assets.models import Asset, AssetMediaReference, AssetVersion
from app.modules.assets.schemas import (
    AssetCreateRequest,
    AssetCurrentVersionRequest,
    AssetDeleteBlocker,
    AssetDeletePreflightResponse,
    AssetKind,
    AssetMediaReferenceResponse,
    AssetReadinessBlocker,
    AssetReadinessDependencySnapshot,
    AssetReadinessResponse,
    AssetResponse,
    AssetSpec,
    AssetStateRequest,
    AssetUpdateRequest,
    AssetVersionCreateRequest,
    AssetVersionCreateResponse,
    AssetVersionResponse,
    PaginatedAssets,
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


def _normalize_name(value: str) -> str:
    return " ".join(value.strip().casefold().split())


def _clean_values(values: list[str]) -> list[str]:
    cleaned: list[str] = []
    seen: set[str] = set()
    for value in values:
        item = value.strip()
        if item and item not in seen:
            seen.add(item)
            cleaned.append(item)
    return cleaned


def _asset_response(
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
        current_version_id=asset.current_version_id,
        revision=asset.revision,
        created_at=asset.created_at,
        updated_at=asset.updated_at,
        warnings=["duplicate_name"] if duplicate else [],
    )


def _version_response(
    version: AssetVersion,
    references: list[AssetMediaReference],
) -> AssetVersionResponse:
    return AssetVersionResponse(
        id=version.id,
        workspace_id=version.workspace_id,
        asset_id=version.asset_id,
        version_no=version.version_no,
        schema_version=version.schema_version,
        spec=parse_asset_spec(str(version.spec["kind"]), version.spec),
        prompt_description=version.prompt_description,
        source_type=cast(Literal["manual", "candidate"], version.source_type),
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


def _not_found(resource: str = "Asset") -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, f"{resource} not found", status_code=404)


def _require_revision(asset: Asset, expected_revision: int) -> None:
    if asset.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Asset has changed",
            status_code=409,
            details={"current_revision": asset.revision},
        )


def _require_expected_current(
    asset: Asset, expected_current_version_id: UUID | None
) -> None:
    if asset.current_version_id != expected_current_version_id:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Current asset version has changed",
            status_code=409,
            details={
                "current_version_id": (
                    str(asset.current_version_id)
                    if asset.current_version_id is not None
                    else None
                )
            },
        )


async def _asset_for_read(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
) -> Asset:
    asset = await repository.find_asset(session, asset_id)
    if asset is None:
        raise _not_found()
    try:
        context = await project_for_content_read(session, claims, asset.project_id)
    except ApiError as error:
        if error.code == ErrorCode.NOT_FOUND:
            raise _not_found() from error
        raise
    if context.workspace_id != asset.workspace_id:
        raise _not_found()
    return asset


async def _locked_asset_for_write(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
) -> Asset:
    asset = await repository.find_asset(session, asset_id, for_update=True)
    if asset is None:
        raise _not_found()
    try:
        context = await lock_active_project_for_content_write(
            session, claims, asset.project_id
        )
    except ApiError as error:
        if error.code == ErrorCode.NOT_FOUND:
            raise _not_found() from error
        raise
    if context.workspace_id != asset.workspace_id:
        raise _not_found()
    return asset


async def create_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: AssetCreateRequest,
) -> AssetResponse:
    name = request.name.strip()
    if not name:
        raise ApiError(ErrorCode.INVALID_REQUEST, "Asset name is required", status_code=422)
    async with session.begin():
        project = await lock_active_project_for_content_write(
            session, claims, project_id
        )
        normalized_name = _normalize_name(name)
        duplicate = (
            await repository.find_duplicate_name(
                session, project_id, request.kind, normalized_name
            )
            is not None
        )
        asset = Asset(
            id=uuid7(),
            workspace_id=project.workspace_id,
            project_id=project.project_id,
            kind=request.kind,
            name=name,
            normalized_name=normalized_name,
            aliases=_clean_values(request.aliases),
            tags=_clean_values(request.tags),
            status="active",
            revision=1,
            created_by=claims.sub,
        )
        session.add(asset)
        await session.flush()
    return _asset_response(asset, duplicate=duplicate)


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
        items=[_asset_response(item) for item in rows],
        total=total,
        limit=limit,
        offset=offset,
    )


async def get_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
) -> AssetResponse:
    return _asset_response(await _asset_for_read(session, claims, asset_id))


async def update_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetUpdateRequest,
) -> AssetResponse:
    changes = request.model_dump(exclude={"expected_revision"}, exclude_unset=True)
    if not changes:
        raise ApiError(ErrorCode.INVALID_REQUEST, "No asset changes supplied", status_code=422)
    async with session.begin():
        asset = await _locked_asset_for_write(session, claims, asset_id)
        _require_revision(asset, request.expected_revision)
        duplicate = False
        if request.name is not None:
            name = request.name.strip()
            if not name:
                raise ApiError(
                    ErrorCode.INVALID_REQUEST, "Asset name is required", status_code=422
                )
            normalized_name = _normalize_name(name)
            duplicate = (
                await repository.find_duplicate_name(
                    session,
                    asset.project_id,
                    asset.kind,
                    normalized_name,
                    excluding_id=asset.id,
                )
                is not None
            )
            asset.name = name
            asset.normalized_name = normalized_name
        if request.aliases is not None:
            asset.aliases = _clean_values(request.aliases)
        if request.tags is not None:
            asset.tags = _clean_values(request.tags)
        asset.revision += 1
        asset.updated_at = datetime.now(UTC)
        await session.flush()
    return _asset_response(asset, duplicate=duplicate)


async def set_asset_archived(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetStateRequest,
    *,
    archived: bool,
) -> AssetResponse:
    expected_status = "active" if archived else "archived"
    async with session.begin():
        asset = await _locked_asset_for_write(session, claims, asset_id)
        _require_revision(asset, request.expected_revision)
        if asset.status != expected_status:
            raise ApiError(
                ErrorCode.STATE_CONFLICT, "Asset state conflict", status_code=409
            )
        asset.status = "archived" if archived else "active"
        asset.archived_at = datetime.now(UTC) if archived else None
        asset.archived_by = claims.sub if archived else None
        asset.revision += 1
        asset.updated_at = datetime.now(UTC)
        await session.flush()
    return _asset_response(asset)


async def delete_preflight(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
) -> AssetDeletePreflightResponse:
    asset = await _asset_for_read(session, claims, asset_id)
    version_count = await repository.count_versions(session, asset.id)
    blockers = (
        [
            AssetDeleteBlocker(
                code="asset_has_versions",
                summary=f"Asset has {version_count} immutable version(s)",
            )
        ]
        if version_count
        else []
    )
    return AssetDeletePreflightResponse(allowed=not blockers, blockers=blockers)


async def delete_asset(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    expected_revision: int,
) -> None:
    async with session.begin():
        asset = await _locked_asset_for_write(session, claims, asset_id)
        _require_revision(asset, expected_revision)
        if await repository.count_versions(session, asset.id):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Asset has immutable versions",
                status_code=409,
                next_action="review_delete_blockers",
            )
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
    related = await repository.find_asset(session, related_id)
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
            raise _not_found("Media version")
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
    media_blockers, required_media_missing = _media_blockers(
        asset.kind, references, media
    )
    blockers.extend(media_blockers)
    draft = draft or required_media_missing
    return blockers, draft


def _compose_readiness(
    asset: Asset,
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
    if not draft and asset.status == "active":
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

    status: ReadinessStatus = "draft" if draft else "blocked" if blockers else "ready"
    next_actions = list(dict.fromkeys(item.next_action for item in blockers))
    return AssetReadinessResponse(
        status=status,
        blockers=blockers,
        warnings=[],
        next_actions=next_actions,
        dependency_snapshot=AssetReadinessDependencySnapshot(
            asset_version_id=version.id,
            media_version_ids=[item.media_version_id for item in references],
            consent_ids=consent_ids,
            evaluated_at=at_time,
        ),
    )


async def _evaluate_readiness(
    session: AsyncSession,
    asset: Asset,
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
    if not draft and asset.status == "active":
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
    asset_id: UUID,
    request: AssetVersionCreateRequest,
) -> AssetVersionCreateResponse:
    async with session.begin():
        asset = await _locked_asset_for_write(session, claims, asset_id)
        if asset.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Archived asset cannot accept a version",
                status_code=409,
                next_action="restore_asset",
            )
        _require_expected_current(asset, request.expected_current_version_id)
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
            asset.current_version_id = version.id
            asset.revision += 1
            asset.updated_at = now
        await session.flush()
        readiness = await _evaluate_readiness(
            session,
            asset,
            version,
            stored_references,
            purpose="ai_short_drama_generation",
            channel="lanverse_preview",
            region="CN",
            at_time=now,
            known_media=media,
        )
    return AssetVersionCreateResponse(
        asset=_asset_response(asset),
        version=_version_response(version, stored_references),
        readiness=readiness,
    )


async def list_versions(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    *,
    limit: int,
    offset: int,
) -> PaginatedAssetVersions:
    asset = await _asset_for_read(session, claims, asset_id)
    rows, total = await repository.list_versions(
        session, asset.id, limit=limit, offset=offset
    )
    references = await repository.list_media_references(
        session, [item.id for item in rows]
    )
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
        raise _not_found("Asset version")
    version, asset = result
    await _asset_for_read(session, claims, asset.id)
    references = await repository.list_media_references(session, [version.id])
    return _version_response(version, references)


async def set_current_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    asset_id: UUID,
    request: AssetCurrentVersionRequest,
) -> AssetResponse:
    async with session.begin():
        asset = await _locked_asset_for_write(session, claims, asset_id)
        _require_revision(asset, request.expected_revision)
        _require_expected_current(asset, request.expected_current_version_id)
        result = await repository.find_version(session, request.version_id)
        if result is None or result[0].asset_id != asset.id:
            raise _not_found("Asset version")
        asset.current_version_id = request.version_id
        asset.revision += 1
        asset.updated_at = datetime.now(UTC)
        await session.flush()
    return _asset_response(asset)


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
        raise _not_found("Asset version")
    version, asset = result
    await _asset_for_read(session, claims, asset.id)
    references = await repository.list_media_references(session, [version.id])
    return await _evaluate_readiness(
        session,
        asset,
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
    rows, total = await repository.list_active_assets_with_current_version(
        session, project_id
    )
    if any(asset.workspace_id != workspace_id for asset, _version in rows):
        raise ApiError(
            ErrorCode.DEPENDENCY_UNAVAILABLE,
            "Asset summary scope is inconsistent",
            status_code=503,
        )
    references = await repository.list_media_references(
        session, [version.id for _asset, version in rows]
    )
    references_by_version: dict[UUID, list[AssetMediaReference]] = {}
    for reference in references:
        references_by_version.setdefault(reference.asset_version_id, []).append(reference)
    evaluated_at = datetime.now(UTC)
    evaluated = [
        (
            asset.kind,
            await _evaluate_readiness(
                session,
                asset,
                version,
                references_by_version.get(version.id, []),
                purpose=purpose,
                channel=channel,
                region=region,
                at_time=evaluated_at,
            ),
        )
        for asset, version in rows
    ]
    ready_kinds = tuple(
        sorted({kind for kind, readiness in evaluated if readiness.status == "ready"})
    )
    ready = sum(readiness.status == "ready" for _kind, readiness in evaluated)
    draft = sum(readiness.status == "draft" for _kind, readiness in evaluated)
    blocked = sum(readiness.status == "blocked" for _kind, readiness in evaluated)
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
        versioned=len(rows),
        ready=ready,
        draft=draft,
        blocked=blocked,
        ready_kinds=ready_kinds,
    )


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
    version, asset = result
    if version.workspace_id != workspace_id:
        return None
    return AssetVersionReference(
        id=version.id,
        workspace_id=version.workspace_id,
        project_id=asset.project_id,
        asset_id=asset.id,
        kind=asset.kind,
        asset_status=asset.status,
    )


def _readiness_reference(
    asset: Asset,
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
        "asset_status": asset.status,
        "content_hash": version.content_hash,
        "status": status,
        "blockers": [
            blocker.model_dump(mode="json", exclude_none=True)
            for blocker in readiness.blockers
        ],
        "media_version_ids": [
            str(media_id)
            for media_id in readiness.dependency_snapshot.media_version_ids
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
        kind=asset.kind,
        asset_status=asset.status,
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
        (version, asset)
        for version, asset in rows
        if version.workspace_id == workspace_id and asset.project_id == project_id
    ]
    references = await repository.list_media_references(
        session,
        [version.id for version, _asset in scoped],
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
    for version, asset in scoped:
        version_references = by_version.get(version.id, [])
        blockers, draft = _readiness_prerequisites(
            asset,
            version,
            version_references,
            media=media,
        )
        prerequisites[version.id] = (blockers, draft)
        if not draft and asset.status == "active":
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
    for version, asset in scoped:
        blockers, draft = prerequisites[version.id]
        subject = SubjectReference(
            subject_type=SubjectType.ASSET_VERSION,
            subject_id=version.id,
        )
        readiness = _compose_readiness(
            asset,
            version,
            by_version.get(version.id, []),
            blockers=blockers,
            draft=draft,
            rights=rights_by_subject.get(subject),
            rights_unavailable=rights_unavailable and subject in rights_subjects,
            at_time=at_time,
        )
        results[version.id] = _readiness_reference(asset, version, readiness)
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
    existing_version = await repository.find_candidate_version(
        session, command.candidate_id
    )
    if existing_version is not None:
        return AssetCandidateResult(
            asset_id=existing_version.asset_id,
            asset_version_id=existing_version.id,
        )
    if command.action == "link_existing":
        if command.target_asset_id is None:
            raise ApiError(
                ErrorCode.INVALID_REQUEST, "Target asset is required", status_code=422
            )
        asset = await repository.find_asset(
            session, command.target_asset_id, for_update=True
        )
        if (
            asset is None
            or asset.workspace_id != command.workspace_id
            or asset.project_id != command.project_id
            or asset.kind != command.kind
        ):
            raise _not_found()
        return AssetCandidateResult(
            asset_id=asset.id,
            asset_version_id=asset.current_version_id,
        )

    normalized_name = _normalize_name(command.name)
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
        revision=1,
        created_by=actor.user_id,
    )
    spec = _candidate_spec(command)
    version = AssetVersion(
        id=uuid7(),
        workspace_id=command.workspace_id,
        asset_id=asset.id,
        version_no=1,
        schema_version=1,
        spec=spec_to_json(spec),
        prompt_description=command.description,
        source_type="candidate",
        source_id=command.candidate_id,
        content_hash=_version_hash(
            spec, command.description, [], "candidate", command.candidate_id
        ),
        created_by=actor.user_id,
    )
    session.add(asset)
    await session.flush([asset])
    session.add(version)
    await session.flush([version])
    asset.current_version_id = version.id
    asset.revision = 2
    await session.flush([asset])
    return AssetCandidateResult(asset_id=asset.id, asset_version_id=version.id)
