from __future__ import annotations

import asyncio
import json
from collections.abc import Sequence
from typing import Any, cast

from openai_codex import ApprovalMode, AsyncCodex, CodexConfig, Sandbox
from openai_codex.models import JsonObject
from pydantic import ValidationError

from app.modules.scripts.extractions.ports import (
    SCRIPT_STRUCTURE_EXTRACTOR_VERSION,
    ScriptExtractionProviderError,
)
from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.skills import SkillDefinition, SkillExecutionContext, SkillExecutionError
from app.modules.skills.script_structure import (
    DEFAULT_MAX_CHUNK_CHARS,
    ScriptStructureExtractionWorkflow,
)
from app.modules.skills.script_structure_prompt import script_structure_system_prompt

CODEX_LOCAL_SCRIPT_STRUCTURE_SKILL = SkillDefinition(
    name="script.structure.extract",
    version=SCRIPT_STRUCTURE_EXTRACTOR_VERSION,
    max_input_chars=DEFAULT_MAX_CHUNK_CHARS,
    timeout_seconds=120,
)


def _message_content(message: Any) -> str:
    content = getattr(message, "content", "")
    if isinstance(content, str):
        return content
    return json.dumps(content, ensure_ascii=False, separators=(",", ":"))


def _user_prompt(messages: Sequence[Any]) -> str:
    contents = [
        _message_content(message)
        for message in messages
        if message.__class__.__name__ != "SystemMessage"
    ]
    if contents:
        return "\n\n".join(contents)
    return "\n\n".join(_message_content(message) for message in messages)


def _decode_json_response(response: str) -> dict[str, object]:
    text = response.strip()
    value: object
    try:
        value = json.loads(text)
    except json.JSONDecodeError as error:
        decoder = json.JSONDecoder()
        value = None
        for index, character in enumerate(text):
            if character != "{":
                continue
            try:
                candidate = cast(object, decoder.raw_decode(text[index:])[0])
            except json.JSONDecodeError:
                continue
            if isinstance(candidate, dict):
                value = cast(dict[str, object], candidate)
                break
        if value is None:
            raise ValueError("Codex did not return a JSON object") from error
    if not isinstance(value, dict):
        raise ValueError("Codex returned a non-object JSON value")
    return cast(dict[str, object], value)


def _codex_output_schema() -> JsonObject:
    schema = ScriptExtractionResult.model_json_schema()

    def normalize(node: object) -> None:
        if isinstance(node, dict):
            mapping = cast(dict[str, object], node)
            if "oneOf" in mapping:
                mapping["anyOf"] = mapping.pop("oneOf")
                mapping.pop("discriminator", None)
            properties = mapping.get("properties")
            if isinstance(properties, dict):
                property_map = cast(dict[str, object], properties)
                mapping["required"] = list(property_map.keys())
                mapping["additionalProperties"] = False
            for child in mapping.values():
                normalize(child)
        elif isinstance(node, list):
            for child in cast(list[object], node):
                normalize(child)

    normalize(schema)
    return cast(JsonObject, schema)


class CodexLocalStructuredModel:
    """Adapts one local Codex turn to the Harness structured-model port."""

    def __init__(
        self,
        *,
        codex_cli_path: str | None = None,
        model: str | None = None,
        max_concurrency: int = 2,
    ) -> None:
        config = CodexConfig(codex_bin=codex_cli_path) if codex_cli_path else None
        self._config = config
        self._model = model
        self._codex: AsyncCodex | None = None
        self._start_lock = asyncio.Lock()
        self._semaphore = asyncio.Semaphore(max_concurrency)

    async def _client(self) -> AsyncCodex:
        if self._codex is not None:
            return self._codex
        async with self._start_lock:
            if self._codex is None:
                client = AsyncCodex(self._config)
                try:
                    await client.__aenter__()
                except Exception:
                    await client.close()
                    raise
                self._codex = client
        assert self._codex is not None
        return self._codex

    async def ainvoke(self, messages: Sequence[Any]) -> object:
        system_prompt = next(
            (
                _message_content(message)
                for message in messages
                if message.__class__.__name__ == "SystemMessage"
            ),
            "",
        )
        user_prompt = _user_prompt(messages)
        async with self._semaphore:
            codex = await self._client()
            thread = await codex.thread_start(
                approval_mode=ApprovalMode.deny_all,
                base_instructions=system_prompt,
                ephemeral=True,
                model=self._model,
                sandbox=Sandbox.read_only,
                service_name="lanverse-script-structure",
            )
            turn = await thread.run(
                user_prompt,
                approval_mode=ApprovalMode.deny_all,
                model=self._model,
                output_schema=_codex_output_schema(),
                sandbox=Sandbox.read_only,
            )
        if turn.final_response is None:
            raise ValueError("Codex returned no final response")
        return ScriptExtractionResult.model_validate(_decode_json_response(turn.final_response))

    async def aclose(self) -> None:
        codex = self._codex
        self._codex = None
        if codex is not None:
            await codex.close()


class CodexLocalScriptStructureExtractor:
    def __init__(
        self,
        *,
        codex_cli_path: str | None = None,
        model: str | None = None,
        max_concurrency: int = 2,
    ) -> None:
        self._model = CodexLocalStructuredModel(
            codex_cli_path=codex_cli_path,
            model=model,
            max_concurrency=max_concurrency,
        )
        self._workflow = ScriptStructureExtractionWorkflow(
            skill=CODEX_LOCAL_SCRIPT_STRUCTURE_SKILL,
            model=self._model,
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
            return await self._workflow.run(
                script_body,
                context=SkillExecutionContext(
                    skill_name=CODEX_LOCAL_SCRIPT_STRUCTURE_SKILL.name,
                    skill_version=CODEX_LOCAL_SCRIPT_STRUCTURE_SKILL.version,
                    trace_id=trace_id,
                ),
                episode_number=episode_number,
            )
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
                code="codex_output_invalid",
                summary="Local Codex returned an invalid extraction result",
                retryable=False,
                next_action="start_new_extraction",
            ) from error
        except TimeoutError as error:
            raise ScriptExtractionProviderError(
                outcome="unknown",
                code="codex_result_unknown",
                summary="Local Codex response outcome is unknown",
                retryable=False,
                next_action="reconcile_skill_run",
            ) from error
        except Exception as error:
            raise ScriptExtractionProviderError(
                outcome="unknown",
                code="codex_local_unavailable",
                summary="Local Codex service is unavailable",
                retryable=True,
                next_action="retry",
            ) from error

    async def aclose(self) -> None:
        await self._model.aclose()
