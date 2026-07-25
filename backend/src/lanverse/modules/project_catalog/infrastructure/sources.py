from __future__ import annotations

from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from lanverse.modules.project_catalog.application.contracts import SourceRevisionSnapshot
from lanverse.modules.project_catalog.domain.values import SourceTextV1


class SourceRevisionRepository:
    async def lock_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> bool:
        return (
            await connection.fetchval(
                "SELECT true FROM episodes WHERE id = $1 FOR UPDATE", episode_id
            )
            is True
        )

    async def insert(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        revision_id: UUID,
        episode_id: UUID,
        parent_id: UUID | None,
        text: SourceTextV1,
        rights_basis: str,
    ) -> SourceRevisionSnapshot:
        version = await connection.fetchval(
            "SELECT coalesce(max(version), 0) + 1 FROM source_revisions WHERE episode_id = $1",
            episode_id,
        )
        row = await connection.fetchrow(
            """
            INSERT INTO source_revisions(
                id, episode_id, version, parent_id, content, normalization_version,
                codepoint_count, sha256, rights_basis, rights_declared_at
            ) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
            RETURNING *
            """,
            revision_id,
            episode_id,
            version,
            parent_id,
            text.content,
            text.normalization_version,
            text.codepoint_count,
            text.sha256,
            rights_basis,
        )
        if row is None:
            raise RuntimeError("inserted source revision could not be read")
        return self._map(row)

    async def get(
        self, connection: asyncpg.Connection[asyncpg.Record], revision_id: UUID
    ) -> SourceRevisionSnapshot | None:
        row = await connection.fetchrow(
            "SELECT * FROM source_revisions WHERE id = $1", revision_id
        )
        return self._map(row) if row else None

    async def list_for_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> tuple[SourceRevisionSnapshot, ...]:
        rows = await connection.fetch(
            """
            SELECT * FROM source_revisions
            WHERE episode_id = $1 ORDER BY version DESC
            """,
            episode_id,
        )
        return tuple(self._map(row) for row in rows)

    @staticmethod
    def _map(row: asyncpg.Record) -> SourceRevisionSnapshot:
        return SourceRevisionSnapshot(
            id=row["id"],
            episode_id=row["episode_id"],
            version=row["version"],
            parent_id=row["parent_id"],
            content=row["content"],
            normalization_version=row["normalization_version"],
            codepoint_count=row["codepoint_count"],
            sha256=row["sha256"],
            rights_basis=row["rights_basis"],
            rights_declared_at=row["rights_declared_at"],
            status=row["status"],
            resource_version=row["resource_version"],
            created_at=row["created_at"],
            updated_at=row["updated_at"],
            confirmed_at=row["confirmed_at"],
        )
