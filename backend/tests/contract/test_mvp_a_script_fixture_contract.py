import json
from enum import StrEnum
from pathlib import Path

import pytest
from pydantic import BaseModel, ConfigDict, Field, TypeAdapter, model_validator

FIXTURE_DIRECTORY = Path(__file__).parents[1] / "fixtures/mvp_a"
FORMAT_CASES_FILE = FIXTURE_DIRECTORY / "script_format_cases.json"


class ExpectedAnalysis(StrEnum):
    DETERMINISTIC = "deterministic"
    REJECTED = "rejected"
    AI_CANDIDATE_REQUIRED = "ai_candidate_required"


class FormatProblemKind(StrEnum):
    NUMBER_GAP = "number_gap"
    DUPLICATE_NUMBER = "duplicate_number"
    NUMBER_OUT_OF_ORDER = "number_out_of_order"
    EMPTY_EPISODE = "empty_episode"
    NO_MARKER = "no_marker"
    PREAMBLE_REQUIRES_DECISION = "preamble_requires_decision"


class EpisodeMarkerFixture(BaseModel):
    model_config = ConfigDict(extra="forbid")

    episode_number: int = Field(ge=1, le=10)
    marker_text: str = Field(min_length=1)
    line_number: int = Field(ge=1)
    start_codepoint: int = Field(ge=0)
    end_codepoint: int = Field(gt=0)

    @model_validator(mode="after")
    def validate_span(self) -> "EpisodeMarkerFixture":
        if self.end_codepoint <= self.start_codepoint:
            raise ValueError("marker span must be a non-empty half-open range")
        return self


class ScriptFormatCase(BaseModel):
    model_config = ConfigDict(extra="forbid")

    fixture_id: str = Field(pattern=r"^mvp-a-format-[a-z0-9-]+-001$")
    description: str = Field(min_length=1)
    input_text: str = Field(min_length=1, max_length=100_000)
    expected_analysis: ExpectedAnalysis
    expected_markers: list[EpisodeMarkerFixture]
    expected_problem_kinds: list[FormatProblemKind]


def load_format_cases() -> list[ScriptFormatCase]:
    raw: object = json.loads(FORMAT_CASES_FILE.read_text(encoding="utf-8"))
    return TypeAdapter(list[ScriptFormatCase]).validate_python(raw)


def _line_number_at(text: str, start: int) -> int:
    return text.count("\n", 0, start) + 1


def test_format_fixture_matrix_covers_the_mvp_a_pre_gate() -> None:
    fixtures = load_format_cases()

    assert {fixture.fixture_id for fixture in fixtures} == {
        "mvp-a-format-explicit-five-001",
        "mvp-a-format-number-gap-001",
        "mvp-a-format-duplicate-number-001",
        "mvp-a-format-empty-episode-001",
        "mvp-a-format-no-marker-001",
        "mvp-a-format-unicode-codepoint-001",
        "mvp-a-format-number-out-of-order-001",
        "mvp-a-format-preamble-001",
        "mvp-a-format-inline-title-001",
        "mvp-a-format-crlf-whitespace-001",
    }
    assert {problem for fixture in fixtures for problem in fixture.expected_problem_kinds} == set(
        FormatProblemKind
    )


def test_marker_expectations_use_exact_unicode_codepoint_ranges() -> None:
    for fixture in load_format_cases():
        previous_end = 0
        for marker in fixture.expected_markers:
            assert marker.start_codepoint >= previous_end
            assert fixture.input_text[marker.start_codepoint : marker.end_codepoint] == (
                marker.marker_text
            )
            line_start = fixture.input_text.rfind("\n", 0, marker.start_codepoint) + 1
            line_end = fixture.input_text.find("\n", marker.end_codepoint)
            if line_end < 0:
                line_end = len(fixture.input_text)
            assert fixture.input_text[line_start : marker.start_codepoint].strip() == ""
            assert fixture.input_text[marker.end_codepoint : line_end].strip() == ""
            assert marker.line_number == _line_number_at(fixture.input_text, marker.start_codepoint)
            previous_end = marker.end_codepoint


def test_valid_explicit_fixture_has_five_contiguous_episode_numbers() -> None:
    fixture = next(
        case for case in load_format_cases() if case.fixture_id == "mvp-a-format-explicit-five-001"
    )

    assert fixture.expected_analysis is ExpectedAnalysis.DETERMINISTIC
    assert fixture.expected_problem_kinds == []
    assert [marker.episode_number for marker in fixture.expected_markers] == [1, 2, 3, 4, 5]


def test_unmarked_fixture_requires_an_ai_candidate_but_does_not_invent_a_boundary() -> None:
    fixture = next(
        case for case in load_format_cases() if case.fixture_id == "mvp-a-format-no-marker-001"
    )

    assert fixture.expected_analysis is ExpectedAnalysis.AI_CANDIDATE_REQUIRED
    assert fixture.expected_markers == []
    assert fixture.expected_problem_kinds == [FormatProblemKind.NO_MARKER]


def test_preamble_fixture_keeps_deterministic_markers_but_requires_a_user_decision() -> None:
    fixture = next(
        case for case in load_format_cases() if case.fixture_id == "mvp-a-format-preamble-001"
    )

    assert fixture.expected_analysis is ExpectedAnalysis.DETERMINISTIC
    assert [marker.episode_number for marker in fixture.expected_markers] == [1, 2]
    assert fixture.expected_problem_kinds == [FormatProblemKind.PREAMBLE_REQUIRES_DECISION]


def test_unicode_fixture_proves_offsets_are_not_utf16_code_units_or_utf8_bytes() -> None:
    fixture = next(
        case
        for case in load_format_cases()
        if case.fixture_id == "mvp-a-format-unicode-codepoint-001"
    )

    assert any(ord(character) > 0xFFFF for character in fixture.input_text)
    assert len(fixture.input_text) != len(fixture.input_text.encode("utf-16-le")) // 2
    assert len(fixture.input_text) != len(fixture.input_text.encode("utf-8"))


def test_fixture_contract_rejects_more_than_100k_codepoints() -> None:
    with pytest.raises(ValueError, match="at most 100000 characters"):
        ScriptFormatCase.model_validate(
            {
                "fixture_id": "mvp-a-format-over-limit-001",
                "description": "synthetic：超限输入",
                "input_text": "甲" * 100_001,
                "expected_analysis": "rejected",
                "expected_markers": [],
                "expected_problem_kinds": [],
            }
        )


def test_committed_format_fixtures_are_synthetic_and_contain_no_external_reference() -> None:
    serialized = FORMAT_CASES_FILE.read_text(encoding="utf-8").lower()

    assert "synthetic" in serialized
    assert "http://" not in serialized
    assert "https://" not in serialized
    for forbidden_field in ("real_name", "phone", "email", "address", "id_card"):
        assert f'"{forbidden_field}"' not in serialized
