from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import Field

from schemas.common import StrictContract


class RenderMediaRefV1(StrictContract):
    usage_type: Literal["shot_video", "speech_audio"]
    usage_id: UUID
    input_version_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    adoption_id: UUID
    candidate_id: UUID
    media_version_id: UUID
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    duration_ticks: int = Field(gt=0)
    timebase: Literal[90000] = 90000


class RenderInputRefsV1(StrictContract):
    schema_version: Literal["render-input-v1"] = "render-input-v1"
    shot_spec_version_id: UUID
    subtitle_version_id: UUID
    subtitle_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    video_adoptions: tuple[RenderMediaRefV1, ...] = Field(min_length=6, max_length=10)
    tts_adoptions: tuple[RenderMediaRefV1, ...] = Field(min_length=1, max_length=100)


class RenderSegmentV1(StrictContract):
    shot_id: UUID
    ordinal: int = Field(ge=1, le=10)
    start_ticks: int = Field(ge=0)
    end_ticks: int = Field(gt=0)
    duration_ticks: int = Field(gt=0)
    video_adoption_id: UUID
    tts_adoption_ids: tuple[UUID, ...]


class RenderRecipeV1(StrictContract):
    schema_version: Literal["render-recipe-v1"] = "render-recipe-v1"
    runtime_image: str = Field(
        pattern=r"^(?:[a-z0-9./_-]+@sha256:[0-9a-f]{64}|sha256:[0-9a-f]{64})$"
    )
    ffmpeg_version: str = Field(min_length=1, max_length=128, pattern=r".*\S.*")
    ffprobe_version: str = Field(min_length=1, max_length=128, pattern=r".*\S.*")
    font_name: str = Field(pattern=r"^[A-Za-z0-9][A-Za-z0-9 _-]{0,127}$")
    font_file: str = Field(
        pattern=(
            r"^/usr/share/fonts/(?:[A-Za-z0-9_.-]+/)*"
            r"[A-Za-z0-9_.-]+\.(?:otf|ttf|ttc)$"
        )
    )
    font_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    font_license: Literal["OFL-1.1", "Bitstream-Vera"]
    timebase: Literal[90000] = 90000
    width: Literal[720] = 720
    height: Literal[1280] = 1280
    fps: Literal[24] = 24
    audio_rate: Literal[48000] = 48000
    audio_channels: Literal[2] = 2
    video_codec: Literal["h264"] = "h264"
    video_preset: Literal["veryfast"] = "veryfast"
    pixel_format: Literal["yuv420p"] = "yuv420p"
    audio_codec: Literal["aac"] = "aac"
    audio_bitrate: Literal["192k"] = "192k"
    scale_mode: Literal["contain"] = "contain"
    padding_color: Literal["#000000"] = "#000000"
    video_tolerance_ticks: Literal[90000] = 90000
    remove_source_audio: Literal[True] = True
    preserve_tts_speed: Literal[True] = True
    burn_subtitles: Literal[True] = True


@dataclass(frozen=True, slots=True)
class RenderSnapshot:
    id: UUID
    episode_id: UUID
    submission_scope: str
    idempotency_key: str
    request_hash: str
    initial_task_id: UUID | None
    shot_spec_version_id: UUID
    subtitle_version_id: UUID
    input_refs: RenderInputRefsV1
    segments: tuple[RenderSegmentV1, ...]
    recipe: RenderRecipeV1
    recipe_hash: str
    content_hash: str
    created_at: datetime
