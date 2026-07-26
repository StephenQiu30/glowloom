from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from repositories.adopted_media import AdoptedMediaRepository, AdoptedMediaRow
from repositories.storyboard_versions import StoryboardVersionRepository
from repositories.subtitles import SubtitleRepository
from schemas.rendering import RenderInputRefsV1, RenderMediaRefV1, RenderSegmentV1
from schemas.story_content import ShotV1
from schemas.subtitle_versions import SubtitleVersionSnapshot
from schemas.subtitles import SubtitleTtsRefV1


class RenderInputInvalid(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class FrozenRenderInput:
    refs: RenderInputRefsV1
    segments: tuple[RenderSegmentV1, ...]


class RenderInputBuilder:
    def __init__(self) -> None:
        self._boards = StoryboardVersionRepository()
        self._subtitles = SubtitleRepository()
        self._media = AdoptedMediaRepository()

    async def build(
        self, connection: asyncpg.Connection[asyncpg.Record], episode_id: UUID
    ) -> FrozenRenderInput:
        board = await self._boards.get_current(connection, episode_id)
        subtitle = await self._subtitles.get_current(connection, episode_id)
        if board is None or board.input_outdated:
            raise RenderInputInvalid("current confirmed storyboard is required")
        if subtitle is None or subtitle.input_outdated or subtitle.shot_spec_version_id != board.id:
            raise RenderInputInvalid("current confirmed subtitle is required")
        videos = await self._media.list_active_shot_videos(
            connection,
            episode_id=episode_id,
            shot_spec_version_id=board.id,
            shot_ids=tuple(shot.shot_id for shot in board.content.shots),
        )
        video_by_shot = {item.usage_id: item for item in videos}
        missing = [
            str(shot.shot_id) for shot in board.content.shots if shot.shot_id not in video_by_shot
        ]
        if missing or len(videos) != len(board.content.shots):
            raise RenderInputInvalid(f"active shot video is required: {','.join(missing)}")
        video_refs = tuple(
            self._video_ref(video_by_shot[item.shot_id]) for item in board.content.shots
        )
        for shot, video in zip(board.content.shots, video_refs, strict=True):
            if abs(video.duration_ticks - shot.duration_ticks) > 90000:
                raise RenderInputInvalid(f"shot video duration is invalid: {shot.shot_id}")
        await self._validate_tts(connection, episode_id, subtitle)
        tts_refs = tuple(
            RenderMediaRefV1(
                usage_type="speech_audio",
                usage_id=item.speech_line_id,
                input_version_id=subtitle.script_version_id,
                input_hash=item.input_hash,
                adoption_id=item.adoption_id,
                candidate_id=item.candidate_id,
                media_version_id=item.media_version_id,
                sha256=item.sha256,
                duration_ticks=item.duration_ticks,
                timebase=item.timebase,
            )
            for item in subtitle.input_refs.tts_adoptions
        )
        return FrozenRenderInput(
            RenderInputRefsV1(
                shot_spec_version_id=board.id,
                subtitle_version_id=subtitle.id,
                subtitle_content_hash=subtitle.content_hash,
                video_adoptions=video_refs,
                tts_adoptions=tts_refs,
            ),
            self._segments(board.content.shots, video_refs, subtitle),
        )

    async def _validate_tts(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        episode_id: UUID,
        subtitle: SubtitleVersionSnapshot,
    ) -> None:
        adopted = await self._media.list_active_speech_media(
            connection,
            episode_id=episode_id,
            script_version_id=subtitle.script_version_id,
            speech_line_ids=tuple(
                item.speech_line_id for item in subtitle.input_refs.tts_adoptions
            ),
        )
        current = {item.usage_id: item for item in adopted}
        for frozen in subtitle.input_refs.tts_adoptions:
            actual = current.get(frozen.speech_line_id)
            if actual is None or not _same_tts(actual, frozen):
                raise RenderInputInvalid(f"TTS adoption is outdated: {frozen.speech_line_id}")

    @staticmethod
    def _video_ref(item: AdoptedMediaRow) -> RenderMediaRefV1:
        if item.duration_ticks is None or item.timebase != 90000:
            raise RenderInputInvalid(f"media timing is invalid: {item.usage_id}")
        return RenderMediaRefV1(
            usage_type="shot_video",
            usage_id=item.usage_id,
            input_version_id=item.input_version_id,
            input_hash=item.input_hash,
            adoption_id=item.adoption_id,
            candidate_id=item.candidate_id,
            media_version_id=item.media_version_id,
            sha256=item.sha256,
            duration_ticks=item.duration_ticks,
            timebase=90000,
        )

    @staticmethod
    def _segments(
        shots: tuple[ShotV1, ...],
        videos: tuple[RenderMediaRefV1, ...],
        subtitle: SubtitleVersionSnapshot,
    ) -> tuple[RenderSegmentV1, ...]:
        cursor = 0
        values = []
        for shot, video in zip(shots, videos, strict=True):
            end = cursor + shot.duration_ticks
            adoption_ids = {
                item.speech_line_id: item.adoption_id for item in subtitle.input_refs.tts_adoptions
            }
            values.append(
                RenderSegmentV1(
                    shot_id=shot.shot_id,
                    ordinal=shot.ordinal,
                    start_ticks=cursor,
                    end_ticks=end,
                    duration_ticks=shot.duration_ticks,
                    video_adoption_id=video.adoption_id,
                    tts_adoption_ids=tuple(
                        adoption_ids[cue.speech_line_id]
                        for cue in subtitle.content.cues
                        if cue.shot_id == shot.shot_id
                    ),
                )
            )
            cursor = end
        return tuple(values)


def _same_tts(current: AdoptedMediaRow, frozen: SubtitleTtsRefV1) -> bool:
    fields = (
        "adoption_id",
        "candidate_id",
        "media_version_id",
        "input_hash",
        "sha256",
        "duration_ticks",
        "timebase",
    )
    return all(getattr(current, name) == getattr(frozen, name) for name in fields)
