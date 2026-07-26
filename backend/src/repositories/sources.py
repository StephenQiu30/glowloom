from __future__ import annotations

from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from domain.projects import SourceTextV1
from schemas.projects import SourceRevisionSnapshot


class SourceRevisionRepository:
    async def episode_id_for_revision(
        self, connection: asyncpg.Connection[asyncpg.Record], revision_id: UUID
    ) -> UUID | None:
        value = await connection.fetchval(
            "SELECT episode_id FROM source_revisions WHERE id = $1", revision_id
        )
        return value if isinstance(value, UUID) else None

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

    async def get_for_update(
        self, connection: asyncpg.Connection[asyncpg.Record], revision_id: UUID
    ) -> SourceRevisionSnapshot | None:
        row = await connection.fetchrow(
            "SELECT * FROM source_revisions WHERE id = $1 FOR UPDATE", revision_id
        )
        return self._map(row) if row else None

    async def confirm(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        revision: SourceRevisionSnapshot,
    ) -> SourceRevisionSnapshot:
        await connection.execute(
            """
            UPDATE source_revisions
            SET status = 'superseded', resource_version = resource_version + 1,
                updated_at = now()
            WHERE episode_id = $1 AND status = 'confirmed' AND id <> $2
            """,
            revision.episode_id,
            revision.id,
        )
        row = await connection.fetchrow(
            """
            UPDATE source_revisions
            SET status = 'confirmed', resource_version = resource_version + 1,
                updated_at = now(), confirmed_at = now()
            WHERE id = $1 AND status = 'draft' AND resource_version = $2
            RETURNING *
            """,
            revision.id,
            revision.resource_version,
        )
        if row is None:
            raise RuntimeError("source confirmation compare-and-set failed")
        return self._map(row)

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

    async def get_current(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> SourceRevisionSnapshot | None:
        row = await connection.fetchrow(
            """
            SELECT * FROM source_revisions
            WHERE episode_id = $1 AND status = 'confirmed'
            """,
            episode_id,
        )
        return self._map(row) if row else None

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
