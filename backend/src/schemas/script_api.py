from __future__ import annotations

import json
from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import Field, field_validator

from schemas.common import StrictContract
from schemas.story_content import ScriptContentV1
from schemas.story_snapshots import (
    ScriptVersionSnapshot,
)


class SaveScriptRequest(StrictContract):
    content: ScriptContentV1

    @field_validator("content", mode="before")
    @classmethod
    def parse_json_content(cls, value: object) -> ScriptContentV1:
        if isinstance(value, ScriptContentV1):
            return value
        return ScriptContentV1.model_validate_json(json.dumps(value, ensure_ascii=False))


class ScriptVersionResponse(StrictContract):
    id: UUID
    episode_id: UUID
    version: int = Field(ge=1)
    parent_id: UUID | None
    source_revision_id: UUID
    schema_version: Literal["script-v1"] = "script-v1"
    content: ScriptContentV1
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    origin_task_id: UUID | None
    status: Literal["draft", "confirmed", "superseded"]
    resource_version: int = Field(ge=1)
    input_outdated: bool
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None


class ScriptVersionListResponse(StrictContract):
    items: tuple[ScriptVersionResponse, ...]


def script_response(value: ScriptVersionSnapshot) -> ScriptVersionResponse:
    return ScriptVersionResponse(
        id=value.id,
        episode_id=value.episode_id,
        version=value.version,
        parent_id=value.parent_id,
        source_revision_id=value.source_revision_id,
        content=value.content,
        content_hash=value.content_hash,
        origin_task_id=value.origin_task_id,
        status=value.status,
        resource_version=value.resource_version,
        input_outdated=value.input_outdated,
        created_at=value.created_at,
        updated_at=value.updated_at,
        confirmed_at=value.confirmed_at,
    )
