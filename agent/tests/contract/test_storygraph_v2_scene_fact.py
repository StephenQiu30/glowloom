from __future__ import annotations

import hashlib
import json
from datetime import UTC, datetime
from pathlib import Path
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.candidate_runtime.v2_schemas import (
    OwnerVersionIdentityV1,
    StageControlProofV2,
    StageReleaseIdentityV2,
    StageVariantKeyV2,
    StoryGraphV2ExecutionBudget,
    StoryGraphV2Invocation,
    StoryGraphV2Payload,
    StoryGraphV2Scope,
    StoryGraphV2Shard,
)
from app.modules.storygraph.v2_bundle import StoryGraphV2Bundle
from app.modules.storygraph.v2_candidate_schemas import (
    EvidenceSpanV2,
    SceneFactCandidateV2,
    SceneFactV2,
    ScriptSpanCandidateV2,
    ScriptSpanV2,
)

ZERO = "0" * 64
TWO = "2" * 64
WORKSPACE_ID = UUID("11111111-1111-4111-8111-111111111111")
PROJECT_ID = UUID("22222222-2222-4222-8222-222222222222")
SOURCE_ID = UUID("33333333-3333-4333-8333-333333333333")
INVOCATION_ID = UUID("44444444-4444-4444-8444-444444444444")
ATTEMPT_ID = UUID("55555555-5555-4555-8555-555555555555")
RELEASE_ID = UUID("66666666-6666-4666-8666-666666666666")
CONTROL_ID = UUID("77777777-7777-4777-8777-777777777777")
MANIFEST_ID = UUID("88888888-8888-4888-8888-888888888888")
REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-stage-wire-v2.json"


def _hash(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _source_ref(text: str) -> OwnerVersionIdentityV1:
    return OwnerVersionIdentityV1(
        owner_kind="production/script-source",
        logical_id="script:demo",
        version_id=SOURCE_ID,
        revision=1,
        content_hash=_hash(text),
        created_at=datetime(2026, 8, 31, tzinfo=UTC),
    )


def _invocation(text: str) -> StoryGraphV2Invocation:
    bundle = StoryGraphV2Bundle()
    return StoryGraphV2Invocation.build(
        invocation_id=INVOCATION_ID,
        attempt_id=ATTEMPT_ID,
        stage_release=StageReleaseIdentityV2(
            release_id=RELEASE_ID,
            definition_hash=TWO,
            bundle_hash=bundle.manifest.skill_bundle_hash,
            agent_image_digest=f"sha256:{ZERO}",
        ),
        control=StageControlProofV2(
            record_id=CONTROL_ID,
            revision=1,
            status="approved",
            content_hash=TWO,
        ),
        budget=StoryGraphV2ExecutionBudget(
            max_model_calls=1,
            max_execution_seconds=120,
            max_output_bytes=131072,
        ),
        payload=StoryGraphV2Payload(
            variant=StageVariantKeyV2(
                stage_key="propose_script_spans",
                profile_key="default",
                lane_key="primary",
                output_schema_version="script-span-candidate-v1",
            ),
            scope=StoryGraphV2Scope(
                workspace_id=WORKSPACE_ID,
                project_id=PROJECT_ID,
                episode_id=None,
            ),
            source_refs=[_source_ref(text)],
            upstream_candidates=[],
            shard=StoryGraphV2Shard(
                manifest_id=MANIFEST_ID,
                manifest_hash=TWO,
                shard_key="script:full",
                codepoint_start=0,
                codepoint_end=len(text),
            ),
            stage_input={
                "source_version_id": str(SOURCE_ID),
                "source_hash": _hash(text),
                "normalized_text": text,
                "codepoint_count": len(text),
                "newline_normalization": "lf",
            },
        ),
    )


def test_v2_script_span_candidate_requires_exact_codepoint_coverage() -> None:
    text = "第一场 夜 内\n林舟握住门把。\n第二场 日 外\n林舟离开。"
    invocation = _invocation(text)
    assert invocation.wire_schema_version == "storygraph-stage-wire-v2"
    assert invocation.input_hash == invocation.compute_input_hash()
    stage_instance_key = invocation.stage_instance_key()
    assert len(stage_instance_key) == 64
    int(stage_instance_key, 16)

    candidate = ScriptSpanCandidateV2(
        source_version_id=SOURCE_ID,
        source_hash=_hash(text),
        codepoint_count=len(text),
        spans=[
            ScriptSpanV2(
                temporary_span_id="span_0001",
                kind="scene",
                codepoint_start=0,
                codepoint_end=16,
                heading="第一场 夜 内",
                evidence=EvidenceSpanV2(
                    source_start=0,
                    source_end=7,
                    text_hash=_hash("第一场 夜 内"),
                    exact_anchor="第一场 夜 内",
                ),
            ),
            ScriptSpanV2(
                temporary_span_id="span_0002",
                kind="scene",
                codepoint_start=16,
                codepoint_end=len(text),
                heading="第二场 日 外",
                evidence=EvidenceSpanV2(
                    source_start=16,
                    source_end=23,
                    text_hash=_hash("第二场 日 外"),
                    exact_anchor="第二场 日 外",
                ),
            ),
        ],
        review_issues=[],
    )
    candidate.validate_for_text(text)

    with pytest.raises(ValidationError):
        ScriptSpanCandidateV2(
            source_version_id=SOURCE_ID,
            source_hash=_hash(text),
            codepoint_count=len(text),
            spans=[
                ScriptSpanV2(
                    temporary_span_id="span_0001",
                    kind="scene",
                    codepoint_start=0,
                    codepoint_end=15,
                    heading="第一场 夜 内",
                    evidence=EvidenceSpanV2(
                        source_start=0,
                        source_end=7,
                        text_hash=_hash("第一场 夜 内"),
                        exact_anchor="第一场 夜 内",
                    ),
                ),
                ScriptSpanV2(
                    temporary_span_id="span_0002",
                    kind="scene",
                    codepoint_start=16,
                    codepoint_end=len(text),
                    heading="第二场 日 外",
                    evidence=EvidenceSpanV2(
                        source_start=16,
                        source_end=23,
                        text_hash=_hash("第二场 日 外"),
                        exact_anchor="第二场 日 外",
                    ),
                ),
            ],
            review_issues=[],
        )


def test_v2_scene_fact_is_style_blind_and_bound_to_script_spans() -> None:
    text = "第一场 夜 内\n林舟握住门把。"
    fact = SceneFactCandidateV2(
        source_version_id=SOURCE_ID,
        source_hash=_hash(text),
        span_candidate_revision_id=UUID("99999999-9999-4999-8999-999999999999"),
        span_candidate_revision_hash=TWO,
        scenes=[
            SceneFactV2.model_validate(
                {
                    "temporary_scene_id": "scene_0001",
                    "span_id": "span_0001",
                    "source_start": 0,
                    "source_end": len(text),
                    "location_text": "室内",
                    "time_text": "夜",
                    "actions": [
                        {
                            "text": "林舟握住门把",
                            "evidence": {
                                "source_start": 8,
                                "source_end": len(text) - 1,
                                "text_hash": _hash("林舟握住门把"),
                                "exact_anchor": "林舟握住门把",
                            },
                        }
                    ],
                    "dialogues": [],
                    "raw_character_mentions": [
                        {
                            "text": "林舟",
                            "evidence": {
                                "source_start": 8,
                                "source_end": 10,
                                "text_hash": _hash("林舟"),
                                "exact_anchor": "林舟",
                            },
                        }
                    ],
                    "raw_prop_mentions": [
                        {
                            "text": "门把",
                            "evidence": {
                                "source_start": 12,
                                "source_end": 14,
                                "text_hash": _hash("门把"),
                                "exact_anchor": "门把",
                            },
                        }
                    ],
                }
            )
        ],
        review_issues=[],
    )
    fact.validate_for_spans(
        text,
        [
            ScriptSpanV2(
                temporary_span_id="span_0001",
                kind="scene",
                codepoint_start=0,
                codepoint_end=len(text),
                heading="第一场 夜 内",
                evidence=EvidenceSpanV2(
                    source_start=0,
                    source_end=7,
                    text_hash=_hash("第一场 夜 内"),
                    exact_anchor="第一场 夜 内",
                ),
            )
        ],
    )

    with pytest.raises(ValidationError):
        SceneFactV2.model_validate(
            {
                **fact.scenes[0].model_dump(mode="json"),
                "visual_style": "赛博朋克",
            }
        )


def test_v2_bundle_discloses_only_stage_specific_references() -> None:
    bundle = StoryGraphV2Bundle()
    assert bundle.loaded_paths("propose_script_spans") == (
        "SKILL.md",
        "references/script-spans.md",
    )
    assert bundle.loaded_paths("extract_scene_facts") == (
        "SKILL.md",
        "references/scene-facts.md",
    )
    assert bundle.compute_hash() == bundle.manifest.skill_bundle_hash


def test_v2_wire_matches_the_shared_go_python_fixture_and_rejects_mutations() -> None:
    fixture = json.loads(WIRE_FIXTURE.read_text(encoding="utf-8"))
    invocation = StoryGraphV2Invocation.model_validate(fixture["valid_invocation"])
    assert invocation.input_hash == fixture["expected_input_hash"]
    assert invocation.stage_instance_key() == fixture["expected_stage_instance_key"]

    changed_identity = invocation.model_copy(
        update={
            "invocation_id": UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
            "attempt_id": UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
        }
    )
    assert changed_identity.compute_input_hash() == invocation.input_hash

    for mutation in fixture["reject_mutations"]:
        raw = json.loads(json.dumps(fixture["valid_invocation"], ensure_ascii=False))
        operation = mutation["operation"]
        if operation == "remove":
            del raw[mutation["path"]]
        elif operation == "add_stage_input":
            raw["payload"]["stage_input"][mutation["path"]] = mutation["value"]
        elif operation == "replace_scope":
            raw["payload"]["scope"][mutation["path"]] = mutation["value"]
        else:
            raw[mutation["path"]] = mutation["value"]
        with pytest.raises(ValidationError):
            StoryGraphV2Invocation.model_validate(raw)
