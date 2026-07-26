from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from db.pool import DatabasePool
from integrations.ffmpeg_recipe import (
    RenderAudioSource,
    RenderSources,
    RenderVideoSource,
)
from integrations.object_storage import ObjectStore
from repositories.render_snapshots import RenderSnapshotRepository
from repositories.subtitles import SubtitleRepository
from schemas.delivery_manifest import DeliveryMediaLineageV1
from schemas.delivery_media_lineage import media_lineage
from schemas.rendering import RenderSnapshot
from schemas.subtitle_versions import SubtitleVersionSnapshot
from services.subtitle_srt import render_srt

VIDEO_MAX_BYTES = 256 * 1024 * 1024
AUDIO_MAX_BYTES = 32 * 1024 * 1024


class RenderSourceInvalid(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class RenderSourceBundle:
    snapshot: RenderSnapshot
    subtitle: SubtitleVersionSnapshot
    sources: RenderSources
    subtitles_srt: str
    target_duration_ticks: int
    media_lineage: tuple[DeliveryMediaLineageV1, ...]


@dataclass(frozen=True, slots=True)
class _Location:
    bucket: str
    object_key: str
    sha256: str
    status: str
    lineage: DeliveryMediaLineageV1


class RenderSourceLoader:
    def __init__(self, database: DatabasePool, object_store: ObjectStore) -> None:
        self._database = database
        self._objects = object_store
        self._snapshots = RenderSnapshotRepository()
        self._subtitles = SubtitleRepository()

    async def load(self, snapshot_id: UUID) -> RenderSourceBundle:
        async with self._database.transaction() as connection:
            snapshot = await self._snapshots.get(connection, snapshot_id)
            if snapshot is None:
                raise RenderSourceInvalid("render snapshot is missing")
            subtitle = await self._subtitles.get(connection, snapshot.subtitle_version_id)
            if (
                subtitle is None
                or subtitle.content_hash != snapshot.input_refs.subtitle_content_hash
            ):
                raise RenderSourceInvalid("frozen subtitle does not match")
            references = (
                *snapshot.input_refs.video_adoptions,
                *snapshot.input_refs.tts_adoptions,
            )
            rows = await connection.fetch(
                """
                SELECT version.id media_version_id,version.bucket,version.object_key,
                       version.sha256 media_sha256,
                       version.status,version.mime_type,version.byte_size,
                       version.duration_ticks,version.timebase,version.probe_summary_json,
                       object.media_kind,object.source_kind,
                       candidate.usage_type,candidate.usage_id,
                       candidate.input_version_id,candidate.input_hash,
                       adoption.id adoption_id,candidate.id candidate_id,
                       attempt.id origin_attempt_id,task.id origin_task_id,
                       submission.id origin_submission_snapshot_id,
                       submission.capability,submission.model_profile_id,
                       submission.provider_id,submission.model_id,
                       submission.route_version,submission.schema_version
                FROM media_versions version
                JOIN media_objects object ON object.id=version.media_object_id
                JOIN generation_candidates candidate
                  ON candidate.media_version_id=version.id
                JOIN adoptions adoption ON adoption.candidate_id=candidate.id
                  AND adoption.id=ANY($2::uuid[])
                JOIN production_attempts attempt ON attempt.id=version.origin_attempt_id
                JOIN production_tasks task ON task.id=attempt.task_id
                JOIN submission_snapshots submission ON submission.id=attempt.snapshot_id
                WHERE version.id=ANY($1::uuid[])
                """,
                [item.media_version_id for item in references],
                [item.adoption_id for item in references],
            )
        locations = {
            row["media_version_id"]: _Location(
                row["bucket"],
                row["object_key"],
                row["media_sha256"],
                row["status"],
                media_lineage(row),
            )
            for row in rows
        }
        if len(locations) != len(references):
            raise RenderSourceInvalid("frozen media is missing")
        videos = await self._videos(snapshot, locations)
        audios = await self._audios(snapshot, subtitle, locations)
        srt = render_srt(subtitle.content)
        target = sum(item.duration_ticks for item in snapshot.segments)
        return RenderSourceBundle(
            snapshot=snapshot,
            subtitle=subtitle,
            sources=RenderSources(videos=videos, audios=audios, subtitles_srt=srt),
            subtitles_srt=srt,
            target_duration_ticks=target,
            media_lineage=tuple(locations[item.media_version_id].lineage for item in references),
        )

    async def _videos(
        self, snapshot: RenderSnapshot, locations: dict[UUID, _Location]
    ) -> tuple[RenderVideoSource, ...]:
        by_adoption = {item.adoption_id: item for item in snapshot.input_refs.video_adoptions}
        values = []
        for segment in snapshot.segments:
            reference = by_adoption.get(segment.video_adoption_id)
            if reference is None:
                raise RenderSourceInvalid("render segment video is missing")
            data = await self._read(
                reference.media_version_id, reference.sha256, locations, VIDEO_MAX_BYTES
            )
            values.append(RenderVideoSource(data, segment.duration_ticks))
        return tuple(values)

    async def _audios(
        self,
        snapshot: RenderSnapshot,
        subtitle: SubtitleVersionSnapshot,
        locations: dict[UUID, _Location],
    ) -> tuple[RenderAudioSource, ...]:
        starts = {cue.speech_line_id: cue.start_ticks for cue in subtitle.content.cues}
        values = []
        for reference in snapshot.input_refs.tts_adoptions:
            if reference.usage_id not in starts:
                raise RenderSourceInvalid("render TTS cue is missing")
            data = await self._read(
                reference.media_version_id, reference.sha256, locations, AUDIO_MAX_BYTES
            )
            values.append(RenderAudioSource(data, starts[reference.usage_id]))
        return tuple(values)

    async def _read(
        self,
        media_version_id: UUID,
        expected_sha256: str,
        locations: dict[UUID, _Location],
        max_bytes: int,
    ) -> bytes:
        location = locations[media_version_id]
        if location.status != "ready" or location.sha256 != expected_sha256:
            raise RenderSourceInvalid("frozen media facts do not match")
        return await self._objects.read(
            bucket=location.bucket,
            object_key=location.object_key,
            expected_sha256=expected_sha256,
            max_bytes=max_bytes,
        )
