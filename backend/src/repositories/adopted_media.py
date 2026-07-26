from __future__ import annotations

from dataclasses import dataclass
from typing import cast
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from schemas.media_registration import UsageType


@dataclass(frozen=True, slots=True)
class AdoptedMediaRow:
    adoption_id: UUID
    candidate_id: UUID
    media_version_id: UUID
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    sha256: str

    def frozen_ref(self) -> dict[str, object]:
        return {
            "usage_type": self.usage_type,
            "usage_id": str(self.usage_id),
            "input_version_id": str(self.input_version_id),
            "input_hash": self.input_hash,
            "adoption_id": str(self.adoption_id),
            "candidate_id": str(self.candidate_id),
            "media_version_id": str(self.media_version_id),
            "sha256": self.sha256,
        }


class AdoptedMediaRepository:
    async def find_active_media(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        episode_id: UUID,
        usage_type: UsageType,
        usage_id: UUID,
        input_version_id: UUID,
        input_hash: str,
    ) -> AdoptedMediaRow | None:
        row = await connection.fetchrow(
            """
            SELECT adoption.id adoption_id,adoption.candidate_id,
                   candidate.media_version_id,adoption.usage_type,adoption.usage_id,
                   adoption.input_version_id,adoption.input_hash,version.sha256
            FROM adoptions adoption
            JOIN generation_candidates candidate ON candidate.id=adoption.candidate_id
            JOIN media_versions version ON version.id=candidate.media_version_id
            WHERE adoption.episode_id=$1 AND adoption.usage_type=$2
              AND adoption.usage_id=$3 AND adoption.input_version_id=$4
              AND adoption.input_hash=$5 AND adoption.status='active'
              AND candidate.status='ready' AND version.status='ready'
            FOR UPDATE OF adoption
            """,
            episode_id,
            usage_type,
            usage_id,
            input_version_id,
            input_hash,
        )
        if row is None:
            return None
        return AdoptedMediaRow(
            adoption_id=row["adoption_id"],
            candidate_id=row["candidate_id"],
            media_version_id=row["media_version_id"],
            usage_type=cast(UsageType, row["usage_type"]),
            usage_id=row["usage_id"],
            input_version_id=row["input_version_id"],
            input_hash=row["input_hash"],
            sha256=row["sha256"],
        )
