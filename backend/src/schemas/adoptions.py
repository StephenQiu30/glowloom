from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Literal
from uuid import UUID

from schemas.media_registration import UsageType

AdoptionStatus = Literal["active", "superseded"]


@dataclass(frozen=True, slots=True)
class AdoptionSnapshot:
    id: UUID
    episode_id: UUID
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    version: int
    candidate_id: UUID
    supersedes_id: UUID | None
    status: AdoptionStatus
    created_at: datetime
    superseded_at: datetime | None
