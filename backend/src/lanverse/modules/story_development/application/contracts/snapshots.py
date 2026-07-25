from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

from lanverse.modules.story_development.application.contracts.content_v1 import (
    CreativeAssetContentV1,
    ScriptContentV1,
    ShotSpecCollectionV1,
)


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


@dataclass(frozen=True, slots=True)
class CreativeAssetVersionSnapshot:
    id: UUID
    asset_id: UUID
    episode_id: UUID
    version: int
    parent_id: UUID | None
    source_script_version_id: UUID
    content: CreativeAssetContentV1
    content_hash: str
    origin_task_id: UUID | None
    status: str
    resource_version: int
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None
    input_outdated: bool = False

    @property
    def asset_type(self) -> str:
        return self.content.asset_type

    @property
    def name(self) -> str:
        return self.content.name

    @property
    def description(self) -> str:
        return self.content.description


@dataclass(frozen=True, slots=True)
class StoryboardVersionSnapshot:
    id: UUID
    episode_id: UUID
    version: int
    parent_id: UUID | None
    content: ShotSpecCollectionV1
    content_hash: str
    origin_task_id: UUID | None
    status: str
    resource_version: int
    created_at: datetime
    updated_at: datetime
    confirmed_at: datetime | None
    input_outdated: bool = False


@dataclass(frozen=True, slots=True)
class StoryboardGenerationSnapshot:
    assets: tuple[CreativeAssetVersionSnapshot, ...]
    storyboard: StoryboardVersionSnapshot
