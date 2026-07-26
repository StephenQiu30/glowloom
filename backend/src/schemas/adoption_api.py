from __future__ import annotations

from dataclasses import asdict
from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import Field

from schemas.adoptions import AdoptionSnapshot
from schemas.common import StrictContract
from schemas.media_registration import UsageType


class AdoptCandidateRequest(StrictContract):
    usage_type: UsageType
    usage_id: UUID = Field(strict=False)
    input_version_id: UUID = Field(strict=False)
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    candidate_id: UUID = Field(strict=False)


class AdoptionResponse(StrictContract):
    id: UUID
    episode_id: UUID
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    version: int = Field(ge=1)
    candidate_id: UUID
    supersedes_id: UUID | None
    status: Literal["active", "superseded"]
    created_at: datetime
    superseded_at: datetime | None


def adoption_response(value: AdoptionSnapshot) -> AdoptionResponse:
    return AdoptionResponse(**asdict(value))
