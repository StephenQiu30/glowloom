from __future__ import annotations

import io
import struct
import wave
import zlib

import pytest

from integrations.ai.deterministic_media import (
    DeterministicImageProvider,
    DeterministicTtsProvider,
)

INPUT_HASH = "a" * 64


def decode_png(value: bytes) -> tuple[int, int, bytes]:
    assert value.startswith(b"\x89PNG\r\n\x1a\n")
    offset = 8
    width = height = 0
    image_data = bytearray()
    while offset < len(value):
        length = struct.unpack(">I", value[offset : offset + 4])[0]
        kind = value[offset + 4 : offset + 8]
        data = value[offset + 8 : offset + 8 + length]
        checksum = struct.unpack(">I", value[offset + 8 + length : offset + 12 + length])[0]
        assert zlib.crc32(kind + data) & 0xFFFFFFFF == checksum
        offset += 12 + length
        if kind == b"IHDR":
            width, height = struct.unpack(">II", data[:8])
            assert data[8:] == b"\x08\x02\x00\x00\x00"
        elif kind == b"IDAT":
            image_data.extend(data)
        elif kind == b"IEND":
            break
    return width, height, zlib.decompress(image_data)


@pytest.mark.asyncio
async def test_image_mock_is_stable_isolated_and_decodable() -> None:
    provider = DeterministicImageProvider()

    first = await provider.generate(INPUT_HASH, "primary")
    replay = await provider.generate(INPUT_HASH, "primary")
    other = await provider.generate(INPUT_HASH, "extra/0")

    assert first == replay
    assert first.data != other.data
    assert first.content_type == "image/png"
    assert first.width == 720 and first.height == 1280
    assert len(first.sha256) == 64
    width, height, pixels = decode_png(first.data)
    assert (width, height) == (720, 1280)
    assert len(pixels) == 1280 * (1 + 720 * 3)
    assert all(pixels[row * (1 + 720 * 3)] == 0 for row in range(1280))
    assert provider.call_count == 3


@pytest.mark.asyncio
async def test_tts_mock_is_stable_isolated_and_exact_duration_wav() -> None:
    provider = DeterministicTtsProvider()

    arguments = {"text": "雨夜里，她终于找到了回家的路。", "voice_id": "mock.narrator_female"}
    first = await provider.generate(INPUT_HASH, "primary", **arguments)
    replay = await provider.generate(INPUT_HASH, "primary", **arguments)
    other = await provider.generate(INPUT_HASH, "extra/0", **arguments)
    other_voice = await provider.generate(
        INPUT_HASH,
        "primary",
        text=arguments["text"],
        voice_id="mock.narrator_male",
    )

    assert first == replay
    assert first.data != other.data
    assert first.data != other_voice.data
    assert first.content_type == "audio/wav"
    assert first.duration_ticks is not None and first.duration_ticks > 0
    assert first.sample_rate == 48000 and first.channels == 1
    assert len(first.sha256) == 64
    with wave.open(io.BytesIO(first.data), "rb") as stream:
        assert stream.getframerate() == 48000
        assert stream.getnchannels() == 1
        assert stream.getsampwidth() == 2
        assert stream.getnframes() * 90000 == first.duration_ticks * 48000
        assert stream.readframes(stream.getnframes())
    assert provider.call_count == 4


@pytest.mark.asyncio
async def test_media_mocks_reject_invalid_hash_slot_and_duration() -> None:
    image = DeterministicImageProvider()
    tts = DeterministicTtsProvider()

    with pytest.raises(ValueError, match="input hash"):
        await image.generate("invalid", "primary")
    with pytest.raises(ValueError, match="output slot"):
        await image.generate(INPUT_HASH, "../escape")
    with pytest.raises(ValueError, match="text"):
        await tts.generate(INPUT_HASH, "primary", text="", voice_id="mock.voice")
    with pytest.raises(ValueError, match="voice"):
        await tts.generate(INPUT_HASH, "primary", text="旁白", voice_id="")
