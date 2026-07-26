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

    first = await provider.generate(INPUT_HASH, "shot/1")
    replay = await provider.generate(INPUT_HASH, "shot/1")
    other = await provider.generate(INPUT_HASH, "shot/2")

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

    first = await provider.generate(INPUT_HASH, "speech/1", duration_ticks=180000)
    replay = await provider.generate(INPUT_HASH, "speech/1", duration_ticks=180000)
    other = await provider.generate(INPUT_HASH, "speech/2", duration_ticks=180000)

    assert first == replay
    assert first.data != other.data
    assert first.content_type == "audio/wav"
    assert first.duration_ticks == 180000
    assert first.sample_rate == 48000 and first.channels == 1
    assert len(first.sha256) == 64
    with wave.open(io.BytesIO(first.data), "rb") as stream:
        assert stream.getframerate() == 48000
        assert stream.getnchannels() == 1
        assert stream.getsampwidth() == 2
        assert stream.getnframes() == 96000
        assert stream.readframes(stream.getnframes())
    assert provider.call_count == 3


@pytest.mark.asyncio
async def test_media_mocks_reject_invalid_hash_slot_and_duration() -> None:
    image = DeterministicImageProvider()
    tts = DeterministicTtsProvider()

    with pytest.raises(ValueError, match="input hash"):
        await image.generate("invalid", "shot/1")
    with pytest.raises(ValueError, match="output slot"):
        await image.generate(INPUT_HASH, "../escape")
    with pytest.raises(ValueError, match="duration"):
        await tts.generate(INPUT_HASH, "speech/1", duration_ticks=0)
