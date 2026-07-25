from __future__ import annotations

import json

import asyncpg  # type: ignore[import-untyped]

from lanverse.modules.story_development.application.contracts.content_v1 import (
    CreativeAssetContentV1,
    ShotSpecCollectionV1,
)
from lanverse.modules.story_development.application.contracts.snapshots import (
    CreativeAssetVersionSnapshot,
    StoryboardVersionSnapshot,
)


def map_asset(row: asyncpg.Record) -> CreativeAssetVersionSnapshot:
    content = CreativeAssetContentV1(
        asset_id=row["asset_id"],
        asset_type=row["asset_type"],
        name=row["name"],
        description=row["description"],
    )
    return CreativeAssetVersionSnapshot(
        id=row["id"],
        asset_id=row["asset_id"],
        episode_id=row["episode_id"],
        version=row["version"],
        parent_id=row["parent_id"],
        source_script_version_id=row["source_script_version_id"],
        content=content,
        content_hash=row["content_hash"],
        origin_task_id=row["origin_task_id"],
        status=row["status"],
        resource_version=row["resource_version"],
        created_at=row["created_at"],
        updated_at=row["updated_at"],
        confirmed_at=row["confirmed_at"],
    )


def map_storyboard(row: asyncpg.Record) -> StoryboardVersionSnapshot:
    assets = json.loads(row["asset_version_refs_json"])
    shots = json.loads(row["shots_json"])
    speech = [item for shot in shots for item in shot["speech_line_ids"]]
    payload = {
        "script_version_id": str(row["script_version_id"]),
        "asset_version_ids": assets,
        "speech_line_ids": speech,
        "shots": shots,
    }
    content = ShotSpecCollectionV1.model_validate_json(json.dumps(payload))
    return StoryboardVersionSnapshot(
        id=row["id"],
        episode_id=row["episode_id"],
        version=row["version"],
        parent_id=row["parent_id"],
        content=content,
        content_hash=row["content_hash"],
        origin_task_id=row["origin_task_id"],
        status=row["status"],
        resource_version=row["resource_version"],
        created_at=row["created_at"],
        updated_at=row["updated_at"],
        confirmed_at=row["confirmed_at"],
    )
