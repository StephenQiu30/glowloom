from __future__ import annotations

import asyncio
import hashlib
import json
import os

from integrations.ai.deterministic_media import DeterministicTtsProvider
from integrations.ai.deterministic_video import DockerFfmpegRuntime
from integrations.ffmpeg_recipe import (
    RenderAudioSource,
    RenderSources,
    RenderVideoSource,
)
from integrations.ffmpeg_render import DockerRenderRuntime
from schemas.rendering import RenderRecipeV1

FONT_PATH = "/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf"
FONT_SHA256 = "2c76254f6fc379fddfce0a7e84fb5385bb135d3e399294f6eeb6680d0365b74b"


async def verify() -> dict[str, object]:
    runtime_image = os.environ.get("LANVERSE_RENDER_IMAGE", "")
    recipe = RenderRecipeV1(
        runtime_image=runtime_image,
        ffmpeg_version="8.1",
        ffprobe_version="8.1",
        font_name="Noto Sans CJK SC",
        font_file=FONT_PATH,
        font_sha256=FONT_SHA256,
        font_license="OFL-1.1",
    )
    probe_runtime = DockerFfmpegRuntime()
    video = await probe_runtime.render_color("224466", "4.000000", width=640, height=360)
    tts = await DeterministicTtsProvider().generate(
        "c" * 64,
        "primary",
        text="雨夜重逢",
        voice_id="narrator_female",
    )
    sources = RenderSources(
        videos=tuple(RenderVideoSource(video, 450000) for _ in range(6)),
        audios=tuple(RenderAudioSource(tts.data, index * 450000) for index in range(6)),
        subtitles_srt="".join(
            f"{index + 1}\n00:00:{index * 5:02d},000 --> "
            f"00:00:{index * 5 + 1:02d},000\n第{index + 1}幕：雨夜重逢\n\n"
            for index in range(6)
        ),
    )
    output = await DockerRenderRuntime(timeout_seconds=120).render(sources, recipe)
    probe = await probe_runtime.probe(output)
    expected = (
        probe.codec_name == "h264"
        and probe.pixel_format == "yuv420p"
        and probe.width == 720
        and probe.height == 1280
        and probe.frame_rate == "24/1"
        and abs(probe.duration_seconds - 30.0) <= 0.05
        and probe.audio_stream_count == 1
    )
    if not expected:
        raise RuntimeError("render runtime output does not satisfy the MVP recipe")
    return {
        "runtime_image": runtime_image,
        "font_sha256": FONT_SHA256,
        "output_sha256": hashlib.sha256(output).hexdigest(),
        "byte_size": len(output),
        "width": probe.width,
        "height": probe.height,
        "frame_rate": probe.frame_rate,
        "duration_seconds": probe.duration_seconds,
        "audio_stream_count": probe.audio_stream_count,
    }


def main() -> None:
    print(json.dumps(asyncio.run(verify()), ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
