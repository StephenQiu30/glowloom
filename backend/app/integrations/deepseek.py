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

DEEPSEEK_SCRIPT_EXTRACTOR_VERSION = SCRIPT_STRUCTURE_EXTRACTOR_VERSION
_DEEPSEEK_MODEL = "deepseek-v4-pro"
_DEEPSEEK_API_BASE = "https://api.deepseek.com"
_PROMPT_VERSION = "prompt-v1"
_EPISODE_PLAN_PROMPT_VERSION = "episode-plan-prompt-v2"


def _system_prompt() -> str:
    schema = json.dumps(
        ScriptExtractionResult.model_json_schema(),
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return (
        "你是 AI 漫剧平台的剧本结构提取器。只提取用户给出的剧本文本，不改写、"
        "补写或猜测不存在的内容。必须返回一个符合下列 JSON Schema 的 JSON 对象；"
        "source_range 使用 Python 字符串的零起始字符索引，end 为开区间；dialogue 和 "
        "shot 的 scene_candidate_key 必须引用同一响应中的 scene candidate_key。"
        f"当前提示版本为 {_PROMPT_VERSION}。JSON Schema: {schema}"
    )


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
        f"{target_duration_ms} 毫秒，最多 {maximum_episode_count} 集。"
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

    async def extract(self, script_body: str) -> ScriptExtractionResult:
        try:
            value = await self._structured_model.ainvoke(
                [SystemMessage(content=_system_prompt()), HumanMessage(content=script_body)]
            )
            result = ScriptExtractionResult.model_validate(value)
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
