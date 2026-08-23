import json
from datetime import datetime
from enum import StrEnum
from pathlib import Path

from pydantic import BaseModel, ConfigDict, Field, TypeAdapter

FIXTURE_FILE = Path(__file__).parents[1] / "fixtures/governance/consent_gate_cases.json"


class SubjectKind(StrEnum):
    FICTIONAL_ADULT = "fictional_adult"
    MINOR = "minor"


class ExpectedDecision(StrEnum):
    ALLOWED = "allowed"
    BLOCKED = "blocked"


class ConsentScopeFixture(BaseModel):
    model_config = ConfigDict(extra="forbid")

    rights_holder_role: str = Field(min_length=1)
    rights_types: list[str] = Field(min_length=1)
    authorized_purposes: list[str] = Field(min_length=1)
    channels: list[str] = Field(min_length=1)
    regions: list[str] = Field(min_length=1)
    valid_from: datetime
    valid_to: datetime


class ExpectedGateFixture(BaseModel):
    model_config = ConfigDict(extra="forbid")

    decision: ExpectedDecision
    reason_code: str = Field(min_length=1)


class ConsentGateFixture(BaseModel):
    model_config = ConfigDict(extra="forbid")

    fixture_id: str = Field(pattern=r"^consent-[a-z-]+-001$")
    subject_ref: str = Field(pattern=r"^synthetic-subject-[a-z-]+$")
    subject_kind: SubjectKind
    asset_version_ref: str = Field(pattern=r"^synthetic-asset-version-[a-z-]+$")
    proof_media_version_ref: str = Field(pattern=r"^synthetic-proof-version-[a-z-]+$")
    submitter_role: str = Field(min_length=1)
    reviewer_role: str = Field(min_length=1)
    scope: ConsentScopeFixture
    revoked_at: datetime | None
    evaluation_at: datetime
    requested_purpose: str = Field(min_length=1)
    expected: ExpectedGateFixture


def load_fixtures() -> list[ConsentGateFixture]:
    raw: object = json.loads(FIXTURE_FILE.read_text(encoding="utf-8"))
    return TypeAdapter(list[ConsentGateFixture]).validate_python(raw)


def evaluate_fixture(fixture: ConsentGateFixture) -> tuple[ExpectedDecision, str]:
    if fixture.subject_kind is SubjectKind.MINOR:
        return ExpectedDecision.BLOCKED, "minor_not_supported"
    if fixture.revoked_at is not None and fixture.revoked_at <= fixture.evaluation_at:
        return ExpectedDecision.BLOCKED, "consent_revoked"
    if not fixture.scope.valid_from <= fixture.evaluation_at <= fixture.scope.valid_to:
        return ExpectedDecision.BLOCKED, "consent_expired"
    if fixture.requested_purpose not in fixture.scope.authorized_purposes:
        return ExpectedDecision.BLOCKED, "purpose_not_covered"
    return ExpectedDecision.ALLOWED, "consent_valid"


def test_consent_gate_fixtures_are_complete_and_have_stable_ids() -> None:
    fixtures = load_fixtures()

    assert {fixture.fixture_id for fixture in fixtures} == {
        "consent-valid-adult-001",
        "consent-revoked-001",
        "consent-insufficient-001",
        "consent-expired-001",
        "consent-minor-001",
    }
    assert all(fixture.scope.valid_from < fixture.scope.valid_to for fixture in fixtures)


def test_consent_gate_fixtures_are_synthetic_and_reference_no_external_file() -> None:
    fixtures = load_fixtures()
    serialized = FIXTURE_FILE.read_text(encoding="utf-8").lower()

    assert all(fixture.subject_ref.startswith("synthetic-") for fixture in fixtures)
    assert all(fixture.asset_version_ref.startswith("synthetic-") for fixture in fixtures)
    assert all(fixture.proof_media_version_ref.startswith("synthetic-") for fixture in fixtures)
    assert "http://" not in serialized
    assert "https://" not in serialized
    for forbidden_field in ("real_name", "id_card", "phone", "email", "address", "signature"):
        assert f'"{forbidden_field}"' not in serialized


def test_consent_gate_fixture_expectations_match_the_accepted_policy_examples() -> None:
    for fixture in load_fixtures():
        decision, reason_code = evaluate_fixture(fixture)
        assert decision is fixture.expected.decision
        assert reason_code == fixture.expected.reason_code
