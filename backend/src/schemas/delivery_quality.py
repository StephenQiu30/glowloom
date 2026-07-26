from __future__ import annotations

from typing import Literal

from pydantic import Field

from schemas.common import StrictContract


class DeliveryProbeSummaryV1(StrictContract):
    schema_version: Literal["delivery-probe-v1"] = "delivery-probe-v1"
    video_codec: Literal["h264"]
    pixel_format: Literal["yuv420p"]
    width: Literal[720]
    height: Literal[1280]
    frame_rate: Literal["24/1"]
    audio_codec: Literal["aac"]
    audio_sample_rate: Literal[48000]
    audio_channels: Literal[2]
    duration_ticks: int = Field(ge=2_700_000, le=5_400_000)
    video_start_ticks: int
    video_duration_ticks: int
    audio_start_ticks: int
    audio_duration_ticks: int
    timebase: Literal[90000] = 90000
