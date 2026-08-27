from __future__ import annotations

import json
from dataclasses import asdict
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.modules.storygraph.bundle import StoryGraphBundle
from app.modules.storygraph.skill_registry import REGISTRY, RegistryError, stage_spec

EXPECTED = {
    "extract_source_evidence": ("source_evidence_candidate", ("source-evidence.md",)),
    "analyze_story": (
        "story_analysis_candidate",
        ("story-analysis.md", "entity-reconciliation.md"),
    ),
    "reconcile_story": (
        "story_reconciliation_candidate",
        ("entity-reconciliation.md", "story-analysis.md"),
    ),
    "segment_episodes": ("episode_segmentation_candidate", ("episode-segmentation.md",)),
    "analyze_episode": (
        "episode_analysis_candidate",
        ("scene-structure.md", "visual-identity.md"),
    ),
    "reconcile_episode": (
        "episode_reconciliation_candidate",
        ("scene-structure.md", "continuity-review.md"),
    ),
    "draft_storyboard": (
        "storyboard_row_candidate",
        ("storyboard-table.md", "visual-identity.md"),
    ),
    "detail_shots": (
        "shot_detail_candidate",
        ("shot-detail.md", "visual-identity.md"),
    ),
    "review_storygraph": ("storygraph_review_candidate", ("continuity-review.md",)),
    "repair_candidate": ("candidate_repair_patch", ("continuity-review.md",)),
}


def test_registry_has_exactly_ten_backend_owned_stages_and_candidate_schemas() -> None:
    assert len(REGISTRY) == 10
    assert {
        stage: (spec.candidate_type, spec.references) for stage, spec in REGISTRY.items()
    } == EXPECTED
    assert all(spec.candidate_model.model_json_schema() for spec in REGISTRY.values())
    assert all(
        spec.candidate_model.model_json_schema().get("additionalProperties") is False
        for spec in REGISTRY.values()
    )

    repository_root = Path(__file__).resolve().parents[3]
    fixture = cast(
        dict[str, Any],
        json.loads(
            (
                repository_root / "backend/tests/fixtures/agent/storygraph-definition-v1.json"
            ).read_text(encoding="utf-8")
        ),
    )
    manifest = asdict(StoryGraphBundle(repository_root).manifest)
    manifest["allowed_tools"] = list(manifest["allowed_tools"])
    assert {key: fixture[key] for key in manifest} == manifest
    assert fixture["stages"] == [
        {
            "stage": stage,
            "candidate_type": spec.candidate_type,
            "references": list(spec.references),
        }
        for stage, spec in REGISTRY.items()
    ]


def test_registry_rejects_unknown_stage_and_does_not_track_generated_schemas() -> None:
    with pytest.raises(RegistryError):
        stage_spec("production_bible")

    repository_root = Path(__file__).resolve().parents[3]
    assert not list((repository_root / "agent/app").rglob("*.schema.json"))
    assert not list((repository_root / "agent/tests").rglob("*.schema.json"))


def test_candidate_schema_rejects_nested_business_write_shapes() -> None:
    episode = stage_spec("analyze_episode").candidate_model
    with pytest.raises(ValidationError):
        episode.model_validate(
            {
                "fragments": [
                    {
                        "temporary_key": "scene-1",
                        "kind": "scene",
                        "source_keys": ["source-1"],
                        "summary": "test",
                        "evidence": [
                            {
                                "source_start": 0,
                                "source_end": 1,
                                "text_hash": "a" * 64,
                                "exact_anchor": "测",
                                "episode_number": None,
                            }
                        ],
                        "attributes": {
                            "scene_key": None,
                            "speaker_key": None,
                            "participant_keys": [],
                            "location_key": None,
                            "time_hint": None,
                            "dialogue_text": None,
                            "action": None,
                            "occurrence_entity_key": None,
                            "state_key": None,
                            "continuity_notes": [],
                            "sql": "DELETE FROM story_graphs",
                        },
                    }
                ],
                "claims": [],
                "review_issues": [],
            }
        )


def test_every_candidate_schema_is_accepted_by_codex_strict_output_rules() -> None:
    def visit(node: Any, definitions: dict[str, Any]) -> None:
        if not isinstance(node, dict):
            return
        mapping = cast(dict[str, Any], node)
        reference = mapping.get("$ref")
        if isinstance(reference, str):
            visit(definitions[reference.removeprefix("#/$defs/")], definitions)
            return
        if mapping.get("type") == "object":
            properties = cast(dict[str, Any], mapping.get("properties", {}))
            assert mapping.get("additionalProperties") is False
            assert set(cast(list[str], mapping.get("required", []))) == set(properties)
            for child in properties.values():
                visit(child, definitions)
        for branch in cast(list[Any], mapping.get("anyOf", [])):
            visit(branch, definitions)
        items = mapping.get("items")
        if isinstance(items, dict):
            visit(items, definitions)

    for spec in REGISTRY.values():
        schema = spec.candidate_model.model_json_schema()
        definitions = cast(dict[str, Any], schema.get("$defs", {}))
        visit(schema, definitions)
