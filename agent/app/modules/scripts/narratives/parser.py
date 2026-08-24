import re
from dataclasses import dataclass
from typing import Literal

NarrativeKind = Literal["scene_heading", "action", "dialogue", "narration"]

_EPISODE_MARKER = re.compile(
    r"^(?:第[一二三四五六七八九十百零〇两\d]+(?:集|话|章)|EP(?:ISODE)?\s*\d+)\s*$",
    re.IGNORECASE,
)
_TITLE = re.compile(r"^[《【].+[》】]$")
_SCENE_HEADING = re.compile(
    r"^(?:内景|外景|内/外|外/内|INT\.?|EXT\.?|INT\.?/EXT\.?)\s*[·.、\- ]",
    re.IGNORECASE,
)
_SPEAKER_PREFIX = re.compile(r"^[^：:\n]{1,30}[：:]")
_NARRATION_PREFIX = re.compile(r"^(?:旁白|系统播报|画外音|内心独白)[：:]")
_MARKDOWN_HEADING = re.compile(r"^#{1,6}\s+\S")


@dataclass(frozen=True, slots=True)
class ParsedUnit:
    kind: NarrativeKind
    source_start: int
    source_end: int
    exact_text: str


def _kind(text: str) -> NarrativeKind | None:
    if _MARKDOWN_HEADING.match(text):
        return None
    if _EPISODE_MARKER.fullmatch(text) or _TITLE.fullmatch(text):
        return None
    if _SCENE_HEADING.match(text):
        return "scene_heading"
    if _NARRATION_PREFIX.match(text):
        return "narration"
    if _SPEAKER_PREFIX.match(text):
        return "dialogue"
    return "action"


def parse_narrative_units(body: str) -> list[ParsedUnit]:
    units: list[ParsedUnit] = []
    offset = 0
    for line in body.splitlines(keepends=True):
        content = line.rstrip("\r\n")
        leading = len(content) - len(content.lstrip())
        trailing = len(content.rstrip())
        exact = content[leading:trailing]
        if exact:
            kind = _kind(exact)
            if kind is not None:
                units.append(
                    ParsedUnit(
                        kind=kind,
                        source_start=offset + leading,
                        source_end=offset + trailing,
                        exact_text=exact,
                    )
                )
        offset += len(line)
    if body and not body.endswith(("\n", "\r")) and offset < len(body):
        offset = len(body)
    return units
