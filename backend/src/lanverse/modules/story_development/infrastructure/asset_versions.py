from __future__ import annotations

from dataclasses import replace
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from lanverse.modules.story_development.application.contracts.content_v1 import (
    CreativeAssetContentV1,
    canonical_content_hash,
)
from lanverse.modules.story_development.application.contracts.snapshots import (
    CreativeAssetVersionSnapshot,
)
from lanverse.modules.story_development.infrastructure.storyboard_rows import map_asset
from lanverse.shared_kernel.ids import new_id


class CreativeAssetVersionRepository:
    SELECT = """
        SELECT av.*,
               NOT EXISTS (
                   SELECT 1 FROM script_versions current_script
                   JOIN source_revisions current_source
                     ON current_source.episode_id=current_script.episode_id
                    AND current_source.id=current_script.source_revision_id
                    AND current_source.status='confirmed'
                   WHERE current_script.id=av.source_script_version_id
                     AND current_script.episode_id=av.episode_id
                     AND current_script.status='confirmed'
               ) input_outdated
        FROM creative_asset_versions av
    """

    async def episode_id_for(
        self, connection: asyncpg.Connection[asyncpg.Record], version_id: UUID
    ) -> UUID | None:
        value = await connection.fetchval(
            "SELECT episode_id FROM creative_asset_versions WHERE id=$1", version_id
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
    ) -> CreativeAssetVersionSnapshot | None:
        suffix = " WHERE av.id=$1"
        if for_update:
            suffix += " FOR UPDATE OF av"
        row = await connection.fetchrow(self.SELECT + suffix, version_id)
        return self._map(row) if row else None

    async def get_many(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        version_ids: tuple[UUID, ...],
        *,
        for_update: bool = False,
    ) -> tuple[CreativeAssetVersionSnapshot, ...]:
        suffix = " WHERE av.id=ANY($1::uuid[]) ORDER BY av.id"
        if for_update:
            suffix += " FOR UPDATE OF av"
        rows = await connection.fetch(self.SELECT + suffix, list(version_ids))
        return tuple(self._map(row) for row in rows)

    async def list_for_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> tuple[CreativeAssetVersionSnapshot, ...]:
        rows = await connection.fetch(
            self.SELECT + " WHERE av.episode_id=$1 ORDER BY av.asset_type,av.version DESC",
            episode_id,
        )
        return tuple(self._map(row) for row in rows)

    async def update_draft(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        version: CreativeAssetVersionSnapshot,
        content: CreativeAssetContentV1,
    ) -> CreativeAssetVersionSnapshot:
        row = await connection.fetchrow(
            """
            UPDATE creative_asset_versions
            SET name=$3,description=$4,content_hash=$5,
                resource_version=resource_version+1,updated_at=now()
            WHERE id=$1 AND resource_version=$2 AND status='draft' RETURNING *
            """,
            version.id,
            version.resource_version,
            content.name,
            content.description,
            canonical_content_hash(content),
        )
        if row is None:
            raise RuntimeError("asset update compare-and-set failed")
        return map_asset(row)

    async def derive_many(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        parents: tuple[CreativeAssetVersionSnapshot, ...],
    ) -> tuple[CreativeAssetVersionSnapshot, ...]:
        derived = []
        for parent in sorted(parents, key=lambda item: item.id):
            version = await connection.fetchval(
                "SELECT coalesce(max(version),0)+1 FROM creative_asset_versions "
                "WHERE asset_id=$1",
                parent.asset_id,
            )
            row = await connection.fetchrow(
                """
                INSERT INTO creative_asset_versions(
                    id,asset_id,episode_id,version,parent_id,source_script_version_id,
                    asset_type,name,description,content_hash
                ) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *
                """,
                new_id(), parent.asset_id, parent.episode_id, version, parent.id,
                parent.source_script_version_id, parent.asset_type, parent.name,
                parent.description, parent.content_hash,
            )
            if row is None:
                raise RuntimeError("derived asset could not be read")
            derived.append(map_asset(row))
        return tuple(derived)

    async def confirm_many(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        versions: tuple[CreativeAssetVersionSnapshot, ...],
    ) -> tuple[CreativeAssetVersionSnapshot, ...]:
        confirmed = []
        for version in sorted(versions, key=lambda item: item.id):
            await connection.execute(
                """
                UPDATE creative_asset_versions SET status='superseded',
                    resource_version=resource_version+1,updated_at=now()
                WHERE asset_id=$1 AND status='confirmed' AND id<>$2
                """,
                version.asset_id, version.id,
            )
            row = await connection.fetchrow(
                """
                UPDATE creative_asset_versions SET status='confirmed',
                    resource_version=resource_version+1,updated_at=now(),confirmed_at=now()
                WHERE id=$1 AND status='draft' RETURNING *
                """,
                version.id,
            )
            if row is None:
                raise RuntimeError("asset confirmation compare-and-set failed")
            confirmed.append(map_asset(row))
        return tuple(confirmed)

    @staticmethod
    def _map(row: asyncpg.Record) -> CreativeAssetVersionSnapshot:
        value = map_asset(row)
        return replace(value, input_outdated=row["input_outdated"] is True)
