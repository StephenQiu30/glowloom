import json
from enum import StrEnum
from pathlib import Path

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
    EMPTY_EPISODE = "empty_episode"
    NO_MARKER = "no_marker"


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
    }
    assert {
        problem
        for fixture in fixtures
        for problem in fixture.expected_problem_kinds
    } == set(FormatProblemKind)


def test_marker_expectations_use_exact_unicode_codepoint_ranges() -> None:
    for fixture in load_format_cases():
        previous_end = 0
        for marker in fixture.expected_markers:
            assert marker.start_codepoint >= previous_end
            assert fixture.input_text[marker.start_codepoint : marker.end_codepoint] == (
                marker.marker_text
            )
            starts_on_line_boundary = (
                marker.start_codepoint == 0
                or fixture.input_text[marker.start_codepoint - 1] == "\n"
            )
            assert starts_on_line_boundary
            assert marker.end_codepoint == len(fixture.input_text) or fixture.input_text[
                marker.end_codepoint
            ] == "\n"
            assert marker.line_number == _line_number_at(fixture.input_text, marker.start_codepoint)
            previous_end = marker.end_codepoint


def test_valid_explicit_fixture_has_five_contiguous_episode_numbers() -> None:
    fixture = next(
        case
        for case in load_format_cases()
        if case.fixture_id == "mvp-a-format-explicit-five-001"
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


def test_unicode_fixture_proves_offsets_are_not_utf16_code_units_or_utf8_bytes() -> None:
    fixture = next(
        case
        for case in load_format_cases()
        if case.fixture_id == "mvp-a-format-unicode-codepoint-001"
    )

    assert any(ord(character) > 0xFFFF for character in fixture.input_text)
    assert len(fixture.input_text) != len(fixture.input_text.encode("utf-16-le")) // 2
    assert len(fixture.input_text) != len(fixture.input_text.encode("utf-8"))


def test_committed_format_fixtures_are_synthetic_and_contain_no_external_reference() -> None:
    serialized = FORMAT_CASES_FILE.read_text(encoding="utf-8").lower()

    assert "synthetic" in serialized
    assert "http://" not in serialized
    assert "https://" not in serialized
    for forbidden_field in ("real_name", "phone", "email", "address", "id_card"):
        assert f'"{forbidden_field}"' not in serialized
