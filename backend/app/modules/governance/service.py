from datetime import UTC, datetime
from typing import Any
from uuid import UUID

from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.errors import ApiError, ErrorCode
from app.modules.governance import repository
from app.modules.governance.contracts import (
    ConsentGrant,
    ConsentStatus,
    RightsGateResult,
    RightsUsage,
    SubjectIdentityKind,
    SubjectReference,
)
from app.modules.governance.rights import evaluate_grants
from app.modules.governance.subjects import resolve_subject
from app.modules.media import media_version_accessible


def _identity_kind(identity: dict[str, Any]) -> SubjectIdentityKind:
    try:
        return SubjectIdentityKind(str(identity["kind"]))
    except (KeyError, ValueError) as error:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Consent identity is invalid",
            status_code=500,
        ) from error


async def check_rights(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    subject: SubjectReference,
    usage: RightsUsage,
) -> RightsGateResult:
    if usage.at_time.tzinfo is None:
        raise ApiError(
            ErrorCode.VALIDATION_FAILED,
            "Rights evaluation time must include a timezone",
            status_code=422,
        )
    await resolve_subject(session, workspace_id, subject)
    rows = await repository.list_current_consents_for_subject(
        session,
        workspace_id,
        subject.subject_type,
        subject.subject_id,
    )
    revision_ids = [revision.id for _, revision in rows]
    proofs = await repository.list_proofs(session, revision_ids)
    proof_ids: dict[UUID, list[UUID]] = {}
    for proof in proofs:
        proof_ids.setdefault(proof.consent_revision_id, []).append(
            proof.media_version_id
        )

    grants: list[ConsentGrant] = []
    for consent, revision in rows:
        revision_proofs = proof_ids.get(revision.id, [])
        try:
            proof_accessible = bool(revision_proofs) and all(
                [
                    await media_version_accessible(
                        session, workspace_id, media_version_id
                    )
                    for media_version_id in revision_proofs
                ]
            )
        except SQLAlchemyError as error:
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Consent proof lookup is unavailable",
                status_code=503,
                next_action="retry",
                details={"retryable": True},
            ) from error
        scope = revision.scope
        grants.append(
            ConsentGrant(
                consent_id=consent.id,
                status=ConsentStatus(consent.status),
                subject_identity_kind=_identity_kind(consent.subject_identity),
                purposes=frozenset(scope.get("authorized_purposes", [])),
                channels=frozenset(scope.get("channels", [])),
                regions=frozenset(scope.get("regions", [])),
                valid_from=revision.valid_from,
                valid_to=revision.valid_to,
                proof_accessible=proof_accessible,
                revoked_at=(
                    revision.created_at if revision.action == "revoke" else None
                ),
            )
        )
    return evaluate_grants(grants, usage)


def effective_status(
    status: str, valid_from: datetime, valid_to: datetime, *, now: datetime | None = None
) -> ConsentStatus:
    if status == ConsentStatus.REVOKED:
        return ConsentStatus.REVOKED
    current = now or datetime.now(UTC)
    if current > valid_to:
        return ConsentStatus.EXPIRED
    return ConsentStatus.ACTIVE
