import hashlib
from datetime import UTC, datetime, timedelta
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import ActorContext, Capability, actor_context
from app.modules.media import repository
from app.modules.media.contracts import MediaVersionReference
from app.modules.media.models import MediaLocation, MediaObject, MediaVersion, UploadSession
from app.modules.media.schemas import (
    AppendVersionRequest,
    ArchiveMediaRequest,
    CurrentMediaVersionRequest,
    MediaAccessRequest,
    MediaAccessResponse,
    MediaKind,
    MediaObjectResponse,
    MediaSource,
    MediaVersionResponse,
    PaginatedMedia,
    ProbeRetryRequest,
    ProbeStatus,
    UploadCapabilityResponse,
    UploadCompletionResponse,
    UploadDeclaration,
    UploadInitializationResponse,
    UploadSessionResponse,
)
from app.modules.media.storage import (
    MediaStorage,
    StorageObjectNotFound,
    StorageUnavailable,
)
from app.modules.production import (
    MediaProbeTaskCommand,
    TaskResponse,
    create_media_probe_task,
    get_internal_task,
)

ALLOWED_MIME_TYPES: dict[str, frozenset[str]] = {
    "image": frozenset({"image/jpeg", "image/png", "image/webp"}),
    "video": frozenset({"video/mp4", "video/quicktime", "video/webm"}),
    "audio": frozenset(
        {"audio/mpeg", "audio/wav", "audio/x-wav", "audio/mp4", "audio/ogg"}
    ),
    "subtitle": frozenset({"text/vtt", "application/x-subrip"}),
    "delivery": frozenset(
        {"application/zip", "application/json", "video/mp4", "application/x-subrip"}
    ),
}


def _media_object_response(media_object: MediaObject) -> MediaObjectResponse:
    return MediaObjectResponse(
        id=media_object.id,
        workspace_id=media_object.workspace_id,
        kind=cast(MediaKind, media_object.kind),
        source_type=cast(MediaSource, media_object.source_type),
        status=cast(Literal["active", "archived"], media_object.status),
        current_version_id=media_object.current_version_id,
        revision=media_object.revision,
    )


def _media_version_response(
    version: MediaVersion, media_object: MediaObject
) -> MediaVersionResponse:
    return MediaVersionResponse(
        id=version.id,
        workspace_id=version.workspace_id,
        media_object_id=version.media_object_id,
        media_object_kind=cast(MediaKind, media_object.kind),
        media_object_source_type=cast(MediaSource, media_object.source_type),
        media_object_status=cast(
            Literal["active", "archived"], media_object.status
        ),
        media_object_current_version_id=media_object.current_version_id,
        media_object_revision=media_object.revision,
        version_no=version.version_no,
        filename=version.filename,
        sha256=version.sha256,
        size_bytes=version.size_bytes,
        mime_type=version.mime_type,
        probe_status=cast(ProbeStatus, version.probe_status),
        probe_attempt=version.probe_attempt,
        probe_error_code=version.probe_error_code,
        probe_error_summary=version.probe_error_summary,
        probe_next_action=version.probe_next_action,
        width=version.width,
        height=version.height,
        duration_ms=version.duration_ms,
        codec=version.codec,
        container=version.container,
        created_at=version.created_at,
    )


def _upload_session_response(upload: UploadSession) -> UploadSessionResponse:
    return UploadSessionResponse(
        id=upload.id,
        workspace_id=upload.workspace_id,
        media_object_id=upload.media_object_id,
        status=cast(
            Literal["pending", "completed", "expired", "failed"], upload.status
        ),
        kind=cast(MediaKind, upload.declared_kind),
        filename=upload.filename,
        size_bytes=upload.declared_size_bytes,
        mime_type=upload.declared_mime_type,
        sha256=upload.declared_sha256,
        expires_at=upload.expires_at,
    )


def _validate_declaration(request: UploadDeclaration, settings: Settings) -> None:
    if request.size_bytes > settings.media_max_upload_bytes:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Media exceeds the upload size limit",
            status_code=422,
            details={"max_size_bytes": settings.media_max_upload_bytes},
        )
    if request.mime_type.lower() not in ALLOWED_MIME_TYPES[request.kind]:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Media MIME type is not allowed for this kind",
            status_code=422,
        )


def _same_upload(upload: UploadSession, request: UploadDeclaration) -> bool:
    expected_current = getattr(request, "expected_current_version_id", None)
    return (
        upload.declared_kind == request.kind
        and upload.filename == request.filename.strip()
        and upload.declared_size_bytes == request.size_bytes
        and upload.declared_mime_type == request.mime_type.lower()
        and upload.declared_sha256 == request.sha256
        and upload.expected_current_version_id == expected_current
    )


async def _create_upload_session(
    session: AsyncSession,
    request: UploadDeclaration,
    storage: MediaStorage,
    settings: Settings,
    actor: ActorContext,
    now: datetime,
    *,
    media_object_id: UUID | None = None,
    expected_current_version_id: UUID | None = None,
) -> UploadSession:
    upload_id = uuid7()
    inserted_id = await session.scalar(
        insert(UploadSession)
        .values(
            id=upload_id,
            workspace_id=request.workspace_id,
            media_object_id=media_object_id,
            expected_current_version_id=expected_current_version_id,
            storage_profile=storage.profile,
            bucket=storage.bucket,
            object_key=f"workspaces/{request.workspace_id}/uploads/{uuid7()}",
            filename=request.filename.strip(),
            declared_kind=request.kind,
            declared_size_bytes=request.size_bytes,
            declared_mime_type=request.mime_type.lower(),
            declared_sha256=request.sha256,
            status="pending",
            expires_at=now + timedelta(seconds=settings.media_upload_ttl_seconds),
            idempotency_key=request.idempotency_key,
            created_by=actor.user_id,
            created_at=now,
            updated_at=now,
        )
        .on_conflict_do_nothing(constraint="uq_med_upload_idempotency")
        .returning(UploadSession.id)
    )
    if inserted_id is not None:
        upload = await repository.find_upload_session(session, inserted_id)
    else:
        upload = await repository.find_idempotent_upload(
            session, request.workspace_id, request.idempotency_key
        )
    if upload is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR, "Upload session is unavailable", status_code=500
        )
    if upload.media_object_id != media_object_id or not _same_upload(upload, request):
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Idempotency key was used with different input",
            status_code=409,
        )
    return upload


def _dependency_error() -> ApiError:
    return ApiError(
        ErrorCode.DEPENDENCY_UNAVAILABLE,
        "Object storage is temporarily unavailable",
        status_code=503,
        next_action="retry",
        details={"retryable": True},
    )


async def _presigned_upload(
    upload: UploadSession,
    storage: MediaStorage,
    now: datetime,
) -> UploadInitializationResponse:
    remaining = max(1, int((upload.expires_at - now).total_seconds()))
    try:
        url = await storage.port.presign_upload(upload.object_key, remaining)
    except StorageUnavailable as error:
        raise _dependency_error() from error
    except Exception as error:
        raise _dependency_error() from error
    return UploadInitializationResponse(
        upload_session=_upload_session_response(upload),
        upload=UploadCapabilityResponse(
            url=url,
            headers={"content-type": upload.declared_mime_type},
            expires_at=upload.expires_at,
        ),
    )


async def _owned_media_object(
    session: AsyncSession,
    claims: AccessTokenClaims,
    media_object_id: UUID,
    capability: Capability,
    *,
    for_update: bool = False,
) -> tuple[MediaObject, ActorContext]:
    media_object = await repository.find_media_object(
        session, media_object_id, for_update=for_update
    )
    if media_object is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Media object not found", status_code=404)
    try:
        actor = await actor_context(session, claims, media_object.workspace_id, capability)
    except ApiError as error:
        if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
            raise ApiError(
                ErrorCode.NOT_FOUND, "Media object not found", status_code=404
            ) from error
        raise
    return media_object, actor


async def _owned_media_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
    capability: Capability,
    *,
    for_update: bool = False,
) -> tuple[MediaVersion, MediaObject, ActorContext]:
    result = await repository.find_media_version(session, version_id, for_update=for_update)
    if result is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Media version not found", status_code=404)
    version, media_object = result
    try:
        actor = await actor_context(session, claims, version.workspace_id, capability)
    except ApiError as error:
        if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
            raise ApiError(
                ErrorCode.NOT_FOUND, "Media version not found", status_code=404
            ) from error
        raise
    return version, media_object, actor


async def initialize_upload(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: UploadDeclaration,
    storage: MediaStorage,
    settings: Settings,
) -> UploadInitializationResponse:
    _validate_declaration(request, settings)
    now = datetime.now(UTC)
    async with session.begin():
        actor = await actor_context(
            session, claims, request.workspace_id, Capability.CONTENT_WRITE
        )
        upload = await _create_upload_session(
            session, request, storage, settings, actor, now
        )
        response = await _presigned_upload(upload, storage, now)
    return response


async def initialize_version_upload(
    session: AsyncSession,
    claims: AccessTokenClaims,
    media_object_id: UUID,
    request: AppendVersionRequest,
    storage: MediaStorage,
    settings: Settings,
) -> UploadInitializationResponse:
    _validate_declaration(request, settings)
    now = datetime.now(UTC)
    async with session.begin():
        media_object, actor = await _owned_media_object(
            session,
            claims,
            media_object_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        if request.workspace_id != media_object.workspace_id:
            raise ApiError(ErrorCode.NOT_FOUND, "Media object not found", status_code=404)
        if media_object.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Archived media cannot accept new versions",
                status_code=409,
                next_action="restore_media",
            )
        if media_object.current_version_id != request.expected_current_version_id:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Media current version has changed",
                status_code=409,
                details={"current_version_id": str(media_object.current_version_id)},
            )
        if media_object.kind != request.kind:
            raise ApiError(
                ErrorCode.VALIDATION_FAILED,
                "Replacement kind must match the media object",
                status_code=422,
            )
        upload = await _create_upload_session(
            session,
            request,
            storage,
            settings,
            actor,
            now,
            media_object_id=media_object.id,
            expected_current_version_id=request.expected_current_version_id,
        )
        response = await _presigned_upload(upload, storage, now)
    return response


async def _verified_object_hash(upload: UploadSession, storage: MediaStorage) -> str:
    try:
        actual_size, actual_mime = await storage.port.stat(upload.object_key)
        normalized_mime = (actual_mime or "").split(";", 1)[0].strip().lower()
        if (
            actual_size != upload.declared_size_bytes
            or normalized_mime != upload.declared_mime_type
        ):
            return ""
        digest = hashlib.sha256()
        async for chunk in storage.port.stream(upload.object_key):
            digest.update(chunk)
        return digest.hexdigest()
    except StorageObjectNotFound:
        return ""
    except StorageUnavailable as error:
        raise _dependency_error() from error
    except Exception as error:
        raise _dependency_error() from error


async def complete_upload(
    session: AsyncSession,
    claims: AccessTokenClaims,
    upload_session_id: UUID,
    storage: MediaStorage,
    *,
    trace_id: str,
) -> UploadCompletionResponse:
    now = datetime.now(UTC)
    deferred_error: ApiError | None = None
    result: UploadCompletionResponse | None = None
    async with session.begin():
        upload = await repository.find_upload_session(
            session, upload_session_id, for_update=True
        )
        if upload is None:
            raise ApiError(ErrorCode.NOT_FOUND, "Upload session not found", status_code=404)
        try:
            actor = await actor_context(
                session, claims, upload.workspace_id, Capability.CONTENT_WRITE
            )
        except ApiError as error:
            if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
                raise ApiError(
                    ErrorCode.NOT_FOUND, "Upload session not found", status_code=404
                ) from error
            raise
        if upload.status == "completed":
            if (
                upload.completed_version_id is None
                or upload.completed_probe_task_id is None
            ):
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR, "Upload result is unavailable", status_code=500
                )
            owned = await repository.find_media_version(
                session, upload.completed_version_id
            )
            if owned is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR, "Upload result is unavailable", status_code=500
                )
            version, media_object = owned
            task = await get_internal_task(session, upload.completed_probe_task_id)
            return UploadCompletionResponse(
                media_object=_media_object_response(media_object),
                version=_media_version_response(version, media_object),
                probe_task=task,
            )
        if upload.status != "pending":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Upload session cannot be completed",
                status_code=409,
                next_action="initialize_upload",
            )
        if upload.expires_at <= now:
            upload.status = "expired"
            upload.error_code = "upload_expired"
            deferred_error = ApiError(
                ErrorCode.STATE_CONFLICT,
                "Upload session has expired",
                status_code=409,
                next_action="initialize_upload",
            )
        else:
            actual_hash = await _verified_object_hash(upload, storage)
            if actual_hash != upload.declared_sha256:
                upload.status = "failed"
                upload.error_code = "object_validation_failed"
                deferred_error = ApiError(
                    ErrorCode.VALIDATION_FAILED,
                    "Uploaded object does not match its declaration",
                    status_code=422,
                    next_action="upload_again",
                )
            else:
                if upload.media_object_id is None:
                    media_object = MediaObject(
                        id=uuid7(),
                        workspace_id=upload.workspace_id,
                        kind=upload.declared_kind,
                        source_type="upload",
                        status="active",
                        revision=1,
                    )
                    session.add(media_object)
                    version_no = 1
                else:
                    media_object = await repository.find_media_object(
                        session, upload.media_object_id, for_update=True
                    )
                    if media_object is None or media_object.workspace_id != upload.workspace_id:
                        raise ApiError(
                            ErrorCode.NOT_FOUND, "Media object not found", status_code=404
                        )
                    if media_object.status != "active":
                        raise ApiError(
                            ErrorCode.STATE_CONFLICT,
                            "Archived media cannot accept new versions",
                            status_code=409,
                            next_action="restore_media",
                        )
                    if media_object.current_version_id != upload.expected_current_version_id:
                        raise ApiError(
                            ErrorCode.VERSION_CONFLICT,
                            "Media current version has changed",
                            status_code=409,
                            details={
                                "current_version_id": str(media_object.current_version_id)
                            },
                        )
                    current = await repository.find_media_version(
                        session, cast(UUID, media_object.current_version_id)
                    )
                    if current is None:
                        raise ApiError(
                            ErrorCode.INTERNAL_ERROR,
                            "Media current version is unavailable",
                            status_code=500,
                        )
                    version_no = current[0].version_no + 1
                    media_object.revision += 1
                version = MediaVersion(
                    id=uuid7(),
                    workspace_id=upload.workspace_id,
                    media_object_id=media_object.id,
                    version_no=version_no,
                    filename=upload.filename,
                    sha256=upload.declared_sha256,
                    size_bytes=upload.declared_size_bytes,
                    mime_type=upload.declared_mime_type,
                    probe_status="pending",
                    probe_attempt=1,
                    probe_idempotency_key="initial",
                    created_by=actor.user_id,
                    created_at=now,
                )
                session.add(version)
                media_object.current_version_id = version.id
                session.add(
                    MediaLocation(
                        id=uuid7(),
                        workspace_id=upload.workspace_id,
                        media_version_id=version.id,
                        storage_profile=upload.storage_profile,
                        bucket=upload.bucket,
                        object_key=upload.object_key,
                        status="active",
                        verified_at=now,
                        created_at=now,
                    )
                )
                await session.flush()
                task = await create_media_probe_task(
                    session,
                    actor,
                    MediaProbeTaskCommand(
                        workspace_id=upload.workspace_id,
                        media_version_id=version.id,
                        idempotency_key=f"media-probe:{version.id}:1",
                    ),
                    trace_id=trace_id,
                )
                version.probe_task_id = task.id
                upload.status = "completed"
                upload.completed_version_id = version.id
                upload.completed_probe_task_id = task.id
                upload.error_code = None
                result = UploadCompletionResponse(
                    media_object=_media_object_response(media_object),
                    version=_media_version_response(version, media_object),
                    probe_task=task,
                )
                append_audit_event(
                    session,
                    workspace_id=media_object.workspace_id,
                    actor_id=actor.user_id,
                    action="media.version_created",
                    target_type="media_object",
                    target_id=media_object.id,
                    trace_id=trace_id,
                    metadata={
                        "revision": media_object.revision,
                        "version_id": str(version.id),
                        "version_no": version.version_no,
                        "kind": media_object.kind,
                        "source_type": media_object.source_type,
                    },
                    occurred_at=now,
                )
        await session.flush()
    if deferred_error is not None:
        raise deferred_error
    if result is None:
        raise ApiError(ErrorCode.INTERNAL_ERROR, "Upload result is unavailable", status_code=500)
    return result


async def list_media(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    *,
    kind: MediaKind | None,
    source_type: MediaSource | None,
    include_archived: bool,
    created_from: datetime | None,
    created_to: datetime | None,
    limit: int,
    offset: int,
) -> PaginatedMedia:
    await actor_context(session, claims, workspace_id, Capability.CONTENT_READ)
    versions, total = await repository.list_media_versions(
        session,
        workspace_id,
        kind=kind,
        source_type=source_type,
        include_archived=include_archived,
        created_from=created_from,
        created_to=created_to,
        limit=limit,
        offset=offset,
    )
    return PaginatedMedia(
        items=[
            _media_version_response(version, media_object)
            for version, media_object in versions
        ],
        total=total,
        limit=limit,
        offset=offset,
    )


async def get_media(
    session: AsyncSession, claims: AccessTokenClaims, version_id: UUID
) -> MediaVersionResponse:
    version, media_object, _ = await _owned_media_version(
        session, claims, version_id, Capability.CONTENT_READ
    )
    return _media_version_response(version, media_object)


async def media_version_exists(
    session: AsyncSession, workspace_id: UUID, version_id: UUID
) -> bool:
    result = await repository.find_media_version(session, version_id)
    return result is not None and result[0].workspace_id == workspace_id


async def media_version_accessible(
    session: AsyncSession, workspace_id: UUID, version_id: UUID
) -> bool:
    result = await repository.find_media_version(session, version_id)
    if result is None:
        return False
    version, _ = result
    if version.workspace_id != workspace_id or version.probe_status == "quarantined":
        return False
    return await repository.find_active_location(session, version.id) is not None


async def resolve_media_version_reference(
    session: AsyncSession, workspace_id: UUID, version_id: UUID
) -> MediaVersionReference | None:
    result = await repository.find_media_version(session, version_id)
    if result is None:
        return None
    version, media_object = result
    if version.workspace_id != workspace_id:
        return None
    return MediaVersionReference(
        id=version.id,
        workspace_id=version.workspace_id,
        kind=media_object.kind,
        object_status=media_object.status,
        probe_status=version.probe_status,
        has_active_location=(
            await repository.find_active_location(session, version.id) is not None
        ),
    )


async def resolve_media_version_references(
    session: AsyncSession,
    workspace_id: UUID,
    version_ids: list[UUID],
) -> dict[UUID, MediaVersionReference]:
    unique_ids = list(dict.fromkeys(version_ids))
    rows = await repository.find_media_versions_with_active_locations(
        session,
        unique_ids,
    )
    return {
        version.id: MediaVersionReference(
            id=version.id,
            workspace_id=version.workspace_id,
            kind=media_object.kind,
            object_status=media_object.status,
            probe_status=version.probe_status,
            has_active_location=location is not None,
        )
        for version, media_object, location in rows
        if version.workspace_id == workspace_id
    }


async def create_access(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
    request: MediaAccessRequest,
    storage: MediaStorage,
    settings: Settings,
) -> MediaAccessResponse:
    version, _, _ = await _owned_media_version(
        session, claims, version_id, Capability.CONTENT_READ
    )
    if version.probe_status == "quarantined":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Quarantined media cannot be accessed",
            status_code=409,
        )
    location = await repository.find_active_location(session, version.id)
    if location is None:
        raise ApiError(
            ErrorCode.DEPENDENCY_UNAVAILABLE,
            "Media location is unavailable",
            status_code=503,
            details={"retryable": True},
        )
    try:
        url = await storage.port.presign_download(
            location.object_key, settings.media_access_ttl_seconds
        )
    except Exception as error:
        raise _dependency_error() from error
    return MediaAccessResponse(
        url=url,
        purpose=request.purpose,
        expires_at=datetime.now(UTC)
        + timedelta(seconds=settings.media_access_ttl_seconds),
    )


async def archive_media(
    session: AsyncSession,
    claims: AccessTokenClaims,
    media_object_id: UUID,
    request: ArchiveMediaRequest,
    *,
    trace_id: str,
) -> MediaObjectResponse:
    async with session.begin():
        media_object, _ = await _owned_media_object(
            session,
            claims,
            media_object_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        if media_object.revision != request.expected_revision:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Media object revision has changed",
                status_code=409,
                details={"current_revision": media_object.revision},
            )
        if media_object.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Media object is already archived",
                status_code=409,
            )
        now = datetime.now(UTC)
        media_object.status = "archived"
        media_object.archived_at = now
        media_object.archived_by = claims.sub
        media_object.revision += 1
        media_object.updated_at = now
        append_audit_event(
            session,
            workspace_id=media_object.workspace_id,
            actor_id=claims.sub,
            action="media.archived",
            target_type="media_object",
            target_id=media_object.id,
            trace_id=trace_id,
            metadata={
                "revision": media_object.revision,
                "current_version_id": str(media_object.current_version_id),
            },
            occurred_at=now,
        )
        await session.flush()
    return _media_object_response(media_object)


async def restore_media(
    session: AsyncSession,
    claims: AccessTokenClaims,
    media_object_id: UUID,
    request: ArchiveMediaRequest,
    *,
    trace_id: str,
) -> MediaObjectResponse:
    async with session.begin():
        media_object, _ = await _owned_media_object(
            session,
            claims,
            media_object_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        if media_object.revision != request.expected_revision:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Media object revision has changed",
                status_code=409,
                details={"current_revision": media_object.revision},
            )
        if media_object.status != "archived":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Media object is already active",
                status_code=409,
            )
        now = datetime.now(UTC)
        media_object.status = "active"
        media_object.archived_at = None
        media_object.archived_by = None
        media_object.revision += 1
        media_object.updated_at = now
        append_audit_event(
            session,
            workspace_id=media_object.workspace_id,
            actor_id=claims.sub,
            action="media.restored",
            target_type="media_object",
            target_id=media_object.id,
            trace_id=trace_id,
            metadata={
                "revision": media_object.revision,
                "current_version_id": str(media_object.current_version_id),
            },
            occurred_at=now,
        )
        await session.flush()
    return _media_object_response(media_object)


async def set_current_version(
    session: AsyncSession,
    claims: AccessTokenClaims,
    media_object_id: UUID,
    request: CurrentMediaVersionRequest,
    *,
    trace_id: str,
) -> MediaObjectResponse:
    async with session.begin():
        media_object, _ = await _owned_media_object(
            session,
            claims,
            media_object_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        if media_object.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Archived media cannot change its current version",
                status_code=409,
                next_action="restore_media",
            )
        if (
            media_object.revision != request.expected_revision
            or media_object.current_version_id
            != request.expected_current_version_id
        ):
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Media object has changed",
                status_code=409,
                details={
                    "current_revision": media_object.revision,
                    "current_version_id": str(media_object.current_version_id),
                },
            )
        target = await repository.find_media_version(session, request.version_id)
        if target is None or target[0].media_object_id != media_object.id:
            raise ApiError(
                ErrorCode.NOT_FOUND, "Media version not found", status_code=404
            )
        previous_version_id = media_object.current_version_id
        now = datetime.now(UTC)
        media_object.current_version_id = request.version_id
        media_object.revision += 1
        media_object.updated_at = now
        append_audit_event(
            session,
            workspace_id=media_object.workspace_id,
            actor_id=claims.sub,
            action="media.current_changed",
            target_type="media_object",
            target_id=media_object.id,
            trace_id=trace_id,
            metadata={
                "revision": media_object.revision,
                "previous_version_id": str(previous_version_id),
                "current_version_id": str(media_object.current_version_id),
            },
            occurred_at=now,
        )
        await session.flush()
    return _media_object_response(media_object)


async def retry_probe(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
    request: ProbeRetryRequest,
    *,
    trace_id: str,
) -> TaskResponse:
    async with session.begin():
        version, _, actor = await _owned_media_version(
            session,
            claims,
            version_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        if version.probe_status == "pending":
            if (
                version.probe_idempotency_key == request.idempotency_key
                and version.probe_task_id is not None
            ):
                return await get_internal_task(session, version.probe_task_id)
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Media probe is already pending",
                status_code=409,
            )
        if version.probe_status != "failed":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Only a failed media probe can be retried",
                status_code=409,
            )
        version.probe_status = "pending"
        version.probe_attempt += 1
        version.probe_error_code = None
        version.probe_error_summary = None
        version.probe_next_action = None
        version.probe_idempotency_key = request.idempotency_key
        task = await create_media_probe_task(
            session,
            actor,
            MediaProbeTaskCommand(
                workspace_id=version.workspace_id,
                media_version_id=version.id,
                idempotency_key=f"media-probe:{version.id}:{request.idempotency_key}",
            ),
            trace_id=trace_id,
        )
        version.probe_task_id = task.id
        await session.flush()
    return task
