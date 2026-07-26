from __future__ import annotations

import hashlib
import unicodedata
from dataclasses import dataclass
from typing import ClassVar

from resources.unicode_15_1 import is_han, strip_white_space


class ProjectCatalogValidationError(ValueError):
    def __init__(self, code: str, *, metadata: dict[str, int] | None = None) -> None:
        super().__init__(code)
        self.code = code
        self.metadata = metadata or {}


def _invalid_codepoint(value: str) -> tuple[str, int] | None:
    for position, character in enumerate(value):
        codepoint = ord(character)
        if 0xD800 <= codepoint <= 0xDFFF:
            return "SOURCE_INVALID_SCALAR", position
        if (codepoint <= 0x001F and codepoint not in {0x0009, 0x000A}) or (
            0x007F <= codepoint <= 0x009F
        ):
            return "SOURCE_FORBIDDEN_CONTROL", position
        if 0xFDD0 <= codepoint <= 0xFDEF or codepoint & 0xFFFF in {0xFFFE, 0xFFFF}:
            return "SOURCE_NONCHARACTER", position
    return None


@dataclass(frozen=True, slots=True)
class ProjectTitle:
    value: str

    @classmethod
    def create(cls, raw: str) -> ProjectTitle:
        normalized = strip_white_space(unicodedata.normalize("NFC", raw))
        if not 1 <= len(normalized) <= 120:
            raise ProjectCatalogValidationError("PROJECT_TITLE_LENGTH_OUT_OF_RANGE")
        return cls(normalized)


@dataclass(frozen=True, slots=True)
class SourceTextV1:
    normalization_version: ClassVar[str] = "text-normalization-v1"
    content: str
    codepoint_count: int
    sha256: str

    @classmethod
    def create(cls, raw: str) -> SourceTextV1:
        scalar_error = next(
            (
                position
                for position, character in enumerate(raw)
                if 0xD800 <= ord(character) <= 0xDFFF
            ),
            None,
        )
        if scalar_error is not None:
            raise ProjectCatalogValidationError(
                "SOURCE_INVALID_SCALAR", metadata={"position": scalar_error}
            )
        normalized = strip_white_space(
            unicodedata.normalize("NFC", raw.replace("\r\n", "\n").replace("\r", "\n"))
        )
        count = len(normalized)
        if not 300 <= count <= 3000:
            raise ProjectCatalogValidationError(
                "SOURCE_LENGTH_OUT_OF_RANGE",
                metadata={"actual": count, "minimum": 300, "maximum": 3000},
            )
        invalid = _invalid_codepoint(normalized)
        if invalid is not None:
            code, position = invalid
            raise ProjectCatalogValidationError(code, metadata={"position": position})
        if not any(is_han(character) for character in normalized):
            raise ProjectCatalogValidationError("SOURCE_HAN_REQUIRED")
        digest = hashlib.sha256(normalized.encode("utf-8")).hexdigest()
        return cls(content=normalized, codepoint_count=count, sha256=digest)


@dataclass(frozen=True, slots=True)
class ProductionSpec:
    aspect_ratio: ClassVar[str] = "9:16"
    width: ClassVar[int] = 720
    height: ClassVar[int] = 1280
    fps: ClassVar[int] = 24
    timebase: ClassVar[int] = 90000
    target_min_ticks: ClassVar[int] = 2700000
    target_max_ticks: ClassVar[int] = 5400000

    @classmethod
    def standard(cls) -> ProductionSpec:
        return cls()

    def as_dict(self) -> dict[str, str | int]:
        return {
            "aspect_ratio": self.aspect_ratio,
            "width": self.width,
            "height": self.height,
            "fps": self.fps,
            "timebase": self.timebase,
            "target_min_ticks": self.target_min_ticks,
            "target_max_ticks": self.target_max_ticks,
        }
