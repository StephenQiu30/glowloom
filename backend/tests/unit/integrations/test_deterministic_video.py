from __future__ import annotations

import pytest

from integrations.ai.deterministic_video import (
    FFMPEG_IMAGE,
    DeterministicVideoProvider,
    DockerFfmpegRuntime,
)

INPUT_HASH = "a" * 64


@pytest.mark.asyncio
async def test_video_mock_is_stable_isolated_and_decodable() -> None:
    runtime = DockerFfmpegRuntime()
    provider = DeterministicVideoProvider(runtime)

    first = await provider.generate(INPUT_HASH, "shot/1", duration_ticks=270000)
    replay = await provider.generate(INPUT_HASH, "shot/1", duration_ticks=270000)
    other = await provider.generate(INPUT_HASH, "shot/2", duration_ticks=270000)

    assert first == replay
    assert first.data != other.data
    assert first.content_type == "video/mp4"
    assert first.width == 720 and first.height == 1280
    assert first.duration_ticks == 270000
    assert len(first.sha256) == 64
    assert provider.call_count == 3

    probe = await runtime.probe(first.data)
    assert probe.codec_name == "h264"
    assert probe.pixel_format == "yuv420p"
    assert probe.width == 720 and probe.height == 1280
    assert probe.frame_rate == "24/1"
    assert probe.duration_seconds == pytest.approx(3.0, abs=0.001)
    assert probe.audio_stream_count == 0


@pytest.mark.asyncio
async def test_video_mock_rejects_invalid_hash_slot_and_duration() -> None:
    provider = DeterministicVideoProvider(DockerFfmpegRuntime())

    with pytest.raises(ValueError, match="input hash"):
        await provider.generate("invalid", "shot/1", duration_ticks=270000)
    with pytest.raises(ValueError, match="output slot"):
        await provider.generate(INPUT_HASH, "../escape", duration_ticks=270000)
    with pytest.raises(ValueError, match="between 3 and 8 seconds"):
        await provider.generate(INPUT_HASH, "shot/1", duration_ticks=180000)
    with pytest.raises(ValueError, match="whole video frames"):
        await provider.generate(INPUT_HASH, "shot/1", duration_ticks=270001)


def test_ffmpeg_runtime_uses_an_immutable_image_reference() -> None:
    assert FFMPEG_IMAGE == (
        "docker.io/jrottenberg/ffmpeg@"
        "sha256:83ef82d9850314baa3504821e2ea6598e40e2096ac8f967a842d31234be2be92"
    )
