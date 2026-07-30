from app.modules.governance.contracts import (
    ConsentGrant,
    ConsentStatus,
    RightsBlocker,
    RightsGateResult,
    RightsUsage,
    SubjectIdentityKind,
)


def _blocker(grant: ConsentGrant, usage: RightsUsage) -> RightsBlocker | None:
    if grant.subject_identity_kind is SubjectIdentityKind.MINOR:
        return RightsBlocker("minor_not_supported", grant.consent_id)
    if grant.status is ConsentStatus.REVOKED and (
        grant.revoked_at is None or grant.revoked_at <= usage.at_time
    ):
        return RightsBlocker("consent_revoked", grant.consent_id)
    if usage.at_time < grant.valid_from:
        return RightsBlocker("consent_not_yet_valid", grant.consent_id)
    if usage.at_time > grant.valid_to:
        return RightsBlocker("consent_expired", grant.consent_id)
    if usage.purpose not in grant.purposes:
        return RightsBlocker("purpose_not_covered", grant.consent_id)
    if usage.channel not in grant.channels:
        return RightsBlocker("channel_not_covered", grant.consent_id)
    if usage.region not in grant.regions:
        return RightsBlocker("region_not_covered", grant.consent_id)
    if not grant.proof_accessible:
        return RightsBlocker("proof_unavailable", grant.consent_id)
    return None


def evaluate_grants(
    grants: list[ConsentGrant], usage: RightsUsage
) -> RightsGateResult:
    if not grants:
        return RightsGateResult(
            allowed=False,
            blockers=(RightsBlocker("consent_missing"),),
            consent_ids=(),
        )

    valid_ids = tuple(
        grant.consent_id for grant in grants if _blocker(grant, usage) is None
    )
    if valid_ids:
        return RightsGateResult(allowed=True, blockers=(), consent_ids=valid_ids)

    blockers = tuple(
        blocker
        for grant in grants
        if (blocker := _blocker(grant, usage)) is not None
    )
    return RightsGateResult(allowed=False, blockers=blockers, consent_ids=())
