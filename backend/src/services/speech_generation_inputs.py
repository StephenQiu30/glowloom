from __future__ import annotations

import asyncpg  # type: ignore[import-untyped]

from repositories.script_versions import ScriptVersioningRepository
from schemas.story_content import canonical_content_hash
from services.media_generation_inputs import (
    FrozenMediaInput,
    MediaInputNotFound,
    MediaInputOutdated,
    MediaInputRequest,
)


class SpeechInputFreezer:
    def __init__(self) -> None:
        self._scripts = ScriptVersioningRepository()

    async def freeze(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        request: MediaInputRequest,
    ) -> FrozenMediaInput:
        script = await self._scripts.get(
            connection, request.input_version_id, for_update=True
        )
        if script is None or script.episode_id != request.episode_id:
            raise MediaInputNotFound("confirmed script version was not found")
        if script.status != "confirmed" or script.input_outdated:
            raise MediaInputOutdated("script is not the current confirmed input")
        line = next(
            (
                item
                for scene in script.content.scenes
                for item in scene.speech_lines
                if item.speech_line_id == request.usage_id
            ),
            None,
        )
        if line is None:
            raise MediaInputNotFound("speech line was not found in the script")
        refs: dict[str, object] = {
            "usage_type": "speech_audio",
            "speech_line_id": str(line.speech_line_id),
            "input_version_id": str(script.id),
            "text_hash": canonical_content_hash(line.text),
            "voice_id": line.voice_id,
        }
        return FrozenMediaInput("tts", refs, line.text)
