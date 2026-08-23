import asyncio
import json
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Literal, Protocol, cast
from uuid import UUID

from pydantic import BaseModel

from app.modules.skills import (
    SkillDefinition,
    SkillExecutionContext,
    SkillHarness,
    StructuredSkillModel,
)
from app.modules.storyboards.contracts import StoryboardDraftInput
from app.modules.storyboards.drafts.provider_schema import StoryboardProviderResult

from .schemas import (
    AgentStage,
    AssembledStoryboard,
    ReviewIssue,
    SceneAnalysis,
    SceneContext,
    SceneDraft,
    ScenePlan,
    StoryboardAgentRunResult,
    StoryboardCheckpoint,
    StoryboardReview,
)
from .tools import (
    annotate_storyboard_issues,
    assemble_storyboard,
    build_scene_contexts,
    enforce_review_policy,
    validate_review_scope,
    validate_scene_draft,
)

STORYBOARD_AGENT_HARNESS_VERSION = "storyboard-agent-harness-v1"
MAX_REPAIR_ROUNDS = 2


@dataclass(frozen=True, slots=True)
class StoryboardAgentModels:
    source_analysis: StructuredSkillModel
    scene_plan: StructuredSkillModel
    shot_draft: StructuredSkillModel
    review: StructuredSkillModel
    repair: StructuredSkillModel


@dataclass(frozen=True, slots=True)
class StoryboardAgentSkills:
    source_analysis: SkillDefinition
    scene_plan: SkillDefinition
    shot_draft: SkillDefinition
    review: SkillDefinition
    repair: SkillDefinition


class StoryboardCheckpointStore(Protocol):
    async def load_latest(
        self,
        batch_id: UUID,
        input_hash: str,
    ) -> StoryboardCheckpoint | None: ...

    async def save(self, checkpoint: StoryboardCheckpoint) -> None: ...


class NullStoryboardCheckpointStore:
    """Default checkpoint boundary: deliberately performs no persistent writes."""

    async def load_latest(
        self,
        batch_id: UUID,
        input_hash: str,
    ) -> StoryboardCheckpoint | None:
        del batch_id, input_hash
        return None

    async def save(self, checkpoint: StoryboardCheckpoint) -> None:
        del checkpoint


def default_storyboard_agent_skills() -> StoryboardAgentSkills:
    def definition(name: str) -> SkillDefinition:
        return SkillDefinition(
            name=name,
            version="v1",
            max_input_chars=300_000,
            timeout_seconds=180.0,
            candidate_only=True,
            allowed_tools=frozenset[str](),
        )

    return StoryboardAgentSkills(
        source_analysis=definition("storyboard-source-analysis"),
        scene_plan=definition("storyboard-scene-plan"),
        shot_draft=definition("storyboard-shot-draft"),
        review=definition("storyboard-review"),
        repair=definition("storyboard-repair"),
    )


class StoryboardAgentHarness:
    """Orchestrates candidate-only storyboard agents and deterministic hard gates."""

    def __init__(
        self,
        *,
        models: StoryboardAgentModels,
        skills: StoryboardAgentSkills | None = None,
        skill_harness: SkillHarness | None = None,
        checkpoint_store: StoryboardCheckpointStore | None = None,
    ) -> None:
        self._models = models
        self._skills = skills or default_storyboard_agent_skills()
        self._skill_harness = skill_harness or SkillHarness()
        self._checkpoint_store = checkpoint_store or NullStoryboardCheckpointStore()
        self._checkpoints_saved = 0

    async def run(self, value: StoryboardDraftInput) -> StoryboardAgentRunResult:
        self._checkpoints_saved = 0
        resume = await self._load_checkpoint(value)
        if resume is not None and resume.stage in {"final_gate_passed", "failed"}:
            return self._result_from_terminal_checkpoint(resume)

        contexts = resume.scene_contexts if resume is not None else build_scene_contexts(value)
        analyses = resume.analyses if resume is not None else ()
        plans = resume.plans if resume is not None else ()
        drafts = resume.scene_drafts if resume is not None else ()
        issues = resume.issues if resume is not None else ()
        assembled = resume.assembled if resume is not None else None
        repair_round = resume.repair_round if resume is not None else 0
        resume_stage = resume.stage if resume is not None else None

        if resume is None:
            await self._checkpoint(
                value=value,
                stage="contexts_built",
                contexts=contexts,
            )

        if not analyses:
            analyses = tuple(
                await asyncio.gather(*(self._analyze_scene(value, context) for context in contexts))
            )
            stage_issues = self._validate_analyses(contexts, analyses)
            if stage_issues:
                return await self._failed(
                    value=value,
                    contexts=contexts,
                    analyses=analyses,
                    issues=stage_issues,
                    repair_round=repair_round,
                )
            await self._checkpoint(
                value=value,
                stage="source_analyzed",
                contexts=contexts,
                analyses=analyses,
            )

        if not plans:
            plans = tuple(
                await asyncio.gather(
                    *(
                        self._plan_scene(value, context, analysis)
                        for context, analysis in zip(contexts, analyses, strict=True)
                    )
                )
            )
            stage_issues = self._validate_plans(contexts, analyses, plans)
            if stage_issues:
                return await self._failed(
                    value=value,
                    contexts=contexts,
                    analyses=analyses,
                    plans=plans,
                    issues=stage_issues,
                    repair_round=repair_round,
                )
            await self._checkpoint(
                value=value,
                stage="scenes_planned",
                contexts=contexts,
                analyses=analyses,
                plans=plans,
            )

        if not drafts:
            draft_results = await asyncio.gather(
                *(
                    self._draft_scene(value, context, analysis, plan)
                    for context, analysis, plan in zip(
                        contexts,
                        analyses,
                        plans,
                        strict=True,
                    )
                )
            )
            drafts = tuple(
                SceneDraft(scene_key=context.scene_key, result=result)
                for context, result in zip(contexts, draft_results, strict=True)
            )
            await self._checkpoint(
                value=value,
                stage="shots_drafted",
                contexts=contexts,
                analyses=analyses,
                plans=plans,
                drafts=drafts,
            )

        hard_gate_was_saved = resume_stage in {
            "hard_gates_passed",
            "reviewed",
        }
        if not hard_gate_was_saved:
            issues = self._hard_gate(contexts, drafts)
            while self._has_blockers(issues) and repair_round < MAX_REPAIR_ROUNDS:
                repair_round += 1
                drafts = await self._repair_scenes(
                    value=value,
                    contexts=contexts,
                    analyses=analyses,
                    plans=plans,
                    drafts=drafts,
                    issues=issues,
                    repair_round=repair_round,
                )
                await self._checkpoint(
                    value=value,
                    stage="repaired",
                    attempt=repair_round,
                    contexts=contexts,
                    analyses=analyses,
                    plans=plans,
                    drafts=drafts,
                    issues=issues,
                    repair_round=repair_round,
                )
                issues = self._hard_gate(contexts, drafts)

            if self._has_blockers(issues):
                return await self._failed(
                    value=value,
                    contexts=contexts,
                    analyses=analyses,
                    plans=plans,
                    drafts=drafts,
                    issues=issues,
                    repair_round=repair_round,
                )
            await self._checkpoint(
                value=value,
                stage="hard_gates_passed",
                contexts=contexts,
                analyses=analyses,
                plans=plans,
                drafts=drafts,
                repair_round=repair_round,
            )

        assembled = assembled or assemble_storyboard(contexts, drafts)
        while True:
            review = await self._review_storyboard(
                value=value,
                contexts=contexts,
                analyses=analyses,
                plans=plans,
                drafts=drafts,
                assembled=assembled,
            )
            issues = enforce_review_policy(contexts, drafts, review.issues)
            await self._checkpoint(
                value=value,
                stage="reviewed",
                attempt=repair_round + 1,
                contexts=contexts,
                analyses=analyses,
                plans=plans,
                drafts=drafts,
                issues=issues,
                assembled=assembled,
                repair_round=repair_round,
            )
            if not self._has_blockers(issues):
                break
            if repair_round >= MAX_REPAIR_ROUNDS:
                return await self._failed(
                    value=value,
                    contexts=contexts,
                    analyses=analyses,
                    plans=plans,
                    drafts=drafts,
                    issues=issues,
                    assembled=assembled,
                    repair_round=repair_round,
                )
            repair_round += 1
            drafts = await self._repair_scenes(
                value=value,
                contexts=contexts,
                analyses=analyses,
                plans=plans,
                drafts=drafts,
                issues=issues,
                repair_round=repair_round,
            )
            await self._checkpoint(
                value=value,
                stage="repaired",
                attempt=repair_round,
                contexts=contexts,
                analyses=analyses,
                plans=plans,
                drafts=drafts,
                issues=issues,
                repair_round=repair_round,
            )
            hard_issues = self._hard_gate(contexts, drafts)
            while self._has_blockers(hard_issues) and repair_round < MAX_REPAIR_ROUNDS:
                repair_round += 1
                drafts = await self._repair_scenes(
                    value=value,
                    contexts=contexts,
                    analyses=analyses,
                    plans=plans,
                    drafts=drafts,
                    issues=hard_issues,
                    repair_round=repair_round,
                )
                await self._checkpoint(
                    value=value,
                    stage="repaired",
                    attempt=repair_round,
                    contexts=contexts,
                    analyses=analyses,
                    plans=plans,
                    drafts=drafts,
                    issues=hard_issues,
                    repair_round=repair_round,
                )
                hard_issues = self._hard_gate(contexts, drafts)
            if self._has_blockers(hard_issues):
                return await self._failed(
                    value=value,
                    contexts=contexts,
                    analyses=analyses,
                    plans=plans,
                    drafts=drafts,
                    issues=hard_issues,
                    repair_round=repair_round,
                )
            assembled = assemble_storyboard(contexts, drafts)

        final_issues = tuple(
            (
                *self._hard_gate(contexts, drafts),
                *validate_review_scope(contexts, drafts, issues),
            )
        )
        if self._has_blockers(final_issues):
            return await self._failed(
                value=value,
                contexts=contexts,
                analyses=analyses,
                plans=plans,
                drafts=drafts,
                issues=final_issues,
                assembled=assembled,
                repair_round=repair_round,
            )
        assembled = annotate_storyboard_issues(assembled, issues)
        await self._checkpoint(
            value=value,
            stage="final_gate_passed",
            status="completed",
            contexts=contexts,
            analyses=analyses,
            plans=plans,
            drafts=drafts,
            issues=issues,
            assembled=assembled,
            repair_round=repair_round,
        )
        return StoryboardAgentRunResult(
            status="needs_review",
            input_hash=value.input_hash,
            result_hash=assembled.result_hash,
            candidate=assembled.candidate,
            timeline=assembled.timeline,
            issues=issues,
            repair_rounds=repair_round,
            skill_versions=self._skill_versions(),
            checkpoints_saved=self._checkpoints_saved,
        )

    async def _analyze_scene(
        self,
        value: StoryboardDraftInput,
        context: SceneContext,
    ) -> SceneAnalysis:
        return (
            await self._skill_harness.run(
                skill=self._skills.source_analysis,
                model=self._models.source_analysis,
                system_prompt=(
                    "$storyboard-source-analysis 仅分析固定来源，将场景组织为可拍的"
                    "语义节拍；不得改写来源或设计完整镜头表。只返回指定 JSON。"
                ),
                user_payload=context.model_dump_json(),
                output_model=SceneAnalysis,
                context=self._execution_context(value, self._skills.source_analysis, context),
            )
        ).output

    async def _plan_scene(
        self,
        value: StoryboardDraftInput,
        context: SceneContext,
        analysis: SceneAnalysis,
    ) -> ScenePlan:
        payload = _json_payload(context=context, analysis=analysis)
        return (
            await self._skill_harness.run(
                skill=self._skills.scene_plan,
                model=self._models.scene_plan,
                system_prompt=(
                    "$storyboard-scene-plan 根据已验证节拍规划空间轴、调度、节奏、"
                    "时长预算和有限镜头 seed；不得补写剧情。只返回指定 JSON。"
                ),
                user_payload=payload,
                output_model=ScenePlan,
                context=self._execution_context(value, self._skills.scene_plan, context),
            )
        ).output

    async def _draft_scene(
        self,
        value: StoryboardDraftInput,
        context: SceneContext,
        analysis: SceneAnalysis,
        plan: ScenePlan,
    ) -> StoryboardProviderResult:
        payload = _json_payload(context=context, analysis=analysis, plan=plan)
        return (
            await self._skill_harness.run(
                skill=self._skills.shot_draft,
                model=self._models.shot_draft,
                system_prompt=(
                    "$storyboard-shot-draft 只按固定场景、节拍与计划填写候选分镜行；"
                    "只能引用输入 position，不得写数据库或声明正式镜头已创建。只返回指定 JSON。"
                ),
                user_payload=payload,
                output_model=StoryboardProviderResult,
                context=self._execution_context(value, self._skills.shot_draft, context),
            )
        ).output

    async def _review_storyboard(
        self,
        *,
        value: StoryboardDraftInput,
        contexts: Sequence[SceneContext],
        analyses: Sequence[SceneAnalysis],
        plans: Sequence[ScenePlan],
        drafts: Sequence[SceneDraft],
        assembled: AssembledStoryboard,
    ) -> StoryboardReview:
        payload = _json_payload(
            contexts=contexts,
            analyses=analyses,
            plans=plans,
            scene_drafts=drafts,
            assembled=assembled,
        )
        return (
            await self._skill_harness.run(
                skill=self._skills.review,
                model=self._models.review,
                system_prompt=(
                    "$storyboard-review 独立审核候选关键分镜表，只输出有证据的问题；"
                    "镜头问题使用 scene_key 和场景内局部镜号，不直接修改分镜。只返回指定 JSON。"
                ),
                user_payload=payload,
                output_model=StoryboardReview,
                context=self._execution_context(value, self._skills.review),
            )
        ).output

    async def _repair_scenes(
        self,
        *,
        value: StoryboardDraftInput,
        contexts: Sequence[SceneContext],
        analyses: Sequence[SceneAnalysis],
        plans: Sequence[ScenePlan],
        drafts: Sequence[SceneDraft],
        issues: Sequence[ReviewIssue],
        repair_round: int,
    ) -> tuple[SceneDraft, ...]:
        global_blocker = any(
            issue.severity == "blocker" and issue.scope == "global" for issue in issues
        )
        affected = {
            issue.scene_key
            for issue in issues
            if issue.severity == "blocker" and issue.scene_key is not None
        }
        context_by_key = {context.scene_key: context for context in contexts}
        analysis_by_key = {analysis.scene_key: analysis for analysis in analyses}
        plan_by_key = {plan.scene_key: plan for plan in plans}
        repaired: list[SceneDraft] = []
        for draft in drafts:
            if not global_blocker and draft.scene_key not in affected:
                repaired.append(draft)
                continue
            scene_issues = tuple(
                issue
                for issue in issues
                if issue.scope == "global" or issue.scene_key == draft.scene_key
            )
            context = context_by_key[draft.scene_key]
            payload = _json_payload(
                repair_round=repair_round,
                context=context,
                analysis=analysis_by_key[draft.scene_key],
                plan=plan_by_key[draft.scene_key],
                current_draft=draft,
                failed_issues=scene_issues,
            )
            result = (
                await self._skill_harness.run(
                    skill=self._skills.repair,
                    model=self._models.repair,
                    system_prompt=(
                        "$storyboard-repair 只修复列出的 blocker 及其相邻上下文；"
                        "不得自由重写已通过内容，不得新增来源事实。只返回指定 JSON。"
                    ),
                    user_payload=payload,
                    output_model=StoryboardProviderResult,
                    context=self._execution_context(
                        value,
                        self._skills.repair,
                        context,
                        attempt=repair_round,
                    ),
                )
            ).output
            repaired.append(SceneDraft(scene_key=draft.scene_key, result=result))
        return tuple(repaired)

    def _hard_gate(
        self,
        contexts: Sequence[SceneContext],
        drafts: Sequence[SceneDraft],
    ) -> tuple[ReviewIssue, ...]:
        scene_lookup = {
            unit.position: context.scene_key for context in contexts for unit in context.units
        }
        drafts_by_key = {draft.scene_key: draft for draft in drafts}
        issues: list[ReviewIssue] = []
        for context in contexts:
            draft = drafts_by_key.get(context.scene_key)
            if draft is None:
                issues.append(
                    _stage_issue(
                        code="draft.scene_missing",
                        scene_key=context.scene_key,
                        evidence="场景没有候选分镜结果",
                        repair_hint="为该场景生成候选分镜",
                    )
                )
                continue
            issues.extend(
                validate_scene_draft(
                    context,
                    draft.result,
                    unit_scene_keys=scene_lookup,
                )
            )
        return tuple(issues)

    def _validate_analyses(
        self,
        contexts: Sequence[SceneContext],
        analyses: Sequence[SceneAnalysis],
    ) -> tuple[ReviewIssue, ...]:
        issues: list[ReviewIssue] = []
        for context, analysis in zip(contexts, analyses, strict=True):
            valid_positions = {unit.position for unit in context.units}
            required_positions = {
                unit.position for unit in context.units if unit.required_for_coverage
            }
            referenced = {position for beat in analysis.beats for position in beat.unit_positions}
            if analysis.scene_key != context.scene_key or not referenced.issubset(valid_positions):
                issues.append(
                    _stage_issue(
                        code="analysis.reference_invalid",
                        scene_key=context.scene_key,
                        evidence="来源分析引用了错误场景或未知 position",
                        repair_hint="重新运行来源分析阶段",
                    )
                )
            if not required_positions.issubset(referenced):
                missing = sorted(required_positions.difference(referenced))
                issues.append(
                    _stage_issue(
                        code="analysis.required_missing",
                        scene_key=context.scene_key,
                        evidence=f"来源分析遗漏必需叙事位置：{missing}",
                        repair_hint="重新分析场景并覆盖全部 required position",
                    )
                )
        return tuple(issues)

    def _validate_plans(
        self,
        contexts: Sequence[SceneContext],
        analyses: Sequence[SceneAnalysis],
        plans: Sequence[ScenePlan],
    ) -> tuple[ReviewIssue, ...]:
        issues: list[ReviewIssue] = []
        for context, analysis, plan in zip(contexts, analyses, plans, strict=True):
            valid_positions = {unit.position for unit in context.units}
            required_positions = {
                unit.position for unit in context.units if unit.required_for_coverage
            }
            valid_beats = {beat.beat_key for beat in analysis.beats}
            plan_positions = {
                position for seed in plan.shot_seeds for position in seed.unit_positions
            }
            plan_beats = {beat for seed in plan.shot_seeds for beat in seed.beat_keys}
            if (
                plan.scene_key != context.scene_key
                or not plan_positions.issubset(valid_positions)
                or not plan_beats.issubset(valid_beats)
            ):
                issues.append(
                    _stage_issue(
                        code="plan.reference_invalid",
                        scene_key=context.scene_key,
                        evidence="场景计划引用了未知来源位置或语义节拍",
                        repair_hint="重新运行场景计划阶段",
                    )
                )
            if not required_positions.issubset(plan_positions) or not valid_beats.issubset(
                plan_beats
            ):
                missing_positions = sorted(required_positions.difference(plan_positions))
                missing_beats = sorted(valid_beats.difference(plan_beats))
                issues.append(
                    _stage_issue(
                        code="plan.required_missing",
                        scene_key=context.scene_key,
                        evidence=(
                            f"场景计划遗漏必需位置 {missing_positions} 或节拍 {missing_beats}"
                        ),
                        repair_hint="重新规划场景并覆盖全部已接受节拍与 required position",
                    )
                )
        return tuple(issues)

    async def _load_checkpoint(
        self,
        value: StoryboardDraftInput,
    ) -> StoryboardCheckpoint | None:
        checkpoint = await self._checkpoint_store.load_latest(
            value.batch_id,
            value.input_hash,
        )
        if checkpoint is None:
            return None
        if (
            checkpoint.batch_id != value.batch_id
            or checkpoint.task_id != value.task_id
            or checkpoint.input_hash != value.input_hash
            or checkpoint.harness_version != STORYBOARD_AGENT_HARNESS_VERSION
            or not self._checkpoint_is_consistent(checkpoint)
        ):
            return None
        return checkpoint

    @staticmethod
    def _checkpoint_is_consistent(checkpoint: StoryboardCheckpoint) -> bool:
        if not checkpoint.scene_contexts:
            return False
        scene_keys = {context.scene_key for context in checkpoint.scene_contexts}
        if len(scene_keys) != len(checkpoint.scene_contexts):
            return False

        analyses_complete = (
            len(checkpoint.analyses) == len(scene_keys)
            and {analysis.scene_key for analysis in checkpoint.analyses} == scene_keys
        )
        plans_complete = (
            len(checkpoint.plans) == len(scene_keys)
            and {plan.scene_key for plan in checkpoint.plans} == scene_keys
        )
        drafts_complete = (
            len(checkpoint.scene_drafts) == len(scene_keys)
            and {draft.scene_key for draft in checkpoint.scene_drafts} == scene_keys
        )

        if checkpoint.stage == "contexts_built":
            return True
        if checkpoint.stage == "source_analyzed":
            return analyses_complete
        if checkpoint.stage == "scenes_planned":
            return analyses_complete and plans_complete
        if checkpoint.stage in {
            "shots_drafted",
            "repaired",
            "hard_gates_passed",
        }:
            return analyses_complete and plans_complete and drafts_complete
        if checkpoint.stage in {"reviewed", "final_gate_passed"}:
            return (
                analyses_complete
                and plans_complete
                and drafts_complete
                and checkpoint.assembled is not None
            )
        return checkpoint.stage == "failed" and checkpoint.status == "failed"

    def _result_from_terminal_checkpoint(
        self,
        checkpoint: StoryboardCheckpoint,
    ) -> StoryboardAgentRunResult:
        if checkpoint.stage == "failed":
            return StoryboardAgentRunResult(
                status="failed",
                input_hash=checkpoint.input_hash,
                issues=checkpoint.issues,
                repair_rounds=checkpoint.repair_round,
                skill_versions=self._skill_versions(),
                checkpoints_saved=0,
            )
        assembled = checkpoint.assembled
        assert assembled is not None
        return StoryboardAgentRunResult(
            status="needs_review",
            input_hash=checkpoint.input_hash,
            result_hash=assembled.result_hash,
            candidate=assembled.candidate,
            timeline=assembled.timeline,
            issues=checkpoint.issues,
            repair_rounds=checkpoint.repair_round,
            skill_versions=self._skill_versions(),
            checkpoints_saved=0,
        )

    async def _failed(
        self,
        *,
        value: StoryboardDraftInput,
        contexts: Sequence[SceneContext],
        issues: Sequence[ReviewIssue],
        repair_round: int,
        analyses: Sequence[SceneAnalysis] = (),
        plans: Sequence[ScenePlan] = (),
        drafts: Sequence[SceneDraft] = (),
        assembled: AssembledStoryboard | None = None,
    ) -> StoryboardAgentRunResult:
        await self._checkpoint(
            value=value,
            stage="failed",
            status="failed",
            attempt=max(1, repair_round),
            contexts=contexts,
            analyses=analyses,
            plans=plans,
            drafts=drafts,
            issues=issues,
            assembled=assembled,
            repair_round=repair_round,
        )
        return StoryboardAgentRunResult(
            status="failed",
            input_hash=value.input_hash,
            candidate=None,
            timeline=(),
            issues=tuple(issues),
            repair_rounds=repair_round,
            skill_versions=self._skill_versions(),
            checkpoints_saved=self._checkpoints_saved,
        )

    async def _checkpoint(
        self,
        *,
        value: StoryboardDraftInput,
        stage: AgentStage,
        contexts: Sequence[SceneContext],
        status: Literal["running", "completed", "failed"] = "running",
        attempt: int = 1,
        analyses: Sequence[SceneAnalysis] = (),
        plans: Sequence[ScenePlan] = (),
        drafts: Sequence[SceneDraft] = (),
        issues: Sequence[ReviewIssue] = (),
        assembled: AssembledStoryboard | None = None,
        repair_round: int = 0,
    ) -> None:
        checkpoint = StoryboardCheckpoint.model_validate(
            {
                "batch_id": value.batch_id,
                "task_id": value.task_id,
                "harness_version": STORYBOARD_AGENT_HARNESS_VERSION,
                "input_hash": value.input_hash,
                "stage": stage,
                "stage_attempt": attempt,
                "status": status,
                "repair_round": repair_round,
                "scene_contexts": contexts,
                "analyses": analyses,
                "plans": plans,
                "scene_drafts": drafts,
                "issues": issues,
                "assembled": assembled,
            }
        )
        await self._checkpoint_store.save(checkpoint)
        self._checkpoints_saved += 1

    @staticmethod
    def _has_blockers(issues: Sequence[ReviewIssue]) -> bool:
        return any(issue.severity == "blocker" for issue in issues)

    @staticmethod
    def _execution_context(
        value: StoryboardDraftInput,
        skill: SkillDefinition,
        scene: SceneContext | None = None,
        *,
        attempt: int = 1,
    ) -> SkillExecutionContext:
        scene_suffix = f":scene-{scene.scene_key}" if scene is not None else ""
        return SkillExecutionContext(
            skill_name=skill.name,
            skill_version=skill.version,
            task_id=f"{value.task_id}:{skill.name}{scene_suffix}:attempt-{attempt}",
        )

    def _skill_versions(self) -> dict[str, str]:
        return {
            skill.name: skill.version
            for skill in (
                self._skills.source_analysis,
                self._skills.scene_plan,
                self._skills.shot_draft,
                self._skills.review,
                self._skills.repair,
            )
        }


def _json_payload(**values: object) -> str:
    def serialize(value: object) -> object:
        if isinstance(value, BaseModel):
            return value.model_dump(mode="json")
        if isinstance(value, Sequence) and not isinstance(value, (str, bytes, bytearray)):
            return [serialize(item) for item in cast(Sequence[object], value)]
        return value

    return json.dumps(
        {key: serialize(value) for key, value in values.items()},
        ensure_ascii=False,
        separators=(",", ":"),
    )


def _stage_issue(
    *,
    code: str,
    scene_key: int,
    evidence: str,
    repair_hint: str,
) -> ReviewIssue:
    return ReviewIssue(
        issue_id=f"{code}:{scene_key}",
        code=code,
        severity="blocker",
        scope="scene",
        scene_key=scene_key,
        evidence=evidence,
        repair_hint=repair_hint,
        source="tool",
    )
