from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

from lanverse.modules.story_development.application.contracts.content_v1 import ScriptContentV1


@dataclass(frozen=True, slots=True)
class ScriptVersionSnapshot:
    id: UUID
    episode_id: UUID
    version: int
    parent_id: UUID | None
    source_revision_id: UUID
    content: ScriptContentV1
    content_hash: str
    origin_task_id: UUID | None
    status: str
    resource_version: int
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None
    input_outdated: bool = False
