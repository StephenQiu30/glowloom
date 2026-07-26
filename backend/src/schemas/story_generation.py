from __future__ import annotations

from typing import Literal
from uuid import UUID

from pydantic import Field, model_validator

from schemas.common import StrictContract
from schemas.story_content import (
    CreativeAssetContentV1,
    ShotSpecCollectionV1,
)


class GeneratedCreativeAssetV1(StrictContract):
    version_id: UUID
    content: CreativeAssetContentV1


class StoryboardGenerationV1(StrictContract):
    schema_version: Literal["storyboard-generation-v1"] = "storyboard-generation-v1"
    assets: tuple[GeneratedCreativeAssetV1, ...] = Field(min_length=3, max_length=20)
    storyboard: ShotSpecCollectionV1

    @model_validator(mode="after")
    def validate_assets(self) -> StoryboardGenerationV1:
        version_ids = [asset.version_id for asset in self.assets]
        if len(version_ids) != len(set(version_ids)):
            raise ValueError("generated asset version ids must be unique")
        asset_ids = [asset.content.asset_id for asset in self.assets]
        if len(asset_ids) != len(set(asset_ids)):
            raise ValueError("generated asset ids must be unique")
        if {asset.content.asset_type for asset in self.assets} != {
            "character",
            "scene",
            "visual_style",
        }:
            raise ValueError("storyboard requires character, scene and visual style assets")
        if set(version_ids) != set(self.storyboard.asset_version_ids):
            raise ValueError("storyboard asset references must exactly match generated assets")
        return self
