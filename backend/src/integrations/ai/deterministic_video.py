from __future__ import annotations

import asyncio
import json
import subprocess
from dataclasses import dataclass
from typing import Protocol, cast

from integrations.ai.deterministic_media import TIMEBASE, GeneratedMedia, _seed

FFMPEG_IMAGE = (
    "docker.io/jrottenberg/ffmpeg@"
    "sha256:83ef82d9850314baa3504821e2ea6598e40e2096ac8f967a842d31234be2be92"
)
FRAME_TICKS = TIMEBASE // 24
MIN_DURATION_TICKS = 3 * TIMEBASE
MAX_DURATION_TICKS = 8 * TIMEBASE


class MediaRuntimeError(RuntimeError):
    pass


@dataclass(frozen=True, slots=True)
class VideoProbe:
    codec_name: str
    pixel_format: str
    width: int
    height: int
    frame_rate: str
    duration_seconds: float
    audio_stream_count: int
    video_start_seconds: float | None = None
    video_duration_seconds: float | None = None
    audio_codec_name: str | None = None
    audio_sample_rate: int | None = None
    audio_channels: int | None = None
    audio_start_seconds: float | None = None
    audio_duration_seconds: float | None = None


class VideoRuntime(Protocol):
    async def render_color(self, color: str, duration_seconds: str) -> bytes: ...


class DockerFfmpegRuntime:
    def __init__(self, *, timeout_seconds: float = 30) -> None:
        self._timeout_seconds = timeout_seconds

    async def render_color(
        self,
        color: str,
        duration_seconds: str,
        *,
        width: int = 720,
        height: int = 1280,
    ) -> bytes:
        if width <= 0 or height <= 0:
            raise ValueError("video dimensions must be positive")
        arguments = (
            "-hide_banner",
            "-loglevel",
            "error",
            "-f",
            "lavfi",
            "-i",
            f"color=c=0x{color}:s={width}x{height}:r=24:d={duration_seconds}",
            "-an",
            "-c:v",
            "libx264",
            "-preset",
            "ultrafast",
            "-tune",
            "stillimage",
            "-pix_fmt",
            "yuv420p",
            "-threads",
            "1",
            "-metadata",
            "creation_time=1970-01-01T00:00:00Z",
            "-movflags",
            "+frag_keyframe+empty_moov+default_base_moof",
            "-f",
            "mp4",
            "pipe:1",
        )
        return await self._run("ffmpeg", arguments)

    async def probe(self, data: bytes) -> VideoProbe:
        arguments = (
            "-v",
            "error",
            "-show_entries",
            "stream=codec_type,codec_name,pix_fmt,width,height,avg_frame_rate,"
            "sample_rate,channels,start_time,duration:format=duration",
            "-of",
            "json",
            "-i",
            "pipe:0",
        )
        raw = await self._run("ffprobe", arguments, stdin=data)
        payload = cast(dict[str, object], json.loads(raw))
        streams = cast(list[dict[str, object]], payload.get("streams", []))
        videos = [stream for stream in streams if stream.get("codec_type") == "video"]
        audios = [stream for stream in streams if stream.get("codec_type") == "audio"]
        if len(videos) != 1:
            raise MediaRuntimeError("expected exactly one video stream")
        video = videos[0]
        audio = audios[0] if len(audios) == 1 else None
        media_format = cast(dict[str, object], payload.get("format", {}))
        try:
            return VideoProbe(
                codec_name=str(video["codec_name"]),
                pixel_format=str(video["pix_fmt"]),
                width=int(cast(int, video["width"])),
                height=int(cast(int, video["height"])),
                frame_rate=str(video["avg_frame_rate"]),
                duration_seconds=float(cast(str, media_format["duration"])),
                audio_stream_count=len(audios),
                video_start_seconds=_optional_float(video.get("start_time")),
                video_duration_seconds=_optional_float(video.get("duration")),
                audio_codec_name=str(audio["codec_name"]) if audio else None,
                audio_sample_rate=int(cast(str, audio["sample_rate"])) if audio else None,
                audio_channels=int(cast(int, audio["channels"])) if audio else None,
                audio_start_seconds=(_optional_float(audio.get("start_time")) if audio else None),
                audio_duration_seconds=(_optional_float(audio.get("duration")) if audio else None),
            )
        except (KeyError, TypeError, ValueError) as error:
            raise MediaRuntimeError("ffprobe returned an invalid response") from error

    async def verify_video_decode(self, data: bytes) -> None:
        arguments = (
            "-hide_banner",
            "-loglevel",
            "error",
            "-i",
            "pipe:0",
            "-map",
            "0:v:0",
            "-f",
            "null",
            "-",
        )
        await self._run("ffmpeg", arguments, stdin=data)

    async def _run(
        self,
        entrypoint: str,
        arguments: tuple[str, ...],
        *,
        stdin: bytes | None = None,
    ) -> bytes:
        docker_options = (
            "docker",
            "run",
            "--rm",
            "--pull=missing",
            "--network",
            "none",
            "--read-only",
            "--cap-drop",
            "ALL",
            "--security-opt",
            "no-new-privileges",
            "--pids-limit",
            "64",
            "--cpus",
            "1",
            "--memory",
            "512m",
            "--entrypoint",
            entrypoint,
        )
        stdin_option = ("-i",) if stdin is not None else ()
        command = (*docker_options, *stdin_option, FFMPEG_IMAGE, *arguments)
        try:
            result = await asyncio.to_thread(
                subprocess.run,
                command,
                input=stdin,
                capture_output=True,
                check=False,
                timeout=self._timeout_seconds,
            )
        except (OSError, subprocess.TimeoutExpired) as error:
            raise MediaRuntimeError("FFmpeg runtime is unavailable") from error
        if result.returncode != 0:
            detail = result.stderr.decode(errors="replace")[-1000:].strip()
            raise MediaRuntimeError(f"FFmpeg command failed: {detail}")
        return result.stdout


def _optional_float(value: object) -> float | None:
    if value in (None, "N/A"):
        return None
    return float(cast(str, value))


class DeterministicVideoProvider:
    def __init__(self, runtime: VideoRuntime) -> None:
        self._runtime = runtime
        self.call_count = 0

    async def generate(
        self,
        input_hash: str,
        output_slot: str,
        *,
        duration_ticks: int,
    ) -> GeneratedMedia:
        seed = _seed(input_hash, output_slot, "video-v1")
        if not MIN_DURATION_TICKS <= duration_ticks <= MAX_DURATION_TICKS:
            raise ValueError("duration must be between 3 and 8 seconds")
        if duration_ticks % FRAME_TICKS:
            raise ValueError("duration must align to whole video frames")
        self.call_count += 1
        data = await self._runtime.render_color(seed[:3].hex(), f"{duration_ticks / TIMEBASE:.6f}")
        return GeneratedMedia(
            output_slot=output_slot,
            content_type="video/mp4",
            data=data,
            width=720,
            height=1280,
            duration_ticks=duration_ticks,
        )
