from __future__ import annotations

from app.modules.scripts.production_bibles.codex_local import (
    build_evidence_catalog,
    normalize_candidate,
)
from app.modules.scripts.production_bibles.contracts import ProductionBibleCandidate


def _empty_character_spec() -> dict[str, object]:
    return {
        "kind": "character",
        "identity": None,
        "appearance": None,
        "age_impression": None,
        "temperament": [],
        "goals": [],
        "relationships": [],
        "arc_summary": None,
        "voice_profile": None,
        "spatial_description": None,
        "time_weather": None,
        "visual_elements": [],
        "lighting": None,
        "material": None,
        "usage_context": None,
        "visual_language": None,
        "palette": None,
        "lighting_language": None,
        "negative_constraints": [],
        "source_kind": None,
        "language": None,
        "performance_traits": [],
        "allowed_usage": [],
    }


def test_evidence_catalog_uses_document_absolute_rune_offsets() -> None:
    source = "第一集\n沈岚打开铜制钥匙。\n"

    catalog = build_evidence_catalog(source)

    evidence = catalog["e0002"]
    assert evidence["episode_number"] == 1
    assert source[evidence["source_start"] : evidence["source_end"]] == "沈岚打开铜制钥匙。"


def test_candidate_normalization_adds_evidence_backed_base_state() -> None:
    source = "第一集\n沈岚打开铜制钥匙。\n"
    evidence = build_evidence_catalog(source)["e0002"]
    candidate = ProductionBibleCandidate.model_validate(
        {
            "entities": [
                {
                    "entity_key": "character.shen_lan",
                    "kind": "character",
                    "canonical_name": "沈岚",
                    "normalized_name": "incorrect",
                    "aliases": [],
                    "stable_spec": _empty_character_spec(),
                    "episode_numbers": [1, 1],
                    "evidence": [evidence],
                    "states": [],
                    "ambiguities": [],
                }
            ],
            "world_entries": [],
            "review_issues": [],
        }
    )

    result = normalize_candidate(candidate, source)

    entity = result["entities"][0]
    assert entity["normalized_name"] == "沈岚"
    assert entity["episode_numbers"] == [1]
    assert entity["states"] == [
        {
            "state_key": "base",
            "label": "基础状态",
                "state_spec": {"kind": "character"},
            "episode_numbers": [1],
            "evidence": [evidence],
            "ambiguities": [],
        }
    ]
