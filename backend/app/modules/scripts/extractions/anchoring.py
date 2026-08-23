import unicodedata
from hashlib import sha256
from typing import cast

from app.modules.scripts.extractions.schemas import (
    AssetCandidateProposal,
    DialogueCandidateProposal,
    SceneCandidateProposal,
    ScriptExtractionResult,
)
from app.modules.scripts.narratives.parser import ParsedUnit, parse_narrative_units


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
    dialogue_units = [
        (unit, parts)
        for unit in parse_narrative_units(script_body)
        if unit.kind in {"dialogue", "narration"}
        and (parts := _dialogue_parts(unit.exact_text)) is not None
    ]
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
            (unit, parts)
            for unit, parts in dialogue_units
            if scene_range[0] <= unit.source_start < scene_range[1]
            and (unit.source_start, unit.source_end) not in used_dialogue_ranges
            and _normalized_source_text(unit.exact_text) == expected
        ]
        speaker_matches = [
            (unit, parts)
            for unit, parts in dialogue_units
            if scene_range[0] <= unit.source_start < scene_range[1]
            and (unit.source_start, unit.source_end) not in used_dialogue_ranges
            and _normalized_source_text(parts[0])
            == _normalized_source_text(proposal.speaker_candidate)
        ]
        selected: tuple[ParsedUnit, tuple[str, str]] | None = None
        if len(exact_matches) == 1:
            selected = exact_matches[0]
        elif len(speaker_matches) == 1:
            selected = speaker_matches[0]
        elif speaker_matches:
            selected = min(
                speaker_matches,
                key=lambda item: abs(item[0].source_start - candidate.source_range.start),
            )
        if selected is None:
            continue
        unit, parts = selected
        used_dialogue_ranges.add((unit.source_start, unit.source_end))
        candidate_payload["source_range"] = {
            "start": unit.source_start,
            "end": unit.source_end,
        }
        proposal_payload = cast(dict[str, object], candidate_payload["proposal"])
        proposal_payload["speaker_candidate"] = parts[0]
        proposal_payload["text"] = parts[1]
        anchored_payloads.append(candidate_payload)

    existing_keys = {
        str(candidate_payload["candidate_key"]) for candidate_payload in anchored_payloads
    }
    for unit, parts in dialogue_units:
        source_range = (unit.source_start, unit.source_end)
        if source_range in used_dialogue_ranges:
            continue
        scene_key = next(
            (
                candidate_key
                for candidate_key, (start, end) in scene_ranges.items()
                if start <= unit.source_start < end
            ),
            None,
        )
        if scene_key is None:
            raise ValueError("screenplay dialogue is outside every anchored scene")
        digest = sha256(f"{unit.source_start}:{unit.source_end}".encode()).hexdigest()[:16]
        candidate_key = f"tool-dialogue-{digest}"
        if candidate_key in existing_keys:
            raise ValueError("deterministic dialogue candidate key is not unique")
        existing_keys.add(candidate_key)
        anchored_payloads.append(
            {
                "candidate_key": candidate_key,
                "source_range": {
                    "start": unit.source_start,
                    "end": unit.source_end,
                },
                "proposal": {
                    "kind": "dialogue",
                    "scene_candidate_key": scene_key,
                    "speaker_candidate": parts[0],
                    "dialogue_kind": ("narration" if unit.kind == "narration" else "spoken"),
                    "text": parts[1],
                },
                "confidence_note": "由结构工具按原文补齐",
            }
        )
    payload["candidates"] = anchored_payloads
    return ScriptExtractionResult.model_validate(payload)
