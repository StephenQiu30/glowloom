from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol, cast, get_args
from uuid import UUID

from integrations.ai.deterministic_media import GeneratedMedia
from integrations.ai.profiles import Capability
from repositories.task_executions import MediaExecutionInput
from schemas.media import MediaKind
from schemas.media_registration import UsageType
from schemas.story_content import VoiceId


class InvalidMediaProviderInput(ValueError):
    pass


class RetryableMediaProviderError(RuntimeError):
    pass


class ImageProvider(Protocol):
    async def generate(self, input_hash: str, output_slot: str) -> GeneratedMedia: ...


class VideoProvider(Protocol):
    async def generate(
        self,
        input_hash: str,
        output_slot: str,
        *,
        duration_ticks: int,
    ) -> GeneratedMedia: ...


class TtsProvider(Protocol):
    async def generate(
        self,
        text_hash: str,
        output_slot: str,
        *,
        text: str,
        voice_id: str,
    ) -> GeneratedMedia: ...


@dataclass(frozen=True, slots=True)
class MediaProviderRequest:
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    provider_hash: str
    media_kind: MediaKind
    target_duration_ticks: int | None = None
    text: str | None = None
    logical_voice_id: VoiceId | None = None


def parse_media_request(
    job_input: MediaExecutionInput, prompt: str
) -> MediaProviderRequest:
    refs = job_input.input_refs
    usage_type = refs.get("usage_type")
    allowed = {
        "image": {"asset_image", "shot_image"},
        "video": {"shot_video"},
        "tts": {"speech_audio"},
    }.get(job_input.capability, set())
    if usage_type not in allowed:
        raise InvalidMediaProviderInput("media usage type is invalid")
    parsed_usage_type = cast(UsageType, usage_type)
    usage_key = {
        "asset_image": "asset_id",
        "shot_image": "shot_id",
        "shot_video": "shot_id",
        "speech_audio": "speech_line_id",
    }[parsed_usage_type]
    try:
        usage_id = UUID(str(refs[usage_key]))
        version_id = UUID(str(refs["input_version_id"]))
    except (KeyError, ValueError) as error:
        raise InvalidMediaProviderInput("media usage references are invalid") from error
    input_hash = refs.get("input_hash")
    if not isinstance(input_hash, str):
        raise InvalidMediaProviderInput("media input hash is missing")
    if job_input.capability == "tts":
        return _tts_request(refs, parsed_usage_type, usage_id, version_id, input_hash, prompt)
    duration = refs.get("duration_ticks") if job_input.capability == "video" else None
    if duration is not None and (not isinstance(duration, int) or duration <= 0):
        raise InvalidMediaProviderInput("video target duration is invalid")
    return MediaProviderRequest(
        parsed_usage_type,
        usage_id,
        version_id,
        input_hash,
        input_hash,
        cast(MediaKind, job_input.capability),
        target_duration_ticks=duration,
    )


def _tts_request(
    refs: dict[str, object],
    usage_type: UsageType,
    usage_id: UUID,
    version_id: UUID,
    input_hash: str,
    prompt: str,
) -> MediaProviderRequest:
    text_hash = refs.get("text_hash")
    voice_id = refs.get("voice_id")
    if not isinstance(text_hash, str) or len(text_hash) != 64 or not prompt:
        raise InvalidMediaProviderInput("speech text input is invalid")
    if voice_id not in get_args(VoiceId):
        raise InvalidMediaProviderInput("logical voice input is invalid")
    return MediaProviderRequest(
        usage_type,
        usage_id,
        version_id,
        input_hash,
        text_hash,
        "audio",
        text=prompt,
        logical_voice_id=cast(VoiceId, voice_id),
    )


async def invoke_media_provider(
    adapter: object,
    capability: Capability,
    request: MediaProviderRequest,
    *,
    provider_voice_id: str | None = None,
) -> GeneratedMedia:
    if capability == "image":
        return await cast(ImageProvider, adapter).generate(request.provider_hash, "primary")
    if capability == "video" and request.target_duration_ticks is not None:
        return await cast(VideoProvider, adapter).generate(
            request.provider_hash,
            "primary",
            duration_ticks=request.target_duration_ticks,
        )
    if capability == "tts" and request.text is not None and provider_voice_id is not None:
        return await cast(TtsProvider, adapter).generate(
            request.provider_hash,
            "primary",
            text=request.text,
            voice_id=provider_voice_id,
        )
    raise InvalidMediaProviderInput("media provider input is incomplete")
