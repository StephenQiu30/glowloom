from __future__ import annotations

from collections.abc import Mapping
from types import MappingProxyType

from schemas.story_content import VoiceId


class VoiceMappingUnavailable(LookupError):
    pass


class VoiceCatalog:
    def __init__(self, mappings: Mapping[tuple[str, str, VoiceId], str]) -> None:
        if any(not value.strip() for value in mappings.values()):
            raise ValueError("provider voice ids must be non-empty")
        self._mappings = MappingProxyType(dict(mappings))

    def resolve(self, provider_id: str, route_version: str, voice_id: VoiceId) -> str:
        value = self._mappings.get((provider_id, route_version, voice_id))
        if value is None:
            raise VoiceMappingUnavailable(
                f"voice {voice_id} is unavailable for {provider_id}/{route_version}"
            )
        return value


def create_mvp_voice_catalog() -> VoiceCatalog:
    return VoiceCatalog(
        {
            ("mock", "tts-route-v1", "narrator_female"): "mock.narrator_female",
            ("mock", "tts-route-v1", "narrator_male"): "mock.narrator_male",
            (
                "mock",
                "tts-route-v1",
                "character_young_female",
            ): "mock.character_young_female",
            (
                "mock",
                "tts-route-v1",
                "character_young_male",
            ): "mock.character_young_male",
        }
    )
