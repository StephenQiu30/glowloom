from __future__ import annotations

import json
from typing import Any
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from schemas.story_content import (
    ScriptContentV1,
    canonical_content_hash,
)
from schemas.story_snapshots import (
    ScriptVersionSnapshot,
)


def json_object(value: Any) -> dict[str, object]:
    if isinstance(value, str):
        value = json.loads(value)
    if not isinstance(value, dict):
        raise RuntimeError("expected a persisted JSON object")
    return {str(key): item for key, item in value.items()}


class ScriptVersionRepository:
    async def insert_generated(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        task_id: UUID,
        content: ScriptContentV1,
    ) -> ScriptVersionSnapshot:
        existing = await self._get_by_origin_task(connection, task_id)
        if existing is not None:
            if existing.content_hash != canonical_content_hash(content):
                raise ValueError("task output changed across registration attempts")
            return existing
        task = await connection.fetchrow(
            """
            SELECT t.episode_id,t.type,s.input_refs_json,s.schema_version,
                   s.model_profile_id,s.provider_id,s.model_id
            FROM production_tasks t JOIN submission_snapshots s ON s.id=t.snapshot_id
            WHERE t.id=$1 FOR UPDATE OF t
            """,
            task_id,
        )
        if task is None or task["type"] != "generate_script":
            raise LookupError("generate script task was not found")
        input_refs = json_object(task["input_refs_json"])
        source_id = UUID(str(input_refs["source_revision_id"]))
        source_exists = await connection.fetchval(
            "SELECT true FROM source_revisions WHERE id=$1 AND episode_id=$2",
            source_id,
            task["episode_id"],
        )
        if source_exists is not True:
            raise LookupError("task source revision was not found")
        version = await connection.fetchval(
            "SELECT coalesce(max(version),0)+1 FROM script_versions WHERE episode_id=$1",
            task["episode_id"],
        )
        script_id = new_id()
        content_hash = canonical_content_hash(content)
        row = await connection.fetchrow(
            """
            INSERT INTO script_versions(
                id,episode_id,version,source_revision_id,schema_version,content_json,
                content_hash,origin_task_id,model_profile_id,provider_id,model_id,prompt_version
            ) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,'script-prompt-v1')
            RETURNING *
            """,
            script_id,
            task["episode_id"],
            version,
            source_id,
            task["schema_version"],
            content.model_dump_json(),
            content_hash,
            task_id,
            task["model_profile_id"],
            task["provider_id"],
            task["model_id"],
        )
        await connection.execute(
            """
            INSERT INTO task_outputs(id,task_id,output_type,output_id,ordinal)
            VALUES($1,$2,'script_version',$3,0)
            """,
            new_id(),
            task_id,
            script_id,
        )
        if row is None:
            raise RuntimeError("inserted script version could not be read")
        return self._map(row)

    async def _get_by_origin_task(
        self, connection: asyncpg.Connection[asyncpg.Record], task_id: UUID
    ) -> ScriptVersionSnapshot | None:
        row = await connection.fetchrow(
            "SELECT * FROM script_versions WHERE origin_task_id=$1", task_id
        )
        return self._map(row) if row else None

    @staticmethod
    def _map(row: asyncpg.Record) -> ScriptVersionSnapshot:
        persisted_content = row["content_json"]
        content_json = (
            persisted_content
            if isinstance(persisted_content, str)
            else json.dumps(persisted_content, ensure_ascii=False)
        )
        return ScriptVersionSnapshot(
            id=row["id"],
            episode_id=row["episode_id"],
            version=row["version"],
            parent_id=row["parent_id"],
            source_revision_id=row["source_revision_id"],
            content=ScriptContentV1.model_validate_json(content_json),
            content_hash=row["content_hash"],
            origin_task_id=row["origin_task_id"],
            status=row["status"],
            resource_version=row["resource_version"],
            created_at=row["created_at"],
            updated_at=row["updated_at"],
            confirmed_at=row["confirmed_at"],
        )
