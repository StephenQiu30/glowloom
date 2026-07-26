from __future__ import annotations

import json
from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import field_validator

from schemas.common import StrictContract
from schemas.subtitle_versions import SubtitleVersionSnapshot
from schemas.subtitles import SubtitleContentV1


class SaveSubtitleRequest(StrictContract):
    content: SubtitleContentV1

    @field_validator("content", mode="before")
    @classmethod
    def parse_json_content(cls, value: object) -> SubtitleContentV1:
        if isinstance(value, SubtitleContentV1):
            return value
        return SubtitleContentV1.model_validate_json(
            json.dumps(value, ensure_ascii=False)
        )


class SubtitleVersionResponse(StrictContract):
    id: UUID
    episode_id: UUID
    version: int
    parent_id: UUID | None
    script_version_id: UUID
    shot_spec_version_id: UUID
    content: SubtitleContentV1
    content_hash: str
    status: Literal["draft", "confirmed", "superseded"]
    resource_version: int
    input_outdated: bool
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None


class SubtitleVersionListResponse(StrictContract):
    items: tuple[SubtitleVersionResponse, ...]


def subtitle_response(value: SubtitleVersionSnapshot) -> SubtitleVersionResponse:
    return SubtitleVersionResponse(
        id=value.id,
        episode_id=value.episode_id,
        version=value.version,
        parent_id=value.parent_id,
        script_version_id=value.script_version_id,
        shot_spec_version_id=value.shot_spec_version_id,
        content=value.content,
        content_hash=value.content_hash,
        status=value.status,
        resource_version=value.resource_version,
        input_outdated=value.input_outdated,
        created_at=value.created_at,
        updated_at=value.updated_at,
        confirmed_at=value.confirmed_at,
    )
