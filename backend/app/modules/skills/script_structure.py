from __future__ import annotations

import hashlib
import json
import operator
import re
import unicodedata
from collections.abc import Callable
from dataclasses import asdict, dataclass
from typing import Annotated, Any, TypedDict, cast

from langchain_core.messages import HumanMessage, SystemMessage
from langgraph.graph import END, START, StateGraph  # pyright: ignore[reportMissingTypeStubs]
from langgraph.types import Send  # pyright: ignore[reportMissingTypeStubs]
from pydantic import ValidationError

from app.modules.scripts import (
    AssetOccurrenceCandidateProposal,
    ContinuityCandidateProposal,
    DialogueCandidateProposal,
    ScriptExtractionResult,
    ShotCandidateProposal,
    analyze_document,
)
from app.modules.scripts.contracts import ProductionBibleExtractionInput
from app.modules.skills.contracts import (
    SkillDefinition,
    SkillExecutionContext,
    SkillExecutionError,
    StructuredSkillModel,
    input_hash,
)

DEFAULT_MAX_CHUNK_CHARS = 18_000
_SCENE_HEADING = re.compile(
    r"^(?:内景|外景|场景\s*\d+|INT\.?|EXT\.?|I/E\.?|INT\s*/\s*EXT\.?)",
    re.IGNORECASE,
)


@dataclass(frozen=True, slots=True)
class ScriptExtractionChunk:
    key: str
    episode_number: int | None
    start: int
    end: int
    text: str

    def as_state(self) -> dict[str, object]:
        return {
            "chunk_key": self.key,
            "chunk_episode_number": self.episode_number,
            "chunk_start": self.start,
            "chunk_end": self.end,
            "chunk_text": self.text,
        }


class _ScriptStructureState(TypedDict, total=False):
    script_body: str
    episode_number: int | None
    production_bible: dict[str, object] | None
    chunks: list[dict[str, object]]
    chunk_key: str
    chunk_episode_number: int | None
    chunk_start: int
    chunk_end: int
    chunk_text: str
    chunk_results: Annotated[list[dict[str, object]], operator.add]
    output: ScriptExtractionResult


def _chunk_key(episode_number: int | None, sequence: int) -> str:
    prefix = f"ep{episode_number:03d}" if episode_number is not None else "pre"
    return f"{prefix}-c{sequence:03d}"


def _line_break_before(text: str, start: int, limit: int) -> int:
    line_break = text.rfind("\n", start, limit)
    if line_break <= start:
        return limit
    return line_break + 1


def _episode_ranges(
    script_body: str,
    *,
    authoritative_episode_number: int | None = None,
) -> list[tuple[int | None, int, int]]:
    analysis = analyze_document(script_body)
    blocking_issues = [issue for issue in analysis.issues if issue.severity == "blocking"]
    if authoritative_episode_number is not None and analysis.markers:
        marker_numbers = {marker.episode_number for marker in analysis.markers}
        if marker_numbers != {authoritative_episode_number}:
            raise SkillExecutionError(
                outcome="failed",
                code="script_structure_input_invalid",
                summary="Script episode marker does not match the authoritative episode",
                retryable=False,
                next_action="fix_skill_input",
            )
        blocking_issues = [issue for issue in blocking_issues if issue.code != "number_gap"]
    if blocking_issues:
        issue = blocking_issues[0]
        raise SkillExecutionError(
            outcome="failed",
            code="script_structure_input_invalid",
            summary=f"Script structure cannot be segmented: {issue.code}",
            retryable=False,
            next_action=issue.next_action,
        )
    if not analysis.markers:
        return [(None, 0, len(script_body))]
    ranges: list[tuple[int | None, int, int]] = []
    first_start = 0
    first_marker = analysis.markers[0]
    if first_marker.start_codepoint > 0:
        ranges.append((None, 0, first_marker.start_codepoint))
    for index, marker in enumerate(analysis.markers):
        end = (
            analysis.markers[index + 1].start_codepoint
            if index + 1 < len(analysis.markers)
            else len(script_body)
        )
        ranges.append((marker.episode_number, marker.start_codepoint, end))
        first_start = end
    if first_start == 0:
        return [(None, 0, len(script_body))]
    return ranges


def _scene_starts(script_body: str, start: int, end: int) -> list[int]:
    analysis = analyze_document(script_body[start:end])
    starts: list[int] = []
    for block in analysis.blocks:
        if block.kind != "scene_heading":
            continue
        heading = script_body[start + block.start_codepoint : start + block.end_codepoint]
        if _SCENE_HEADING.match(heading.strip()):
            starts.append(start + block.start_codepoint)
    return starts


def _scene_heading_parts(heading: str) -> tuple[str, str]:
    remainder = _SCENE_HEADING.sub("", heading, count=1).strip(" .-–—")
    parts = [
        part.strip()
        for part in re.split(r"\s+(?:-|–|—)\s+|\s*·\s*", remainder)
        if part.strip()
    ]
    if len(parts) >= 2:
        return " - ".join(parts[:-1])[:200], parts[-1][:100]
    return (remainder or heading)[:200], "UNSPECIFIED"


def _deterministic_scene_candidates(
    script_body: str,
    *,
    episode_number: int | None,
) -> ScriptExtractionResult:
    blocks = [
        block
        for block in analyze_document(script_body).blocks
        if block.kind == "scene_heading"
        and _SCENE_HEADING.match(
            script_body[block.start_codepoint : block.end_codepoint].strip()
        )
    ]
    episode_ranges = _episode_ranges(
        script_body,
        authoritative_episode_number=episode_number,
    )
    candidates: list[dict[str, object]] = []
    for index, block in enumerate(blocks, start=1):
        heading = script_body[block.start_codepoint : block.end_codepoint].strip()
        source_end = (
            blocks[index].start_codepoint if index < len(blocks) else len(script_body)
        )
        location, time_of_day = _scene_heading_parts(heading)
        scene_body = script_body[block.end_codepoint : source_end]
        summary = " ".join(scene_body.split())[:1000] or heading
        resolved_episode_number = episode_number
        if resolved_episode_number is None:
            resolved_episode_number = next(
                (
                    candidate_episode
                    for candidate_episode, range_start, range_end in episode_ranges
                    if range_start <= block.start_codepoint < range_end
                ),
                None,
            )
        candidates.append(
            {
                "candidate_key": f"deterministic-scene-{index:03d}-{block.start_codepoint}",
                "source_range": {"start": block.start_codepoint, "end": source_end},
                "proposal": {
                    "kind": "scene",
                    "heading": heading,
                    "location": location,
                    "time_of_day": time_of_day,
                    "summary": summary,
                    "episode_number": resolved_episode_number,
                    "scene_number": index,
                },
                "confidence_note": (
                    "Deterministic scene heading fallback; semantic enrichment requires review."
                ),
            }
        )
    return ScriptExtractionResult.model_validate({"candidates": candidates})


def _exact_name_mention(
    script_body: str,
    *,
    source_start: int,
    source_end: int,
    names: list[str],
) -> tuple[int, int] | None:
    scene_text = script_body[source_start:source_end]
    for name in sorted({item.strip() for item in names if item.strip()}, key=len, reverse=True):
        tokens = [token for token in re.split(r"[\s\-–—]+", name) if token]
        escaped = r"[\s\-–—]+".join(re.escape(token) for token in tokens)
        prefix = r"(?<!\w)" if name[0].isalnum() else ""
        suffix = r"(?!\w)" if name[-1].isalnum() else ""
        match = re.search(f"{prefix}{escaped}{suffix}", scene_text, re.IGNORECASE)
        if match is not None:
            return source_start + match.start(), source_start + match.end()
    return None


def _complete_bible_occurrences(
    candidate_payloads: list[dict[str, object]],
    *,
    selected_scene_ranges: list[tuple[int, int, str]],
    script_body: str,
    production_bible: dict[str, object],
) -> None:
    raw_entities = production_bible.get("entities", [])
    if not isinstance(raw_entities, list):
        return
    existing_occurrences: set[tuple[str, str, str]] = set()
    existing_entity_scenes: set[tuple[str, str]] = set()
    existing_keys = {str(candidate["candidate_key"]) for candidate in candidate_payloads}
    for candidate in candidate_payloads:
        proposal = candidate.get("proposal")
        if not isinstance(proposal, dict):
            continue
        proposal_payload = cast(dict[str, object], proposal)
        if proposal_payload.get("kind") != "asset_occurrence":
            continue
        entity_key = str(proposal_payload.get("entity_key", ""))
        state_key = str(proposal_payload.get("state_key", ""))
        scene_key = str(proposal_payload.get("scene_candidate_key", ""))
        existing_occurrences.add((entity_key, state_key, scene_key))
        existing_entity_scenes.add((entity_key, scene_key))

    supported_roles = {"character", "location", "prop", "costume", "visual_style", "voice"}
    for raw_entity in cast(list[object], raw_entities):
        if not isinstance(raw_entity, dict):
            continue
        entity = cast(dict[str, object], raw_entity)
        entity_key = str(entity.get("entity_key", ""))
        role = str(entity.get("kind", ""))
        raw_states = entity.get("states", [])
        if not entity_key or role not in supported_roles or not isinstance(raw_states, list):
            continue
        states: list[dict[str, object]] = []
        for raw_state in cast(list[object], raw_states):
            if not isinstance(raw_state, dict):
                continue
            state = cast(dict[str, object], raw_state)
            if state.get("state_key"):
                states.append(state)
        if not states:
            continue
        raw_aliases = entity.get("aliases", [])
        aliases = (
            [str(alias) for alias in cast(list[object], raw_aliases)]
            if isinstance(raw_aliases, list)
            else []
        )
        names = [str(entity.get("canonical_name", "")), *aliases]
        for scene_start, scene_end, scene_key in selected_scene_ranges:
            if (entity_key, scene_key) in existing_entity_scenes:
                continue
            mention = _exact_name_mention(
                script_body,
                source_start=scene_start,
                source_end=scene_end,
                names=names,
            )
            if mention is None:
                continue
            for state in states:
                state_key = str(state["state_key"])
                occurrence_key = (entity_key, state_key, scene_key)
                if occurrence_key in existing_occurrences:
                    continue
                digest = hashlib.sha256(
                    (
                        f"{entity_key}:{state_key}:{scene_key}:"
                        f"{mention[0]}:{mention[1]}"
                    ).encode()
                ).hexdigest()[:24]
                candidate_key = f"tool-occurrence-{digest}"
                if candidate_key in existing_keys:
                    continue
                existing_keys.add(candidate_key)
                existing_occurrences.add(occurrence_key)
                candidate_payloads.append(
                    {
                        "candidate_key": candidate_key,
                        "source_range": {"start": mention[0], "end": mention[1]},
                        "proposal": {
                            "kind": "asset_occurrence",
                            "entity_key": entity_key,
                            "state_key": state_key,
                            "scene_candidate_key": scene_key,
                            "role": role,
                        },
                        "confidence_note": (
                            "Deterministic Production Bible name or alias match with one "
                            "episode-valid state."
                            if len(states) == 1
                            else "Deterministic Production Bible mention; multiple "
                            "episode-valid states require review."
                        ),
                    }
                )


def _complete_deterministic_scenes(
    result: ScriptExtractionResult,
    script_body: str,
    *,
    episode_number: int | None,
    production_bible: dict[str, object] | None = None,
) -> ScriptExtractionResult:
    deterministic = list(
        _deterministic_scene_candidates(
            script_body,
            episode_number=episode_number,
        ).candidates
    )
    model_scenes = [
        candidate for candidate in result.candidates if candidate.proposal.kind == "scene"
    ]
    unused_scene_keys = {candidate.candidate_key for candidate in model_scenes}
    selected_scenes = list(deterministic[:0])
    scene_key_map: dict[str, str] = {}
    selected_by_range: list[tuple[int, int, str]] = []
    for expected in deterministic:
        assert expected.proposal.kind == "scene"
        matches = [
            candidate
            for candidate in model_scenes
            if candidate.candidate_key in unused_scene_keys
            and candidate.proposal.kind == "scene"
            and candidate.proposal.heading == expected.proposal.heading
        ]
        selected = (
            min(
                matches,
                key=lambda candidate: abs(
                    candidate.source_range.start - expected.source_range.start
                ),
            )
            if matches
            else expected
        )
        selected_scenes.append(selected)
        selected_by_range.append(
            (
                expected.source_range.start,
                expected.source_range.end,
                selected.candidate_key,
            )
        )
        if matches:
            unused_scene_keys.remove(selected.candidate_key)
            scene_key_map[selected.candidate_key] = selected.candidate_key

    def scene_key_for_offset(offset: int) -> str | None:
        containing = next(
            (
                candidate_key
                for start, end, candidate_key in selected_by_range
                if start <= offset < end
            ),
            None,
        )
        if containing is not None:
            return containing
        if not selected_by_range:
            return None
        return min(
            selected_by_range,
            key=lambda item: abs(item[0] - offset),
        )[2]

    for candidate in model_scenes:
        if candidate.candidate_key not in unused_scene_keys:
            continue
        replacement = scene_key_for_offset(candidate.source_range.start)
        if replacement is not None:
            scene_key_map[candidate.candidate_key] = replacement

    scene_range_by_key = {
        candidate_key: (start, end) for start, end, candidate_key in selected_by_range
    }

    def referenced_scene_key(raw_scene_key: str, source_start: int) -> str | None:
        mapped = scene_key_map.get(raw_scene_key)
        if mapped is not None:
            mapped_range = scene_range_by_key.get(mapped)
            if (
                mapped_range is not None
                and mapped_range[0] <= source_start < mapped_range[1]
            ):
                return mapped
        return scene_key_for_offset(source_start) or mapped

    candidate_payloads: list[dict[str, object]] = [
        cast(dict[str, object], candidate.model_dump(mode="json"))
        for candidate in selected_scenes
    ]
    for candidate in result.candidates:
        if candidate.proposal.kind == "scene":
            continue
        candidate_payload = cast(dict[str, object], candidate.model_dump(mode="json"))
        proposal = cast(dict[str, object], candidate_payload["proposal"])
        raw_scene_key = proposal.get("scene_candidate_key")
        if isinstance(raw_scene_key, str):
            replacement = referenced_scene_key(
                raw_scene_key,
                candidate.source_range.start,
            )
            if replacement is not None:
                proposal["scene_candidate_key"] = replacement
        raw_scene_keys = proposal.get("scene_candidate_keys")
        if isinstance(raw_scene_keys, list):
            proposal["scene_candidate_keys"] = list(
                dict.fromkeys(
                    replacement
                    for item in cast(list[object], raw_scene_keys)
                    if (
                        replacement := referenced_scene_key(
                            str(item),
                            candidate.source_range.start,
                        )
                    )
                    is not None
                )
            )
        candidate_payloads.append(candidate_payload)

    if production_bible is not None:
        _complete_bible_occurrences(
            candidate_payloads,
            selected_scene_ranges=selected_by_range,
            script_body=script_body,
            production_bible=production_bible,
        )

    reconciled = ScriptExtractionResult.model_validate({"candidates": candidate_payloads})
    candidates = list(reconciled.candidates)
    kind_order = {
        "scene": 0,
        "asset_occurrence": 1,
        "dialogue": 2,
        "continuity": 3,
        "asset": 4,
        "shot": 5,
    }
    candidates.sort(
        key=lambda candidate: (
            candidate.source_range.start,
            kind_order[candidate.proposal.kind],
            candidate.candidate_key,
        )
    )
    return ScriptExtractionResult(candidates=candidates)


def segment_script(
    script_body: str,
    *,
    max_chunk_chars: int = DEFAULT_MAX_CHUNK_CHARS,
    authoritative_episode_number: int | None = None,
) -> tuple[ScriptExtractionChunk, ...]:
    if not script_body.strip():
        raise SkillExecutionError(
            outcome="failed",
            code="skill_input_invalid",
            summary="Script input is empty",
            retryable=False,
            next_action="fix_skill_input",
        )
    if max_chunk_chars < 1_000:
        raise ValueError("max_chunk_chars must be at least 1000")

    chunks: list[ScriptExtractionChunk] = []
    sequence = 0
    for episode_number, episode_start, episode_end in _episode_ranges(
        script_body,
        authoritative_episode_number=authoritative_episode_number,
    ):
        scene_starts = _scene_starts(script_body, episode_start, episode_end)
        natural_starts = [episode_start, *scene_starts[1:]] if scene_starts else [episode_start]
        natural_ends = [*natural_starts[1:], episode_end]
        for natural_start, natural_end in zip(natural_starts, natural_ends, strict=True):
            cursor = natural_start
            while cursor < natural_end:
                end = min(cursor + max_chunk_chars, natural_end)
                if end < natural_end:
                    end = _line_break_before(script_body, cursor, end)
                if end <= cursor:
                    end = min(cursor + max_chunk_chars, natural_end)
                sequence += 1
                chunks.append(
                    ScriptExtractionChunk(
                        key=_chunk_key(episode_number, sequence),
                        episode_number=episode_number,
                        start=cursor,
                        end=end,
                        text=script_body[cursor:end],
                    )
                )
                cursor = end

    if not chunks:
        raise SkillExecutionError(
            outcome="failed",
            code="script_structure_input_invalid",
            summary="Script did not produce any extraction chunks",
            retryable=False,
            next_action="fix_skill_input",
        )
    return tuple(chunks)


def _qualified_candidate_key(chunk_key: str, candidate_key: str) -> str:
    qualified = f"{chunk_key}:{candidate_key}"
    if len(qualified) <= 100:
        return qualified
    digest = hashlib.sha256(qualified.encode("utf-8")).hexdigest()[:24]
    return f"{chunk_key}:{digest}"


def _normalized_asset_key(asset_kind: str, name: str) -> str:
    normalized = unicodedata.normalize("NFKC", name).casefold()
    return f"{asset_kind}:{' '.join(normalized.split())}"


def _state_int(value: object, field_name: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        raise SkillExecutionError(
            outcome="failed",
            code="skill_output_invalid",
            summary=f"Extraction state field {field_name} is invalid",
            retryable=False,
            next_action="start_new_skill_run",
        )
    return value


def _state_optional_positive_int(value: object, field_name: str) -> int | None:
    if value is None:
        return None
    parsed = _state_int(value, field_name)
    if parsed < 1:
        raise SkillExecutionError(
            outcome="failed",
            code="skill_output_invalid",
            summary=f"Extraction state field {field_name} is invalid",
            retryable=False,
            next_action="start_new_skill_run",
        )
    return parsed


def _validate_chunk_result(result: ScriptExtractionResult, chunk_text: str) -> None:
    candidate_keys = {candidate.candidate_key for candidate in result.candidates}
    candidate_by_key = {candidate.candidate_key: candidate for candidate in result.candidates}
    for candidate in result.candidates:
        if candidate.source_range.end > len(chunk_text):
            raise SkillExecutionError(
                outcome="failed",
                code="skill_output_invalid",
                summary="Chunk candidate source range exceeds the chunk",
                retryable=False,
                next_action="start_new_skill_run",
            )
        proposal = candidate.proposal
        if isinstance(
            proposal,
            (
                DialogueCandidateProposal,
                AssetOccurrenceCandidateProposal,
                ShotCandidateProposal,
            ),
        ):
            if proposal.scene_candidate_key not in candidate_keys:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_output_invalid",
                    summary="Chunk candidate references an unknown scene",
                    retryable=False,
                    next_action="start_new_skill_run",
                )
        if isinstance(proposal, ContinuityCandidateProposal):
            scene_keys = set(proposal.scene_candidate_keys)
            if proposal.scene_candidate_key is not None:
                scene_keys.add(proposal.scene_candidate_key)
            if any(
                scene_key not in candidate_keys
                or candidate_by_key[scene_key].proposal.kind != "scene"
                for scene_key in scene_keys
            ):
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_output_invalid",
                    summary="Chunk continuity candidate references an unknown scene",
                    retryable=False,
                    next_action="start_new_skill_run",
                )


def _bound_chunk_result(
    result: ScriptExtractionResult,
    chunk_text: str,
) -> ScriptExtractionResult:
    """Keep probabilistic evidence ranges inside the immutable Chunk boundary."""

    if not chunk_text:
        return result
    payload = cast(dict[str, object], result.model_dump(mode="json"))
    candidates = cast(list[dict[str, object]], payload["candidates"])
    last_start = len(chunk_text) - 1
    for candidate in candidates:
        source_range = cast(dict[str, object], candidate["source_range"])
        raw_start = _state_int(source_range["start"], "source_range.start")
        raw_end = _state_int(source_range["end"], "source_range.end")
        start = min(raw_start, last_start)
        end = min(max(raw_end, start + 1), len(chunk_text))
        source_range["start"] = start
        source_range["end"] = end
    return ScriptExtractionResult.model_validate(payload)


def _merge_strings(
    target: dict[str, object],
    field_name: str,
    values: object,
    *,
    limit: int,
) -> None:
    current_raw = target.get(field_name)
    current = (
        [str(item) for item in cast(list[object], current_raw)]
        if isinstance(current_raw, list)
        else []
    )
    incoming = (
        [str(item) for item in cast(list[object], values)] if isinstance(values, list) else []
    )
    for value in incoming:
        if value and value not in current:
            current.append(value)
    target[field_name] = current[:limit]


def _merge_optional_text(
    target: dict[str, object],
    field_name: str,
    value: object,
) -> None:
    if target.get(field_name) in (None, "") and isinstance(value, str) and value:
        target[field_name] = value


def _merge_episode_appearances(
    target: dict[str, object],
    values: object,
    *episode_hints: object,
) -> None:
    episode_numbers: set[int] = set()
    for source in (target.get("episode_numbers"), values):
        if not isinstance(source, list):
            continue
        for value in cast(list[object], source):
            if isinstance(value, int) and not isinstance(value, bool) and value > 0:
                episode_numbers.add(value)
    for value in (target.get("first_seen_episode"), *episode_hints):
        if isinstance(value, int) and not isinstance(value, bool) and value > 0:
            episode_numbers.add(value)
    ordered_episode_numbers = sorted(episode_numbers)[:1000]
    target["episode_numbers"] = ordered_episode_numbers
    if ordered_episode_numbers:
        target["first_seen_episode"] = ordered_episode_numbers[0]


def _aggregate_results(
    chunk_results: list[dict[str, object]],
) -> ScriptExtractionResult:
    candidates: list[dict[str, object]] = []
    assets_by_key: dict[str, dict[str, object]] = {}
    continuity_by_key: dict[str, dict[str, object]] = {}
    qualify_candidate_keys = len(chunk_results) > 1
    ordered_results = sorted(
        chunk_results,
        key=lambda item: _state_int(item["chunk_start"], "chunk_start"),
    )
    for item in ordered_results:
        chunk_key = str(item["chunk_key"])
        chunk_start = _state_int(item["chunk_start"], "chunk_start")
        chunk_episode_number = _state_optional_positive_int(
            item.get("chunk_episode_number"),
            "chunk_episode_number",
        )
        chunk_text = str(item["chunk_text"])
        raw_result = item["result"]
        try:
            result = ScriptExtractionResult.model_validate(raw_result)
        except ValidationError as error:
            raise SkillExecutionError(
                outcome="failed",
                code="skill_output_invalid",
                summary="Chunk result does not satisfy the extraction schema",
                retryable=False,
                next_action="start_new_skill_run",
            ) from error
        _validate_chunk_result(result, chunk_text)
        scene_key_map = {
            candidate.candidate_key: (
                _qualified_candidate_key(chunk_key, candidate.candidate_key)
                if qualify_candidate_keys
                else candidate.candidate_key
            )
            for candidate in result.candidates
            if candidate.proposal.kind == "scene"
        }
        for candidate in result.candidates:
            candidate_data = cast(dict[str, object], candidate.model_dump(mode="json"))
            candidate_data["candidate_key"] = (
                _qualified_candidate_key(chunk_key, candidate.candidate_key)
                if qualify_candidate_keys
                else candidate.candidate_key
            )
            source_range = cast(dict[str, object], candidate_data["source_range"])
            source_range["start"] = (
                _state_int(source_range["start"], "source_range.start") + chunk_start
            )
            source_range["end"] = _state_int(source_range["end"], "source_range.end") + chunk_start
            proposal = cast(dict[str, object], candidate_data["proposal"])
            if (
                candidate.proposal.kind == "scene"
                and proposal.get("episode_number") is None
                and chunk_episode_number is not None
            ):
                proposal["episode_number"] = chunk_episode_number
            if candidate.proposal.kind in {"dialogue", "asset_occurrence", "shot"}:
                scene_key = str(proposal["scene_candidate_key"])
                proposal["scene_candidate_key"] = scene_key_map.get(
                    scene_key,
                    (
                        _qualified_candidate_key(chunk_key, scene_key)
                        if qualify_candidate_keys
                        else scene_key
                    ),
                )
            if candidate.proposal.kind == "continuity":
                if (
                    proposal.get("scope") == "episode"
                    and proposal.get("episode_number") is None
                    and chunk_episode_number is not None
                ):
                    proposal["episode_number"] = chunk_episode_number
                scene_key = proposal.get("scene_candidate_key")
                if isinstance(scene_key, str):
                    proposal["scene_candidate_key"] = scene_key_map.get(
                        scene_key,
                        (
                            _qualified_candidate_key(chunk_key, scene_key)
                            if qualify_candidate_keys
                            else scene_key
                        ),
                    )
                raw_scene_keys = proposal.get("scene_candidate_keys")
                if isinstance(raw_scene_keys, list):
                    scene_keys = cast(list[object], raw_scene_keys)
                    proposal["scene_candidate_keys"] = [
                        scene_key_map.get(
                            str(scene_key),
                            (
                                _qualified_candidate_key(chunk_key, str(scene_key))
                                if qualify_candidate_keys
                                else str(scene_key)
                            ),
                        )
                        for scene_key in scene_keys
                    ]
                continuity_scope = str(proposal.get("scope", "scene"))
                if continuity_scope == "episode":
                    continuity_key = f"episode:{proposal.get('episode_number')}"
                elif continuity_scope == "world":
                    continuity_key = "world:" + " ".join(
                        str(proposal.get("topic") or proposal.get("issue") or "").casefold().split()
                    )
                else:
                    continuity_key = str(candidate_data["candidate_key"])
                existing_continuity = continuity_by_key.get(continuity_key)
                if existing_continuity is not None:
                    existing_proposal = cast(dict[str, object], existing_continuity["proposal"])
                    for field_name, limit in (
                        ("entities", 50),
                        ("facts", 100),
                        ("rules", 100),
                        ("scene_candidate_keys", 1000),
                    ):
                        _merge_strings(
                            existing_proposal,
                            field_name,
                            proposal.get(field_name),
                            limit=limit,
                        )
                    for field_name in ("title", "logline", "summary", "topic"):
                        _merge_optional_text(
                            existing_proposal,
                            field_name,
                            proposal.get(field_name),
                        )
                    continue
                continuity_by_key[continuity_key] = candidate_data
            if candidate.proposal.kind == "asset":
                _merge_episode_appearances(
                    proposal,
                    proposal.get("episode_numbers"),
                    proposal.get("first_seen_episode"),
                    chunk_episode_number,
                )
                asset_key = _normalized_asset_key(
                    str(proposal["asset_kind"]), str(proposal["name"])
                )
                existing = assets_by_key.get(asset_key)
                if existing is not None:
                    existing_proposal = cast(dict[str, object], existing["proposal"])
                    raw_aliases: object = existing_proposal.get("aliases")
                    aliases: list[str] = []
                    if isinstance(raw_aliases, list):
                        aliases = [str(alias) for alias in cast(list[object], raw_aliases)]
                    current_name = str(proposal["name"])
                    if (
                        current_name != str(existing_proposal["name"])
                        and current_name not in aliases
                    ):
                        aliases.append(current_name)
                    existing_proposal["aliases"] = aliases[:20]
                    _merge_strings(
                        existing_proposal,
                        "aliases",
                        proposal.get("aliases"),
                        limit=20,
                    )
                    for field_name, limit in (
                        ("goals", 50),
                        ("relationships", 100),
                        ("continuity_notes", 50),
                    ):
                        _merge_strings(
                            existing_proposal,
                            field_name,
                            proposal.get(field_name),
                            limit=limit,
                        )
                    for field_name in (
                        "appearance",
                        "voice_profile",
                        "arc_summary",
                        "visual_identity",
                        "role",
                    ):
                        _merge_optional_text(
                            existing_proposal,
                            field_name,
                            proposal.get(field_name),
                        )
                    _merge_episode_appearances(
                        existing_proposal,
                        proposal.get("episode_numbers"),
                        proposal.get("first_seen_episode"),
                        chunk_episode_number,
                    )
                    continue
                assets_by_key[asset_key] = candidate_data
            candidates.append(candidate_data)

    try:
        return ScriptExtractionResult.model_validate({"candidates": candidates})
    except ValidationError as error:
        raise SkillExecutionError(
            outcome="failed",
            code="skill_output_invalid",
            summary="Aggregated extraction result does not satisfy the schema",
            retryable=False,
            next_action="start_new_skill_run",
        ) from error


class ScriptStructureExtractionWorkflow:
    """LangGraph map-reduce workflow for long screenplay structure extraction."""

    def __init__(
        self,
        *,
        skill: SkillDefinition,
        model: StructuredSkillModel,
        system_prompt: str,
        max_chunk_chars: int = DEFAULT_MAX_CHUNK_CHARS,
        chunker: Callable[[str], tuple[ScriptExtractionChunk, ...]] | None = None,
        checkpointer: Any | None = None,
    ) -> None:
        self._skill = skill
        self._model = model
        self._system_prompt = system_prompt
        self._max_chunk_chars = max_chunk_chars
        self._chunker = chunker
        self._checkpointer = checkpointer

    async def run(
        self,
        script_body: str,
        *,
        context: SkillExecutionContext,
        episode_number: int | None = None,
        production_bible: ProductionBibleExtractionInput | None = None,
    ) -> ScriptExtractionResult:
        if context.skill_name != self._skill.name or context.skill_version != self._skill.version:
            raise SkillExecutionError(
                outcome="failed",
                code="skill_context_invalid",
                summary="Skill execution context does not match the Skill",
                retryable=False,
                next_action="contact_support",
            )
        graph = self._build_graph()
        config = {
            "configurable": {
                "thread_id": context.task_id
                or f"{context.workspace_id or 'local'}:{input_hash(script_body)}"
            }
        }
        result = await graph.ainvoke(
            {
                "script_body": script_body,
                "episode_number": episode_number,
                "production_bible": (
                    json.loads(json.dumps(asdict(production_bible), default=str))
                    if production_bible is not None
                    else None
                ),
            },
            config=config,
        )
        output = result.get("output")
        if not isinstance(output, ScriptExtractionResult):
            raise SkillExecutionError(
                outcome="failed",
                code="skill_output_invalid",
                summary="Script structure workflow returned no result",
                retryable=False,
                next_action="start_new_skill_run",
            )
        return output

    def _build_graph(self) -> Any:
        async def validate_input(state: _ScriptStructureState) -> dict[str, object]:
            script_body = state.get("script_body", "")
            if not script_body.strip():
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_input_invalid",
                    summary="Script input is empty",
                    retryable=False,
                    next_action="fix_skill_input",
                )
            return {}

        async def segment(state: _ScriptStructureState) -> dict[str, object]:
            script_body = state.get("script_body", "")
            chunks = (
                self._chunker(script_body)
                if self._chunker is not None
                else segment_script(
                    script_body,
                    max_chunk_chars=self._max_chunk_chars,
                    authoritative_episode_number=state.get("episode_number"),
                )
            )
            explicit_episode_number = state.get("episode_number")
            chunk_states = [chunk.as_state() for chunk in chunks]
            if explicit_episode_number is not None:
                for chunk_state in chunk_states:
                    if chunk_state["chunk_episode_number"] is None:
                        chunk_state["chunk_episode_number"] = explicit_episode_number
            for chunk_state in chunk_states:
                chunk_state["production_bible"] = state.get("production_bible")
            return {"chunks": chunk_states}

        def fan_out(state: _ScriptStructureState) -> list[Send]:
            return [Send("extract_chunk", chunk) for chunk in state.get("chunks", [])]

        async def extract_chunk(state: _ScriptStructureState) -> dict[str, object]:
            chunk_text = state.get("chunk_text", "")
            if len(chunk_text) > self._skill.max_input_chars:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_chunk_too_large",
                    summary="Script chunk exceeds the Skill input limit",
                    retryable=False,
                    next_action="adjust_chunk_size",
                )
            payload = json.dumps(
                {
                    "episode_number": state.get("chunk_episode_number"),
                    "source_start": state.get("chunk_start"),
                    "source_end": state.get("chunk_end"),
                    "script_text": chunk_text,
                    "production_bible": state.get("production_bible"),
                    "asset_policy": (
                        "Reference only production_bible entities/states using "
                        "asset_occurrence proposals; never emit asset proposals."
                        if state.get("production_bible") is not None
                        else "Asset proposals are allowed."
                    ),
                },
                ensure_ascii=False,
                separators=(",", ":"),
            )
            try:
                raw_output = await self._model.ainvoke(
                    [
                        SystemMessage(content=self._system_prompt),
                        HumanMessage(content=payload),
                    ]
                )
                result = ScriptExtractionResult.model_validate(raw_output)
                result = _bound_chunk_result(result, chunk_text)
                _validate_chunk_result(result, chunk_text)
            except SkillExecutionError:
                raise
            except (TypeError, ValueError, ValidationError) as error:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_output_invalid",
                    summary="Skill returned an invalid chunk result",
                    retryable=False,
                    next_action="start_new_skill_run",
                ) from error
            return {
                "chunk_results": [
                    {
                        "chunk_key": state.get("chunk_key"),
                        "chunk_start": state.get("chunk_start"),
                        "chunk_end": state.get("chunk_end"),
                        "chunk_episode_number": state.get("chunk_episode_number"),
                        "chunk_text": chunk_text,
                        "result": result.model_dump(mode="json"),
                    }
                ]
            }

        async def aggregate(state: _ScriptStructureState) -> dict[str, object]:
            return {"output": _aggregate_results(state.get("chunk_results", []))}

        async def validate_output(state: _ScriptStructureState) -> dict[str, object]:
            output = state.get("output")
            if not isinstance(output, ScriptExtractionResult) or not output.candidates:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_output_invalid",
                    summary="Script structure extraction produced no candidates",
                    retryable=False,
                    next_action="start_new_skill_run",
                )
            script_body = state.get("script_body", "")
            expected_scene_headings: set[str] = set()
            if self._chunker is None:
                output = _complete_deterministic_scenes(
                    output,
                    script_body,
                    episode_number=state.get("episode_number"),
                    production_bible=state.get("production_bible"),
                )
                expected_scene_headings = {
                    script_body[block.start_codepoint : block.end_codepoint].strip()
                    for block in analyze_document(script_body).blocks
                    if block.kind == "scene_heading"
                    and _SCENE_HEADING.match(
                        script_body[block.start_codepoint : block.end_codepoint].strip()
                    )
                }
            actual_scene_headings = {
                candidate.proposal.heading
                for candidate in output.candidates
                if candidate.proposal.kind == "scene"
            }
            missing_scene_headings = sorted(expected_scene_headings - actual_scene_headings)
            if missing_scene_headings:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_output_incomplete",
                    summary="Script extraction omitted deterministic scene headings",
                    retryable=False,
                    next_action="start_new_skill_run",
                )
            explicit_episode_number = state.get("episode_number")
            expected_episodes = {
                episode_number
                for episode_number, _, _ in _episode_ranges(
                    script_body,
                    authoritative_episode_number=explicit_episode_number,
                )
                if episode_number is not None
            }
            if explicit_episode_number is not None:
                expected_episodes.add(explicit_episode_number)
            episode_summaries = {
                candidate.proposal.episode_number
                for candidate in output.candidates
                if isinstance(candidate.proposal, ContinuityCandidateProposal)
                and candidate.proposal.scope == "episode"
                and candidate.proposal.episode_number is not None
            }
            missing_episodes = sorted(expected_episodes - episode_summaries)
            if missing_episodes:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_output_incomplete",
                    summary=(
                        "Deep script extraction did not produce episode summaries for: "
                        + ", ".join(str(item) for item in missing_episodes)
                    ),
                    retryable=False,
                    next_action="start_new_skill_run",
                )
            production_bible = state.get("production_bible")
            if production_bible is not None:
                raw_entities = production_bible.get("entities", [])
                entity_states: dict[str, set[str]] = {}
                entity_kinds: dict[str, str] = {}
                if isinstance(raw_entities, list):
                    for raw_entity in cast(list[object], raw_entities):
                        if not isinstance(raw_entity, dict):
                            continue
                        entity = cast(dict[str, object], raw_entity)
                        entity_key = str(entity.get("entity_key", ""))
                        entity_kinds[entity_key] = str(entity.get("kind", ""))
                        raw_states = entity.get("states", [])
                        entity_states[entity_key] = {
                            str(cast(dict[str, object], item).get("state_key", ""))
                            for item in cast(list[object], raw_states)
                            if isinstance(item, dict)
                        }
                for candidate in output.candidates:
                    proposal = candidate.proposal
                    if proposal.kind == "asset":
                        raise SkillExecutionError(
                            outcome="failed",
                            code="skill_output_invalid",
                            summary="Bible-bound extraction emitted an independent asset",
                            retryable=False,
                            next_action="start_new_skill_run",
                        )
                    if isinstance(proposal, AssetOccurrenceCandidateProposal) and (
                        proposal.entity_key not in entity_states
                        or proposal.state_key not in entity_states[proposal.entity_key]
                        or proposal.role != entity_kinds[proposal.entity_key]
                    ):
                        raise SkillExecutionError(
                            outcome="failed",
                            code="skill_output_invalid",
                            summary="Asset occurrence references an unknown Production Bible state",
                            retryable=False,
                            next_action="start_new_skill_run",
                        )
            return {"output": output}

        async def candidate_gate(state: _ScriptStructureState) -> dict[str, object]:
            del state
            if not self._skill.candidate_only:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_side_effect_policy_invalid",
                    summary="MVP Skills must produce candidates only",
                    retryable=False,
                    next_action="contact_support",
                )
            return {}

        builder: Any = StateGraph(_ScriptStructureState)
        builder.add_node("validate_input", validate_input)
        builder.add_node("segment_script", segment)
        builder.add_node("extract_chunk", extract_chunk)
        builder.add_node("aggregate_candidates", aggregate)
        builder.add_node("validate_output", validate_output)
        builder.add_node("candidate_gate", candidate_gate)
        builder.add_edge(START, "validate_input")
        builder.add_edge("validate_input", "segment_script")
        builder.add_conditional_edges("segment_script", fan_out)
        builder.add_edge("extract_chunk", "aggregate_candidates")
        builder.add_edge("aggregate_candidates", "validate_output")
        builder.add_edge("validate_output", "candidate_gate")
        builder.add_edge("candidate_gate", END)
        if self._checkpointer is None:
            return builder.compile()
        return builder.compile(checkpointer=self._checkpointer)
