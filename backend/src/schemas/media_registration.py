from __future__ import annotations

from dataclasses import dataclass
from typing import Literal
from uuid import UUID

from schemas.media import MediaKind

UsageType = Literal["asset_image", "shot_image", "shot_video", "speech_audio"]


@dataclass(frozen=True, slots=True)
class MediaRegistrationCommand:
    episode_id: UUID
    task_id: UUID
    attempt_id: UUID
    output_slot: str
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    media_kind: MediaKind
    content_type: str
    data: bytes
    target_duration_ticks: int | None = None


@dataclass(frozen=True, slots=True)
class MediaRegistrationSnapshot:
    media_object_id: UUID
    media_version_id: UUID
    candidate_id: UUID
    media_status: str
    candidate_status: str
    bucket: str
    object_key: str
    sha256: str
