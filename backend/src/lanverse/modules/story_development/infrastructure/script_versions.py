from __future__ import annotations

from dataclasses import replace
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from lanverse.modules.story_development.application.contracts.content_v1 import (
    ScriptContentV1,
    canonical_content_hash,
)
from lanverse.modules.story_development.application.contracts.snapshots import (
    ScriptVersionSnapshot,
)
from lanverse.modules.story_development.infrastructure.scripts import ScriptVersionRepository
from lanverse.shared_kernel.ids import new_id


class ScriptVersioningRepository(ScriptVersionRepository):
    SELECT = """
        SELECT sv.*,
               (SELECT id FROM source_revisions
                WHERE episode_id=sv.episode_id AND status='confirmed') current_source_id
        FROM script_versions sv
    """

    async def episode_id_for(
        self, connection: asyncpg.Connection[asyncpg.Record], version_id: UUID
    ) -> UUID | None:
        value = await connection.fetchval(
            "SELECT episode_id FROM script_versions WHERE id=$1", version_id
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
    ) -> ScriptVersionSnapshot | None:
        suffix = " WHERE sv.id=$1"
        if for_update:
            suffix += " FOR UPDATE OF sv"
        row = await connection.fetchrow(self.SELECT + suffix, version_id)
        return self._map_with_outdated(row) if row else None

    async def list_for_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> tuple[ScriptVersionSnapshot, ...]:
        rows = await connection.fetch(
            self.SELECT + " WHERE sv.episode_id=$1 ORDER BY sv.version DESC", episode_id
        )
        return tuple(self._map_with_outdated(row) for row in rows)

    async def get_current(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> ScriptVersionSnapshot | None:
        row = await connection.fetchrow(
            self.SELECT + " WHERE sv.episode_id=$1 AND sv.status='confirmed'", episode_id
        )
        return self._map_with_outdated(row) if row else None

    async def update_draft(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        version: ScriptVersionSnapshot,
        content: ScriptContentV1,
    ) -> ScriptVersionSnapshot:
        row = await connection.fetchrow(
            """
            UPDATE script_versions
            SET content_json=$3::jsonb,content_hash=$4,resource_version=resource_version+1,
                updated_at=now()
            WHERE id=$1 AND resource_version=$2 AND status='draft' RETURNING *
            """,
            version.id,
            version.resource_version,
            content.model_dump_json(),
            canonical_content_hash(content),
        )
        if row is None:
            raise RuntimeError("script update compare-and-set failed")
        return self._map(row)

    async def confirm(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        version: ScriptVersionSnapshot,
    ) -> ScriptVersionSnapshot:
        await connection.execute(
            """
            UPDATE script_versions
            SET status='superseded',resource_version=resource_version+1,updated_at=now()
            WHERE episode_id=$1 AND status='confirmed' AND id<>$2
            """,
            version.episode_id,
            version.id,
        )
        row = await connection.fetchrow(
            """
            UPDATE script_versions
            SET status='confirmed',resource_version=resource_version+1,
                updated_at=now(),confirmed_at=now()
            WHERE id=$1 AND resource_version=$2 AND status='draft' RETURNING *
            """,
            version.id,
            version.resource_version,
        )
        if row is None:
            raise RuntimeError("script confirmation compare-and-set failed")
        return self._map(row)

    async def derive(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        parent: ScriptVersionSnapshot,
    ) -> ScriptVersionSnapshot:
        version = await connection.fetchval(
            "SELECT coalesce(max(version),0)+1 FROM script_versions WHERE episode_id=$1",
            parent.episode_id,
        )
        row = await connection.fetchrow(
            """
            INSERT INTO script_versions(
                id,episode_id,version,parent_id,source_revision_id,schema_version,
                content_json,content_hash
            ) VALUES($1,$2,$3,$4,$5,'script-v1',$6::jsonb,$7) RETURNING *
            """,
            new_id(),
            parent.episode_id,
            version,
            parent.id,
            parent.source_revision_id,
            parent.content.model_dump_json(),
            parent.content_hash,
        )
        if row is None:
            raise RuntimeError("derived script could not be read")
        return self._map(row)

    def _map_with_outdated(self, row: asyncpg.Record) -> ScriptVersionSnapshot:
        value = self._map(row)
        return replace(value, input_outdated=row["current_source_id"] != value.source_revision_id)
