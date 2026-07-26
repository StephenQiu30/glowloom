from __future__ import annotations

from typing import Literal
from uuid import UUID

from pydantic import Field, model_validator

from schemas.common import StrictContract
from schemas.story_content import VoiceId, canonical_content_hash


class SubtitleMappingInvalid(ValueError):
    pass


class SubtitleCueV1(StrictContract):
    cue_id: UUID
    ordinal: int = Field(ge=1)
    speech_line_id: UUID
    shot_id: UUID
    text: str = Field(min_length=1, max_length=500)
    voice_id: VoiceId
    source_text_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    start_ticks: int = Field(ge=0)
    end_ticks: int = Field(gt=0)
    tts_duration_ticks: int = Field(gt=0)
    shot_start_ticks: int = Field(ge=0)
    shot_end_ticks: int = Field(gt=0)

    @model_validator(mode="after")
    def validate_timing(self) -> SubtitleCueV1:
        if self.end_ticks != self.start_ticks + self.tts_duration_ticks:
            raise ValueError("cue end must equal start plus TTS duration")
        if not (
            self.shot_start_ticks <= self.start_ticks
            and self.end_ticks <= self.shot_end_ticks
        ):
            raise ValueError("cue must stay inside its shot")
        return self


class SubtitleContentV1(StrictContract):
    schema_version: Literal["subtitle-v1"] = "subtitle-v1"
    language: str = Field(min_length=1, max_length=35)
    timebase: Literal[90000] = 90000
    cues: tuple[SubtitleCueV1, ...] = Field(min_length=1, max_length=100)

    @model_validator(mode="after")
    def validate_timeline(self) -> SubtitleContentV1:
        if [cue.ordinal for cue in self.cues] != list(range(1, len(self.cues) + 1)):
            raise ValueError("cue ordinals must be continuous")
        for attribute in ("cue_id", "speech_line_id"):
            values = [getattr(cue, attribute) for cue in self.cues]
            if len(values) != len(set(values)):
                raise ValueError(f"{attribute} values must be unique")
        if any(
            current.start_ticks < previous.end_ticks
            for previous, current in zip(self.cues, self.cues[1:], strict=False)
        ):
            raise ValueError("cues must be ordered and non-overlapping")
        if sum(len(cue.text) for cue in self.cues) > 20000:
            raise ValueError("episode subtitle text is too long")
        return self


def validate_speech_mapping(
    content: SubtitleContentV1, expected_speech_line_ids: tuple[UUID, ...]
) -> None:
    actual = tuple(cue.speech_line_id for cue in content.cues)
    if len(expected_speech_line_ids) != len(set(expected_speech_line_ids)):
        raise SubtitleMappingInvalid("expected speech line ids must be unique")
    if set(actual) != set(expected_speech_line_ids):
        raise SubtitleMappingInvalid("every speech line requires exactly one cue")


def subtitle_content_hash(content: SubtitleContentV1) -> str:
    return canonical_content_hash(content)
