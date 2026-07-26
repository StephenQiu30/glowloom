from __future__ import annotations

import json
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from repositories.subtitle_rows import map_subtitle, with_projection
from schemas.subtitle_versions import SubtitleVersionSnapshot
from schemas.subtitles import (
    SubtitleContentV1,
    SubtitleInputRefsV1,
    subtitle_content_hash,
)


class SubtitleRepository:
    SELECT = """
        SELECT subtitle.*,
               (
                   NOT EXISTS (
                       SELECT 1 FROM script_versions script
                       WHERE script.id=subtitle.script_version_id
                         AND script.episode_id=subtitle.episode_id
                         AND script.status='confirmed'
                   )
                   OR NOT EXISTS (
                       SELECT 1 FROM shot_spec_versions board
                       WHERE board.id=subtitle.shot_spec_version_id
                         AND board.episode_id=subtitle.episode_id
                         AND board.status='confirmed'
                   )
                   OR EXISTS (
                       SELECT 1
                       FROM jsonb_array_elements(
                           subtitle.input_refs_json->'tts_adoptions'
                       ) reference
                       LEFT JOIN adoptions adoption
                         ON adoption.id=(reference->>'adoption_id')::uuid
                        AND adoption.status='active'
                       LEFT JOIN generation_candidates candidate
                         ON candidate.id=adoption.candidate_id
                        AND candidate.status='ready'
                       LEFT JOIN media_versions media
                         ON media.id=candidate.media_version_id
                        AND media.status='ready'
                       WHERE adoption.id IS NULL OR media.id IS NULL
                   )
               ) input_outdated
        FROM subtitle_versions subtitle
    """

    async def lock_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> bool:
        return (
            await connection.fetchval(
                "SELECT true FROM episodes WHERE id=$1 FOR UPDATE", episode_id
            )
            is True
        )

    async def episode_id_for(
        self, connection: asyncpg.Connection[asyncpg.Record], version_id: UUID
    ) -> UUID | None:
        value = await connection.fetchval(
            "SELECT episode_id FROM subtitle_versions WHERE id=$1", version_id
        )
        return value if isinstance(value, UUID) else None

    async def get(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        version_id: UUID,
        *,
        for_update: bool = False,
    ) -> SubtitleVersionSnapshot | None:
        suffix = " WHERE subtitle.id=$1"
        if for_update:
            suffix += " FOR UPDATE OF subtitle"
        row = await connection.fetchrow(self.SELECT + suffix, version_id)
        return map_subtitle(row) if row else None

    async def list_for_episode(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> tuple[SubtitleVersionSnapshot, ...]:
        rows = await connection.fetch(
            self.SELECT
            + " WHERE subtitle.episode_id=$1 ORDER BY subtitle.version DESC",
            episode_id,
        )
        return tuple(map_subtitle(row) for row in rows)

    async def get_current(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> SubtitleVersionSnapshot | None:
        row = await connection.fetchrow(
            self.SELECT
            + " WHERE subtitle.episode_id=$1 AND subtitle.status='confirmed'",
            episode_id,
        )
        return map_subtitle(row) if row else None

    async def insert(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        episode_id: UUID,
        parent_id: UUID | None,
        input_refs: SubtitleInputRefsV1,
        content: SubtitleContentV1,
    ) -> SubtitleVersionSnapshot:
        version = await connection.fetchval(
            "SELECT coalesce(max(version),0)+1 FROM subtitle_versions WHERE episode_id=$1",
            episode_id,
        )
        row = await connection.fetchrow(
            """
            INSERT INTO subtitle_versions(
                id,episode_id,version,parent_id,script_version_id,shot_spec_version_id,
                input_refs_json,language,cues_json,cue_count,content_hash
            ) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9::jsonb,$10,$11)
            RETURNING *
            """,
            new_id(),
            episode_id,
            version,
            parent_id,
            input_refs.script_version_id,
            input_refs.shot_spec_version_id,
            input_refs.model_dump_json(),
            content.language,
            json.dumps([cue.model_dump(mode="json") for cue in content.cues]),
            len(content.cues),
            subtitle_content_hash(content),
        )
        if row is None:
            raise RuntimeError("created subtitle version could not be read")
        return map_subtitle(with_projection(row, input_outdated=False))

    async def update_draft(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        current: SubtitleVersionSnapshot,
        content: SubtitleContentV1,
    ) -> SubtitleVersionSnapshot:
        row = await connection.fetchrow(
            """
            UPDATE subtitle_versions SET language=$3,cues_json=$4::jsonb,cue_count=$5,
                content_hash=$6,resource_version=resource_version+1,updated_at=now()
            WHERE id=$1 AND resource_version=$2 AND status='draft' RETURNING *
            """,
            current.id,
            current.resource_version,
            content.language,
            json.dumps([cue.model_dump(mode="json") for cue in content.cues]),
            len(content.cues),
            subtitle_content_hash(content),
        )
        if row is None:
            raise RuntimeError("subtitle update compare-and-set failed")
        return map_subtitle(
            with_projection(row, input_outdated=current.input_outdated)
        )

    async def confirm(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        current: SubtitleVersionSnapshot,
    ) -> SubtitleVersionSnapshot:
        await connection.execute(
            """
            UPDATE subtitle_versions SET status='superseded',
                resource_version=resource_version+1,updated_at=now()
            WHERE episode_id=$1 AND status='confirmed' AND id<>$2
            """,
            current.episode_id,
            current.id,
        )
        row = await connection.fetchrow(
            """
            UPDATE subtitle_versions SET status='confirmed',
                resource_version=resource_version+1,updated_at=now(),confirmed_at=now()
            WHERE id=$1 AND resource_version=$2 AND status='draft' RETURNING *
            """,
            current.id,
            current.resource_version,
        )
        if row is None:
            raise RuntimeError("subtitle confirmation compare-and-set failed")
        return map_subtitle(with_projection(row, input_outdated=False))
