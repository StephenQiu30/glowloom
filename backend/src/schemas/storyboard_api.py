from __future__ import annotations

import json
from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import Field, field_validator

from schemas.common import StrictContract
from schemas.story_content import (
    CreativeAssetContentV1,
    ShotSpecCollectionV1,
)
from schemas.story_snapshots import (
    CreativeAssetVersionSnapshot,
    StoryboardGenerationSnapshot,
    StoryboardVersionSnapshot,
)


class SaveCreativeAssetRequest(StrictContract):
    content: CreativeAssetContentV1

    @field_validator("content", mode="before")
    @classmethod
    def parse_json_content(cls, value: object) -> CreativeAssetContentV1:
        if isinstance(value, CreativeAssetContentV1):
            return value
        return CreativeAssetContentV1.model_validate_json(
            json.dumps(value, ensure_ascii=False)
        )


class SaveStoryboardRequest(StrictContract):
    content: ShotSpecCollectionV1

    @field_validator("content", mode="before")
    @classmethod
    def parse_json_content(cls, value: object) -> ShotSpecCollectionV1:
        if isinstance(value, ShotSpecCollectionV1):
            return value
        return ShotSpecCollectionV1.model_validate_json(
            json.dumps(value, ensure_ascii=False)
        )


class CreativeAssetVersionResponse(StrictContract):
    id: UUID
    asset_id: UUID
    episode_id: UUID
    version: int = Field(ge=1)
    parent_id: UUID | None
    source_script_version_id: UUID
    content: CreativeAssetContentV1
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    origin_task_id: UUID | None
    status: Literal["draft", "confirmed", "superseded"]
    resource_version: int = Field(ge=1)
    input_outdated: bool
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None


class CreativeAssetListResponse(StrictContract):
    items: tuple[CreativeAssetVersionResponse, ...]


class StoryboardVersionResponse(StrictContract):
    id: UUID
    episode_id: UUID
    version: int = Field(ge=1)
    parent_id: UUID | None
    content: ShotSpecCollectionV1
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    origin_task_id: UUID | None
    status: Literal["draft", "confirmed", "superseded"]
    resource_version: int = Field(ge=1)
    input_outdated: bool
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None


class StoryboardVersionListResponse(StrictContract):
    items: tuple[StoryboardVersionResponse, ...]


class StoryboardGenerationResponse(StrictContract):
    assets: tuple[CreativeAssetVersionResponse, ...]
    storyboard: StoryboardVersionResponse


def asset_response(value: CreativeAssetVersionSnapshot) -> CreativeAssetVersionResponse:
    return CreativeAssetVersionResponse(
        id=value.id,
        asset_id=value.asset_id,
        episode_id=value.episode_id,
        version=value.version,
        parent_id=value.parent_id,
        source_script_version_id=value.source_script_version_id,
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


def storyboard_response(value: StoryboardVersionSnapshot) -> StoryboardVersionResponse:
    return StoryboardVersionResponse(
        id=value.id,
        episode_id=value.episode_id,
        version=value.version,
        parent_id=value.parent_id,
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


def generation_response(value: StoryboardGenerationSnapshot) -> StoryboardGenerationResponse:
    return StoryboardGenerationResponse(
        assets=tuple(asset_response(item) for item in value.assets),
        storyboard=storyboard_response(value.storyboard),
    )
