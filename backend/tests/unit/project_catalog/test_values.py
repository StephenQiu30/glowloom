from __future__ import annotations

from hashlib import sha256

import pytest

from lanverse.modules.project_catalog.domain.values import (
    ProductionSpec,
    ProjectCatalogValidationError,
    ProjectTitle,
    SourceTextV1,
)


def source_with_length(length: int) -> str:
    return "汉" + "a" * (length - 1)


@pytest.mark.parametrize(
    ("length", "accepted"),
    [(299, False), (300, True), (3000, True), (3001, False)],
)
def test_source_text_enforces_normalized_codepoint_bounds(
    length: int, accepted: bool
) -> None:
    raw = source_with_length(length)

    if accepted:
        value = SourceTextV1.create(raw)
        assert value.codepoint_count == length
        assert value.content == raw
    else:
        with pytest.raises(ProjectCatalogValidationError) as raised:
            SourceTextV1.create(raw)
        assert raised.value.code == "SOURCE_LENGTH_OUT_OF_RANGE"
        assert raised.value.metadata == {"actual": length, "minimum": 300, "maximum": 3000}


def test_source_text_normalizes_line_endings_nfc_and_edge_whitespace() -> None:
    raw = "\u3000汉e\u0301\r\n内容\r" + "a" * 294 + "\u00a0"

    value = SourceTextV1.create(raw)

    assert value.content == "汉é\n内容\n" + "a" * 294
    assert value.codepoint_count == 300
    assert value.sha256 == sha256(value.content.encode()).hexdigest()
    assert value.normalization_version == "text-normalization-v1"


@pytest.mark.parametrize(
    ("raw", "code", "position"),
    [
        ("a" * 150 + "\x00" + "汉" + "a" * 149, "SOURCE_FORBIDDEN_CONTROL", 150),
        ("a" * 150 + "\u0085" + "汉" + "a" * 149, "SOURCE_FORBIDDEN_CONTROL", 150),
        ("a" * 150 + "\ufdd0" + "汉" + "a" * 149, "SOURCE_NONCHARACTER", 150),
        ("a" * 150 + "\U0010ffff" + "汉" + "a" * 149, "SOURCE_NONCHARACTER", 150),
        ("a" * 150 + "\ud800" + "汉" + "a" * 149, "SOURCE_INVALID_SCALAR", 150),
    ],
)
def test_source_text_rejects_first_forbidden_codepoint(
    raw: str, code: str, position: int
) -> None:
    with pytest.raises(ProjectCatalogValidationError) as raised:
        SourceTextV1.create(raw)

    assert raised.value.code == code
    assert raised.value.metadata == {"position": position}


def test_source_text_requires_unicode_15_1_han_script() -> None:
    with pytest.raises(ProjectCatalogValidationError) as raised:
        SourceTextV1.create("a" * 300)
    assert raised.value.code == "SOURCE_HAN_REQUIRED"

    assert SourceTextV1.create("a" * 299 + "\U00031350").codepoint_count == 300


@pytest.mark.parametrize(
    ("raw", "expected"),
    [
        (" 剧名 ", "剧名"),
        ("e\u0301", "é"),
        ("a", "a"),
        ("汉" * 120, "汉" * 120),
    ],
)
def test_project_title_normalizes_and_accepts_boundaries(raw: str, expected: str) -> None:
    assert ProjectTitle.create(raw).value == expected


@pytest.mark.parametrize("raw", ["", "\u3000", "a" * 121])
def test_project_title_rejects_out_of_range_values(raw: str) -> None:
    with pytest.raises(ProjectCatalogValidationError) as raised:
        ProjectTitle.create(raw)
    assert raised.value.code == "PROJECT_TITLE_LENGTH_OUT_OF_RANGE"


def test_production_spec_is_the_only_supported_mvp_profile() -> None:
    assert ProductionSpec.standard().as_dict() == {
        "aspect_ratio": "9:16",
        "width": 720,
        "height": 1280,
        "fps": 24,
        "timebase": 90000,
        "target_min_ticks": 2700000,
        "target_max_ticks": 5400000,
    }
