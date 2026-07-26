from __future__ import annotations

import io
import struct
import wave
import zlib
from dataclasses import dataclass

TIMEBASE = 90000


class LocalMediaDecodeError(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class ImageBytesProbe:
    codec: str
    width: int
    height: int


@dataclass(frozen=True, slots=True)
class AudioBytesProbe:
    codec: str
    sample_rate: int
    channels: int
    duration_ticks: int


def probe_png(data: bytes) -> ImageBytesProbe:
    if not data.startswith(b"\x89PNG\r\n\x1a\n"):
        raise LocalMediaDecodeError("PNG signature is invalid")
    offset = 8
    width = height = 0
    compressed = bytearray()
    ended = False
    while offset + 12 <= len(data):
        length = struct.unpack(">I", data[offset : offset + 4])[0]
        end = offset + 12 + length
        if end > len(data):
            raise LocalMediaDecodeError("PNG chunk is truncated")
        kind = data[offset + 4 : offset + 8]
        payload = data[offset + 8 : offset + 8 + length]
        checksum = struct.unpack(">I", data[offset + 8 + length : end])[0]
        if zlib.crc32(kind + payload) & 0xFFFFFFFF != checksum:
            raise LocalMediaDecodeError("PNG checksum is invalid")
        if kind == b"IHDR":
            if length != 13 or width or height:
                raise LocalMediaDecodeError("PNG header is invalid")
            width, height, depth, color, compression, filtering, interlace = struct.unpack(
                ">IIBBBBB", payload
            )
            if (depth, color, compression, filtering, interlace) != (8, 2, 0, 0, 0):
                raise LocalMediaDecodeError("PNG encoding is unsupported")
        elif kind == b"IDAT":
            compressed.extend(payload)
        elif kind == b"IEND":
            ended = True
            offset = end
            break
        offset = end
    if not ended or offset != len(data) or width <= 0 or height <= 0 or not compressed:
        raise LocalMediaDecodeError("PNG structure is invalid")
    try:
        pixels = zlib.decompress(compressed)
    except zlib.error as error:
        raise LocalMediaDecodeError("PNG pixels cannot be decoded") from error
    if len(pixels) != height * (1 + width * 3):
        raise LocalMediaDecodeError("PNG pixel length is invalid")
    return ImageBytesProbe(codec="png", width=width, height=height)


def probe_wav(data: bytes) -> AudioBytesProbe:
    try:
        with wave.open(io.BytesIO(data), "rb") as stream:
            channels = stream.getnchannels()
            sample_rate = stream.getframerate()
            sample_width = stream.getsampwidth()
            frame_count = stream.getnframes()
            frames = stream.readframes(frame_count)
            if stream.getcomptype() != "NONE" or stream.readframes(1):
                raise LocalMediaDecodeError("WAV encoding is unsupported")
    except (EOFError, wave.Error) as error:
        raise LocalMediaDecodeError("WAV cannot be decoded") from error
    if channels <= 0 or sample_rate <= 0 or sample_width != 2 or frame_count <= 0:
        raise LocalMediaDecodeError("WAV shape is invalid")
    if len(frames) != frame_count * channels * sample_width:
        raise LocalMediaDecodeError("WAV samples are truncated")
    duration_ticks = (frame_count * TIMEBASE + sample_rate // 2) // sample_rate
    return AudioBytesProbe("pcm_s16le", sample_rate, channels, duration_ticks)
