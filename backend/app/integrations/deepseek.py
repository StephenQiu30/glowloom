import json
from typing import Any

from langchain_core.messages import HumanMessage, SystemMessage
from langchain_deepseek import ChatDeepSeek
from openai import (
    APIConnectionError,
    APITimeoutError,
    AuthenticationError,
    BadRequestError,
    InternalServerError,
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
from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.scripts.planning.ports import EpisodePlanningProviderError
from app.modules.scripts.planning.schemas import EpisodePlanningProviderResult
from app.modules.skills import (
    SkillDefinition,
    SkillExecutionContext,
    SkillExecutionError,
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
_STORYBOARD_DRAFT_PROMPT_VERSION = "storyboard-draft-prompt-v1"


def _provider_error(error: Exception) -> ScriptExtractionProviderError:
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
        "连续递增；60–120 秒短剧通常生成 12–24 镜。required_for_coverage=true 的单元"
        "必须至少被一个镜头的 unit_positions 引用。只能引用输入中的整数 position，禁止生成"
        "UUID；scene_unit_position 指向当前镜头所属场景中的任一输入单元，"
        "dialogue_unit_positions 只引用 has_dialogue_reference=true 的单元。"
        "asset_bindings 只按 asset_position 绑定确有拍摄用途的固定资产。"
        "title 不超过 20 个汉字，purpose、composition、environment、mood_lighting、action "
        "和 ambient 各不超过 80 个汉字，避免解释性长文。risk_codes 只报告需要人工复核的问题。"
        "必须返回符合 JSON Schema 的 JSON 对象。"
        f"当前提示版本为 {_STORYBOARD_DRAFT_PROMPT_VERSION}。JSON Schema: {schema}"
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
    ) -> ScriptExtractionResult:
        try:
            result = await self._workflow.run(
                script_body,
                context=SkillExecutionContext(
                    skill_name=SCRIPT_STRUCTURE_EXTRACTION_SKILL.name,
                    skill_version=SCRIPT_STRUCTURE_EXTRACTION_SKILL.version,
                    trace_id=trace_id,
                ),
            )
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
        except ValidationError as error:
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
            StoryboardProviderResult,
            method="json_mode",
        )

    async def draft(self, value: StoryboardDraftInput) -> dict[str, object]:
        try:
            provider_result = await self._structured_model.ainvoke(
                [
                    SystemMessage(content=_storyboard_draft_system_prompt()),
                    HumanMessage(content=_storyboard_draft_payload(value)),
                ]
            )
            result = StoryboardProviderResult.model_validate(provider_result)
            return expand_provider_result(result, value).model_dump(mode="json")
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
