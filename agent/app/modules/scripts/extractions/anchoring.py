import re
import unicodedata
from dataclasses import dataclass
from hashlib import sha256
from typing import cast

from app.modules.scripts.extractions.schemas import (
    AssetCandidateProposal,
    DialogueCandidateProposal,
    SceneCandidateProposal,
    ScriptExtractionResult,
)
from app.modules.scripts.narratives.parser import parse_narrative_units

_SCREENPLAY_SPEAKER_CUE = re.compile(
    r"^(?P<speaker>[A-Z][A-Z0-9 .&'’/\-]{0,80}?)(?:\s+\((?P<modifier>[^()\n]{1,30})\))?$"
)
_TECHNICAL_SPEAKER_PREFIXES = (
    "CARD",
    "CLOSE ON",
    "CUT TO",
    "FADE",
    "INSERT",
    "SFX",
    "SUPER",
    "SYSTEM",
    "TITLE",
    "VFX",
)
_SCENE_PREFIXES = ("INT.", "EXT.", "I/E.", "INT/EXT.", "INT./EXT.")
_SENTENCE_END = re.compile(r"[.!?…][\"'’”)]*$")
_PERFORMANCE_PREFIX = re.compile(r"^\((?P<note>[^()\n]{1,100})\)\s*(?P<text>.*)$")


@dataclass(frozen=True, slots=True)
class _DialogueAnchor:
    source_start: int
    source_end: int
    exact_text: str
    speaker: str
    text: str
    dialogue_kind: str
    performance_note: str | None = None


def _normalized_source_text(value: str) -> str:
    normalized = unicodedata.normalize("NFKC", value).replace("：", ":")
    return "".join(normalized.split())


def _dialogue_parts(line: str) -> tuple[str, str] | None:
    colon_positions = [position for mark in ("：", ":") if (position := line.find(mark)) >= 0]
    if not colon_positions:
        return None
    position = min(colon_positions)
    speaker = line[:position].strip()
    dialogue = line[position + 1 :].strip()
    if not speaker or not dialogue:
        return None
    return speaker, dialogue


def _is_technical_speaker(value: str) -> bool:
    normalized = " ".join(value.upper().split())
    return any(
        normalized == prefix or normalized.startswith(f"{prefix} ")
        for prefix in _TECHNICAL_SPEAKER_PREFIXES
    )


def _screenplay_speaker_cue(value: str) -> tuple[str, str | None] | None:
    normalized = " ".join(value.split())
    if normalized.startswith(_SCENE_PREFIXES):
        return None
    match = _SCREENPLAY_SPEAKER_CUE.fullmatch(normalized)
    if match is None:
        return None
    speaker = match.group("speaker").strip()
    if _is_technical_speaker(speaker):
        return None
    return speaker, match.group("modifier")


def _dialogue_kind(modifier: str | None, *, narration: bool = False) -> str:
    if narration:
        return "narration"
    normalized = modifier.upper().replace(" ", "") if modifier is not None else ""
    if "V.O" in normalized or "O.S" in normalized:
        return "voice_over"
    return "spoken"


def _performed_text(value: str) -> tuple[str | None, str]:
    match = _PERFORMANCE_PREFIX.match(value.strip())
    if match is None:
        return None, value.strip()
    return match.group("note").strip(), match.group("text").strip()


def _in_scene(source_start: int, scene_ranges: dict[str, tuple[int, int]]) -> bool:
    return any(start <= source_start < end for start, end in scene_ranges.values())


def _dialogue_anchors(
    script_body: str,
    scene_ranges: dict[str, tuple[int, int]],
) -> list[_DialogueAnchor]:
    units = parse_narrative_units(script_body)
    anchors: list[_DialogueAnchor] = []
    used_ranges: set[tuple[int, int]] = set()

    for unit in units:
        parts = _dialogue_parts(unit.exact_text)
        if (
            unit.kind not in {"dialogue", "narration"}
            or parts is None
            or _is_technical_speaker(parts[0])
            or not _in_scene(unit.source_start, scene_ranges)
        ):
            continue
        anchor = _DialogueAnchor(
            source_start=unit.source_start,
            source_end=unit.source_end,
            exact_text=unit.exact_text,
            speaker=parts[0],
            text=parts[1],
            dialogue_kind=_dialogue_kind(None, narration=unit.kind == "narration"),
        )
        anchors.append(anchor)
        used_ranges.add((anchor.source_start, anchor.source_end))

    for index, cue_unit in enumerate(units):
        cue = _screenplay_speaker_cue(cue_unit.exact_text)
        if cue is None or not _in_scene(cue_unit.source_start, scene_ranges):
            continue
        speaker, modifier = cue
        content_index = index + 1
        if content_index >= len(units):
            continue
        content_unit = units[content_index]
        if (
            content_unit.kind == "scene_heading"
            or _screenplay_speaker_cue(content_unit.exact_text) is not None
            or not _in_scene(content_unit.source_start, scene_ranges)
        ):
            continue
        performance_note, first_text = _performed_text(content_unit.exact_text)
        if not first_text:
            continue
        text_lines = [first_text]
        content_end = content_unit.source_end
        continuation_index = content_index + 1
        while continuation_index < len(units):
            continuation = units[continuation_index]
            if (
                continuation.kind == "scene_heading"
                or _screenplay_speaker_cue(continuation.exact_text) is not None
                or not _in_scene(continuation.source_start, scene_ranges)
                or "\n\n" in script_body[content_end : continuation.source_start]
                or _SENTENCE_END.search(text_lines[-1]) is not None
            ):
                break
            text_lines.append(continuation.exact_text.strip())
            content_end = continuation.source_end
            continuation_index += 1
        source_range = (cue_unit.source_start, content_end)
        if source_range not in used_ranges:
            notes = [value for value in (modifier, performance_note) if value]
            anchors.append(
                _DialogueAnchor(
                    source_start=source_range[0],
                    source_end=source_range[1],
                    exact_text=script_body[source_range[0] : source_range[1]],
                    speaker=speaker,
                    text=" ".join(text_lines),
                    dialogue_kind=_dialogue_kind(modifier),
                    performance_note="; ".join(notes) or None,
                )
            )
            used_ranges.add(source_range)

        if continuation_index >= len(units):
            continue
        continuation = units[continuation_index]
        extra_note, extra_text = _performed_text(continuation.exact_text)
        if (
            extra_note is None
            or not extra_text
            or not _in_scene(continuation.source_start, scene_ranges)
            or _screenplay_speaker_cue(continuation.exact_text) is not None
        ):
            continue
        extra_range = (continuation.source_start, continuation.source_end)
        if extra_range in used_ranges:
            continue
        anchors.append(
            _DialogueAnchor(
                source_start=extra_range[0],
                source_end=extra_range[1],
                exact_text=continuation.exact_text,
                speaker=speaker,
                text=extra_text,
                dialogue_kind=_dialogue_kind(modifier),
                performance_note=extra_note,
            )
        )
        used_ranges.add(extra_range)

    anchors.sort(key=lambda item: (item.source_start, item.source_end, item.speaker))
    return anchors


def _all_positions(source: str, value: str) -> list[int]:
    positions: list[int] = []
    search_start = 0
    while (position := source.find(value, search_start)) >= 0:
        positions.append(position)
        search_start = position + len(value)
    return positions


def anchor_script_structure_ranges(
    result: ScriptExtractionResult,
    script_body: str,
) -> ScriptExtractionResult:
    """Replace probabilistic model offsets with exact screenplay anchors."""

    payload = cast(dict[str, object], result.model_dump(mode="json"))
    candidate_payloads = cast(list[dict[str, object]], payload["candidates"])
    scene_anchors: list[tuple[str, int]] = []
    search_start = 0
    for candidate in result.candidates:
        proposal = candidate.proposal
        if not isinstance(proposal, SceneCandidateProposal):
            continue
        start = script_body.find(proposal.heading, search_start)
        if start < 0:
            raise ValueError("scene heading is not anchored in the screenplay")
        scene_anchors.append((candidate.candidate_key, start))
        search_start = start + len(proposal.heading)

    scene_ranges = {
        candidate_key: (
            start,
            scene_anchors[index + 1][1] if index + 1 < len(scene_anchors) else len(script_body),
        )
        for index, (candidate_key, start) in enumerate(scene_anchors)
    }
    dialogue_anchors = _dialogue_anchors(script_body, scene_ranges)
    used_dialogue_ranges: set[tuple[int, int]] = set()
    anchored_payloads: list[dict[str, object]] = []
    for candidate, candidate_payload in zip(result.candidates, candidate_payloads, strict=True):
        proposal = candidate.proposal
        if isinstance(proposal, SceneCandidateProposal):
            start, end = scene_ranges[candidate.candidate_key]
            candidate_payload["source_range"] = {"start": start, "end": end}
            anchored_payloads.append(candidate_payload)
            continue
        if isinstance(proposal, AssetCandidateProposal):
            anchors = _all_positions(script_body, proposal.name)
            if anchors:
                start = min(
                    anchors,
                    key=lambda position: abs(position - candidate.source_range.start),
                )
                candidate_payload["source_range"] = {
                    "start": start,
                    "end": start + len(proposal.name),
                }
            anchored_payloads.append(candidate_payload)
            continue
        if not isinstance(proposal, DialogueCandidateProposal):
            anchored_payloads.append(candidate_payload)
            continue
        scene_range = scene_ranges.get(proposal.scene_candidate_key)
        if scene_range is None:
            raise ValueError("dialogue references an unanchored scene")
        expected = _normalized_source_text(f"{proposal.speaker_candidate}:{proposal.text}")
        exact_matches = [
            anchor
            for anchor in dialogue_anchors
            if scene_range[0] <= anchor.source_start < scene_range[1]
            and (anchor.source_start, anchor.source_end) not in used_dialogue_ranges
            and _normalized_source_text(f"{anchor.speaker}:{anchor.text}") == expected
        ]
        speaker_matches = [
            anchor
            for anchor in dialogue_anchors
            if scene_range[0] <= anchor.source_start < scene_range[1]
            and (anchor.source_start, anchor.source_end) not in used_dialogue_ranges
            and _normalized_source_text(anchor.speaker)
            == _normalized_source_text(proposal.speaker_candidate)
        ]
        selected: _DialogueAnchor | None = None
        if len(exact_matches) == 1:
            selected = exact_matches[0]
        elif len(speaker_matches) == 1:
            selected = speaker_matches[0]
        elif speaker_matches:
            selected = min(
                speaker_matches,
                key=lambda item: abs(item.source_start - candidate.source_range.start),
            )
        if selected is None:
            continue
        used_dialogue_ranges.add((selected.source_start, selected.source_end))
        candidate_payload["source_range"] = {
            "start": selected.source_start,
            "end": selected.source_end,
        }
        proposal_payload = cast(dict[str, object], candidate_payload["proposal"])
        proposal_payload["speaker_candidate"] = selected.speaker
        proposal_payload["text"] = selected.text
        proposal_payload["dialogue_kind"] = selected.dialogue_kind
        if selected.performance_note is not None:
            proposal_payload["performance_note"] = selected.performance_note
        anchored_payloads.append(candidate_payload)

    existing_keys = {
        str(candidate_payload["candidate_key"]) for candidate_payload in anchored_payloads
    }
    for anchor in dialogue_anchors:
        source_range = (anchor.source_start, anchor.source_end)
        if source_range in used_dialogue_ranges:
            continue
        scene_key = next(
            (
                candidate_key
                for candidate_key, (start, end) in scene_ranges.items()
                if start <= anchor.source_start < end
            ),
            None,
        )
        if scene_key is None:
            raise ValueError("screenplay dialogue is outside every anchored scene")
        digest = sha256(f"{anchor.source_start}:{anchor.source_end}".encode()).hexdigest()[:16]
        candidate_key = f"tool-dialogue-{digest}"
        if candidate_key in existing_keys:
            raise ValueError("deterministic dialogue candidate key is not unique")
        existing_keys.add(candidate_key)
        anchored_payloads.append(
            {
                "candidate_key": candidate_key,
                "source_range": {
                    "start": anchor.source_start,
                    "end": anchor.source_end,
                },
                "proposal": {
                    "kind": "dialogue",
                    "scene_candidate_key": scene_key,
                    "speaker_candidate": anchor.speaker,
                    "dialogue_kind": anchor.dialogue_kind,
                    "text": anchor.text,
                    "performance_note": anchor.performance_note,
                },
                "confidence_note": "由结构工具按原文补齐",
            }
        )
    payload["candidates"] = anchored_payloads
    return ScriptExtractionResult.model_validate(payload)
