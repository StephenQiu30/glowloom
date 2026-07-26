from __future__ import annotations

from uuid import UUID

from db.pool import DatabasePool
from repositories.subtitles import SubtitleRepository
from schemas.subtitle_versions import SubtitleVersionSnapshot
from schemas.subtitles import SubtitleContentV1, validate_speech_mapping
from services.script_versions import VersionConflict, VersionImmutable
from services.subtitle_commands import (
    ConfirmSubtitleCommand,
    SaveSubtitleCommand,
    SubtitleVersionNotFound,
)
from services.subtitle_inputs import SubtitleInputBuilder, SubtitleInputInvalid


class _SubtitleHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._subtitles = SubtitleRepository()


class SaveSubtitleHandler(_SubtitleHandler):
    async def execute(self, command: SaveSubtitleCommand) -> SubtitleVersionSnapshot:
        async with self._database.transaction() as connection:
            episode_id = await self._subtitles.episode_id_for(
                connection, command.version_id
            )
            if episode_id is None:
                raise SubtitleVersionNotFound
            await self._subtitles.lock_episode(connection, episode_id)
            current = await self._subtitles.get(
                connection, command.version_id, for_update=True
            )
            if current is None:
                raise SubtitleVersionNotFound
            if current.resource_version != command.expected_resource_version:
                raise VersionConflict
            if current.status != "draft":
                raise VersionImmutable
            _validate_edit(current.content, command.content)
            expected = tuple(
                item.speech_line_id for item in current.input_refs.tts_adoptions
            )
            validate_speech_mapping(command.content, expected)
            return await self._subtitles.update_draft(
                connection, current, command.content
            )


class ConfirmSubtitleHandler(_SubtitleHandler):
    def __init__(self, database: DatabasePool) -> None:
        super().__init__(database)
        self._inputs = SubtitleInputBuilder()

    async def execute(self, command: ConfirmSubtitleCommand) -> SubtitleVersionSnapshot:
        async with self._database.transaction() as connection:
            episode_id = await self._subtitles.episode_id_for(
                connection, command.version_id
            )
            if episode_id is None:
                raise SubtitleVersionNotFound
            await self._subtitles.lock_episode(connection, episode_id)
            current = await self._subtitles.get(
                connection, command.version_id, for_update=True
            )
            if current is None:
                raise SubtitleVersionNotFound
            if current.resource_version != command.expected_resource_version:
                raise VersionConflict
            if current.status != "draft":
                raise VersionImmutable
            frozen = await self._inputs.build(connection, episode_id)
            if current.input_outdated or current.input_refs != frozen.refs:
                raise VersionConflict
            validate_speech_mapping(
                current.content,
                tuple(item.speech_line_id for item in frozen.refs.tts_adoptions),
            )
            return await self._subtitles.confirm(connection, current)


class GetCurrentSubtitleHandler(_SubtitleHandler):
    async def execute(self, episode_id: UUID) -> SubtitleVersionSnapshot:
        async with self._database.transaction() as connection:
            value = await self._subtitles.get_current(connection, episode_id)
        if value is None:
            raise SubtitleVersionNotFound
        return value


class GetSubtitleVersionHandler(_SubtitleHandler):
    async def execute(self, version_id: UUID) -> SubtitleVersionSnapshot:
        async with self._database.transaction() as connection:
            value = await self._subtitles.get(connection, version_id)
        if value is None:
            raise SubtitleVersionNotFound
        return value


class ListSubtitleVersionsHandler(_SubtitleHandler):
    async def execute(self, episode_id: UUID) -> tuple[SubtitleVersionSnapshot, ...]:
        async with self._database.transaction() as connection:
            return await self._subtitles.list_for_episode(connection, episode_id)


def _validate_edit(stored: SubtitleContentV1, changed: SubtitleContentV1) -> None:
    if stored.language != changed.language or len(stored.cues) != len(changed.cues):
        raise SubtitleInputInvalid("subtitle structure cannot be changed")
    immutable = {
        "cue_id",
        "ordinal",
        "speech_line_id",
        "shot_id",
        "voice_id",
        "source_text_hash",
        "tts_duration_ticks",
        "shot_start_ticks",
        "shot_end_ticks",
    }
    for before, after in zip(stored.cues, changed.cues, strict=True):
        if any(getattr(before, field) != getattr(after, field) for field in immutable):
            raise SubtitleInputInvalid("only cue text and start may be edited")
