import json
from datetime import datetime
from pathlib import Path
from uuid import UUID, uuid5

import pytest

from app.modules.governance.contracts import (
    ConsentGrant,
    ConsentStatus,
    RightsUsage,
    SubjectIdentityKind,
)
from app.modules.governance.rights import evaluate_grants

FIXTURE_FILE = (
    Path(__file__).resolve().parents[2]
    / "fixtures/governance/consent_gate_cases.json"
)
NAMESPACE = UUID("9dfaecf8-3cc7-4fe4-b828-1977e58c96e1")


def _grant(case: dict[str, object]) -> ConsentGrant:
    scope = case["scope"]
    assert isinstance(scope, dict)
    revoked_at = case["revoked_at"]
    return ConsentGrant(
        consent_id=uuid5(NAMESPACE, str(case["fixture_id"])),
        status=(
            ConsentStatus.REVOKED if revoked_at is not None else ConsentStatus.ACTIVE
        ),
        subject_identity_kind=SubjectIdentityKind(str(case["subject_kind"])),
        purposes=frozenset(str(value) for value in scope["authorized_purposes"]),
        channels=frozenset(str(value) for value in scope["channels"]),
        regions=frozenset(str(value) for value in scope["regions"]),
        valid_from=datetime.fromisoformat(str(scope["valid_from"])),
        valid_to=datetime.fromisoformat(str(scope["valid_to"])),
        proof_accessible=True,
    )


def _usage(case: dict[str, object]) -> RightsUsage:
    scope = case["scope"]
    assert isinstance(scope, dict)
    return RightsUsage(
        purpose=str(case["requested_purpose"]),
        channel=str(scope["channels"][0]),
        region=str(scope["regions"][0]),
        at_time=datetime.fromisoformat(str(case["evaluation_at"])),
    )


@pytest.mark.parametrize(
    "case",
    json.loads(FIXTURE_FILE.read_text(encoding="utf-8")),
    ids=lambda case: case["fixture_id"],
)
def test_accepted_consent_fixtures_drive_the_rights_decision(
    case: dict[str, object],
) -> None:
    result = evaluate_grants([_grant(case)], _usage(case))
    expected = case["expected"]
    assert isinstance(expected, dict)

    assert result.allowed is (expected["decision"] == "allowed")
    assert [blocker.code for blocker in result.blockers] == (
        [] if result.allowed else [expected["reason_code"]]
    )
    assert result.consent_ids == (
        (_grant(case).consent_id,) if result.allowed else ()
    )


def test_rights_policy_rejects_channel_region_and_inaccessible_proof() -> None:
    case = json.loads(FIXTURE_FILE.read_text(encoding="utf-8"))[0]
    grant = _grant(case)
    baseline = _usage(case)

    assert [
        blocker.code
        for blocker in evaluate_grants(
            [grant],
            RightsUsage(
                purpose=baseline.purpose,
                channel="public_export",
                region=baseline.region,
                at_time=baseline.at_time,
            ),
        ).blockers
    ] == ["channel_not_covered"]
    assert [
        blocker.code
        for blocker in evaluate_grants(
            [grant],
            RightsUsage(
                purpose=baseline.purpose,
                channel=baseline.channel,
                region="US",
                at_time=baseline.at_time,
            ),
        ).blockers
    ] == ["region_not_covered"]
    inaccessible = ConsentGrant(
        consent_id=grant.consent_id,
        status=grant.status,
        subject_identity_kind=grant.subject_identity_kind,
        purposes=grant.purposes,
        channels=grant.channels,
        regions=grant.regions,
        valid_from=grant.valid_from,
        valid_to=grant.valid_to,
        proof_accessible=False,
    )
    assert [
        blocker.code
        for blocker in evaluate_grants([inaccessible], baseline).blockers
    ] == ["proof_unavailable"]


def test_rights_policy_requires_at_least_one_complete_grant() -> None:
    case = json.loads(FIXTURE_FILE.read_text(encoding="utf-8"))[0]
    usage = _usage(case)

    missing = evaluate_grants([], usage)
    assert missing.allowed is False
    assert [blocker.code for blocker in missing.blockers] == ["consent_missing"]

    invalid = _grant(case)
    valid = ConsentGrant(
        consent_id=uuid5(NAMESPACE, "second-valid-consent"),
        status=ConsentStatus.ACTIVE,
        subject_identity_kind=SubjectIdentityKind.FICTIONAL_ADULT,
        purposes=invalid.purposes,
        channels=invalid.channels,
        regions=invalid.regions,
        valid_from=invalid.valid_from,
        valid_to=invalid.valid_to,
        proof_accessible=True,
    )
    revoked = ConsentGrant(
        consent_id=invalid.consent_id,
        status=ConsentStatus.REVOKED,
        subject_identity_kind=invalid.subject_identity_kind,
        purposes=invalid.purposes,
        channels=invalid.channels,
        regions=invalid.regions,
        valid_from=invalid.valid_from,
        valid_to=invalid.valid_to,
        proof_accessible=True,
    )
    result = evaluate_grants([revoked, valid], usage)

    assert result.allowed is True
    assert result.blockers == ()
    assert result.consent_ids == (valid.consent_id,)
