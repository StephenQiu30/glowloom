from __future__ import annotations

from dataclasses import replace

import pytest
from services.delivery_quality import DeliveryQualityInvalid, DeliveryQualityPolicy

from integrations.ai.deterministic_media import DeterministicTtsProvider
from integrations.ai.deterministic_video import (
    FFMPEG_IMAGE,
    DockerFfmpegRuntime,
    MediaRuntimeError,
)
from integrations.ffmpeg_recipe import (
    RenderAudioSource,
    RenderSources,
    RenderVideoSource,
)
from integrations.ffmpeg_render import DockerRenderRuntime
from schemas.rendering import RenderRecipeV1

DEJAVU_PATH = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
DEJAVU_SHA256 = "ae7b7855e115a5966d8b1b3f80f254ccc117ec86f9965e202ee2940453837280"


def recipe() -> RenderRecipeV1:
    return RenderRecipeV1(
        runtime_image=FFMPEG_IMAGE,
        ffmpeg_version="8.1",
        ffprobe_version="8.1",
        font_name="DejaVu Sans",
        font_file=DEJAVU_PATH,
        font_sha256=DEJAVU_SHA256,
        font_license="Bitstream-Vera",
    )


@pytest.mark.asyncio
async def test_real_ffprobe_accepts_the_mvp_delivery_shape() -> None:
    runtime = DockerFfmpegRuntime()
    video = await runtime.render_color("446688", "4.000000", width=640, height=360)
    speech = await DeterministicTtsProvider().generate(
        "d" * 64,
        "primary",
        text="Lanverse",
        voice_id="narrator_female",
    )
    sources = RenderSources(
        videos=tuple(RenderVideoSource(video, 450000) for _ in range(6)),
        audios=tuple(RenderAudioSource(speech.data, index * 450000) for index in range(6)),
        subtitles_srt="".join(
            f"{index + 1}\n00:00:{index * 5:02d},000 --> "
            f"00:00:{index * 5 + 1:02d},000\nLanverse {index + 1}\n\n"
            for index in range(6)
        ),
    )
    output = await DockerRenderRuntime(timeout_seconds=90).render(sources, recipe())

    probe = await runtime.probe(output)
    summary = DeliveryQualityPolicy().validate(probe, target_duration_ticks=2_700_000)

    assert summary.video_codec == "h264"
    assert summary.audio_codec == "aac"
    assert summary.audio_sample_rate == 48000
    assert summary.audio_channels == 2
    assert summary.duration_ticks == 2_700_000

    with pytest.raises(DeliveryQualityInvalid, match="sample rate"):
        DeliveryQualityPolicy().validate(
            replace(probe, audio_sample_rate=44100),
            target_duration_ticks=2_700_000,
        )
    with pytest.raises(DeliveryQualityInvalid, match="duration"):
        DeliveryQualityPolicy().validate(probe, target_duration_ticks=3_600_000)
    with pytest.raises(MediaRuntimeError):
        await runtime.probe(b"damaged media")
