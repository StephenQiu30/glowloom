from __future__ import annotations

from typing import Annotated, Literal

from pydantic import BaseModel, ConfigDict, Field

PositiveInt = Annotated[int, Field(strict=True, gt=0)]
MediaKind = Literal["image", "video", "audio"]


class ProbeSummary(BaseModel):
    model_config = ConfigDict(strict=True, extra="forbid", frozen=True)


class ImageProbeSummaryV1(ProbeSummary):
    codec: str = Field(min_length=1)
    width: PositiveInt
    height: PositiveInt


class VideoProbeSummaryV1(ProbeSummary):
    codec: str = Field(min_length=1)
    pixel_format: str = Field(min_length=1)
    width: PositiveInt
    height: PositiveInt
    frame_rate: str = Field(pattern=r"^[1-9][0-9]*/[1-9][0-9]*$")
    duration_ticks: PositiveInt
    timebase: PositiveInt
    audio_stream_count: Annotated[int, Field(strict=True, ge=0)]


class AudioProbeSummaryV1(ProbeSummary):
    codec: str = Field(min_length=1)
    sample_rate: PositiveInt
    channels: PositiveInt
    duration_ticks: PositiveInt
    timebase: PositiveInt


MediaProbeSummary = ImageProbeSummaryV1 | VideoProbeSummaryV1 | AudioProbeSummaryV1
