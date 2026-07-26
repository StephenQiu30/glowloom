from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Literal
from uuid import UUID

from schemas.subtitles import SubtitleContentV1, SubtitleInputRefsV1

SubtitleStatus = Literal["draft", "confirmed", "superseded"]


@dataclass(frozen=True, slots=True)
class SubtitleVersionSnapshot:
    id: UUID
    episode_id: UUID
    version: int
    parent_id: UUID | None
    script_version_id: UUID
    shot_spec_version_id: UUID
    input_refs: SubtitleInputRefsV1
    content: SubtitleContentV1
    content_hash: str
    status: SubtitleStatus
    resource_version: int
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None
    input_outdated: bool = False
