from __future__ import annotations

from dataclasses import dataclass
from pathlib import PurePosixPath

from integrations.ai.deterministic_media import TIMEBASE
from schemas.rendering import RenderRecipeV1

FRAME_TICKS = TIMEBASE // 24


@dataclass(frozen=True, slots=True)
class RenderVideoSource:
    data: bytes
    duration_ticks: int

    def __post_init__(self) -> None:
        if not self.data or self.duration_ticks <= 0 or self.duration_ticks % FRAME_TICKS:
            raise ValueError("render video source is invalid")


@dataclass(frozen=True, slots=True)
class RenderAudioSource:
    data: bytes
    start_ticks: int

    def __post_init__(self) -> None:
        if not self.data or self.start_ticks < 0:
            raise ValueError("render audio source is invalid")


@dataclass(frozen=True, slots=True)
class RenderSources:
    videos: tuple[RenderVideoSource, ...]
    audios: tuple[RenderAudioSource, ...]
    subtitles_srt: str

    def __post_init__(self) -> None:
        if not 6 <= len(self.videos) <= 10:
            raise ValueError("render requires between 6 and 10 videos")
        if not 1 <= len(self.audios) <= 100:
            raise ValueError("render requires between 1 and 100 audio clips")
        if not self.subtitles_srt or len(self.subtitles_srt) > 100_000:
            raise ValueError("render subtitles are invalid")


def build_ffmpeg_arguments(sources: RenderSources, recipe: RenderRecipeV1) -> tuple[str, ...]:
    inputs = tuple(
        value
        for name in (
            *(f"/work/video-{index:02d}.mp4" for index in range(len(sources.videos))),
            *(f"/work/audio-{index:03d}.wav" for index in range(len(sources.audios))),
        )
        for value in ("-i", name)
    )
    video_filters = []
    for index, video_source in enumerate(sources.videos):
        duration = _seconds(video_source.duration_ticks)
        video_filters.append(
            f"[{index}:v:0]scale={recipe.width}:{recipe.height}:"
            "force_original_aspect_ratio=decrease,"
            f"pad={recipe.width}:{recipe.height}:(ow-iw)/2:(oh-ih)/2:"
            f"color={recipe.padding_color},fps={recipe.fps},"
            f"tpad=stop_mode=clone:stop_duration=1,trim=duration={duration},"
            f"setpts=PTS-STARTPTS[video_{index}]"
        )
    concat = "".join(f"[video_{index}]" for index in range(len(sources.videos)))
    font_dir = PurePosixPath(recipe.font_file).parent
    video_filters.append(f"{concat}concat=n={len(sources.videos)}:v=1:a=0[video_concat]")
    video_filters.append(
        "[video_concat]subtitles=/work/subtitles.srt:"
        f"fontsdir={font_dir}:force_style='FontName={recipe.font_name},"
        "FontSize=32,Alignment=2,MarginV=64'[video_out]"
    )
    audio_offset = len(sources.videos)
    audio_filters = []
    for index, audio_source in enumerate(sources.audios):
        delay_samples = round(audio_source.start_ticks * recipe.audio_rate / TIMEBASE)
        audio_filters.append(
            f"[{audio_offset + index}:a:0]aresample={recipe.audio_rate},"
            "aformat=sample_fmts=fltp:channel_layouts=mono,"
            f"adelay={delay_samples}S:all=1[audio_{index}]"
        )
    mix = "".join(f"[audio_{index}]" for index in range(len(sources.audios)))
    total = _seconds(sum(item.duration_ticks for item in sources.videos))
    audio_filters.append(
        f"{mix}amix=inputs={len(sources.audios)}:duration=longest:normalize=0,"
        f"apad=pad_dur={total},atrim=duration={total},"
        "pan=stereo|c0=c0|c1=c0[audio_out]"
    )
    graph = ";".join((*video_filters, *audio_filters))
    return (
        "-hide_banner",
        "-loglevel",
        "error",
        "-y",
        *inputs,
        "-filter_complex",
        graph,
        "-map",
        "[video_out]",
        "-map",
        "[audio_out]",
        "-c:v",
        "libx264",
        "-preset",
        recipe.video_preset,
        "-pix_fmt",
        recipe.pixel_format,
        "-r",
        str(recipe.fps),
        "-c:a",
        recipe.audio_codec,
        "-b:a",
        recipe.audio_bitrate,
        "-ar",
        str(recipe.audio_rate),
        "-ac",
        str(recipe.audio_channels),
        "-t",
        total,
        "-map_metadata",
        "-1",
        "-metadata",
        "creation_time=1970-01-01T00:00:00Z",
        "-movflags",
        "+faststart",
        "-threads",
        "1",
        "/work/output.mp4",
    )


def _seconds(ticks: int) -> str:
    return f"{ticks / TIMEBASE:.6f}"
