from __future__ import annotations

import pytest
from integrations.ffmpeg_render import (
    DockerRenderRuntime,
    RenderAudioSource,
    RenderSources,
    RenderVideoSource,
    build_ffmpeg_arguments,
)
from pydantic import ValidationError

from integrations.ai.deterministic_media import DeterministicTtsProvider
from integrations.ai.deterministic_video import DockerFfmpegRuntime
from schemas.rendering import RenderRecipeV1

DEJAVU_PATH = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
DEJAVU_SHA256 = "ae7b7855e115a5966d8b1b3f80f254ccc117ec86f9965e202ee2940453837280"


def recipe() -> RenderRecipeV1:
    return RenderRecipeV1(
        ffmpeg_version="8.1",
        ffprobe_version="8.1",
        font_name="DejaVu Sans",
        font_file=DEJAVU_PATH,
        font_sha256=DEJAVU_SHA256,
        font_license="OFL-1.1",
    )


def test_render_arguments_are_structured_and_never_map_source_audio() -> None:
    sources = RenderSources(
        videos=tuple(RenderVideoSource(b"video", 270000) for _ in range(6)),
        audios=(RenderAudioSource(b"audio", 0),),
        subtitles_srt="1\n00:00:00,000 --> 00:00:01,000\nLanverse\n",
    )

    arguments = build_ffmpeg_arguments(sources, recipe())
    graph = arguments[arguments.index("-filter_complex") + 1]

    assert isinstance(arguments, tuple)
    assert "concat=n=6:v=1:a=0" in graph
    assert "subtitles=/work/subtitles.srt" in graph
    assert "adelay=0S:all=1" in graph
    assert all(f"[{index}:a" not in graph for index in range(6))
    assert arguments[arguments.index("-map") + 1] == "[video_out]"
    assert arguments[arguments.index("-map", arguments.index("-map") + 1) + 1] == "[audio_out]"

    with pytest.raises(ValidationError):
        RenderRecipeV1(
            ffmpeg_version="8.1",
            ffprobe_version="8.1",
            font_name="Noto;movie=/etc/passwd",
            font_file="../../font.ttf",
            font_sha256="a" * 64,
            font_license="OFL-1.1",
        )


@pytest.mark.asyncio
async def test_real_ffmpeg_normalizes_six_segments_and_mixes_tts() -> None:
    media_runtime = DockerFfmpegRuntime()
    video = await media_runtime.render_color("335577", "3.000000")
    speech = await DeterministicTtsProvider().generate(
        "a" * 64,
        "primary",
        text="Lanverse",
        voice_id="narrator_female",
    )
    sources = RenderSources(
        videos=tuple(RenderVideoSource(video, 270000) for _ in range(6)),
        audios=tuple(RenderAudioSource(speech.data, index * 270000) for index in range(6)),
        subtitles_srt="".join(
            f"{index + 1}\n00:00:{index * 3:02d},000 --> "
            f"00:00:{index * 3 + 1:02d},000\nLanverse {index + 1}\n\n"
            for index in range(6)
        ),
    )

    output = await DockerRenderRuntime(timeout_seconds=90).render(sources, recipe())
    probe = await media_runtime.probe(output)

    assert probe.codec_name == "h264"
    assert probe.pixel_format == "yuv420p"
    assert probe.width == 720 and probe.height == 1280
    assert probe.frame_rate == "24/1"
    assert probe.duration_seconds == pytest.approx(18.0, abs=0.05)
    assert probe.audio_stream_count == 1
