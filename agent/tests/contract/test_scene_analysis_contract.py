from __future__ import annotations

import hashlib
import json
import shutil
from dataclasses import replace
from datetime import UTC, datetime
from pathlib import Path
from typing import Any
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.candidate_runtime.canonical import production_canonical_hash
from app.candidate_runtime.scene_analysis_schemas import (
    SceneAnalysisAttemptResult,
    SceneAnalysisControlProof,
    SceneAnalysisExecutionBudget,
    SceneAnalysisInvocation,
    SceneAnalysisPayload,
    SceneAnalysisReleaseIdentity,
    SceneAnalysisScope,
    SceneAnalysisShard,
    SceneAnalysisStageVariant,
    ScriptSourceVersionIdentity,
)
from app.modules.storygraph.bundle import BundleInvalid
from app.modules.storygraph.scene_analysis_bundle import SceneAnalysisBundle
from app.modules.storygraph.scene_analysis_candidates import (
    SceneFact,
    SceneFactCandidate,
    ScriptSceneSpan,
    ScriptSpanCandidate,
    ScriptSpanCoverageProof,
    SourceEvidenceSpan,
)
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness

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
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-scene-analysis-wire.json"


def _hash(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _source_ref(text: str) -> ScriptSourceVersionIdentity:
    return ScriptSourceVersionIdentity(
        owner_kind="production/script",
        logical_id="script:demo",
        version_id=SOURCE_ID,
        revision=1,
        content_hash=_hash(text),
        created_at=datetime(2026, 8, 31, tzinfo=UTC),
    )


def _invocation(text: str) -> SceneAnalysisInvocation:
    bundle = SceneAnalysisBundle()
    return SceneAnalysisInvocation.build(
        invocation_id=INVOCATION_ID,
        attempt_id=ATTEMPT_ID,
        stage_release=SceneAnalysisReleaseIdentity(
            skill_release_id=RELEASE_ID,
            skill_release_hash=TWO,
            stage_release_hash=TWO,
            bundle_content_hash=bundle.manifest.skill_bundle_hash,
            agent_image_digest=f"sha256:{ZERO}",
        ),
        control=SceneAnalysisControlProof(
            control_record_id=CONTROL_ID,
            control_revision=1,
            status="approved",
            control_hash=TWO,
            release_fence=0,
        ),
        budget=SceneAnalysisExecutionBudget(
            max_attempts=2,
            max_model_calls=1,
            max_execution_seconds=120,
            max_output_bytes=131072,
        ),
        payload=SceneAnalysisPayload(
            variant=SceneAnalysisStageVariant(
                stage_key="propose_script_spans",
                profile_key="default",
                lane_key="primary",
                output_schema_version="script-span-candidate-production",
            ),
            scope=SceneAnalysisScope(
                workspace_id=WORKSPACE_ID,
                project_id=PROJECT_ID,
                episode_id=None,
                scene_id=None,
                entity_id=None,
                target_id=None,
            ),
            source_refs=[_source_ref(text)],
            upstream_candidates=[],
            shard=SceneAnalysisShard(
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


def test_scene_analysis_script_span_candidate_requires_exact_codepoint_coverage() -> None:
    text = "第一场 夜 内\n林舟握住门把。\n第二场 日 外\n林舟离开。"
    invocation = _invocation(text)
    assert invocation.wire_schema_version == "storygraph-stage-wire-production"
    assert invocation.input_hash == invocation.compute_input_hash()
    stage_instance_key = invocation.stage_instance_key()
    assert len(stage_instance_key) == 64
    int(stage_instance_key, 16)

    candidate = ScriptSpanCandidate(
        source_version_id=SOURCE_ID,
        source_hash=_hash(text),
        codepoint_count=len(text),
        coverage=ScriptSpanCoverageProof(
            source_hash=_hash(text),
            codepoint_start=0,
            codepoint_end=len(text),
            covered_codepoints=len(text),
        ),
        spans=[
            ScriptSceneSpan(
                temporary_span_id="span_0001",
                kind="scene",
                codepoint_start=0,
                codepoint_end=16,
                heading="第一场 夜 内",
                evidence=SourceEvidenceSpan(
                    source_start=0,
                    source_end=7,
                    text_hash=_hash("第一场 夜 内"),
                    exact_anchor="第一场 夜 内",
                ),
            ),
            ScriptSceneSpan(
                temporary_span_id="span_0002",
                kind="scene",
                codepoint_start=16,
                codepoint_end=len(text),
                heading="第二场 日 外",
                evidence=SourceEvidenceSpan(
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
        ScriptSpanCandidate(
            source_version_id=SOURCE_ID,
            source_hash=_hash(text),
            codepoint_count=len(text),
            coverage=ScriptSpanCoverageProof(
                source_hash=_hash(text),
                codepoint_start=0,
                codepoint_end=len(text),
                covered_codepoints=len(text),
            ),
            spans=[
                ScriptSceneSpan(
                    temporary_span_id="span_0001",
                    kind="scene",
                    codepoint_start=0,
                    codepoint_end=15,
                    heading="第一场 夜 内",
                    evidence=SourceEvidenceSpan(
                        source_start=0,
                        source_end=7,
                        text_hash=_hash("第一场 夜 内"),
                        exact_anchor="第一场 夜 内",
                    ),
                ),
                ScriptSceneSpan(
                    temporary_span_id="span_0002",
                    kind="scene",
                    codepoint_start=16,
                    codepoint_end=len(text),
                    heading="第二场 日 外",
                    evidence=SourceEvidenceSpan(
                        source_start=16,
                        source_end=23,
                        text_hash=_hash("第二场 日 外"),
                        exact_anchor="第二场 日 外",
                    ),
                ),
            ],
            review_issues=[],
        )


def test_scene_analysis_scene_fact_is_style_blind_and_bound_to_script_spans() -> None:
    text = "第一场 夜 内\n林舟握住门把。"
    fact = SceneFactCandidate(
        source_version_id=SOURCE_ID,
        source_hash=_hash(text),
        span_candidate_revision_id=UUID("99999999-9999-4999-8999-999999999999"),
        span_candidate_revision_hash=TWO,
        scenes=[
            SceneFact.model_validate(
                {
                    "temporary_scene_id": "scene_0001",
                    "span_id": "span_0001",
                    "source_start": 0,
                    "source_end": len(text),
                    "location": {
                        "text": "内",
                        "evidence": {
                            "source_start": 6,
                            "source_end": 7,
                            "text_hash": _hash("内"),
                            "exact_anchor": "内",
                        },
                    },
                    "time": {
                        "text": "夜",
                        "evidence": {
                            "source_start": 4,
                            "source_end": 5,
                            "text_hash": _hash("夜"),
                            "exact_anchor": "夜",
                        },
                    },
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
            ScriptSceneSpan(
                temporary_span_id="span_0001",
                kind="scene",
                codepoint_start=0,
                codepoint_end=len(text),
                heading="第一场 夜 内",
                evidence=SourceEvidenceSpan(
                    source_start=0,
                    source_end=7,
                    text_hash=_hash("第一场 夜 内"),
                    exact_anchor="第一场 夜 内",
                ),
            )
        ],
    )

    with pytest.raises(ValidationError):
        SceneFact.model_validate(
            {
                **fact.scenes[0].model_dump(mode="json"),
                "visual_style": "赛博朋克",
            }
        )


@pytest.mark.asyncio
async def test_scene_analysis_harness_materializes_evidence_hash_after_anchor_validation(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    text = "第一场 夜 内\n林舟握住门把。"
    candidate = ScriptSpanCandidate(
        source_version_id=SOURCE_ID,
        source_hash=_hash(text),
        codepoint_count=len(text),
        coverage=ScriptSpanCoverageProof(
            source_hash=_hash(text),
            codepoint_start=0,
            codepoint_end=len(text),
            covered_codepoints=len(text),
        ),
        spans=[
            ScriptSceneSpan(
                temporary_span_id="span_0001",
                kind="scene",
                codepoint_start=0,
                codepoint_end=len(text),
                heading="第一场 夜 内",
                evidence=SourceEvidenceSpan(
                    source_start=0,
                    source_end=7,
                    text_hash=ZERO,
                    exact_anchor="第一场 夜 内",
                ),
            )
        ],
        review_issues=[],
    )

    async def return_candidate(*_: object) -> ScriptSpanCandidate:
        return candidate

    monkeypatch.setattr(SceneAnalysisHarness, "_run_codex", return_candidate)
    result = await SceneAnalysisHarness(
        _invocation(text), repository_root=REPOSITORY_ROOT
    ).execute()

    assert isinstance(result, ScriptSpanCandidate)
    assert result.spans[0].evidence.text_hash == _hash("第一场 夜 内")


def test_scene_analysis_bundle_discloses_only_stage_specific_references() -> None:
    bundle = SceneAnalysisBundle()
    assert bundle.loaded_paths("propose_script_spans") == (
        "SKILL.md",
        "references/script-spans.md",
    )
    assert bundle.loaded_paths("extract_scene_facts") == (
        "SKILL.md",
        "references/scene-facts.md",
    )
    assert bundle.compute_hash() == bundle.manifest.skill_bundle_hash


def test_scene_analysis_bundle_hash_covers_declared_files_not_loaded_by_the_stage(
    tmp_path: Path,
) -> None:
    source = REPOSITORY_ROOT / "agent/skills/build-storygraph"
    target = tmp_path / "agent/skills/build-storygraph"
    shutil.copytree(source, target)
    original = SceneAnalysisBundle(tmp_path).compute_hash()

    continuity = target / "references/continuity-review.md"
    continuity.write_text(continuity.read_text(encoding="utf-8") + "\n漂移", encoding="utf-8")

    assert SceneAnalysisBundle(tmp_path).compute_hash() != original


def test_scene_analysis_bundle_rejects_a_symlinked_parent_escape(tmp_path: Path) -> None:
    source = REPOSITORY_ROOT / "agent/skills/build-storygraph"
    outside_agent = tmp_path / "outside/agent"
    shutil.copytree(source, outside_agent / "skills/build-storygraph")
    (tmp_path / "agent").symlink_to(outside_agent, target_is_directory=True)

    with pytest.raises(BundleInvalid, match="root"):
        SceneAnalysisBundle(tmp_path).compute_hash()


def test_scene_analysis_bundle_rejects_invalid_declared_file_sets(tmp_path: Path) -> None:
    source = REPOSITORY_ROOT / "agent/skills/build-storygraph"

    def copy_bundle(case: str) -> tuple[Path, SceneAnalysisBundle]:
        repository = tmp_path / case
        target = repository / "agent/skills/build-storygraph"
        shutil.copytree(source, target)
        return target, SceneAnalysisBundle(repository)

    missing_root, missing = copy_bundle("missing")
    (missing_root / "references/scene-facts.md").unlink()
    with pytest.raises(BundleInvalid, match="file set"):
        missing.compute_hash()

    extra_root, extra = copy_bundle("extra")
    (extra_root / "references/undeclared.md").write_text("undeclared", encoding="utf-8")
    with pytest.raises(BundleInvalid, match="file set"):
        extra.compute_hash()

    invalid_root, invalid = copy_bundle("invalid-utf8")
    (invalid_root / "references/scene-facts.md").write_bytes(b"\xff")
    with pytest.raises(BundleInvalid, match="UTF-8"):
        invalid.compute_hash()

    symlink_root, symlink = copy_bundle("leaf-symlink")
    symlink_reference = symlink_root / "references/scene-facts.md"
    symlink_reference.unlink()
    symlink_reference.symlink_to(source / "references/scene-facts.md")
    with pytest.raises(BundleInvalid, match="symlink"):
        symlink.compute_hash()


def test_scene_analysis_bundle_hash_covers_semantic_versions_and_tool_policy() -> None:
    bundle = SceneAnalysisBundle()
    original = bundle.compute_hash()

    changed_version = SceneAnalysisBundle()
    changed_version.manifest = replace(
        changed_version.manifest,
        prompt_version="build-storygraph-scene-analysis-changed",
    )
    assert changed_version.compute_hash() != original

    changed_tools = SceneAnalysisBundle()
    changed_tools.manifest = replace(
        changed_tools.manifest,
        allowed_tools=("read_media",),
    )
    assert changed_tools.compute_hash() != original


def test_scene_analysis_wire_matches_the_shared_go_python_fixture_and_rejects_mutations() -> None:
    fixture = json.loads(WIRE_FIXTURE.read_text(encoding="utf-8"))
    invocation = SceneAnalysisInvocation.model_validate(fixture["valid_invocation"])
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
            SceneAnalysisInvocation.model_validate(raw)


def test_scene_analysis_result_hash_covers_the_complete_attempt_result() -> None:
    fixture = json.loads(WIRE_FIXTURE.read_text(encoding="utf-8"))
    invocation = SceneAnalysisInvocation.model_validate(fixture["valid_invocation"])
    candidate = fixture["valid_script_span_candidate"]
    result_without_hash: dict[str, Any] = {
        "invocation_id": str(invocation.invocation_id),
        "attempt_id": str(invocation.attempt_id),
        "kind": "storygraph_stage",
        "wire_schema_version": invocation.wire_schema_version,
        "variant": invocation.payload.variant.model_dump(mode="json"),
        "stage_release": invocation.stage_release.model_dump(mode="json"),
        "control": invocation.control.model_dump(mode="json"),
        "claim_version": 1,
        "dispatch_authorization_hash": "6" * 64,
        "status": "accepted",
        "candidate_type": "script_span_candidate",
        "candidate": candidate,
        "input_hash": invocation.input_hash,
        "output_hash": production_canonical_hash(candidate),
        "diagnostics": [],
        "diagnostic_hash": production_canonical_hash([]),
        "completed_at": "2026-08-31T01:00:00Z",
        "executor": {
            "runtime_class": "text",
            "runtime_image_digest": invocation.stage_release.agent_image_digest,
            "harness_version": "scene-analysis-harness",
            "model": "codex-cli-default",
        },
        "error": None,
    }
    with pytest.raises(ValidationError):
        SceneAnalysisAttemptResult.model_validate(result_without_hash)

    result_hash = production_canonical_hash(result_without_hash)
    assert result_hash == fixture["expected_result_hash"]
    result = SceneAnalysisAttemptResult.model_validate(
        {**result_without_hash, "result_hash": result_hash}
    )
    assert result.compute_result_hash() == result_hash

    changed = result.model_copy(update={"completed_at": datetime(2026, 8, 31, 1, 0, 1, tzinfo=UTC)})
    with pytest.raises(ValueError, match="result hash"):
        changed.validate_for(invocation, 1, "6" * 64)
