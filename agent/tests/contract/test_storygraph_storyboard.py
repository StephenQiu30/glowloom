from __future__ import annotations

import json
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.candidate_runtime.schemas import StoryGraphStageInvocation, StoryGraphStagePayload
from app.modules.storygraph.candidate_schemas import StoryboardRowCandidate

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-stage-wire-v1.json"


def source_invocation() -> StoryGraphStageInvocation:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    return StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])


def test_draft_storyboard_rejects_empty_formal_input() -> None:
    source = source_invocation()
    graph_id = "70000000-0000-0000-0000-000000000001"
    with pytest.raises(ValidationError):
        StoryGraphStagePayload.model_validate(
            {
                "stage": "draft_storyboard",
                "shard_key": "scene:scene-1",
                "workspace_id": source.payload.workspace_id,
                "project_id": source.payload.project_id,
                "source_refs": [
                    {
                        "owner_kind": "production/storygraph",
                        "owner_logical_id": str(source.payload.project_id),
                        "owner_version_id": graph_id,
                        "revision": 1,
                        "content_hash": "a" * 64,
                    }
                ],
                "base_storygraph_version_id": graph_id,
                "base_storygraph_hash": "a" * 64,
                "upstream_candidates": [],
                "shard_manifest_ref": {
                    "manifest_id": "70000000-0000-0000-0000-000000000003",
                    "version": 1,
                    "hash": "b" * 64,
                },
                "shard": {
                    "kind": "story_scene",
                    "key": "scene:scene-1",
                    "tree_path": "scene/0001",
                },
                "stage_input": {},
            }
        )


def test_storyboard_candidate_models_reviewable_needs_asset_intent() -> None:
    value = StoryboardRowCandidate.model_validate(
        {
            "scene_story_node_key": "sgn_" + "1" * 64,
            "shot_intents": [
                {
                    "shot_key": "shot-001",
                    "intent_order": 1,
                    "source_beat_story_node_keys": ["sgn_" + "2" * 64],
                    "source_evidence": [
                        {
                            "document_revision_id": "70000000-0000-0000-0000-000000000004",
                            "absolute_start": 10,
                            "absolute_end": 14,
                            "text_hash": "3" * 64,
                        }
                    ],
                    "purpose": "建立人物动作",
                    "proposed_duration_ms": 2500,
                    "camera": {
                        "scale": "medium",
                        "angle": "eye_level",
                        "movement": "static",
                        "composition": "centered",
                    },
                    "action_intent": "人物进入画面",
                    "dialogue_intent": None,
                    "sound_intent": "环境声",
                    "performance_intent": "克制",
                    "continuity_in": "承接入场",
                    "continuity_out": "保持视线方向",
                    "frame_intent": {
                        "first": "空镜",
                        "key": "人物入画",
                        "last": "人物停步",
                    },
                    "visual_requirements": [
                        {
                            "occurrence_story_node_key": "sgn_" + "4" * 64,
                            "identity_story_node_key": "sgn_" + "5" * 64,
                            "specification_story_node_key": "sgn_" + "6" * 64,
                            "asset_state_story_node_key": "sgn_" + "7" * 64,
                            "asset_id": "70000000-0000-0000-0000-000000000005",
                            "specification_version_id": "70000000-0000-0000-0000-000000000006",
                            "asset_state_id": "70000000-0000-0000-0000-000000000007",
                            "asset_role": "subject",
                            "required_view_roles": ["front", "profile", "back"],
                            "asset_readiness": "needs_asset",
                            "asset_version_ref": None,
                        }
                    ],
                    "risk_codes": ["reference_asset_missing"],
                    "review_issues": [],
                }
            ],
            "asset_readiness": "needs_asset",
        }
    )
    assert value.asset_readiness == "needs_asset"
    assert value.shot_intents[0].visual_requirements[0].asset_version_ref is None


def test_storyboard_candidate_rejects_changed_visual_role() -> None:
    stage_input, candidate = exact_storyboard_contract()
    candidate["shot_intents"][0]["visual_requirements"][0]["asset_role"] = "prop"

    with pytest.raises(ValueError, match="changed its frozen identity or state"):
        StoryboardRowCandidate.model_validate(candidate).validate_for(stage_input)


def test_storyboard_candidate_rejects_review_evidence_outside_scene() -> None:
    stage_input, candidate = exact_storyboard_contract()
    candidate["shot_intents"][0]["review_issues"] = [
        {
            "code": "ambiguous_blocking",
            "severity": "blocking",
            "summary": "需要人工确认动作",
            "evidence": [
                {
                    "document_revision_id": "70000000-0000-0000-0000-000000000004",
                    "absolute_start": 20,
                    "absolute_end": 24,
                    "text_hash": "4" * 64,
                }
            ],
        }
    ]

    with pytest.raises(ValueError, match="Review Issue Evidence is outside its exact Scene"):
        StoryboardRowCandidate.model_validate(candidate).validate_for(stage_input)


def exact_storyboard_contract() -> tuple[Any, dict[str, Any]]:
    from app.candidate_runtime.schemas import StoryboardDraftStageInput

    stage_input = StoryboardDraftStageInput.model_validate(
        {
            "graph_version_no": 1,
            "scene": {
                "story_node_key": "sgn_" + "1" * 64,
                "owner_version_id": "70000000-0000-0000-0000-000000000011",
                "owner_revision": 1,
                "owner_hash": "9" * 64,
                "episode_id": "70000000-0000-0000-0000-000000000012",
                "episode_position": 1,
                "scene_position": 1,
                "heading": "内景 客厅 日",
                "evidence": [
                    {
                        "document_revision_id": "70000000-0000-0000-0000-000000000004",
                        "absolute_start": 10,
                        "absolute_end": 14,
                        "text_hash": "3" * 64,
                    }
                ],
            },
            "beats": [
                {
                    "story_node_key": "sgn_" + "2" * 64,
                    "summary": "人物进入",
                    "required_for_coverage": True,
                    "evidence": [
                        {
                            "document_revision_id": "70000000-0000-0000-0000-000000000004",
                            "absolute_start": 10,
                            "absolute_end": 14,
                            "text_hash": "3" * 64,
                        }
                    ],
                }
            ],
            "dialogues": [],
            "occurrences": [
                {
                    "story_node_key": "sgn_" + "4" * 64,
                    "identity_story_node_key": "sgn_" + "5" * 64,
                    "specification_story_node_key": "sgn_" + "6" * 64,
                    "asset_state_story_node_key": "sgn_" + "7" * 64,
                    "asset_id": "70000000-0000-0000-0000-000000000005",
                    "specification_version_id": "70000000-0000-0000-0000-000000000006",
                    "asset_state_id": "70000000-0000-0000-0000-000000000007",
                    "asset_kind": "character",
                    "summary": "人物出现",
                    "evidence": [
                        {
                            "document_revision_id": "70000000-0000-0000-0000-000000000004",
                            "absolute_start": 10,
                            "absolute_end": 14,
                            "text_hash": "3" * 64,
                        }
                    ],
                }
            ],
            "effective_style_snapshot": {
                "owner_version_id": "70000000-0000-0000-0000-000000000013",
                "revision": 1,
                "content_hash": "8" * 64,
                "visual_style": "cinematic",
                "aspect_ratio": "9:16",
            },
            "target_duration_ms": 90_000,
            "asset_versions": [],
        }
    )
    candidate = {
        "scene_story_node_key": "sgn_" + "1" * 64,
        "shot_intents": [
            {
                "shot_key": "shot-001",
                "intent_order": 1,
                "source_beat_story_node_keys": ["sgn_" + "2" * 64],
                "source_evidence": [
                    {
                        "document_revision_id": "70000000-0000-0000-0000-000000000004",
                        "absolute_start": 10,
                        "absolute_end": 14,
                        "text_hash": "3" * 64,
                    }
                ],
                "purpose": "建立人物动作",
                "proposed_duration_ms": 2500,
                "camera": {
                    "scale": "medium",
                    "angle": "eye_level",
                    "movement": "static",
                    "composition": "centered",
                },
                "action_intent": "人物进入画面",
                "dialogue_intent": None,
                "sound_intent": "环境声",
                "performance_intent": "克制",
                "continuity_in": "承接入场",
                "continuity_out": "保持视线方向",
                "frame_intent": {"first": "空镜", "key": "人物入画", "last": "人物停步"},
                "visual_requirements": [
                    {
                        "occurrence_story_node_key": "sgn_" + "4" * 64,
                        "identity_story_node_key": "sgn_" + "5" * 64,
                        "specification_story_node_key": "sgn_" + "6" * 64,
                        "asset_state_story_node_key": "sgn_" + "7" * 64,
                        "asset_id": "70000000-0000-0000-0000-000000000005",
                        "specification_version_id": "70000000-0000-0000-0000-000000000006",
                        "asset_state_id": "70000000-0000-0000-0000-000000000007",
                        "asset_role": "subject",
                        "required_view_roles": ["front", "profile", "back"],
                        "asset_readiness": "needs_asset",
                        "asset_version_ref": None,
                    }
                ],
                "risk_codes": ["reference_asset_missing"],
                "review_issues": [],
            }
        ],
        "asset_readiness": "needs_asset",
    }
    return stage_input, candidate
