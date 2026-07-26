from __future__ import annotations

import hashlib
import io
import math
import re
import struct
import sys
import wave
import zlib
from array import array
from dataclasses import dataclass

INPUT_HASH = re.compile(r"^[0-9a-f]{64}$")
OUTPUT_SLOT = re.compile(r"^(primary|extra/[0-9]+)$")
TIMEBASE = 90000


@dataclass(frozen=True, slots=True)
class GeneratedMedia:
    output_slot: str
    content_type: str
    data: bytes
    width: int | None = None
    height: int | None = None
    duration_ticks: int | None = None
    sample_rate: int | None = None
    channels: int | None = None
    timebase: int = TIMEBASE

    @property
    def sha256(self) -> str:
        return hashlib.sha256(self.data).hexdigest()


def _seed(input_hash: str, output_slot: str, discriminator: str) -> bytes:
    if INPUT_HASH.fullmatch(input_hash) is None:
        raise ValueError("input hash must be 64 lowercase hexadecimal characters")
    if OUTPUT_SLOT.fullmatch(output_slot) is None:
        raise ValueError("output slot must be primary or extra/{index}")
    return hashlib.sha256(f"{discriminator}:{input_hash}:{output_slot}".encode()).digest()


def _png_chunk(kind: bytes, data: bytes) -> bytes:
    checksum = zlib.crc32(kind + data) & 0xFFFFFFFF
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", checksum)


def _solid_png(width: int, height: int, color: bytes) -> bytes:
    header = struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)
    scanline = b"\x00" + color * width
    pixels = scanline * height
    return b"".join(
        (
            b"\x89PNG\r\n\x1a\n",
            _png_chunk(b"IHDR", header),
            _png_chunk(b"IDAT", zlib.compress(pixels, level=9)),
            _png_chunk(b"IEND", b""),
        )
    )


def _mono_wav(sample_rate: int, sample_count: int, frequency: int) -> bytes:
    samples = array(
        "h",
        (
            round(3000 * math.sin(2 * math.pi * frequency * index / sample_rate))
            for index in range(sample_count)
        ),
    )
    if sys.byteorder != "little":
        samples.byteswap()
    output = io.BytesIO()
    with wave.open(output, "wb") as stream:
        stream.setnchannels(1)
        stream.setsampwidth(2)
        stream.setframerate(sample_rate)
        stream.writeframes(samples.tobytes())
    return output.getvalue()


class DeterministicImageProvider:
    def __init__(self) -> None:
        self.call_count = 0

    async def generate(self, input_hash: str, output_slot: str) -> GeneratedMedia:
        seed = _seed(input_hash, output_slot, "image-v1")
        self.call_count += 1
        return GeneratedMedia(
            output_slot=output_slot,
            content_type="image/png",
            data=_solid_png(720, 1280, seed[:3]),
            width=720,
            height=1280,
        )


class DeterministicTtsProvider:
    def __init__(self) -> None:
        self.call_count = 0

    async def generate(
        self,
        text_hash: str,
        output_slot: str,
        *,
        duration_ticks: int,
    ) -> GeneratedMedia:
        seed = _seed(text_hash, output_slot, "tts-v1")
        if duration_ticks <= 0:
            raise ValueError("duration must be positive")
        numerator = duration_ticks * 48000
        if numerator % TIMEBASE:
            raise ValueError("duration must align to the 48kHz sample rate")
        self.call_count += 1
        return GeneratedMedia(
            output_slot=output_slot,
            content_type="audio/wav",
            data=_mono_wav(48000, numerator // TIMEBASE, 220 + seed[0]),
            duration_ticks=duration_ticks,
            sample_rate=48000,
            channels=1,
        )
