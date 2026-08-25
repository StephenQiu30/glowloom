from collections.abc import Mapping, Sequence
from dataclasses import replace
from hashlib import sha256
from typing import Any
from uuid import UUID

import pytest
from uuid6 import uuid7

from app.modules.storyboards import (
    StoryboardDraftAsset,
    StoryboardDraftInput,
    StoryboardDraftUnit,
)
from app.modules.storyboards.agents import (
    STORYBOARD_AGENT_HARNESS_VERSION,
    SceneAnalysis,
    ScenePlan,
    StoryboardAgentHarness,
    StoryboardAgentModels,
    StoryboardCheckpoint,
    StoryboardCheckpointStore,
    StoryboardReview,
    build_scene_contexts,
)


class _QueueModel:
    def __init__(self, *results: object) -> None:
        self.results = list(results)
        self.messages: list[Sequence[Any]] = []

    async def ainvoke(self, messages: Sequence[Any]) -> object:
        self.messages.append(messages)
        return self.results.pop(0)


class _MemoryCheckpointStore(StoryboardCheckpointStore):
    def __init__(self) -> None:
        self.items: list[StoryboardCheckpoint] = []
        self.load_calls: list[tuple[UUID, str]] = []

    async def load_latest(
        self,
        batch_id: UUID,
        input_hash: str,
    ) -> StoryboardCheckpoint | None:
        self.load_calls.append((batch_id, input_hash))
        return next(
            (
                item
                for item in reversed(self.items)
                if item.batch_id == batch_id and item.input_hash == input_hash
            ),
            None,
        )

    async def save(self, checkpoint: StoryboardCheckpoint) -> None:
        self.items.append(checkpoint)


def _input(*, include_mentioned_asset: bool = False) -> StoryboardDraftInput:
    scene_id = uuid7()
    action_unit_version_id = uuid7()
    return StoryboardDraftInput(
        batch_id=uuid7(),
        task_id=uuid7(),
        input_hash="b" * 64,
        script_version_id=uuid7(),
        target_duration_ms=4_000,
        aspect_ratio="9:16",
        visual_style=None,
        units=(
            StoryboardDraftUnit(
                unit_version_id=uuid7(),
                position=1,
                kind="scene_heading",
                exact_text="内景·控制室·夜",
                required_for_coverage=True,
                source_scene_id=scene_id,
                source_dialogue_id=None,
            ),
            StoryboardDraftUnit(
                unit_version_id=action_unit_version_id,
                position=2,
                kind="action",
                exact_text="沈岚拉下总闸。",
                required_for_coverage=True,
                source_scene_id=scene_id,
                source_dialogue_id=None,
            ),
        ),
        assets=(
            (
                StoryboardDraftAsset(
                    asset_version_id=uuid7(),
                    position=1,
                    kind="prop",
                    name="总闸",
                    state_label="初始抬起",
                    unit_version_ids=(action_unit_version_id,),
                ),
            )
            if include_mentioned_asset
            else ()
        ),
    )


def _analysis() -> dict[str, object]:
    return SceneAnalysis.model_validate(
        {
            "scene_key": 1,
            "beats": [
                {
                    "beat_key": "switch",
                    "unit_positions": [1, 2],
                    "dramatic_function": "action",
                    "summary": "确认空间后拉下总闸",
                }
            ],
            "conflict": "必须立即断电",
            "reveal": None,
            "reaction": "沈岚确认总闸落下",
            "continuity_facts": ["沈岚面向控制台"],
        }
    ).model_dump(mode="json")


def _plan() -> dict[str, object]:
    return ScenePlan.model_validate(
        {
            "scene_key": 1,
            "spatial_axis": "人物到控制台",
            "movement_direction": "向画面右侧",
            "eyeline": "看向总闸",
            "entrances_exits": [],
            "prop_states": ["总闸初始抬起"],
            "rhythm": "一次完整动作",
            "duration_budget_ms": 4_000,
            "shot_seeds": [
                {
                    "seed_key": "main",
                    "beat_keys": ["switch"],
                    "unit_positions": [1, 2],
                    "purpose": "交代断电动作",
                    "suggested_duration_ms": 4_000,
                }
            ],
        }
    ).model_dump(mode="json")


def _draft(*, covered_positions: list[int], purpose: str = "交代断电动作") -> dict[str, object]:
    return {
        "shots": [
            {
                "proposal_key": "main",
                "position": 1,
                "title": "拉下总闸",
                "unit_positions": covered_positions,
                "dialogue_unit_positions": [],
                "purpose": purpose,
                "continuity_note": "保持沈岚面向控制台",
                "shot_size": "medium",
                "camera_angle": "eye_level",
                "camera_movement": "static",
                "composition": "沈岚与总闸同框",
                "environment": "控制室",
                "subject_placements": [{"subject_key": "shen-lan", "placement": "画面左侧"}],
                "mood_lighting": "冷白顶光",
                "action_beats": [{"beat_key": "switch", "order": 1, "description": "沈岚拉下总闸"}],
                "duration_ms": 4_000,
                "ambient": "电流声",
                "asset_bindings": [],
                "first_frame": "沈岚的手停在抬起的总闸旁",
                "keyframe_notes": "总闸落下时保留手部动作",
                "risk_codes": [],
            }
        ]
    }


def _review(*issues: Mapping[str, object]) -> dict[str, object]:
    return StoryboardReview.model_validate({"issues": list(issues)}).model_dump(mode="json")


def _models(
    *,
    drafts: tuple[object, ...],
    reviews: tuple[object, ...] = ({"issues": []},),
    repairs: tuple[object, ...] = (),
) -> tuple[StoryboardAgentModels, dict[str, _QueueModel]]:
    models = {
        "analysis": _QueueModel(_analysis()),
        "plan": _QueueModel(_plan()),
        "draft": _QueueModel(*drafts),
        "review": _QueueModel(*reviews),
        "repair": _QueueModel(*repairs),
    }
    return (
        StoryboardAgentModels(
            source_analysis=models["analysis"],
            scene_plan=models["plan"],
            shot_draft=models["draft"],
            review=models["review"],
            repair=models["repair"],
        ),
        models,
    )


@pytest.mark.asyncio
async def test_harness_runs_multiple_skills_tools_and_returns_candidate_only_result() -> None:
    bundle, models = _models(drafts=(_draft(covered_positions=[1, 2]),))
    store = _MemoryCheckpointStore()
    value = _input()

    result = await StoryboardAgentHarness(models=bundle, checkpoint_store=store).run(value)

    assert result.status == "needs_review"
    assert result.candidate_only is True
    assert result.candidate is not None
    assert result.candidate.shots[0].proposal_key == "scene-1-main"
    assert result.timeline[0].timecode_in_ms == 0
    assert result.timeline[0].timecode_out_ms == 4_000
    assert result.repair_rounds == 0
    assert set(result.skill_versions) == {
        "analyze-scene",
        "plan-scene",
        "draft-shots",
        "review-shots",
        "repair-shots",
    }
    assert all(
        len(model.messages) == 1 for model in models.values() if model is not models["repair"]
    )
    assert not models["repair"].messages
    assert store.load_calls == [(value.batch_id, value.input_hash)]
    assert [item.stage for item in store.items] == [
        "contexts_built",
        "source_analyzed",
        "scenes_planned",
        "shots_drafted",
        "hard_gates_passed",
        "reviewed",
        "final_gate_passed",
    ]


@pytest.mark.asyncio
async def test_harness_records_current_run_token_in_every_checkpoint() -> None:
    run_token = uuid7()
    value = replace(_input(), run_token=run_token)
    bundle, _models_by_stage = _models(drafts=(_draft(covered_positions=[1, 2]),))
    store = _MemoryCheckpointStore()

    await StoryboardAgentHarness(models=bundle, checkpoint_store=store).run(value)

    assert store.items
    assert {item.run_token for item in store.items} == {run_token}


@pytest.mark.asyncio
async def test_harness_repairs_a_failed_hard_gate_then_revalidates() -> None:
    bundle, models = _models(
        drafts=(_draft(covered_positions=[1]),),
        repairs=(_draft(covered_positions=[1, 2], purpose="修复后覆盖完整动作"),),
    )

    result = await StoryboardAgentHarness(models=bundle).run(_input())

    assert result.status == "needs_review"
    assert result.repair_rounds == 1
    assert len(models["repair"].messages) == 1
    assert result.candidate is not None
    assert result.candidate.shots[0].purpose == "修复后覆盖完整动作"


@pytest.mark.asyncio
async def test_harness_routes_blocking_review_issue_to_targeted_repair() -> None:
    blocker = {
        "issue_id": "purpose-1",
        "code": "review.purpose_unclear",
        "severity": "blocker",
        "scope": "shot",
        "scene_key": 1,
        "shot_positions": [1],
        "evidence": "镜头目的没有说明动作结果",
        "repair_hint": "明确总闸落下的叙事作用",
        "source": "reviewer",
    }
    bundle, models = _models(
        drafts=(_draft(covered_positions=[1, 2]),),
        reviews=(_review(blocker), _review()),
        repairs=(_draft(covered_positions=[1, 2], purpose="确认总闸落下并完成断电"),),
    )

    result = await StoryboardAgentHarness(models=bundle).run(_input())

    assert result.status == "needs_review"
    assert result.repair_rounds == 1
    assert len(models["review"].messages) == 2
    assert len(models["repair"].messages) == 1
    assert result.candidate is not None
    assert result.candidate.shots[0].purpose == "确认总闸落下并完成断电"


@pytest.mark.asyncio
async def test_harness_restores_explicit_asset_binding_after_review_repair() -> None:
    blocker = {
        "issue_id": "purpose-1",
        "code": "review.purpose_unclear",
        "severity": "blocker",
        "scope": "shot",
        "scene_key": 1,
        "shot_positions": [1],
        "evidence": "镜头目的没有说明动作结果",
        "repair_hint": "明确总闸落下的叙事作用",
        "source": "reviewer",
    }
    bundle, models = _models(
        drafts=(_draft(covered_positions=[1, 2]),),
        reviews=(_review(blocker), _review()),
        repairs=(_draft(covered_positions=[1, 2], purpose="确认总闸落下并完成断电"),),
    )

    result = await StoryboardAgentHarness(models=bundle).run(
        _input(include_mentioned_asset=True)
    )

    assert result.status == "needs_review"
    assert result.candidate is not None
    assert [
        (binding.asset_position, binding.role)
        for binding in result.candidate.shots[0].asset_bindings
    ] == [(1, "prop")]


@pytest.mark.asyncio
async def test_harness_stops_after_two_failed_repair_rounds() -> None:
    invalid = _draft(covered_positions=[1])
    bundle, models = _models(
        drafts=(invalid,),
        repairs=(invalid, invalid),
    )

    result = await StoryboardAgentHarness(models=bundle).run(_input())

    assert result.status == "failed"
    assert result.candidate is None
    assert result.repair_rounds == 2
    assert "coverage.required_missing" in {issue.code for issue in result.issues}
    assert len(models["repair"].messages) == 2
    assert not models["review"].messages


@pytest.mark.asyncio
async def test_harness_resumes_after_source_analysis_without_reinvoking_that_skill() -> None:
    value = _input()
    store = _MemoryCheckpointStore()
    store.items.append(
        StoryboardCheckpoint(
            batch_id=value.batch_id,
            task_id=value.task_id,
            harness_version=STORYBOARD_AGENT_HARNESS_VERSION,
            input_hash=value.input_hash,
            stage="source_analyzed",
            stage_attempt=1,
            status="running",
            repair_round=0,
            scene_contexts=build_scene_contexts(value),
            analyses=(SceneAnalysis.model_validate(_analysis()),),
        )
    )
    bundle, models = _models(drafts=(_draft(covered_positions=[1, 2]),))

    result = await StoryboardAgentHarness(models=bundle, checkpoint_store=store).run(value)

    assert result.status == "needs_review"
    assert not models["analysis"].messages
    assert len(models["plan"].messages) == 1
    assert store.load_calls == [(value.batch_id, value.input_hash)]


@pytest.mark.asyncio
async def test_harness_rejects_non_completed_terminal_checkpoint() -> None:
    value = _input()
    initial_bundle, _initial_models = _models(drafts=(_draft(covered_positions=[1, 2]),))
    store = _MemoryCheckpointStore()
    await StoryboardAgentHarness(
        models=initial_bundle,
        checkpoint_store=store,
    ).run(value)
    terminal = store.items[-1]
    assert terminal.stage == "final_gate_passed"
    store.items = [terminal.model_copy(update={"status": "running"})]
    resumed_bundle, resumed_models = _models(drafts=(_draft(covered_positions=[1, 2]),))

    result = await StoryboardAgentHarness(
        models=resumed_bundle,
        checkpoint_store=store,
    ).run(value)

    assert result.status == "needs_review"
    assert len(resumed_models["analysis"].messages) == 1


@pytest.mark.parametrize(
    "corruption",
    ["result_hash", "candidate", "timeline", "total_duration"],
)
@pytest.mark.asyncio
async def test_harness_rejects_internally_inconsistent_terminal_checkpoint(
    corruption: str,
) -> None:
    value = _input()
    initial_bundle, _initial_models = _models(drafts=(_draft(covered_positions=[1, 2]),))
    store = _MemoryCheckpointStore()
    await StoryboardAgentHarness(
        models=initial_bundle,
        checkpoint_store=store,
    ).run(value)
    terminal = store.items[-1]
    assembled = terminal.assembled
    assert assembled is not None

    if corruption == "result_hash":
        corrupted = assembled.model_copy(update={"result_hash": "f" * 64})
    elif corruption == "candidate":
        candidate_shot = assembled.candidate.shots[0].model_copy(
            update={"title": "与时间线不一致的标题"}
        )
        candidate = assembled.candidate.model_copy(update={"shots": [candidate_shot]})
        corrupted = assembled.model_copy(
            update={
                "candidate": candidate,
                "result_hash": sha256(candidate.model_dump_json().encode("utf-8")).hexdigest(),
            }
        )
    elif corruption == "timeline":
        timeline_shot = assembled.timeline[0].shot.model_copy(
            update={"title": "与候选不一致的标题"}
        )
        timeline_row = assembled.timeline[0].model_copy(update={"shot": timeline_shot})
        corrupted = assembled.model_copy(update={"timeline": (timeline_row,)})
    else:
        corrupted = assembled.model_copy(
            update={"total_duration_ms": assembled.total_duration_ms + 500}
        )
    store.items = [terminal.model_copy(update={"assembled": corrupted})]
    resumed_bundle, resumed_models = _models(drafts=(_draft(covered_positions=[1, 2]),))

    result = await StoryboardAgentHarness(
        models=resumed_bundle,
        checkpoint_store=store,
    ).run(value)

    assert result.status == "needs_review"
    assert len(resumed_models["analysis"].messages) == 1


@pytest.mark.asyncio
async def test_harness_restores_annotated_terminal_checkpoint_without_model_calls() -> None:
    warning = {
        "issue_id": "reaction-1",
        "code": "review.reaction_subtle",
        "severity": "warning",
        "scope": "shot",
        "scene_key": 1,
        "shot_positions": [1],
        "evidence": "接收者反应存在但不够突出",
        "repair_hint": "人工审核时决定是否加强反应",
        "source": "reviewer",
    }
    value = _input()
    initial_bundle, _initial_models = _models(
        drafts=(_draft(covered_positions=[1, 2]),),
        reviews=(_review(warning),),
    )
    store = _MemoryCheckpointStore()
    initial = await StoryboardAgentHarness(
        models=initial_bundle,
        checkpoint_store=store,
    ).run(value)
    resumed_bundle, resumed_models = _models(drafts=())

    resumed = await StoryboardAgentHarness(
        models=resumed_bundle,
        checkpoint_store=store,
    ).run(value)

    assert resumed.candidate == initial.candidate
    assert resumed.timeline == initial.timeline
    assert resumed.result_hash == initial.result_hash
    assert resumed.issues == initial.issues
    assert resumed.candidate is not None
    assert resumed.candidate.shots[0].risk_codes == ["review.reaction_subtle"]
    assert all(not model.messages for model in resumed_models.values())
    assert resumed.checkpoints_saved == 0


@pytest.mark.asyncio
async def test_harness_does_not_reuse_checkpoint_from_another_batch() -> None:
    value = _input()
    other = _input()
    store = _MemoryCheckpointStore()
    store.items.append(
        StoryboardCheckpoint(
            batch_id=other.batch_id,
            task_id=other.task_id,
            harness_version=STORYBOARD_AGENT_HARNESS_VERSION,
            input_hash=value.input_hash,
            stage="source_analyzed",
            stage_attempt=1,
            status="running",
            repair_round=0,
            scene_contexts=build_scene_contexts(other),
            analyses=(SceneAnalysis.model_validate(_analysis()),),
        )
    )
    bundle, models = _models(drafts=(_draft(covered_positions=[1, 2]),))

    result = await StoryboardAgentHarness(models=bundle, checkpoint_store=store).run(value)

    assert result.status == "needs_review"
    assert len(models["analysis"].messages) == 1
    assert store.load_calls == [(value.batch_id, value.input_hash)]


@pytest.mark.asyncio
async def test_harness_stops_when_source_analysis_omits_required_input() -> None:
    bundle, models = _models(drafts=(_draft(covered_positions=[1, 2]),))
    incomplete = _analysis()
    incomplete["beats"][0]["unit_positions"] = [1]  # type: ignore[index]
    models["analysis"].results.clear()
    models["analysis"].results.append(incomplete)

    result = await StoryboardAgentHarness(models=bundle).run(_input())

    assert result.status == "failed"
    assert {issue.code for issue in result.issues} == {"analysis.required_missing"}
    assert not models["plan"].messages
    assert not models["draft"].messages


@pytest.mark.asyncio
async def test_harness_stops_when_scene_plan_omits_required_input() -> None:
    bundle, models = _models(drafts=(_draft(covered_positions=[1, 2]),))
    incomplete = _plan()
    incomplete["shot_seeds"][0]["unit_positions"] = [1]  # type: ignore[index]
    models["plan"].results.clear()
    models["plan"].results.append(incomplete)

    result = await StoryboardAgentHarness(models=bundle).run(_input())

    assert result.status == "failed"
    assert {issue.code for issue in result.issues} == {"plan.required_missing"}
    assert not models["draft"].messages


@pytest.mark.asyncio
async def test_harness_carries_review_warnings_into_candidate_risk_codes() -> None:
    warning = {
        "issue_id": "reaction-1",
        "code": "review.reaction_subtle",
        "severity": "warning",
        "scope": "shot",
        "scene_key": 1,
        "shot_positions": [1],
        "evidence": "接收者反应存在但不够突出",
        "repair_hint": "人工审核时决定是否加强反应",
        "source": "reviewer",
    }
    bundle, _models_by_stage = _models(
        drafts=(_draft(covered_positions=[1, 2]),),
        reviews=(_review(warning),),
    )

    result = await StoryboardAgentHarness(models=bundle).run(_input())

    assert result.status == "needs_review"
    assert result.candidate is not None
    assert result.candidate.shots[0].risk_codes == ["review.reaction_subtle"]
    assert result.result_hash is not None


@pytest.mark.asyncio
async def test_harness_downgrades_asset_blocker_without_asset_evidence() -> None:
    unsupported_blocker = {
        "issue_id": "asset-1",
        "code": "asset_conflict",
        "severity": "blocker",
        "scope": "shot",
        "scene_key": 1,
        "shot_positions": [1],
        "evidence": "镜头没有绑定控制台资产",
        "repair_hint": "补充资产",
        "source": "reviewer",
    }
    bundle, models = _models(
        drafts=(_draft(covered_positions=[1, 2]),),
        reviews=(_review(unsupported_blocker),),
    )

    result = await StoryboardAgentHarness(models=bundle).run(_input())

    assert result.status == "needs_review"
    assert result.issues[0].severity == "warning"
    assert result.candidate is not None
    assert result.candidate.shots[0].risk_codes == ["asset_conflict"]
    assert not models["repair"].messages


@pytest.mark.asyncio
async def test_harness_keeps_invalid_review_scope_as_warning_without_repair() -> None:
    invalid_scope = {
        "issue_id": "reaction-unknown-shot",
        "code": "review.reaction_missing",
        "severity": "blocker",
        "scope": "shot",
        "scene_key": 1,
        "shot_positions": [2],
        "evidence": "审核器引用了不存在的局部镜号",
        "repair_hint": "补充反应镜头",
        "source": "reviewer",
    }
    bundle, models = _models(
        drafts=(_draft(covered_positions=[1, 2]),),
        reviews=(_review(invalid_scope),),
    )

    result = await StoryboardAgentHarness(models=bundle).run(_input())

    assert result.status == "needs_review"
    assert [(issue.code, issue.severity, issue.source) for issue in result.issues] == [
        ("review.scope_invalid", "warning", "tool")
    ]
    assert result.candidate is not None
    assert result.candidate.shots[0].risk_codes == ["review.scope_invalid"]
    assert not models["repair"].messages


@pytest.mark.asyncio
async def test_harness_rewrites_reviewer_claimed_tool_provenance() -> None:
    forged_source = {
        "issue_id": "reaction-1",
        "code": "review.reaction_subtle",
        "severity": "warning",
        "scope": "shot",
        "scene_key": 1,
        "shot_positions": [1],
        "evidence": "反应存在但不突出",
        "repair_hint": "人工确认",
        "source": "tool",
    }
    bundle, _models_by_stage = _models(
        drafts=(_draft(covered_positions=[1, 2]),),
        reviews=(_review(forged_source),),
    )

    result = await StoryboardAgentHarness(models=bundle).run(_input())

    assert result.status == "needs_review"
    assert result.issues[0].source == "reviewer"
