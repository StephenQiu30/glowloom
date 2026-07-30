from datetime import datetime
from typing import Literal, Self
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from app.modules.governance.contracts import ConsentStatus, SubjectIdentityKind, SubjectType


def _normalized_unique(values: list[str], field_name: str) -> list[str]:
    normalized = [value.strip() for value in values]
    if any(not value for value in normalized):
        raise ValueError(f"{field_name} entries must not be blank")
    if len(set(normalized)) != len(normalized):
        raise ValueError(f"{field_name} entries must be unique")
    return normalized


class SubjectIdentity(BaseModel):
    model_config = ConfigDict(extra="forbid")

    reference: str = Field(min_length=1, max_length=160, pattern=r"^[A-Za-z0-9._:-]+$")
    kind: SubjectIdentityKind


class MediaUsageScope(BaseModel):
    model_config = ConfigDict(extra="forbid")

    type: Literal["media_usage"]
    subject_type: SubjectType
    subject_id: UUID
    rights_holder_role: str = Field(min_length=1, max_length=80)
    rights_types: list[str] = Field(min_length=1, max_length=20)
    authorized_purposes: list[str] = Field(min_length=1, max_length=20)
    channels: list[str] = Field(min_length=1, max_length=20)
    regions: list[str] = Field(min_length=1, max_length=20)
    valid_from: datetime
    valid_to: datetime

    @field_validator(
        "rights_types",
        "authorized_purposes",
        "channels",
        mode="after",
    )
    @classmethod
    def validate_terms(cls, values: list[str], info: object) -> list[str]:
        field_name = getattr(info, "field_name", "scope")
        normalized = _normalized_unique(values, field_name)
        if any(
            len(value) > 80
            or not value.replace("_", "").replace("-", "").isalnum()
            for value in normalized
        ):
            raise ValueError(f"{field_name} entries use an invalid format")
        return normalized

    @field_validator("regions", mode="after")
    @classmethod
    def validate_regions(cls, values: list[str]) -> list[str]:
        normalized = _normalized_unique(values, "regions")
        if any(
            len(value) != 2 or not value.isascii() or not value.isupper()
            for value in normalized
        ):
            raise ValueError("regions must contain ISO 3166-1 alpha-2 codes")
        return normalized

    @model_validator(mode="after")
    def validate_period(self) -> Self:
        if self.valid_from.tzinfo is None or self.valid_to.tzinfo is None:
            raise ValueError("validity timestamps must include a timezone")
        if self.valid_from >= self.valid_to:
            raise ValueError("valid_to must be later than valid_from")
        return self


class ConsentCreateRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    workspace_id: UUID
    subject_identity: SubjectIdentity
    scope: MediaUsageScope
    proof_media_version_ids: list[UUID] = Field(min_length=1, max_length=20)
    reason: str = Field(min_length=1, max_length=1000)
    idempotency_key: str = Field(min_length=1, max_length=200)

    @field_validator("proof_media_version_ids", mode="after")
    @classmethod
    def validate_proofs(cls, values: list[UUID]) -> list[UUID]:
        if len(set(values)) != len(values):
            raise ValueError("proof_media_version_ids must be unique")
        return values

    @field_validator("idempotency_key", mode="after")
    @classmethod
    def normalize_idempotency_key(cls, value: str) -> str:
        normalized = value.strip()
        if not normalized:
            raise ValueError("idempotency_key must not be blank")
        return normalized


class ConsentRevisionRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    expected_revision: int = Field(ge=1)
    scope: MediaUsageScope
    proof_media_version_ids: list[UUID] = Field(min_length=1, max_length=20)
    reason: str = Field(min_length=1, max_length=1000)

    @field_validator("proof_media_version_ids", mode="after")
    @classmethod
    def validate_proofs(cls, values: list[UUID]) -> list[UUID]:
        if len(set(values)) != len(values):
            raise ValueError("proof_media_version_ids must be unique")
        return values


class ConsentRevokeRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    expected_revision: int = Field(ge=1)
    reason: str = Field(min_length=1, max_length=1000)


class ConsentRevisionResponse(BaseModel):
    id: UUID
    revision_no: int
    action: Literal["register", "update", "revoke"]
    scope: MediaUsageScope
    proof_media_version_ids: list[UUID]
    reason: str
    created_by: UUID
    created_at: datetime


class ConsentSummaryResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    subject_identity: SubjectIdentity
    status: ConsentStatus
    revision: int
    current_revision_id: UUID
    current_revision: ConsentRevisionResponse
    created_by: UUID
    created_at: datetime
    updated_at: datetime


class ConsentDetailResponse(ConsentSummaryResponse):
    revisions: list[ConsentRevisionResponse]


class PaginatedConsents(BaseModel):
    items: list[ConsentSummaryResponse]
    total: int
    limit: int
    offset: int
