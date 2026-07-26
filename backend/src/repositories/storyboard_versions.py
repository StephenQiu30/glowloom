from __future__ import annotations

import json
from dataclasses import replace
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from repositories.storyboard_rows import map_storyboard
from schemas.story_content import (
    ShotSpecCollectionV1,
    canonical_content_hash,
)
from schemas.story_snapshots import (
    StoryboardVersionSnapshot,
)


class StoryboardVersionRepository:
    SELECT = """
        SELECT bv.*,
               NOT EXISTS (
                   SELECT 1 FROM script_versions current_script
                   JOIN source_revisions current_source
                     ON current_source.episode_id=current_script.episode_id
                    AND current_source.id=current_script.source_revision_id
                    AND current_source.status='confirmed'
                   WHERE current_script.id=bv.script_version_id
                     AND current_script.episode_id=bv.episode_id
                     AND current_script.status='confirmed'
               ) input_outdated
        FROM shot_spec_versions bv
    """

    async def episode_id_for(
        self, connection: asyncpg.Connection[asyncpg.Record], version_id: UUID
    ) -> UUID | None:
        value = await connection.fetchval(
            "SELECT episode_id FROM shot_spec_versions WHERE id=$1", version_id
        )
        return value if isinstance(value, UUID) else None

    async def lock_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> bool:
        return (
            await connection.fetchval(
                "SELECT true FROM episodes WHERE id=$1 FOR UPDATE", episode_id
            )
            is True
        )

    async def get(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        version_id: UUID,
        *,
        for_update: bool = False,
    ) -> StoryboardVersionSnapshot | None:
        suffix = " WHERE bv.id=$1"
        if for_update:
            suffix += " FOR UPDATE OF bv"
        row = await connection.fetchrow(self.SELECT + suffix, version_id)
        return self._map(row) if row else None

    async def list_for_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> tuple[StoryboardVersionSnapshot, ...]:
        rows = await connection.fetch(
            self.SELECT + " WHERE bv.episode_id=$1 ORDER BY bv.version DESC", episode_id
        )
        return tuple(self._map(row) for row in rows)

    async def get_current(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> StoryboardVersionSnapshot | None:
        row = await connection.fetchrow(
            self.SELECT + " WHERE bv.episode_id=$1 AND bv.status='confirmed'", episode_id
        )
        return self._map(row) if row else None

    async def update_draft(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        version: StoryboardVersionSnapshot,
        content: ShotSpecCollectionV1,
    ) -> StoryboardVersionSnapshot:
        row = await connection.fetchrow(
            """
            UPDATE shot_spec_versions
            SET asset_version_refs_json=$3::jsonb,shots_json=$4::jsonb,shot_count=$5,
                total_duration_ticks=$6,content_hash=$7,
                resource_version=resource_version+1,updated_at=now()
            WHERE id=$1 AND resource_version=$2 AND status='draft' RETURNING *
            """,
            version.id,
            version.resource_version,
            json.dumps([str(item) for item in content.asset_version_ids]),
            json.dumps([item.model_dump(mode="json") for item in content.shots]),
            len(content.shots),
            content.total_duration_ticks,
            canonical_content_hash(content),
        )
        if row is None:
            raise RuntimeError("storyboard update compare-and-set failed")
        return map_storyboard(row)

    async def derive(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        parent: StoryboardVersionSnapshot,
        content: ShotSpecCollectionV1,
    ) -> StoryboardVersionSnapshot:
        version = await connection.fetchval(
            "SELECT coalesce(max(version),0)+1 FROM shot_spec_versions WHERE episode_id=$1",
            parent.episode_id,
        )
        row = await connection.fetchrow(
            """
            INSERT INTO shot_spec_versions(
                id,episode_id,version,parent_id,script_version_id,asset_version_refs_json,
                shots_json,shot_count,total_duration_ticks,content_hash
            ) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8,$9,$10) RETURNING *
            """,
            new_id(), parent.episode_id, version, parent.id, content.script_version_id,
            json.dumps([str(item) for item in content.asset_version_ids]),
            json.dumps([item.model_dump(mode="json") for item in content.shots]),
            len(content.shots), content.total_duration_ticks, canonical_content_hash(content),
        )
        if row is None:
            raise RuntimeError("derived storyboard could not be read")
        return map_storyboard(row)

    async def confirm(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        version: StoryboardVersionSnapshot,
    ) -> StoryboardVersionSnapshot:
        await connection.execute(
            """
            UPDATE shot_spec_versions SET status='superseded',
                resource_version=resource_version+1,updated_at=now()
            WHERE episode_id=$1 AND status='confirmed' AND id<>$2
            """,
            version.episode_id, version.id,
        )
        row = await connection.fetchrow(
            """
            UPDATE shot_spec_versions SET status='confirmed',
                resource_version=resource_version+1,updated_at=now(),confirmed_at=now()
            WHERE id=$1 AND status='draft' RETURNING *
            """,
            version.id,
        )
        if row is None:
            raise RuntimeError("storyboard confirmation compare-and-set failed")
        return map_storyboard(row)

    @staticmethod
    def _map(row: asyncpg.Record) -> StoryboardVersionSnapshot:
        value = map_storyboard(row)
        return replace(value, input_outdated=row["input_outdated"] is True)
