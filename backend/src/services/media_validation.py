from __future__ import annotations

import hashlib
from dataclasses import dataclass
from typing import Protocol

from integrations.ai.deterministic_video import MediaRuntimeError, VideoProbe
from integrations.local_media_probe import (
    LocalMediaDecodeError,
    probe_png,
    probe_wav,
)
from schemas.media import (
    AudioProbeSummaryV1,
    ImageProbeSummaryV1,
    MediaKind,
    MediaProbeSummary,
    VideoProbeSummaryV1,
)

TIMEBASE = 90000
IMAGE_MAX_BYTES = 20 * 1024 * 1024
VIDEO_MAX_BYTES = 256 * 1024 * 1024
AUDIO_MAX_BYTES = 32 * 1024 * 1024
VIDEO_MIN_TICKS = 3 * TIMEBASE
VIDEO_MAX_TICKS = 8 * TIMEBASE
VIDEO_TOLERANCE_TICKS = TIMEBASE

class InvalidMedia(ValueError):
    pass


class VideoProbeRuntime(Protocol):
    async def probe(self, data: bytes) -> VideoProbe: ...

    async def verify_video_decode(self, data: bytes) -> None: ...


@dataclass(frozen=True, slots=True)
class ValidatedMedia:
    byte_size: int
    sha256: str
    width: int | None
    height: int | None
    duration_ticks: int | None
    timebase: int | None
    probe_summary: MediaProbeSummary


class MediaValidationService:
    _MIME = {
        "image": frozenset({"image/png"}),
        "video": frozenset({"video/mp4"}),
        "audio": frozenset({"audio/wav"}),
    }
    _MAX_BYTES = {
        "image": IMAGE_MAX_BYTES,
        "video": VIDEO_MAX_BYTES,
        "audio": AUDIO_MAX_BYTES,
    }

    def __init__(self, video_runtime: VideoProbeRuntime) -> None:
        self._video_runtime = video_runtime

    async def validate(
        self,
        media_kind: MediaKind,
        content_type: str,
        data: bytes,
        *,
        target_duration_ticks: int | None = None,
    ) -> ValidatedMedia:
        if content_type not in self._MIME[media_kind]:
            raise InvalidMedia("declared MIME does not match the media kind")
        if not data or len(data) > self._MAX_BYTES[media_kind]:
            raise InvalidMedia("media size is outside the allowed range")
        if media_kind == "image":
            return self._validate_image(data)
        if media_kind == "audio":
            return self._validate_audio(data)
        return await self._validate_video(data, target_duration_ticks)

    def _validate_image(self, data: bytes) -> ValidatedMedia:
        try:
            probe = probe_png(data)
        except LocalMediaDecodeError as error:
            raise InvalidMedia("image decode failed") from error
        summary = ImageProbeSummaryV1(
            codec=probe.codec,
            width=probe.width,
            height=probe.height,
        )
        return self._result(data, summary, width=probe.width, height=probe.height)

    def _validate_audio(self, data: bytes) -> ValidatedMedia:
        try:
            probe = probe_wav(data)
        except LocalMediaDecodeError as error:
            raise InvalidMedia("audio decode failed") from error
        summary = AudioProbeSummaryV1(
            codec=probe.codec,
            sample_rate=probe.sample_rate,
            channels=probe.channels,
            duration_ticks=probe.duration_ticks,
            timebase=TIMEBASE,
        )
        return self._result(
            data,
            summary,
            duration_ticks=probe.duration_ticks,
            timebase=TIMEBASE,
        )

    async def _validate_video(
        self, data: bytes, target_duration_ticks: int | None
    ) -> ValidatedMedia:
        if target_duration_ticks is None:
            raise InvalidMedia("video target duration is required")
        try:
            probe = await self._video_runtime.probe(data)
            await self._video_runtime.verify_video_decode(data)
        except MediaRuntimeError as error:
            raise InvalidMedia("video decode failed") from error
        duration_ticks = round(probe.duration_seconds * TIMEBASE)
        if probe.width * 16 != probe.height * 9:
            raise InvalidMedia("video aspect ratio must be 9:16")
        if not VIDEO_MIN_TICKS <= duration_ticks <= VIDEO_MAX_TICKS:
            raise InvalidMedia("video duration must be between 3 and 8 seconds")
        if abs(duration_ticks - target_duration_ticks) > VIDEO_TOLERANCE_TICKS:
            raise InvalidMedia("video duration exceeds target tolerance")
        summary = VideoProbeSummaryV1(
            codec=probe.codec_name,
            pixel_format=probe.pixel_format,
            width=probe.width,
            height=probe.height,
            frame_rate=probe.frame_rate,
            duration_ticks=duration_ticks,
            timebase=TIMEBASE,
            audio_stream_count=probe.audio_stream_count,
        )
        return self._result(
            data,
            summary,
            width=probe.width,
            height=probe.height,
            duration_ticks=duration_ticks,
            timebase=TIMEBASE,
        )

    @staticmethod
    def _result(
        data: bytes,
        summary: MediaProbeSummary,
        *,
        width: int | None = None,
        height: int | None = None,
        duration_ticks: int | None = None,
        timebase: int | None = None,
    ) -> ValidatedMedia:
        return ValidatedMedia(
            byte_size=len(data),
            sha256=hashlib.sha256(data).hexdigest(),
            width=width,
            height=height,
            duration_ticks=duration_ticks,
            timebase=timebase,
            probe_summary=summary,
        )
