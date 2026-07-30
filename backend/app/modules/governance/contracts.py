from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from uuid import UUID


class SubjectType(StrEnum):
    SCRIPT_VERSION = "SCRIPT_VERSION"
    ASSET_VERSION = "ASSET_VERSION"
    SHOT_SPEC_VERSION = "SHOT_SPEC_VERSION"
    CANDIDATE = "CANDIDATE"
    MEDIA_VERSION = "MEDIA_VERSION"
    TIMELINE_VERSION = "TIMELINE_VERSION"
    DELIVERY = "DELIVERY"


class SubjectIdentityKind(StrEnum):
    ADULT = "adult"
    FICTIONAL_ADULT = "fictional_adult"
    ORGANIZATION = "organization"
    MINOR = "minor"


class ConsentStatus(StrEnum):
    ACTIVE = "active"
    EXPIRED = "expired"
    REVOKED = "revoked"


@dataclass(frozen=True, slots=True)
class SubjectReference:
    subject_type: SubjectType
    subject_id: UUID


@dataclass(frozen=True, slots=True)
class RightsUsage:
    purpose: str
    channel: str
    region: str
    at_time: datetime


@dataclass(frozen=True, slots=True)
class ConsentGrant:
    consent_id: UUID
    status: ConsentStatus
    subject_identity_kind: SubjectIdentityKind
    purposes: frozenset[str]
    channels: frozenset[str]
    regions: frozenset[str]
    valid_from: datetime
    valid_to: datetime
    proof_accessible: bool
    revoked_at: datetime | None = None


@dataclass(frozen=True, slots=True)
class RightsBlocker:
    code: str
    consent_id: UUID | None = None


@dataclass(frozen=True, slots=True)
class RightsGateResult:
    allowed: bool
    blockers: tuple[RightsBlocker, ...]
    consent_ids: tuple[UUID, ...]
