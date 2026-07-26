from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Literal
from uuid import UUID

from schemas.media_registration import UsageType

CandidateStatus = Literal["pending_media", "ready", "blocked"]


@dataclass(frozen=True, slots=True)
class CandidateSnapshot:
    id: UUID
    episode_id: UUID
    task_id: UUID
    attempt_id: UUID
    output_slot: str
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    media_version_id: UUID
    status: CandidateStatus
    blocked_reason: str | None
    mime_type: str
    byte_size: int | None
    sha256: str | None
    width: int | None
    height: int | None
    duration_ticks: int | None
    timebase: int | None
    probe_summary: dict[str, object]
    model_profile_id: str
    provider_id: str
    model_id: str
    route_version: str
    schema_version: str
    active_adoption_id: UUID | None
    created_at: datetime
    finalized_at: datetime | None


@dataclass(frozen=True, slots=True)
class PreviewMediaSnapshot:
    media_version_id: UUID
    bucket: str
    object_key: str
