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

from app.modules.scripts.extractions.ports import (
    SCRIPT_STRUCTURE_EXTRACTOR_VERSION,
    ScriptExtractionProviderError,
)
from app.modules.scripts.extractions.schemas import ScriptExtractionResult

DEEPSEEK_SCRIPT_EXTRACTOR_VERSION = SCRIPT_STRUCTURE_EXTRACTOR_VERSION
_DEEPSEEK_MODEL = "deepseek-v4-pro"
_DEEPSEEK_API_BASE = "https://api.deepseek.com"
_PROMPT_VERSION = "prompt-v1"


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
                candidate.source_range.end > len(script_body)
                for candidate in result.candidates
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
