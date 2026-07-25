from __future__ import annotations

import json
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from lanverse.modules.story_development.application.contracts.content_v1 import (
    CreativeAssetContentV1,
    ScriptContentV1,
    ShotSpecCollectionV1,
    canonical_content_hash,
)
from lanverse.modules.story_development.application.contracts.generation_v1 import (
    StoryboardGenerationV1,
)
from lanverse.modules.story_development.application.contracts.snapshots import (
    CreativeAssetVersionSnapshot,
    StoryboardGenerationSnapshot,
    StoryboardVersionSnapshot,
)
from lanverse.modules.story_development.infrastructure.scripts import json_object
from lanverse.modules.story_development.infrastructure.storyboard_rows import (
    map_asset,
    map_storyboard,
)
from lanverse.shared_kernel.ids import new_id


class StoryboardGenerationRepository:
    async def insert_generated(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        task_id: UUID,
        generated: StoryboardGenerationV1,
    ) -> StoryboardGenerationSnapshot:
        task = await connection.fetchrow(
            """
            SELECT t.episode_id,t.type,s.input_refs_json FROM production_tasks t
            JOIN submission_snapshots s ON s.id=t.snapshot_id
            WHERE t.id=$1 FOR UPDATE OF t
            """,
            task_id,
        )
        if task is None or task["type"] != "generate_storyboard":
            raise LookupError("generate storyboard task was not found")
        existing = await self._get_existing(connection, task_id)
        if existing is not None:
            if existing.storyboard.content_hash != canonical_content_hash(generated.storyboard):
                raise ValueError("task output changed across registration attempts")
            return existing
        script_id = UUID(str(json_object(task["input_refs_json"])["script_version_id"]))
        if generated.storyboard.script_version_id != script_id:
            raise ValueError("generated storyboard references the wrong script")
        script_json = await connection.fetchval(
            "SELECT content_json FROM script_versions WHERE id=$1 AND episode_id=$2",
            script_id,
            task["episode_id"],
        )
        if script_json is None:
            raise LookupError("task script version was not found")
        script = ScriptContentV1.model_validate_json(script_json)
        expected_lines = {
            line.speech_line_id for scene in script.scenes for line in scene.speech_lines
        }
        if set(generated.storyboard.speech_line_ids) != expected_lines:
            raise ValueError("storyboard speech references do not match the script")
        assets = tuple(
            [
                await self._insert_asset(
                    connection,
                    task["episode_id"],
                    script_id,
                    task_id,
                    item.version_id,
                    item.content,
                )
                for item in generated.assets
            ]
        )
        storyboard = await self._insert_storyboard(
            connection, task["episode_id"], task_id, generated.storyboard
        )
        for ordinal, asset in enumerate(assets):
            await self._insert_output(
                connection, task_id, "creative_asset_version", asset.id, ordinal
            )
        await self._insert_output(
            connection, task_id, "shot_spec_version", storyboard.id, 0
        )
        return StoryboardGenerationSnapshot(assets=assets, storyboard=storyboard)

    async def _insert_asset(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        episode_id: UUID,
        script_id: UUID,
        task_id: UUID,
        version_id: UUID,
        content: CreativeAssetContentV1,
    ) -> CreativeAssetVersionSnapshot:
        version = await connection.fetchval(
            "SELECT coalesce(max(version),0)+1 FROM creative_asset_versions WHERE asset_id=$1",
            content.asset_id,
        )
        row = await connection.fetchrow(
            """
            INSERT INTO creative_asset_versions(
                id,asset_id,episode_id,version,source_script_version_id,origin_task_id,
                asset_type,name,description,content_hash
            ) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *
            """,
            version_id,
            content.asset_id,
            episode_id,
            version,
            script_id,
            task_id,
            content.asset_type,
            content.name,
            content.description,
            canonical_content_hash(content),
        )
        if row is None:
            raise RuntimeError("inserted creative asset could not be read")
        return map_asset(row)

    async def _insert_storyboard(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        episode_id: UUID,
        task_id: UUID,
        content: ShotSpecCollectionV1,
    ) -> StoryboardVersionSnapshot:
        version = await connection.fetchval(
            "SELECT coalesce(max(version),0)+1 FROM shot_spec_versions WHERE episode_id=$1",
            episode_id,
        )
        row = await connection.fetchrow(
            """
            INSERT INTO shot_spec_versions(
                id,episode_id,version,script_version_id,asset_version_refs_json,
                shots_json,shot_count,total_duration_ticks,content_hash,origin_task_id
            ) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7,$8,$9,$10) RETURNING *
            """,
            new_id(),
            episode_id,
            version,
            content.script_version_id,
            json.dumps([str(item) for item in content.asset_version_ids]),
            json.dumps([shot.model_dump(mode="json") for shot in content.shots]),
            len(content.shots),
            content.total_duration_ticks,
            canonical_content_hash(content),
            task_id,
        )
        if row is None:
            raise RuntimeError("inserted storyboard could not be read")
        return map_storyboard(row)

    async def _get_existing(
        self, connection: asyncpg.Connection[asyncpg.Record], task_id: UUID
    ) -> StoryboardGenerationSnapshot | None:
        board = await connection.fetchrow(
            "SELECT * FROM shot_spec_versions WHERE origin_task_id=$1", task_id
        )
        if board is None:
            return None
        rows = await connection.fetch(
            "SELECT * FROM creative_asset_versions WHERE origin_task_id=$1 ORDER BY asset_type",
            task_id,
        )
        return StoryboardGenerationSnapshot(
            assets=tuple(map_asset(row) for row in rows),
            storyboard=map_storyboard(board),
        )

    @staticmethod
    async def _insert_output(
        connection: asyncpg.Connection[asyncpg.Record],
        task_id: UUID,
        output_type: str,
        output_id: UUID,
        ordinal: int,
    ) -> None:
        await connection.execute(
            "INSERT INTO task_outputs(id,task_id,output_type,output_id,ordinal) "
            "VALUES($1,$2,$3,$4,$5)",
            new_id(),
            task_id,
            output_type,
            output_id,
            ordinal,
        )
