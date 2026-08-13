import hashlib
import re
import unicodedata
from dataclasses import dataclass, field
from typing import Literal

AnalysisStatus = Literal["deterministic", "ai_candidate_required", "rejected"]
BlockKind = Literal[
    "preamble",
    "episode_marker",
    "scene_heading",
    "dialogue",
    "narration",
    "action",
    "separator",
]
IssueSeverity = Literal["warning", "blocking"]

ANALYZER_VERSION = "whole-script-lines-v1"
MAX_EXPLICIT_EPISODES = 10

_CHINESE_DIGITS = {
    "零": 0,
    "〇": 0,
    "一": 1,
    "二": 2,
    "两": 2,
    "三": 3,
    "四": 4,
    "五": 5,
    "六": 6,
    "七": 7,
    "八": 8,
    "九": 9,
}
_CHINESE_UNITS = {"十": 10, "百": 100, "千": 1000}
_CHINESE_MARKER = re.compile(
    r"第([0-9０-９零〇一二两三四五六七八九十百千]+)(?:集|话|章)"
)
_ENGLISH_MARKER = re.compile(
    r"(?:EPISODE|EP)\s*([0-9０-９]+)",
    re.IGNORECASE,
)
_SCENE_HEADING = re.compile(
    r"^(?:内景|外景|场景\s*\d+|INT\.?|EXT\.?|INT\s*/\s*EXT\.?)",
    re.IGNORECASE,
)
_NARRATION = re.compile(r"^(?:旁白|画外音|内心|独白)\s*[：:]")
_DIALOGUE = re.compile(r"^[^：:\r\n]{1,40}\s*[：:]")


def _empty_object_map() -> dict[str, object]:
    return {}


@dataclass(frozen=True, slots=True)
class EpisodeMarker:
    episode_number: int
    marker_text: str
    line_number: int
    start_codepoint: int
    end_codepoint: int


@dataclass(frozen=True, slots=True)
class NarrativeBlockDraft:
    position: int
    kind: BlockKind
    start_codepoint: int
    end_codepoint: int
    text_hash: str
    metadata: dict[str, object] = field(default_factory=_empty_object_map)


@dataclass(frozen=True, slots=True)
class FormatIssueDraft:
    code: str
    severity: IssueSeverity
    start_codepoint: int
    end_codepoint: int
    line_number: int
    column_number: int
    next_action: str
    details: dict[str, object] = field(default_factory=_empty_object_map)


@dataclass(frozen=True, slots=True)
class DocumentAnalysis:
    status: AnalysisStatus
    analyzer_version: str
    content_hash: str
    codepoint_count: int
    markers: tuple[EpisodeMarker, ...]
    blocks: tuple[NarrativeBlockDraft, ...]
    issues: tuple[FormatIssueDraft, ...]


@dataclass(frozen=True, slots=True)
class _Line:
    number: int
    start: int
    end: int
    content_end: int
    text: str


@dataclass(frozen=True, slots=True)
class _LocatedMarker:
    marker: EpisodeMarker
    line_start: int
    line_end: int


def _sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _lines(text: str) -> tuple[_Line, ...]:
    result: list[_Line] = []
    start = 0
    for number, raw_line in enumerate(text.splitlines(keepends=True), start=1):
        content = raw_line
        if content.endswith("\r\n"):
            content = content[:-2]
        elif content.endswith(("\r", "\n")):
            content = content[:-1]
        end = start + len(raw_line)
        result.append(
            _Line(
                number=number,
                start=start,
                end=end,
                content_end=start + len(content),
                text=raw_line,
            )
        )
        start = end
    return tuple(result)


def _episode_number(token: str) -> int | None:
    normalized = unicodedata.normalize("NFKC", token)
    if normalized.isdecimal():
        return int(normalized)
    total = 0
    current = 0
    for character in normalized:
        if character in _CHINESE_DIGITS:
            current = _CHINESE_DIGITS[character]
            continue
        unit = _CHINESE_UNITS.get(character)
        if unit is None:
            return None
        total += (current or 1) * unit
        current = 0
    value = total + current
    return value if value > 0 else None


def _marker_for_line(line: _Line) -> _LocatedMarker | None:
    without_ending = line.text[: line.content_end - line.start]
    detection_offset = 1 if line.number == 1 and without_ending.startswith("\ufeff") else 0
    candidate_source = without_ending[detection_offset:]
    candidate = candidate_source.strip()
    if not candidate:
        return None
    matched = _CHINESE_MARKER.fullmatch(candidate)
    if matched is None:
        matched = _ENGLISH_MARKER.fullmatch(candidate)
    if matched is None:
        return None
    number = _episode_number(matched.group(1))
    if number is None:
        return None
    leading_whitespace = len(candidate_source) - len(candidate_source.lstrip())
    marker_start = line.start + detection_offset + leading_whitespace
    marker_end = marker_start + len(candidate)
    return _LocatedMarker(
        marker=EpisodeMarker(
            episode_number=number,
            marker_text=candidate,
            line_number=line.number,
            start_codepoint=marker_start,
            end_codepoint=marker_end,
        ),
        line_start=line.start,
        line_end=line.end,
    )


def _issue_for_marker(
    code: str,
    located: _LocatedMarker,
    *,
    next_action: str,
    details: dict[str, object] | None = None,
) -> FormatIssueDraft:
    marker = located.marker
    return FormatIssueDraft(
        code=code,
        severity="blocking",
        start_codepoint=marker.start_codepoint,
        end_codepoint=marker.end_codepoint,
        line_number=marker.line_number,
        column_number=marker.start_codepoint - located.line_start + 1,
        next_action=next_action,
        details=details or {},
    )


def _structural_issues(
    text: str,
    markers: tuple[_LocatedMarker, ...],
) -> list[FormatIssueDraft]:
    if not markers:
        return [
            FormatIssueDraft(
                code="no_marker",
                severity="warning",
                start_codepoint=0,
                end_codepoint=min(1, len(text)),
                line_number=1,
                column_number=1,
                next_action="generate_episode_plan",
            )
        ]

    marker_values = [located.marker.episode_number for located in markers]
    duplicate_index = next(
        (
            index
            for index, value in enumerate(marker_values)
            if value in marker_values[:index]
        ),
        None,
    )
    issues: list[FormatIssueDraft] = []
    if duplicate_index is not None:
        duplicate = markers[duplicate_index]
        issues.append(
            _issue_for_marker(
                "duplicate_number",
                duplicate,
                next_action="renumber_episode_markers",
                details={"episode_number": duplicate.marker.episode_number},
            )
        )
    else:
        out_of_order_index = next(
            (
                index
                for index in range(1, len(marker_values))
                if marker_values[index] < marker_values[index - 1]
            ),
            None,
        )
        if out_of_order_index is not None:
            current = markers[out_of_order_index]
            issues.append(
                _issue_for_marker(
                    "number_out_of_order",
                    current,
                    next_action="reorder_episode_markers",
                    details={
                        "previous_episode_number": marker_values[out_of_order_index - 1],
                        "episode_number": current.marker.episode_number,
                    },
                )
            )
        else:
            gap_index = next(
                (
                    index
                    for index, value in enumerate(marker_values)
                    if value != index + 1
                ),
                None,
            )
            if gap_index is not None:
                current = markers[gap_index]
                issues.append(
                    _issue_for_marker(
                        "number_gap",
                        current,
                        next_action="renumber_episode_markers",
                        details={
                            "expected_episode_number": gap_index + 1,
                            "episode_number": current.marker.episode_number,
                        },
                    )
                )

    if len(markers) > MAX_EXPLICIT_EPISODES:
        marker = markers[MAX_EXPLICIT_EPISODES]
        issues.append(
            _issue_for_marker(
                "episode_limit_exceeded",
                marker,
                next_action="reduce_episode_count",
                details={"maximum_episode_count": MAX_EXPLICIT_EPISODES},
            )
        )

    if text[: markers[0].line_start].strip():
        first = markers[0].marker
        issues.append(
            FormatIssueDraft(
                code="preamble_requires_decision",
                severity="warning",
                start_codepoint=0,
                end_codepoint=markers[0].line_start,
                line_number=1,
                column_number=1,
                next_action="decide_preamble",
                details={"first_episode_number": first.episode_number},
            )
        )

    for index, located in enumerate(markers):
        segment_end = (
            markers[index + 1].line_start if index + 1 < len(markers) else len(text)
        )
        if not text[located.line_end : segment_end].strip():
            issues.append(
                _issue_for_marker(
                    "empty_episode",
                    located,
                    next_action="add_episode_content",
                    details={"episode_number": located.marker.episode_number},
                )
            )
            break
    return issues


def _block_kind(
    line: _Line,
    marker_by_line: dict[int, EpisodeMarker],
    first_marker_line: int | None,
) -> tuple[BlockKind, dict[str, object]]:
    marker = marker_by_line.get(line.number)
    if marker is not None:
        return (
            "episode_marker",
            {
                "episode_number": marker.episode_number,
                "marker_start_codepoint": marker.start_codepoint,
                "marker_end_codepoint": marker.end_codepoint,
            },
        )
    content = line.text.rstrip("\r\n")
    stripped = content.strip()
    if not stripped:
        return "separator", {}
    if first_marker_line is not None and line.number < first_marker_line:
        return "preamble", {}
    if _SCENE_HEADING.match(stripped):
        return "scene_heading", {}
    if _NARRATION.match(stripped):
        return "narration", {}
    if _DIALOGUE.match(stripped):
        return "dialogue", {}
    return "action", {}


def analyze_document(text: str) -> DocumentAnalysis:
    lines = _lines(text)
    located_markers = tuple(
        marker
        for line in lines
        if (marker := _marker_for_line(line)) is not None
    )
    issues: list[FormatIssueDraft] = []
    if text.startswith("\ufeff"):
        issues.append(
            FormatIssueDraft(
                code="utf8_bom_not_allowed",
                severity="blocking",
                start_codepoint=0,
                end_codepoint=1,
                line_number=1,
                column_number=1,
                next_action="remove_utf8_bom",
            )
        )
    issues.extend(_structural_issues(text, located_markers))

    marker_by_line = {
        located.marker.line_number: located.marker for located in located_markers
    }
    first_marker_line = (
        located_markers[0].marker.line_number if located_markers else None
    )
    blocks = tuple(
        NarrativeBlockDraft(
            position=position,
            kind=kind,
            start_codepoint=line.start,
            end_codepoint=line.end,
            text_hash=_sha256(text[line.start : line.end]),
            metadata=metadata,
        )
        for position, line in enumerate(lines, start=1)
        for kind, metadata in [
            _block_kind(line, marker_by_line, first_marker_line)
        ]
    )
    if any(issue.severity == "blocking" for issue in issues):
        status: AnalysisStatus = "rejected"
    elif not located_markers:
        status = "ai_candidate_required"
    else:
        status = "deterministic"
    return DocumentAnalysis(
        status=status,
        analyzer_version=ANALYZER_VERSION,
        content_hash=_sha256(text),
        codepoint_count=len(text),
        markers=tuple(located.marker for located in located_markers),
        blocks=blocks,
        issues=tuple(issues),
    )
