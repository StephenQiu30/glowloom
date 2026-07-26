from __future__ import annotations

from dataclasses import dataclass
from typing import cast
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from schemas.adoptions import AdoptionSnapshot, AdoptionStatus
from schemas.media_registration import UsageType


@dataclass(frozen=True, slots=True)
class CandidateForAdoptionRow:
    id: UUID
    episode_id: UUID
    output_slot: str
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    candidate_status: str
    media_status: str


class AdoptionRepository:
    async def candidate_for_adoption(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        candidate_id: UUID,
    ) -> CandidateForAdoptionRow | None:
        row = await connection.fetchrow(
            """
            SELECT candidate.id,candidate.episode_id,candidate.output_slot,
                   candidate.usage_type,candidate.usage_id,candidate.input_version_id,
                   candidate.input_hash,candidate.status candidate_status,
                   version.status media_status
            FROM generation_candidates candidate
            JOIN media_versions version ON version.id=candidate.media_version_id
            WHERE candidate.id=$1
            """,
            candidate_id,
        )
        if row is None:
            return None
        return CandidateForAdoptionRow(
            id=row["id"],
            episode_id=row["episode_id"],
            output_slot=row["output_slot"],
            usage_type=cast(UsageType, row["usage_type"]),
            usage_id=row["usage_id"],
            input_version_id=row["input_version_id"],
            input_hash=row["input_hash"],
            candidate_status=row["candidate_status"],
            media_status=row["media_status"],
        )

    async def lock_episode(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        episode_id: UUID,
    ) -> bool:
        value = await connection.fetchval(
            "SELECT true FROM episodes WHERE id=$1 FOR UPDATE", episode_id
        )
        return value is True

    async def get(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        adoption_id: UUID,
    ) -> AdoptionSnapshot | None:
        row = await connection.fetchrow("SELECT * FROM adoptions WHERE id=$1", adoption_id)
        return self._adoption(row) if row else None

    async def adopt(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        candidate: CandidateForAdoptionRow,
    ) -> AdoptionSnapshot:
        current = await connection.fetchrow(
            """
            SELECT * FROM adoptions
            WHERE usage_type=$1 AND usage_id=$2 AND input_version_id=$3
              AND input_hash=$4 AND status='active'
            FOR UPDATE
            """,
            candidate.usage_type,
            candidate.usage_id,
            candidate.input_version_id,
            candidate.input_hash,
        )
        if current is not None and current["candidate_id"] == candidate.id:
            return self._adoption(current)
        version = await connection.fetchval(
            """
            SELECT coalesce(max(version),0)+1 FROM adoptions
            WHERE usage_type=$1 AND usage_id=$2 AND input_version_id=$3 AND input_hash=$4
            """,
            candidate.usage_type,
            candidate.usage_id,
            candidate.input_version_id,
            candidate.input_hash,
        )
        supersedes_id = current["id"] if current is not None else None
        if current is not None:
            await connection.execute(
                """
                UPDATE adoptions SET status='superseded',superseded_at=now()
                WHERE id=$1 AND status='active'
                """,
                supersedes_id,
            )
        row = await connection.fetchrow(
            """
            INSERT INTO adoptions(
                id,episode_id,usage_type,usage_id,input_version_id,input_hash,
                version,candidate_id,supersedes_id,status
            ) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'active') RETURNING *
            """,
            new_id(),
            candidate.episode_id,
            candidate.usage_type,
            candidate.usage_id,
            candidate.input_version_id,
            candidate.input_hash,
            version,
            candidate.id,
            supersedes_id,
        )
        if row is None:
            raise RuntimeError("created adoption could not be read")
        return self._adoption(row)

    @staticmethod
    def _adoption(row: asyncpg.Record) -> AdoptionSnapshot:
        return AdoptionSnapshot(
            id=row["id"],
            episode_id=row["episode_id"],
            usage_type=cast(UsageType, row["usage_type"]),
            usage_id=row["usage_id"],
            input_version_id=row["input_version_id"],
            input_hash=row["input_hash"],
            version=row["version"],
            candidate_id=row["candidate_id"],
            supersedes_id=row["supersedes_id"],
            status=cast(AdoptionStatus, row["status"]),
            created_at=row["created_at"],
            superseded_at=row["superseded_at"],
        )
