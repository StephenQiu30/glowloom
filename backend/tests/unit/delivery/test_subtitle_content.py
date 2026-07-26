from __future__ import annotations

from uuid import UUID

import pytest
from pydantic import ValidationError
from schemas.subtitles import (
    SubtitleContentV1,
    SubtitleCueV1,
    SubtitleMappingInvalid,
    subtitle_content_hash,
    validate_speech_mapping,
)
from services.subtitle_srt import render_srt

SHOT_ONE = UUID("00000000-0000-0000-0000-000000000101")
SHOT_TWO = UUID("00000000-0000-0000-0000-000000000102")
LINE_ONE = UUID("00000000-0000-0000-0000-000000000201")
LINE_TWO = UUID("00000000-0000-0000-0000-000000000202")


def cue(
    ordinal: int,
    *,
    shot_id: UUID,
    speech_line_id: UUID,
    start_ticks: int,
    duration_ticks: int = 90000,
    shot_start_ticks: int = 0,
    shot_end_ticks: int = 450000,
) -> SubtitleCueV1:
    return SubtitleCueV1(
        cue_id=UUID(f"00000000-0000-0000-0000-{ordinal:012d}"),
        ordinal=ordinal,
        speech_line_id=speech_line_id,
        shot_id=shot_id,
        text=f"第{ordinal}句旁白",
        voice_id="narrator_female",
        source_text_hash=f"{ordinal:064x}",
        start_ticks=start_ticks,
        end_ticks=start_ticks + duration_ticks,
        tts_duration_ticks=duration_ticks,
        shot_start_ticks=shot_start_ticks,
        shot_end_ticks=shot_end_ticks,
    )


def content(*cues: SubtitleCueV1) -> SubtitleContentV1:
    return SubtitleContentV1(language="zh-CN", cues=cues)


def test_subtitle_contract_has_stable_hash_mapping_and_srt() -> None:
    first = cue(
        1,
        shot_id=SHOT_ONE,
        speech_line_id=LINE_ONE,
        start_ticks=0,
    )
    second = cue(
        2,
        shot_id=SHOT_TWO,
        speech_line_id=LINE_TWO,
        start_ticks=450000,
        shot_start_ticks=450000,
        shot_end_ticks=900000,
    )
    value = content(first, second)

    validate_speech_mapping(value, (LINE_ONE, LINE_TWO))
    assert subtitle_content_hash(value) == subtitle_content_hash(
        SubtitleContentV1.model_validate(value.model_dump(mode="json"))
    )
    assert render_srt(value) == (
        "1\n00:00:00,000 --> 00:00:01,000\n第1句旁白\n\n"
        "2\n00:00:05,000 --> 00:00:06,000\n第2句旁白\n"
    )


@pytest.mark.parametrize(
    "changed",
    [
        {"end_ticks": 0},
        {"end_ticks": 89999},
        {"start_ticks": -1, "end_ticks": 89999},
        {"start_ticks": 400000, "end_ticks": 490000},
    ],
)
def test_cue_rejects_negative_mismatched_or_outside_timing(
    changed: dict[str, int],
) -> None:
    raw = cue(
        1,
        shot_id=SHOT_ONE,
        speech_line_id=LINE_ONE,
        start_ticks=0,
    ).model_dump()
    with pytest.raises(ValidationError):
        SubtitleCueV1.model_validate({**raw, **changed})


def test_content_rejects_order_overlap_duplicate_and_oversized_text() -> None:
    first = cue(
        1,
        shot_id=SHOT_ONE,
        speech_line_id=LINE_ONE,
        start_ticks=0,
    )
    overlapping = cue(
        2,
        shot_id=SHOT_ONE,
        speech_line_id=LINE_TWO,
        start_ticks=45000,
    )
    with pytest.raises(ValidationError):
        content(first, overlapping)
    with pytest.raises(ValidationError):
        content(first.model_copy(update={"ordinal": 2}))
    with pytest.raises(ValidationError):
        content(
            first,
            first.model_copy(
                update={"ordinal": 2, "start_ticks": 90000, "end_ticks": 180000}
            ),
        )
    with pytest.raises(ValidationError):
        SubtitleCueV1.model_validate({**first.model_dump(), "text": "字" * 501})


def test_mapping_rejects_missing_or_unexpected_speech_line() -> None:
    value = content(
        cue(
            1,
            shot_id=SHOT_ONE,
            speech_line_id=LINE_ONE,
            start_ticks=0,
        )
    )
    with pytest.raises(SubtitleMappingInvalid):
        validate_speech_mapping(value, (LINE_ONE, LINE_TWO))


def test_srt_rounds_ticks_half_up_to_milliseconds() -> None:
    value = content(
        cue(
            1,
            shot_id=SHOT_ONE,
            speech_line_id=LINE_ONE,
            start_ticks=45,
            duration_ticks=90000,
        )
    )
    assert "00:00:00,001 --> 00:00:01,001" in render_srt(value)
