from collections.abc import Sequence
from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance import repository
from app.modules.governance.consents.schemas import (
    ConsentCreateRequest,
    ConsentDetailResponse,
    ConsentRevisionRequest,
    ConsentRevisionResponse,
    ConsentRevokeRequest,
    ConsentSummaryResponse,
    MediaUsageScope,
    PaginatedConsents,
    SubjectIdentity,
)
from app.modules.governance.contracts import SubjectReference
from app.modules.governance.models import Consent, ConsentProof, ConsentRevision
from app.modules.governance.service import effective_status
from app.modules.governance.subjects import resolve_subject
from app.modules.identity import ActorContext, Capability, actor_context
from app.modules.media import media_version_accessible, media_version_exists


def _not_found() -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, "Consent not found", status_code=404)


async def _owned_consent(
    session: AsyncSession,
    claims: AccessTokenClaims,
    consent_id: UUID,
    capability: Capability,
    *,
    for_update: bool = False,
) -> tuple[Consent, ActorContext]:
    consent = await repository.find_consent(
        session, consent_id, for_update=for_update
    )
    if consent is None:
        raise _not_found()
    try:
        actor = await actor_context(
            session, claims, consent.workspace_id, capability
        )
    except ApiError as error:
        if error.code is ErrorCode.NOT_FOUND:
            raise _not_found() from error
        raise
    return consent, actor


async def _validate_proofs(
    session: AsyncSession,
    workspace_id: UUID,
    proof_ids: list[UUID],
) -> None:
    try:
        for proof_id in proof_ids:
            if not await media_version_exists(session, workspace_id, proof_id):
                raise ApiError(
                    ErrorCode.NOT_FOUND,
                    "Consent proof media not found",
                    status_code=404,
                )
            if not await media_version_accessible(session, workspace_id, proof_id):
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Consent proof media is unavailable",
                    status_code=409,
                    details={"media_version_id": str(proof_id)},
                )
    except ApiError:
        raise
    except SQLAlchemyError as error:
        raise ApiError(
            ErrorCode.DEPENDENCY_UNAVAILABLE,
            "Consent proof lookup is unavailable",
            status_code=503,
            next_action="retry",
            details={"retryable": True},
        ) from error


async def _validate_scope(
    session: AsyncSession,
    workspace_id: UUID,
    scope: MediaUsageScope,
    proof_ids: list[UUID],
) -> None:
    await resolve_subject(
        session,
        workspace_id,
        SubjectReference(
            subject_type=scope.subject_type,
            subject_id=scope.subject_id,
        ),
    )
    await _validate_proofs(session, workspace_id, proof_ids)


def _proof_map(proofs: Sequence[ConsentProof]) -> dict[UUID, list[UUID]]:
    result: dict[UUID, list[UUID]] = {}
    for proof in proofs:
        result.setdefault(proof.consent_revision_id, []).append(
            proof.media_version_id
        )
    return result


def _revision_response(
    revision: ConsentRevision,
    proof_ids: list[UUID],
) -> ConsentRevisionResponse:
    return ConsentRevisionResponse(
        id=revision.id,
        revision_no=revision.revision_no,
        action=cast(Literal["register", "update", "revoke"], revision.action),
        scope=MediaUsageScope.model_validate(revision.scope),
        proof_media_version_ids=proof_ids,
        reason=revision.reason,
        created_by=revision.created_by,
        created_at=revision.created_at,
    )


def _summary_response(
    consent: Consent,
    current_revision: ConsentRevision,
    proof_ids: list[UUID],
) -> ConsentSummaryResponse:
    if consent.current_revision_id is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Consent current revision is unavailable",
            status_code=500,
        )
    return ConsentSummaryResponse(
        id=consent.id,
        workspace_id=consent.workspace_id,
        subject_identity=SubjectIdentity.model_validate(consent.subject_identity),
        status=effective_status(
            consent.status,
            current_revision.valid_from,
            current_revision.valid_to,
        ),
        revision=consent.revision,
        current_revision_id=consent.current_revision_id,
        current_revision=_revision_response(current_revision, proof_ids),
        created_by=consent.created_by,
        created_at=consent.created_at,
        updated_at=consent.updated_at,
    )


def _detail_response(
    consent: Consent,
    revisions: list[ConsentRevision],
    proofs: list[ConsentProof],
) -> ConsentDetailResponse:
    if consent.current_revision_id is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Consent current revision is unavailable",
            status_code=500,
        )
    by_id = {revision.id: revision for revision in revisions}
    current = by_id.get(consent.current_revision_id)
    if current is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Consent revision history is incomplete",
            status_code=500,
        )
    proof_ids = _proof_map(proofs)
    summary = _summary_response(
        consent, current, proof_ids.get(current.id, [])
    )
    return ConsentDetailResponse(
        **summary.model_dump(),
        revisions=[
            _revision_response(revision, proof_ids.get(revision.id, []))
            for revision in revisions
        ],
    )


def _new_revision(
    *,
    consent: Consent,
    action: Literal["register", "update", "revoke"],
    scope: MediaUsageScope,
    reason: str,
    actor_id: UUID,
    now: datetime,
) -> ConsentRevision:
    return ConsentRevision(
        id=uuid7(),
        workspace_id=consent.workspace_id,
        consent_id=consent.id,
        revision_no=consent.revision,
        action=action,
        scope=scope.model_dump(mode="json"),
        valid_from=scope.valid_from,
        valid_to=scope.valid_to,
        reason=reason.strip(),
        created_by=actor_id,
        created_at=now,
    )


def _proofs(
    workspace_id: UUID,
    revision_id: UUID,
    proof_ids: list[UUID],
    now: datetime,
) -> list[ConsentProof]:
    return [
        ConsentProof(
            id=uuid7(),
            workspace_id=workspace_id,
            consent_revision_id=revision_id,
            media_version_id=media_version_id,
            purpose="authorization_evidence",
            position=position,
            created_at=now,
        )
        for position, media_version_id in enumerate(proof_ids, start=1)
    ]


async def _load_detail(
    session: AsyncSession, consent: Consent
) -> ConsentDetailResponse:
    revisions = await repository.list_revisions(session, consent.id)
    proofs = await repository.list_proofs(
        session, [revision.id for revision in revisions]
    )
    return _detail_response(consent, revisions, proofs)


async def _same_registration(
    session: AsyncSession,
    consent: Consent,
    request: ConsentCreateRequest,
) -> bool:
    revisions = await repository.list_revisions(session, consent.id)
    if not revisions or revisions[0].revision_no != 1:
        return False
    initial = revisions[0]
    proofs = await repository.list_proofs(session, [initial.id])
    return (
        consent.subject_identity
        == request.subject_identity.model_dump(mode="json")
        and initial.scope == request.scope.model_dump(mode="json")
        and [proof.media_version_id for proof in proofs]
        == request.proof_media_version_ids
        and initial.reason == request.reason.strip()
    )


def _idempotency_conflict() -> ApiError:
    return ApiError(
        ErrorCode.RESOURCE_CONFLICT,
        "Idempotency key was used with different input",
        status_code=409,
    )


async def create_consent(
    session: AsyncSession,
    claims: AccessTokenClaims,
    request: ConsentCreateRequest,
) -> ConsentDetailResponse:
    now = datetime.now(UTC)
    async with session.begin():
        actor = await actor_context(
            session, claims, request.workspace_id, Capability.CONTENT_WRITE
        )
        existing = await repository.find_consent_by_idempotency(
            session, request.workspace_id, request.idempotency_key
        )
        if existing is not None:
            if not await _same_registration(session, existing, request):
                raise _idempotency_conflict()
            consent = existing
        else:
            await _validate_scope(
                session,
                request.workspace_id,
                request.scope,
                request.proof_media_version_ids,
            )
            consent_id = uuid7()
            inserted_id = await session.scalar(
                insert(Consent)
                .values(
                    id=consent_id,
                    workspace_id=request.workspace_id,
                    subject_identity=request.subject_identity.model_dump(mode="json"),
                    subject_type=request.scope.subject_type,
                    subject_id=request.scope.subject_id,
                    status="active",
                    current_revision_id=None,
                    revision=1,
                    idempotency_key=request.idempotency_key,
                    created_by=actor.user_id,
                    created_at=now,
                    updated_at=now,
                )
                .on_conflict_do_nothing(
                    constraint="uq_gov_consent_workspace_idempotency"
                )
                .returning(Consent.id)
            )
            if inserted_id is None:
                existing = await repository.find_consent_by_idempotency(
                    session, request.workspace_id, request.idempotency_key
                )
                if existing is None:
                    raise ApiError(
                        ErrorCode.INTERNAL_ERROR,
                        "Consent idempotency state is unavailable",
                        status_code=500,
                    )
                if not await _same_registration(session, existing, request):
                    raise _idempotency_conflict()
                consent = existing
            else:
                created = await repository.find_consent(session, inserted_id)
                if created is None:
                    raise ApiError(
                        ErrorCode.INTERNAL_ERROR,
                        "Consent state is unavailable",
                        status_code=500,
                    )
                consent = created
                revision = _new_revision(
                    consent=consent,
                    action="register",
                    scope=request.scope,
                    reason=request.reason,
                    actor_id=actor.user_id,
                    now=now,
                )
                consent.current_revision_id = revision.id
                session.add(revision)
                await session.flush()
                session.add_all(
                    _proofs(
                        consent.workspace_id,
                        revision.id,
                        request.proof_media_version_ids,
                        now,
                    )
                )
                await session.flush()
    return await _load_detail(session, consent)


async def list_consents(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    *,
    limit: int,
    offset: int,
) -> PaginatedConsents:
    await actor_context(session, claims, workspace_id, Capability.CONTENT_READ)
    consents, total = await repository.list_consents(
        session, workspace_id, limit=limit, offset=offset
    )
    revision_ids = [
        consent.current_revision_id
        for consent in consents
        if consent.current_revision_id is not None
    ]
    revisions = await repository.find_revisions(session, revision_ids)
    by_id = {revision.id: revision for revision in revisions}
    proofs = _proof_map(await repository.list_proofs(session, revision_ids))
    items: list[ConsentSummaryResponse] = []
    for consent in consents:
        if consent.current_revision_id is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Consent current revision is unavailable",
                status_code=500,
            )
        revision = by_id.get(consent.current_revision_id)
        if revision is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Consent current revision is unavailable",
                status_code=500,
            )
        items.append(
            _summary_response(
                consent, revision, proofs.get(revision.id, [])
            )
        )
    return PaginatedConsents(
        items=items,
        total=total,
        limit=limit,
        offset=offset,
    )


async def get_consent(
    session: AsyncSession,
    claims: AccessTokenClaims,
    consent_id: UUID,
) -> ConsentDetailResponse:
    consent, _ = await _owned_consent(
        session, claims, consent_id, Capability.CONTENT_READ
    )
    return await _load_detail(session, consent)


def _require_revision(consent: Consent, expected_revision: int) -> None:
    if consent.revision != expected_revision:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Consent has changed",
            status_code=409,
            details={"current_revision": consent.revision},
        )


async def revise_consent(
    session: AsyncSession,
    claims: AccessTokenClaims,
    consent_id: UUID,
    request: ConsentRevisionRequest,
) -> ConsentDetailResponse:
    now = datetime.now(UTC)
    async with session.begin():
        consent, actor = await _owned_consent(
            session,
            claims,
            consent_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        _require_revision(consent, request.expected_revision)
        if consent.status == "revoked":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Revoked consent cannot be revised",
                status_code=409,
            )
        await _validate_scope(
            session,
            consent.workspace_id,
            request.scope,
            request.proof_media_version_ids,
        )
        consent.revision += 1
        revision = _new_revision(
            consent=consent,
            action="update",
            scope=request.scope,
            reason=request.reason,
            actor_id=actor.user_id,
            now=now,
        )
        consent.subject_type = request.scope.subject_type
        consent.subject_id = request.scope.subject_id
        consent.status = "active"
        consent.current_revision_id = revision.id
        consent.updated_at = now
        session.add(revision)
        await session.flush()
        session.add_all(
            _proofs(
                consent.workspace_id,
                revision.id,
                request.proof_media_version_ids,
                now,
            )
        )
        await session.flush()
    return await _load_detail(session, consent)


async def revoke_consent(
    session: AsyncSession,
    claims: AccessTokenClaims,
    consent_id: UUID,
    request: ConsentRevokeRequest,
) -> ConsentDetailResponse:
    now = datetime.now(UTC)
    async with session.begin():
        consent, actor = await _owned_consent(
            session,
            claims,
            consent_id,
            Capability.CONTENT_WRITE,
            for_update=True,
        )
        _require_revision(consent, request.expected_revision)
        if consent.status == "revoked":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Consent is already revoked",
                status_code=409,
            )
        if consent.current_revision_id is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Consent current revision is unavailable",
                status_code=500,
            )
        current = await repository.find_revision(
            session, consent.current_revision_id
        )
        if current is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Consent current revision is unavailable",
                status_code=500,
            )
        current_proofs = await repository.list_proofs(session, [current.id])
        scope = MediaUsageScope.model_validate(current.scope)
        consent.revision += 1
        revision = _new_revision(
            consent=consent,
            action="revoke",
            scope=scope,
            reason=request.reason,
            actor_id=actor.user_id,
            now=now,
        )
        consent.status = "revoked"
        consent.current_revision_id = revision.id
        consent.updated_at = now
        session.add(revision)
        await session.flush()
        session.add_all(
            _proofs(
                consent.workspace_id,
                revision.id,
                [proof.media_version_id for proof in current_proofs],
                now,
            )
        )
        await session.flush()
    return await _load_detail(session, consent)
