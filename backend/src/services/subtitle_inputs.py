from __future__ import annotations

from dataclasses import dataclass
from uuid import NAMESPACE_URL, UUID, uuid5

import asyncpg  # type: ignore[import-untyped]

from repositories.adopted_media import AdoptedMediaRepository
from repositories.script_versions import ScriptVersioningRepository
from repositories.storyboard_versions import StoryboardVersionRepository
from schemas.story_content import canonical_content_hash
from schemas.subtitles import (
    SubtitleContentV1,
    SubtitleCueV1,
    SubtitleInputRefsV1,
    SubtitleTtsRefV1,
)


class SubtitleInputInvalid(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class FrozenSubtitleInput:
    refs: SubtitleInputRefsV1
    content: SubtitleContentV1


class SubtitleInputBuilder:
    def __init__(self) -> None:
        self._scripts = ScriptVersioningRepository()
        self._storyboards = StoryboardVersionRepository()
        self._media = AdoptedMediaRepository()

    async def build(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        episode_id: UUID,
    ) -> FrozenSubtitleInput:
        script = await self._scripts.get_current(connection, episode_id)
        board = await self._storyboards.get_current(connection, episode_id)
        if script is None or script.input_outdated:
            raise SubtitleInputInvalid("current confirmed script is required")
        if (
            board is None
            or board.input_outdated
            or board.content.script_version_id != script.id
        ):
            raise SubtitleInputInvalid("current confirmed storyboard is required")
        line_ids = tuple(board.content.speech_line_ids)
        adopted = await self._media.list_active_speech_media(
            connection,
            episode_id=episode_id,
            script_version_id=script.id,
            speech_line_ids=line_ids,
        )
        by_line = {item.usage_id: item for item in adopted}
        if set(by_line) != set(line_ids) or len(adopted) != len(line_ids):
            raise SubtitleInputInvalid("every speech line requires active TTS media")
        if any(
            item.duration_ticks is None
            or item.duration_ticks <= 0
            or item.timebase != 90000
            for item in adopted
        ):
            raise SubtitleInputInvalid("TTS media timing is invalid")
        lines = {
            line.speech_line_id: line
            for scene in script.content.scenes
            for line in scene.speech_lines
        }
        cues: list[SubtitleCueV1] = []
        refs: list[SubtitleTtsRefV1] = []
        shot_start = 0
        ordinal = 1
        for shot in board.content.shots:
            cursor = shot_start
            shot_end = shot_start + shot.duration_ticks
            for line_id in shot.speech_line_ids:
                line = lines.get(line_id)
                media = by_line.get(line_id)
                if line is None or media is None or media.duration_ticks is None:
                    raise SubtitleInputInvalid("story speech mapping is incomplete")
                end = cursor + media.duration_ticks
                if end > shot_end:
                    raise SubtitleInputInvalid("TTS media exceeds its shot")
                cues.append(
                    SubtitleCueV1(
                        cue_id=uuid5(NAMESPACE_URL, f"subtitle-cue:{media.adoption_id}"),
                        ordinal=ordinal,
                        speech_line_id=line_id,
                        shot_id=shot.shot_id,
                        text=line.text,
                        voice_id=line.voice_id,
                        source_text_hash=canonical_content_hash(line.text),
                        start_ticks=cursor,
                        end_ticks=end,
                        tts_duration_ticks=media.duration_ticks,
                        shot_start_ticks=shot_start,
                        shot_end_ticks=shot_end,
                    )
                )
                refs.append(
                    SubtitleTtsRefV1(
                        speech_line_id=line_id,
                        adoption_id=media.adoption_id,
                        candidate_id=media.candidate_id,
                        media_version_id=media.media_version_id,
                        input_hash=media.input_hash,
                        sha256=media.sha256,
                        duration_ticks=media.duration_ticks,
                        timebase=90000,
                    )
                )
                cursor = end
                ordinal += 1
            shot_start = shot_end
        return FrozenSubtitleInput(
            SubtitleInputRefsV1(
                script_version_id=script.id,
                shot_spec_version_id=board.id,
                tts_adoptions=tuple(refs),
            ),
            SubtitleContentV1(language="zh-CN", cues=tuple(cues)),
        )
