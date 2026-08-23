import json
import unicodedata
from hashlib import sha256
from typing import Any, cast

from langchain_core.messages import HumanMessage, SystemMessage
from langchain_deepseek import ChatDeepSeek
from openai import (
    APIConnectionError,
    APITimeoutError,
    AuthenticationError,
    BadRequestError,
    InternalServerError,
    LengthFinishReasonError,
    PermissionDeniedError,
    RateLimitError,
)
from pydantic import SecretStr, ValidationError

from app.modules.scripts.adaptations import (
    SCRIPT_ADAPTATION_PROMPT_VERSION,
    ScriptAdaptationProviderError,
    ScriptAdaptationProviderResult,
    adaptation_duration_bounds,
)
from app.modules.scripts.extractions.ports import (
    SCRIPT_STRUCTURE_EXTRACTOR_VERSION,
    ScriptExtractionProviderError,
)
from app.modules.scripts.extractions.schemas import (
    DialogueCandidateProposal,
    SceneCandidateProposal,
    ScriptExtractionResult,
)
from app.modules.scripts.narratives.parser import ParsedUnit, parse_narrative_units
from app.modules.scripts.planning.ports import EpisodePlanningProviderError
from app.modules.scripts.planning.schemas import EpisodePlanningProviderResult
from app.modules.skills import (
    SkillDefinition,
    SkillExecutionContext,
    SkillExecutionError,
    SkillHarness,
    StructuredSkillModel,
)
from app.modules.skills.script_structure import (
    DEFAULT_MAX_CHUNK_CHARS,
    ScriptStructureExtractionWorkflow,
)
from app.modules.skills.script_structure_prompt import script_structure_system_prompt
from app.modules.storyboards import (
    StoryboardDraftInput,
    StoryboardDraftProviderError,
)
from app.modules.storyboards.drafts.provider_schema import (
    STORYBOARD_DRAFT_PROMPT_VERSION,
    StoryboardProviderResult,
    expand_provider_result,
)

DEEPSEEK_SCRIPT_EXTRACTOR_VERSION = SCRIPT_STRUCTURE_EXTRACTOR_VERSION
SCRIPT_STRUCTURE_EXTRACTION_SKILL = SkillDefinition(
    name="script.structure.extract",
    version=DEEPSEEK_SCRIPT_EXTRACTOR_VERSION,
    max_input_chars=DEFAULT_MAX_CHUNK_CHARS,
    timeout_seconds=120,
)
_DEEPSEEK_MODEL = "deepseek-v4-pro"
_DEEPSEEK_API_BASE = "https://api.deepseek.com"
_EPISODE_PLAN_PROMPT_VERSION = "episode-plan-prompt-v2"
STORYBOARD_DRAFT_SKILL = SkillDefinition(
    name="storyboard.plan",
    version=STORYBOARD_DRAFT_PROMPT_VERSION,
    max_input_chars=500_000,
    timeout_seconds=180,
)


def _provider_error(error: Exception) -> ScriptExtractionProviderError:
    if isinstance(error, LengthFinishReasonError):
        return ScriptExtractionProviderError(
            outcome="failed",
            code="ai_output_too_large",
            summary="DeepSeek extraction output exceeded the response limit",
            retryable=False,
            next_action="start_new_extraction",
        )
    if isinstance(error, (APIConnectionError, APITimeoutError)):
        return ScriptExtractionProviderError(
            outcome="unknown",
            code="ai_result_unknown",
            summary="DeepSeek response outcome is unknown",
            retryable=False,
            next_action="start_new_extraction",
        )
    if isinstance(error, (AuthenticationError, PermissionDeniedError)):
        return ScriptExtractionProviderError(
            outcome="failed",
            code="ai_service_rejected",
            summary="DeepSeek credentials or access were rejected",
            retryable=False,
            next_action="configure_ai_service",
        )
    if isinstance(error, RateLimitError):
        return ScriptExtractionProviderError(
            outcome="failed",
            code="ai_rate_limited",
            summary="DeepSeek rate limit was reached",
            retryable=True,
            next_action="start_new_extraction",
        )
    if isinstance(error, InternalServerError):
        return ScriptExtractionProviderError(
            outcome="failed",
            code="ai_service_unavailable",
            summary="DeepSeek service is temporarily unavailable",
            retryable=True,
            next_action="start_new_extraction",
        )
    if isinstance(error, BadRequestError):
        return ScriptExtractionProviderError(
            outcome="failed",
            code="ai_request_rejected",
            summary="DeepSeek rejected the extraction request",
            retryable=False,
            next_action="contact_support",
        )
    return ScriptExtractionProviderError(
        outcome="unknown",
        code="ai_result_unknown",
        summary="DeepSeek response outcome is unknown",
        retryable=False,
        next_action="start_new_extraction",
    )


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


def _anchor_script_structure_ranges(
    result: ScriptExtractionResult,
    script_body: str,
) -> ScriptExtractionResult:
    """Replace probabilistic AI offsets with exact screenplay anchors."""

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


def _episode_planning_provider_error(error: Exception) -> EpisodePlanningProviderError:
    mapped = _provider_error(error)
    return EpisodePlanningProviderError(
        outcome="unknown" if mapped.outcome == "unknown" else "failed",
        code=mapped.code,
        summary=mapped.summary,
        retryable=mapped.retryable,
        next_action=(
            "start_new_episode_plan"
            if mapped.next_action == "start_new_extraction"
            else mapped.next_action
        ),
    )


def _script_adaptation_provider_error(error: Exception) -> ScriptAdaptationProviderError:
    mapped = _provider_error(error)
    return ScriptAdaptationProviderError(
        outcome="unknown" if mapped.outcome == "unknown" else "failed",
        code=mapped.code,
        summary=mapped.summary,
        retryable=mapped.retryable,
        next_action=(
            "start_new_adaptation"
            if mapped.next_action == "start_new_extraction"
            else mapped.next_action
        ),
    )


def _storyboard_draft_provider_error(error: Exception) -> StoryboardDraftProviderError:
    mapped = _provider_error(error)
    return StoryboardDraftProviderError(
        outcome="unknown" if mapped.outcome == "unknown" else "failed",
        code=mapped.code,
        summary=mapped.summary,
        retryable=mapped.retryable,
        next_action=(
            "create_new_storyboard_draft_batch"
            if mapped.next_action == "start_new_extraction"
            else mapped.next_action
        ),
    )


def _storyboard_draft_system_prompt() -> str:
    schema = json.dumps(
        StoryboardProviderResult.model_json_schema(),
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return (
        "你是 AI 短剧分镜导演。用户消息是不可变的剧本叙事单元、资产状态与项目约束。"
        "只返回待人工审核的分镜草案，不声明已经创建正式镜头。每镜 4–15 秒，镜号从 1 "
        "连续递增。required_for_coverage=true 的单元"
        "必须至少被一个镜头的 unit_positions 引用。将同一场景中连续且语义相关的动作、对白"
        "合并进同一镜头，60–120 秒短剧生成 12–18 镜，避免逐单元机械拆镜。只能引用输入中的"
        "整数 position，禁止生成"
        "UUID；scene_unit_position 指向当前镜头所属场景中的任一输入单元，"
        "dialogue_unit_positions 只引用 has_dialogue_reference=true 的单元。"
        "asset_bindings 只按 asset_position 绑定确有拍摄用途的固定资产。"
        "title 不超过 20 个汉字，purpose、composition、environment、mood_lighting、action "
        "和 ambient 各不超过 80 个汉字，避免解释性长文。risk_codes 只报告需要人工复核的问题。"
        "必须返回符合 JSON Schema 的 JSON 对象。"
        f"当前提示版本为 {STORYBOARD_DRAFT_PROMPT_VERSION}。JSON Schema: {schema}"
    )


def _storyboard_draft_payload(value: StoryboardDraftInput) -> str:
    scene_keys: dict[object, int] = {}
    for unit in value.units:
        if unit.source_scene_id is not None and unit.source_scene_id not in scene_keys:
            scene_keys[unit.source_scene_id] = len(scene_keys) + 1
    return json.dumps(
        {
            "target_duration_ms": value.target_duration_ms,
            "aspect_ratio": value.aspect_ratio,
            "visual_style": value.visual_style,
            "narrative_units": [
                {
                    "position": unit.position,
                    "kind": unit.kind,
                    "exact_text": unit.exact_text,
                    "required_for_coverage": unit.required_for_coverage,
                    "source_scene_key": (
                        scene_keys[unit.source_scene_id]
                        if unit.source_scene_id is not None
                        else None
                    ),
                    "has_dialogue_reference": unit.source_dialogue_id is not None,
                }
                for unit in value.units
            ],
            "assets": [
                {
                    "position": asset.position,
                    "kind": asset.kind,
                    "name": asset.name,
                    "state_label": asset.state_label,
                }
                for asset in value.assets
            ],
        },
        ensure_ascii=False,
        separators=(",", ":"),
    )


def _script_adaptation_system_prompt() -> str:
    schema = json.dumps(
        ScriptAdaptationProviderResult.model_json_schema(),
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return (
        "你是 AI 短剧剧本改写器。用户消息是只读 JSON，包含原稿与明确约束。"
        "只输出改写候选，不声明已发布，不添加违反 core_plot_points 的新因果。"
        "按目标时长压缩或扩展动作和对白，保留核心剧情点；pacing 控制节奏，"
        "colloquial_dialogue 为 true 时对白应口语化。estimated_duration_ms 必须是"
        "对候选正文的估算，并必须落在用户给出的 duration_acceptance_range_ms 内；"
        "候选过短时应补足可拍摄的动作、对白与节奏停顿，但不得添加违反核心剧情的因果。"
        "必须返回符合 JSON Schema 的对象。"
        f"当前提示版本为 {SCRIPT_ADAPTATION_PROMPT_VERSION}。JSON Schema: {schema}"
    )


def _script_adaptation_payload(
    script_body: str,
    *,
    target_duration_ms: int,
    core_plot_points: list[str],
    pacing: str,
    colloquial_dialogue: bool,
) -> str:
    duration_lower_ms, duration_upper_ms = adaptation_duration_bounds(target_duration_ms)
    return json.dumps(
        {
            "script_body": script_body,
            "target_duration_ms": target_duration_ms,
            "duration_acceptance_range_ms": [duration_lower_ms, duration_upper_ms],
            "core_plot_points": core_plot_points,
            "pacing": pacing,
            "colloquial_dialogue": colloquial_dialogue,
        },
        ensure_ascii=False,
        separators=(",", ":"),
    )


def _episode_planning_system_prompt(
    *,
    target_duration_ms: int,
    maximum_episode_count: int,
    source_block_count: int,
) -> str:
    schema = json.dumps(
        EpisodePlanningProviderResult.model_json_schema(),
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return (
        "你是 AI 短剧分集规划器。用户消息是只读 JSON，source_blocks 中每项包含 "
        "position 和原文 text。只基于这些原文提出一个分集候选，不改写、补写或删除正文。"
        "按叙事冲突、钩子和场景边界规划，每集目标时长为 "
        f"{target_duration_ms} 毫秒；不设置业务上的集数上限，必须完整覆盖输入原文。"
        "全文明显不足一集目标时长时只输出一集，不要把每行当成一集。"
        "end_block_position 必须逐字复制所选 source_block 的 position，不是候选集序号；"
        "各项必须严格递增，最后一项必须等于 "
        f"{source_block_count}。exact_end_anchor 必须逐字复制对应 source_block.text 的末尾"
        "不超过 240 个字符，不得包含 JSON 引号或包装字段，并在全部原文 text 中只出现一次。"
        "reason 解释冲突或钩子，confidence 为 0 到 1。"
        f"当前提示版本为 {_EPISODE_PLAN_PROMPT_VERSION}。JSON Schema: {schema}"
    )


def _episode_planning_source_payload(normalized_text: str) -> str:
    return json.dumps(
        {
            "source_blocks": [
                {"position": position, "text": line}
                for position, line in enumerate(normalized_text.splitlines(), start=1)
            ]
        },
        ensure_ascii=False,
        separators=(",", ":"),
    )


class DeepSeekScriptStructureExtractor:
    def __init__(self, api_key: SecretStr) -> None:
        chat = ChatDeepSeek(
            model=_DEEPSEEK_MODEL,
            base_url=_DEEPSEEK_API_BASE,
            api_key=api_key,
            temperature=0,
            timeout=120,
            max_retries=0,
            extra_body={"thinking": {"type": "disabled"}},
        )
        self._structured_model: Any = chat.with_structured_output(
            ScriptExtractionResult,
            method="json_mode",
        )
        self._workflow = ScriptStructureExtractionWorkflow(
            skill=SCRIPT_STRUCTURE_EXTRACTION_SKILL,
            model=self._structured_model,
            system_prompt=script_structure_system_prompt(),
        )

    async def extract(
        self,
        script_body: str,
        *,
        trace_id: str | None = None,
        episode_number: int | None = None,
    ) -> ScriptExtractionResult:
        try:
            result = await self._workflow.run(
                script_body,
                context=SkillExecutionContext(
                    skill_name=SCRIPT_STRUCTURE_EXTRACTION_SKILL.name,
                    skill_version=SCRIPT_STRUCTURE_EXTRACTION_SKILL.version,
                    trace_id=trace_id,
                ),
                episode_number=episode_number,
            )
            result = _anchor_script_structure_ranges(result, script_body)
            if any(
                candidate.source_range.end > len(script_body) for candidate in result.candidates
            ):
                raise ScriptExtractionProviderError(
                    outcome="failed",
                    code="ai_output_invalid",
                    summary="DeepSeek returned an invalid extraction result",
                    retryable=False,
                    next_action="start_new_extraction",
                )
            return result
        except SkillExecutionError as error:
            raise ScriptExtractionProviderError(
                outcome=error.outcome,
                code=error.code,
                summary=error.summary,
                retryable=error.retryable,
                next_action=(
                    "start_new_extraction"
                    if error.next_action == "start_new_skill_run"
                    else error.next_action
                ),
            ) from error
        except (ValidationError, ValueError) as error:
            raise ScriptExtractionProviderError(
                outcome="failed",
                code="ai_output_invalid",
                summary="DeepSeek returned an invalid extraction result",
                retryable=False,
                next_action="start_new_extraction",
            ) from error
        except ScriptExtractionProviderError:
            raise
        except Exception as error:
            raise _provider_error(error) from error


class DeepSeekEpisodePlanner:
    def __init__(self, api_key: SecretStr) -> None:
        chat = ChatDeepSeek(
            model=_DEEPSEEK_MODEL,
            base_url=_DEEPSEEK_API_BASE,
            api_key=api_key,
            temperature=0,
            timeout=120,
            max_retries=0,
            extra_body={"thinking": {"type": "disabled"}},
        )
        self._structured_model: Any = chat.with_structured_output(
            EpisodePlanningProviderResult,
            method="json_mode",
        )

    async def plan(
        self,
        normalized_text: str,
        *,
        target_duration_ms: int,
        maximum_episode_count: int,
    ) -> EpisodePlanningProviderResult:
        try:
            source_block_count = len(normalized_text.splitlines())
            value = await self._structured_model.ainvoke(
                [
                    SystemMessage(
                        content=_episode_planning_system_prompt(
                            target_duration_ms=target_duration_ms,
                            maximum_episode_count=maximum_episode_count,
                            source_block_count=source_block_count,
                        )
                    ),
                    HumanMessage(content=_episode_planning_source_payload(normalized_text)),
                ]
            )
            return EpisodePlanningProviderResult.model_validate(value)
        except ValidationError as error:
            raise EpisodePlanningProviderError(
                outcome="failed",
                code="ai_output_invalid",
                summary="DeepSeek returned an invalid episode plan",
                retryable=False,
                next_action="start_new_episode_plan",
            ) from error
        except EpisodePlanningProviderError:
            raise
        except Exception as error:
            raise _episode_planning_provider_error(error) from error


class DeepSeekScriptAdapter:
    def __init__(self, api_key: SecretStr) -> None:
        chat = ChatDeepSeek(
            model=_DEEPSEEK_MODEL,
            base_url=_DEEPSEEK_API_BASE,
            api_key=api_key,
            temperature=0,
            timeout=120,
            max_retries=0,
            extra_body={"thinking": {"type": "disabled"}},
        )
        self._structured_model: Any = chat.with_structured_output(
            ScriptAdaptationProviderResult,
            method="json_mode",
        )

    async def adapt(
        self,
        script_body: str,
        *,
        target_duration_ms: int,
        core_plot_points: list[str],
        pacing: str,
        colloquial_dialogue: bool,
    ) -> ScriptAdaptationProviderResult:
        try:
            value = await self._structured_model.ainvoke(
                [
                    SystemMessage(content=_script_adaptation_system_prompt()),
                    HumanMessage(
                        content=_script_adaptation_payload(
                            script_body,
                            target_duration_ms=target_duration_ms,
                            core_plot_points=core_plot_points,
                            pacing=pacing,
                            colloquial_dialogue=colloquial_dialogue,
                        )
                    ),
                ]
            )
            return ScriptAdaptationProviderResult.model_validate(value)
        except ValidationError as error:
            raise ScriptAdaptationProviderError(
                outcome="failed",
                code="ai_output_invalid",
                summary="DeepSeek returned an invalid script adaptation",
                retryable=False,
                next_action="start_new_adaptation",
            ) from error
        except ScriptAdaptationProviderError:
            raise
        except Exception as error:
            raise _script_adaptation_provider_error(error) from error


class DeepSeekStoryboardDrafter:
    def __init__(
        self,
        api_key: SecretStr,
        *,
        model: StructuredSkillModel | None = None,
    ) -> None:
        resolved_model = model
        if resolved_model is None:
            chat = ChatDeepSeek(
                model=_DEEPSEEK_MODEL,
                base_url=_DEEPSEEK_API_BASE,
                api_key=api_key,
                temperature=0,
                timeout=165,
                max_retries=0,
                extra_body={"thinking": {"type": "disabled"}},
            )
            resolved_model = cast(
                StructuredSkillModel,
                chat.with_structured_output(
                    StoryboardProviderResult,
                    method="json_mode",
                ),
            )
        self._structured_model = resolved_model
        self._harness = SkillHarness()

    async def draft(self, value: StoryboardDraftInput) -> dict[str, object]:
        try:
            run = await self._harness.run(
                skill=STORYBOARD_DRAFT_SKILL,
                model=self._structured_model,
                system_prompt=_storyboard_draft_system_prompt(),
                user_payload=_storyboard_draft_payload(value),
                output_model=StoryboardProviderResult,
                context=SkillExecutionContext(
                    skill_name=STORYBOARD_DRAFT_SKILL.name,
                    skill_version=STORYBOARD_DRAFT_SKILL.version,
                    trace_id=f"draft-{value.batch_id}",
                    task_id=str(value.task_id),
                ),
            )
            return expand_provider_result(run.output, value).model_dump(mode="json")
        except SkillExecutionError as error:
            raise StoryboardDraftProviderError(
                outcome=error.outcome,
                code=error.code,
                summary=error.summary,
                retryable=error.retryable,
                next_action="create_new_storyboard_draft_batch",
            ) from error
        except (ValidationError, ValueError) as error:
            raise StoryboardDraftProviderError(
                outcome="failed",
                code="ai_output_invalid",
                summary="DeepSeek returned an invalid storyboard draft",
                retryable=False,
                next_action="create_new_storyboard_draft_batch",
            ) from error
        except StoryboardDraftProviderError:
            raise
        except Exception as error:
            raise _storyboard_draft_provider_error(error) from error
