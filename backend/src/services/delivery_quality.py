from __future__ import annotations

from integrations.ai.deterministic_video import VideoProbe
from schemas.delivery_quality import DeliveryProbeSummaryV1

TIMEBASE = 90000
AV_TOLERANCE_TICKS = 9000
DELIVERY_TOLERANCE_TICKS = 90000
MIN_DURATION_TICKS = 30 * TIMEBASE
MAX_DURATION_TICKS = 60 * TIMEBASE


class DeliveryQualityInvalid(ValueError):
    pass


class DeliveryQualityPolicy:
    def validate(self, probe: VideoProbe, *, target_duration_ticks: int) -> DeliveryProbeSummaryV1:
        if probe.codec_name != "h264" or probe.pixel_format != "yuv420p":
            raise DeliveryQualityInvalid("video codec is invalid")
        if (probe.width, probe.height, probe.frame_rate) != (720, 1280, "24/1"):
            raise DeliveryQualityInvalid("video shape is invalid")
        if probe.audio_stream_count != 1 or probe.audio_codec_name != "aac":
            raise DeliveryQualityInvalid("exactly one AAC audio stream is required")
        if probe.audio_sample_rate != 48000:
            raise DeliveryQualityInvalid("audio sample rate is invalid")
        if probe.audio_channels != 2:
            raise DeliveryQualityInvalid("audio channel count is invalid")
        if not MIN_DURATION_TICKS <= target_duration_ticks <= MAX_DURATION_TICKS:
            raise DeliveryQualityInvalid("target duration is outside the MVP range")
        duration_ticks = round(probe.duration_seconds * TIMEBASE)
        if abs(duration_ticks - target_duration_ticks) > DELIVERY_TOLERANCE_TICKS:
            raise DeliveryQualityInvalid("delivery duration exceeds target tolerance")
        timings = self._timings(probe)
        if any(abs(value) > AV_TOLERANCE_TICKS for value in timings[::2]):
            raise DeliveryQualityInvalid("audio or video start exceeds tolerance")
        if any(
            abs(value - target_duration_ticks) > DELIVERY_TOLERANCE_TICKS
            for value in timings[1::2]
        ):
            raise DeliveryQualityInvalid("audio or video duration exceeds tolerance")
        return DeliveryProbeSummaryV1(
            video_codec="h264",
            pixel_format="yuv420p",
            width=720,
            height=1280,
            frame_rate="24/1",
            audio_codec="aac",
            audio_sample_rate=48000,
            audio_channels=2,
            duration_ticks=duration_ticks,
            video_start_ticks=timings[0],
            video_duration_ticks=timings[1],
            audio_start_ticks=timings[2],
            audio_duration_ticks=timings[3],
        )

    @staticmethod
    def _timings(probe: VideoProbe) -> tuple[int, int, int, int]:
        values = (
            probe.video_start_seconds,
            probe.video_duration_seconds,
            probe.audio_start_seconds,
            probe.audio_duration_seconds,
        )
        if any(value is None for value in values):
            raise DeliveryQualityInvalid("audio or video timing is missing")
        video_start, video_duration, audio_start, audio_duration = values
        assert video_start is not None and video_duration is not None
        assert audio_start is not None and audio_duration is not None
        return (
            round(video_start * TIMEBASE),
            round(video_duration * TIMEBASE),
            round(audio_start * TIMEBASE),
            round(audio_duration * TIMEBASE),
        )
