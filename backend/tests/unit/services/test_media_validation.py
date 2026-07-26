from __future__ import annotations

import pytest

from integrations.ai.deterministic_media import (
    DeterministicImageProvider,
    DeterministicTtsProvider,
)
from integrations.ai.deterministic_video import (
    DeterministicVideoProvider,
    DockerFfmpegRuntime,
)
from schemas.media import (
    AudioProbeSummaryV1,
    ImageProbeSummaryV1,
    VideoProbeSummaryV1,
)
from services.media_validation import (
    IMAGE_MAX_BYTES,
    InvalidMedia,
    MediaValidationService,
)

INPUT_HASH = "a" * 64


@pytest.mark.asyncio
async def test_validates_image_bytes_and_strict_probe_summary() -> None:
    media = await DeterministicImageProvider().generate(INPUT_HASH, "primary")
    service = MediaValidationService(DockerFfmpegRuntime())

    validated = await service.validate("image", media.content_type, media.data)

    assert validated.byte_size == len(media.data)
    assert validated.sha256 == media.sha256
    assert validated.width == 720 and validated.height == 1280
    assert validated.duration_ticks is None
    assert validated.probe_summary == ImageProbeSummaryV1(
        codec="png", width=720, height=1280
    )


@pytest.mark.asyncio
async def test_validates_video_and_accepts_exact_duration_tolerance_boundary() -> None:
    runtime = DockerFfmpegRuntime()
    media = await DeterministicVideoProvider(runtime).generate(
        INPUT_HASH, "primary", duration_ticks=270000
    )
    service = MediaValidationService(runtime)

    validated = await service.validate(
        "video",
        media.content_type,
        media.data,
        target_duration_ticks=360000,
    )

    assert validated.duration_ticks == 270000
    assert validated.timebase == 90000
    assert validated.probe_summary == VideoProbeSummaryV1(
        codec="h264",
        pixel_format="yuv420p",
        width=720,
        height=1280,
        frame_rate="24/1",
        duration_ticks=270000,
        timebase=90000,
        audio_stream_count=0,
    )


@pytest.mark.asyncio
async def test_validates_exact_wav_duration_and_audio_shape() -> None:
    media = await DeterministicTtsProvider().generate(
        INPUT_HASH, "primary", duration_ticks=180000
    )
    service = MediaValidationService(DockerFfmpegRuntime())

    validated = await service.validate("audio", media.content_type, media.data)

    assert validated.duration_ticks == 180000
    assert validated.probe_summary == AudioProbeSummaryV1(
        codec="pcm_s16le",
        sample_rate=48000,
        channels=1,
        duration_ticks=180000,
        timebase=90000,
    )


@pytest.mark.asyncio
async def test_rejects_mime_size_corruption_and_video_duration_over_tolerance() -> None:
    runtime = DockerFfmpegRuntime()
    service = MediaValidationService(runtime)
    image = await DeterministicImageProvider().generate(INPUT_HASH, "primary")
    video = await DeterministicVideoProvider(runtime).generate(
        INPUT_HASH, "primary", duration_ticks=270000
    )

    with pytest.raises(InvalidMedia, match="MIME"):
        await service.validate("image", "video/mp4", image.data)
    with pytest.raises(InvalidMedia, match="size"):
        await service.validate("image", "image/png", b"x" * (IMAGE_MAX_BYTES + 1))
    with pytest.raises(InvalidMedia, match="decode"):
        await service.validate("image", "image/png", b"not-a-png")
    with pytest.raises(InvalidMedia, match="duration"):
        await service.validate(
            "video",
            "video/mp4",
            video.data,
            target_duration_ticks=360001,
        )
