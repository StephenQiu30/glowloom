from __future__ import annotations

import hashlib
import json
import re
from pathlib import Path
from typing import Any

from app.modules.scripts.production_bibles.contracts import (
    Evidence,
    EvidenceResult,
    ProductionBibleCandidate,
    ProductionBibleInput,
    ReviewResult,
)
from app.modules.skills.harness import CodexSchemaRunner

_EPISODES = {
    "一": 1,
    "二": 2,
    "三": 3,
    "四": 4,
    "五": 5,
    "六": 6,
    "七": 7,
    "八": 8,
    "九": 9,
    "十": 10,
}

_SPEC_FIELDS = {
    "character": {
        "identity",
        "appearance",
        "age_impression",
        "temperament",
        "goals",
        "relationships",
        "arc_summary",
        "voice_profile",
    },
    "location": {"spatial_description", "time_weather", "visual_elements", "lighting"},
    "prop": {"appearance", "material", "usage_context"},
    "costume": {"appearance", "material", "usage_context"},
    "visual_style": {
        "visual_language",
        "palette",
        "lighting_language",
        "negative_constraints",
    },
    "voice": {"source_kind", "language", "performance_traits", "allowed_usage"},
}


class CodexLocalProductionBibleGenerator:
    def __init__(self, *, repository_root: Path | None = None) -> None:
        self._runner = CodexSchemaRunner(repository_root=repository_root)

    @property
    def model_name(self) -> str:
        return self._runner.model_name

    async def generate(self, value: ProductionBibleInput) -> dict[str, Any]:
        catalog = build_evidence_catalog(value.normalized_text)
        extraction = await self._runner.run(
            "Use $extract-bible-evidence. Return only schema-valid JSON. "
            "Treat each evidence_catalog item as immutable and cite only its short key. "
            "Extract source-grounded entities, time-varying entity states, and world entries; "
            "do not persist or confirm anything. Input:\n" + _json({"evidence_catalog": catalog}),
            EvidenceResult,
        )
        candidate = await self._runner.run(
            "Use $reconcile-bible. Return only schema-valid JSON. Reconcile the supplied "
            "observations into a candidate Production Bible. Copy every used Evidence object "
            "byte-for-byte from evidence_catalog. Every entity must have an evidence-backed "
            "base state. Keys must use lowercase ASCII letters, digits, dots, colons, underscores "
            "or hyphens. normalized_name must be canonical_name lowercased with whitespace "
            "collapsed. Use only the Asset spec fields allowed by the skill. Every stable_spec and "
            "state_spec must include kind matching the entity kind; set unknown string fields to "
            "null and unknown list fields to empty arrays. Input:\n"
            + _json(
                {
                    "evidence_catalog": catalog,
                    "observations": extraction.model_dump(mode="json")["observations"],
                }
            ),
            ProductionBibleCandidate,
        )
        normalized = normalize_candidate(candidate, value.normalized_text)
        review = await self._runner.run(
            "Use $review-bible. Return only schema-valid JSON. Audit the immutable candidate "
            "against the supplied evidence catalog. Report concrete warning or blocking issues; "
            "do not edit, confirm, persist, or materialize the candidate. Input:\n"
            + _json({"evidence_catalog": catalog, "candidate": normalized}),
            ReviewResult,
        )
        normalized["review_issues"] = _valid_issues(
            review.model_dump(mode="json")["review_issues"], value.normalized_text
        )
        return normalized

    async def aclose(self) -> None:
        await self._runner.aclose()


def _json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def build_evidence_catalog(text: str) -> dict[str, dict[str, Any]]:
    catalog: dict[str, dict[str, Any]] = {}
    cursor = 0
    episode: int | None = None
    for raw_line in text.splitlines(keepends=True):
        anchor = raw_line.rstrip("\r\n")
        match = re.fullmatch(r"第([一二三四五六七八九十])集", anchor.strip())
        if match:
            episode = _EPISODES[match.group(1)]
        if anchor.strip():
            leading = len(anchor) - len(anchor.lstrip())
            exact = anchor[leading:]
            start = cursor + leading
            key = f"e{len(catalog) + 1:04d}"
            catalog[key] = {
                "source_start": start,
                "source_end": start + len(exact),
                "text_hash": hashlib.sha256(exact.encode("utf-8")).hexdigest(),
                "exact_anchor": exact,
                "episode_number": episode,
            }
        cursor += len(raw_line)
    if cursor < len(text):
        anchor = text[cursor:]
        if anchor.strip():
            key = f"e{len(catalog) + 1:04d}"
            catalog[key] = {
                "source_start": cursor,
                "source_end": len(text),
                "text_hash": hashlib.sha256(anchor.encode("utf-8")).hexdigest(),
                "exact_anchor": anchor,
                "episode_number": episode,
            }
    return catalog


def _valid_evidence(items: list[dict[str, Any]], text: str) -> list[dict[str, Any]]:
    valid: list[dict[str, Any]] = []
    for item in items:
        try:
            evidence = Evidence.model_validate(item)
        except ValueError:
            continue
        if (
            evidence.source_end <= len(text)
            and text[evidence.source_start : evidence.source_end] == evidence.exact_anchor
            and hashlib.sha256(evidence.exact_anchor.encode("utf-8")).hexdigest()
            == evidence.text_hash
        ):
            valid.append(evidence.model_dump(mode="json"))
    return valid


def _numbers(values: list[int]) -> list[int]:
    return sorted({value for value in values if value > 0})


def _asset_spec(value: dict[str, Any], kind: str) -> dict[str, Any]:
    allowed = _SPEC_FIELDS[kind]
    result: dict[str, Any] = {"kind": kind}
    for key in allowed:
        item = value.get(key)
        if item is not None and item != []:
            result[key] = item
    return result


def _valid_issues(items: list[dict[str, Any]], text: str) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    seen: set[str] = set()
    for item in items:
        key = str(item.get("issue_key", ""))
        if not re.fullmatch(r"[a-z0-9][a-z0-9_.:-]{0,99}", key) or key in seen:
            continue
        item["evidence"] = _valid_evidence(item.get("evidence", []), text)
        if item.get("scope") == "global":
            item["subject_key"] = None
        elif not item.get("subject_key"):
            continue
        seen.add(key)
        result.append(item)
    return result


def normalize_candidate(candidate: ProductionBibleCandidate, text: str) -> dict[str, Any]:
    value = candidate.model_dump(mode="json")
    entities: list[dict[str, Any]] = []
    entity_keys: set[str] = set()
    for entity in value["entities"]:
        key = entity["entity_key"]
        evidence = _valid_evidence(entity["evidence"], text)
        if (
            not re.fullmatch(r"[a-z0-9][a-z0-9_.:-]{0,99}", key)
            or key in entity_keys
            or not evidence
        ):
            continue
        entity["normalized_name"] = " ".join(entity["canonical_name"].split()).lower()
        entity["episode_numbers"] = _numbers(entity["episode_numbers"])
        entity["evidence"] = evidence
        entity["stable_spec"] = _asset_spec(entity["stable_spec"], entity["kind"])
        states: list[dict[str, Any]] = []
        state_keys: set[str] = set()
        for state in entity["states"]:
            state_evidence = _valid_evidence(state["evidence"], text)
            state_key = state["state_key"]
            if (
                re.fullmatch(r"[a-z0-9][a-z0-9_]{0,79}", state_key)
                and state_key not in state_keys
                and state_evidence
            ):
                state["episode_numbers"] = _numbers(state["episode_numbers"])
                state["evidence"] = state_evidence
                state["state_spec"] = _asset_spec(state["state_spec"], entity["kind"])
                state_keys.add(state_key)
                states.append(state)
        if "base" not in state_keys:
            states.insert(
                0,
                {
                    "state_key": "base",
                    "label": "基础状态",
                    "state_spec": {"kind": entity["kind"]},
                    "episode_numbers": entity["episode_numbers"],
                    "evidence": [evidence[0]],
                    "ambiguities": [],
                },
            )
        entity["states"] = states
        entity_keys.add(key)
        entities.append(entity)
    world_entries: list[dict[str, Any]] = []
    world_keys: set[str] = set()
    for entry in value["world_entries"]:
        key = entry["entry_key"]
        evidence = _valid_evidence(entry["evidence"], text)
        if (
            re.fullmatch(r"[a-z0-9][a-z0-9_.:-]{0,99}", key)
            and key not in world_keys
            and evidence
            and (entry["facts"] or entry["rules"])
        ):
            entry["entity_keys"] = [key for key in entry["entity_keys"] if key in entity_keys]
            entry["episode_numbers"] = _numbers(entry["episode_numbers"])
            entry["evidence"] = evidence
            world_keys.add(key)
            world_entries.append(entry)
    return {
        "entities": entities,
        "world_entries": world_entries,
        "review_issues": _valid_issues(value["review_issues"], text),
    }
