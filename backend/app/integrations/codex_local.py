from __future__ import annotations

import asyncio
import json
import shutil
import subprocess
import tempfile
from collections.abc import Sequence
from pathlib import Path
from typing import Any, Generic, TypeVar, cast

from langchain_core.messages import HumanMessage, SystemMessage
from pydantic import BaseModel, ValidationError

from app.modules.scripts.adaptations import (
    SCRIPT_ADAPTATION_PROMPT_VERSION,
    ScriptAdaptationProviderError,
    ScriptAdaptationProviderResult,
    adaptation_duration_bounds,
)
from app.modules.scripts.extractions.anchoring import anchor_script_structure_ranges
from app.modules.scripts.extractions.ports import (
    SCRIPT_STRUCTURE_EXTRACTOR_VERSION,
    ScriptExtractionProviderError,
)
from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.scripts.planning.ports import (
    EPISODE_PLANNER_PROMPT_VERSION,
    EpisodePlanningProviderError,
)
from app.modules.scripts.planning.schemas import EpisodePlanningProviderResult
from app.modules.skills import (
    SkillDefinition,
    SkillExecutionContext,
    SkillExecutionError,
    StructuredSkillModel,
)
from app.modules.skills.script_structure import (
    DEFAULT_MAX_CHUNK_CHARS,
    ScriptStructureExtractionWorkflow,
)
from app.modules.skills.script_structure_prompt import script_structure_system_prompt
from app.modules.storyboards import StoryboardDraftInput, StoryboardDraftProviderError
from app.modules.storyboards.agents import (
    SceneAnalysis,
    ScenePlan,
    StoryboardAgentHarness,
    StoryboardAgentModels,
    StoryboardCheckpointStore,
    StoryboardReview,
)
from app.modules.storyboards.drafts.provider_schema import (
    StoryboardProviderResult,
    expand_provider_result,
    normalize_storyboard_provider_payload,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
SHUOHAO_STORYBOARD_REVISION = "0e5eb688ebf1b45e45c9bec31543aaa59e67c7bc"
STORYBOARD_SKILL_NAMES = (
    "storyboard-source-analysis",
    "storyboard-scene-plan",
    "storyboard-shot-draft",
    "storyboard-review",
    "storyboard-repair",
)
_UPSTREAM_REFERENCE_PATH = Path(
    "vendor/shuohao-skills/skills/novel-storyboard/references/storyboard-pass.md"
)

CODEX_LOCAL_SCRIPT_STRUCTURE_SKILL = SkillDefinition(
    name="script.structure.extract",
    version=SCRIPT_STRUCTURE_EXTRACTOR_VERSION,
    max_input_chars=DEFAULT_MAX_CHUNK_CHARS,
    timeout_seconds=120,
)
ResultModelT = TypeVar("ResultModelT", bound=BaseModel)


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


def _codex_output_schema(output_model: type[BaseModel]) -> dict[str, object]:
    schema = output_model.model_json_schema()

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
    return schema


def verify_storyboard_skills(repository_root: Path = REPOSITORY_ROOT) -> None:
    skill_paths = tuple(
        repository_root / ".agents" / "skills" / skill_name / "SKILL.md"
        for skill_name in STORYBOARD_SKILL_NAMES
    )
    policy_paths = tuple(path.parent / "agents" / "openai.yaml" for path in skill_paths)
    required_files = (*skill_paths, *policy_paths, repository_root / _UPSTREAM_REFERENCE_PATH)
    missing = [
        str(path.relative_to(repository_root)) for path in required_files if not path.is_file()
    ]
    if missing:
        raise RuntimeError(f"Lanverse storyboard skills are incomplete: {', '.join(missing)}")
    for skill_name, skill_path, policy_path in zip(
        STORYBOARD_SKILL_NAMES,
        skill_paths,
        policy_paths,
        strict=True,
    ):
        skill_text = skill_path.read_text(encoding="utf-8")
        policy_text = policy_path.read_text(encoding="utf-8")
        if f"name: {skill_name}" not in skill_text:
            raise RuntimeError(f"Storyboard skill name does not match: {skill_name}")
        if "allow_implicit_invocation: false" not in policy_text:
            raise RuntimeError(f"Storyboard skill must disable implicit invocation: {skill_name}")
    completed = subprocess.run(
        [
            "git",
            "-C",
            str(repository_root / "vendor/shuohao-skills"),
            "rev-parse",
            "HEAD",
        ],
        capture_output=True,
        check=False,
        text=True,
        timeout=5,
    )
    if completed.returncode != 0 or completed.stdout.strip() != SHUOHAO_STORYBOARD_REVISION:
        raise RuntimeError(
            "shuohao-skills revision does not match the reviewed storyboard contract"
        )


class CodexLocalStructuredModel(Generic[ResultModelT]):
    """Run one installed Codex CLI turn against a Pydantic output contract."""

    def __init__(
        self,
        *,
        output_model: type[ResultModelT],
        service_name: str,
        skill_name: str | None = None,
        codex_cli_path: str | None = None,
        model: str | None = None,
        max_concurrency: int = 2,
        timeout_seconds: float = 165,
        validation_attempts: int = 1,
    ) -> None:
        if validation_attempts < 1:
            raise ValueError("validation_attempts must be at least 1")
        resolved_cli_path = codex_cli_path or shutil.which("codex")
        if resolved_cli_path is None:
            raise RuntimeError("Local Codex executable is not available")
        self._codex_cli_path = resolved_cli_path
        self._output_model = output_model
        self._service_name = service_name
        self._skill_name = skill_name
        self._model = model
        self._timeout_seconds = timeout_seconds
        self._validation_attempts = validation_attempts
        self._semaphore = asyncio.Semaphore(max_concurrency)

    async def ainvoke(self, messages: Sequence[Any]) -> ResultModelT:
        system_prompt = next(
            (
                _message_content(message)
                for message in messages
                if message.__class__.__name__ == "SystemMessage"
            ),
            "",
        )
        user_prompt = _user_prompt(messages)
        prompt_parts: list[str] = []
        if self._skill_name is not None:
            prompt_parts.append(f"${self._skill_name}")
        if system_prompt:
            prompt_parts.append(system_prompt)
        prompt_parts.extend(
            (
                f"工作流标识：{self._service_name}",
                "不要运行工具或修改文件。只返回符合给定 JSON Schema 的对象。",
                user_prompt,
            )
        )
        with tempfile.TemporaryDirectory(prefix="lanverse-codex-") as temporary_dir:
            temp_path = Path(temporary_dir)
            schema_path = temp_path / "output-schema.json"
            schema_path.write_text(
                json.dumps(
                    _codex_output_schema(self._output_model),
                    ensure_ascii=False,
                    separators=(",", ":"),
                ),
                encoding="utf-8",
            )
            validation_error: ValidationError | None = None
            for attempt in range(1, self._validation_attempts + 1):
                response_path = temp_path / f"response-{attempt}.json"
                command = [
                    self._codex_cli_path,
                    "exec",
                    "--ephemeral",
                    "--sandbox",
                    "read-only",
                    "--cd",
                    str(REPOSITORY_ROOT),
                    "--config",
                    'model_reasoning_effort="low"',
                    "--output-schema",
                    str(schema_path),
                    "--output-last-message",
                    str(response_path),
                    "--color",
                    "never",
                ]
                if self._model is not None:
                    command.extend(("--model", self._model))
                command.append("-")
                attempt_parts = list(prompt_parts)
                if validation_error is not None:
                    attempt_parts.extend(
                        (
                            "上一次候选未通过结构校验。保持原输入事实不变，只修正以下错误后"
                            "重新返回完整 JSON：",
                            json.dumps(
                                [
                                    {
                                        "type": item["type"],
                                        "loc": list(item["loc"]),
                                        "msg": item["msg"],
                                    }
                                    for item in validation_error.errors(
                                        include_url=False,
                                        include_input=False,
                                    )
                                ],
                                ensure_ascii=False,
                                separators=(",", ":"),
                            ),
                        )
                    )
                async with self._semaphore:
                    await _run_codex_exec(
                        command,
                        "\n\n".join(attempt_parts),
                        timeout_seconds=self._timeout_seconds,
                    )
                if not response_path.is_file():
                    raise ValueError("Codex returned no final response")
                response = response_path.read_text(encoding="utf-8")
                try:
                    decoded = _decode_json_response(response)
                    if self._output_model is StoryboardProviderResult:
                        decoded = normalize_storyboard_provider_payload(decoded)
                    return cast(ResultModelT, self._output_model.model_validate(decoded))
                except ValidationError as error:
                    validation_error = error
            assert validation_error is not None
            raise validation_error

    async def aclose(self) -> None:
        return None


async def _run_codex_exec(
    command: list[str],
    prompt: str,
    *,
    timeout_seconds: float,
) -> None:
    process = await asyncio.create_subprocess_exec(
        *command,
        stdin=asyncio.subprocess.PIPE,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        _, stderr = await asyncio.wait_for(
            process.communicate(prompt.encode("utf-8")),
            timeout=timeout_seconds,
        )
    except TimeoutError:
        process.terminate()
        try:
            await asyncio.wait_for(process.wait(), timeout=5)
        except TimeoutError:
            process.kill()
            await process.wait()
        raise
    if process.returncode != 0:
        detail = stderr.decode("utf-8", errors="replace").strip().splitlines()
        suffix = f": {detail[-1]}" if detail else ""
        raise RuntimeError(f"Local Codex exited with status {process.returncode}{suffix}")


async def _close_model(model: StructuredSkillModel) -> None:
    close = getattr(model, "aclose", None)
    if close is not None:
        await close()


def _episode_planning_system_prompt(
    *,
    target_duration_ms: int,
    source_block_count: int,
) -> str:
    return (
        "你是 Lanverse AI 短剧分集规划器。用户消息是只读 JSON，source_blocks 每项包含"
        " position 和原文 text。只基于原文提出一个候选，不改写、补写或删除正文。按叙事"
        f"冲突、钩子和场景边界规划，每集目标 {target_duration_ms} 毫秒，完整覆盖输入原文。"
        "end_block_position 必须引用 source_block.position 并严格递增，最后一项必须等于"
        f" {source_block_count}。exact_end_anchor 必须逐字复制对应 text 的唯一末尾片段。"
        f"当前提示版本为 {EPISODE_PLANNER_PROMPT_VERSION}。"
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


def _script_adaptation_system_prompt() -> str:
    return (
        "你是 Lanverse AI 短剧剧本改写器。用户消息是只读 JSON。只输出改写候选，不声明"
        "已发布，不添加违反 core_plot_points 的新因果。按目标时长调整动作和对白，保留核心"
        "剧情点，并让 estimated_duration_ms 落在 duration_acceptance_range_ms 内。"
        f"当前提示版本为 {SCRIPT_ADAPTATION_PROMPT_VERSION}。"
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


class CodexLocalScriptStructureExtractor:
    def __init__(
        self,
        *,
        codex_cli_path: str | None = None,
        model: str | None = None,
        max_concurrency: int = 2,
    ) -> None:
        self._model = CodexLocalStructuredModel(
            output_model=ScriptExtractionResult,
            service_name="lanverse-script-structure",
            codex_cli_path=codex_cli_path,
            model=model,
            max_concurrency=max_concurrency,
            timeout_seconds=120,
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
            result = await self._workflow.run(
                script_body,
                context=SkillExecutionContext(
                    skill_name=CODEX_LOCAL_SCRIPT_STRUCTURE_SKILL.name,
                    skill_version=CODEX_LOCAL_SCRIPT_STRUCTURE_SKILL.version,
                    trace_id=trace_id,
                ),
                episode_number=episode_number,
            )
            result = anchor_script_structure_ranges(result, script_body)
            if any(
                candidate.source_range.end > len(script_body) for candidate in result.candidates
            ):
                raise ValueError("extraction range is outside the screenplay")
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
        except ScriptExtractionProviderError:
            raise
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


class CodexLocalEpisodePlanner:
    def __init__(
        self,
        *,
        codex_cli_path: str | None = None,
        codex_model: str | None = None,
        max_concurrency: int = 2,
        model: StructuredSkillModel | None = None,
    ) -> None:
        self._model = model or CodexLocalStructuredModel(
            output_model=EpisodePlanningProviderResult,
            service_name="lanverse-episode-planning",
            codex_cli_path=codex_cli_path,
            model=codex_model,
            max_concurrency=max_concurrency,
            timeout_seconds=120,
        )

    async def plan(
        self,
        normalized_text: str,
        *,
        target_duration_ms: int,
        maximum_episode_count: int,
    ) -> EpisodePlanningProviderResult:
        del maximum_episode_count
        try:
            source_block_count = len(normalized_text.splitlines())
            value = await self._model.ainvoke(
                [
                    SystemMessage(
                        content=_episode_planning_system_prompt(
                            target_duration_ms=target_duration_ms,
                            source_block_count=source_block_count,
                        )
                    ),
                    HumanMessage(content=_episode_planning_source_payload(normalized_text)),
                ]
            )
            return EpisodePlanningProviderResult.model_validate(value)
        except (ValidationError, ValueError) as error:
            raise EpisodePlanningProviderError(
                outcome="failed",
                code="codex_output_invalid",
                summary="Local Codex returned an invalid episode plan",
                retryable=False,
                next_action="start_new_episode_plan",
            ) from error
        except EpisodePlanningProviderError:
            raise
        except Exception as error:
            raise EpisodePlanningProviderError(
                outcome="unknown",
                code="codex_local_unavailable",
                summary="Local Codex episode planning is unavailable",
                retryable=True,
                next_action="retry",
            ) from error

    async def aclose(self) -> None:
        await _close_model(self._model)


class CodexLocalScriptAdapter:
    def __init__(
        self,
        *,
        codex_cli_path: str | None = None,
        codex_model: str | None = None,
        max_concurrency: int = 2,
        model: StructuredSkillModel | None = None,
    ) -> None:
        self._model = model or CodexLocalStructuredModel(
            output_model=ScriptAdaptationProviderResult,
            service_name="lanverse-script-adaptation",
            codex_cli_path=codex_cli_path,
            model=codex_model,
            max_concurrency=max_concurrency,
            timeout_seconds=120,
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
            value = await self._model.ainvoke(
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
        except (ValidationError, ValueError) as error:
            raise ScriptAdaptationProviderError(
                outcome="failed",
                code="codex_output_invalid",
                summary="Local Codex returned an invalid script adaptation",
                retryable=False,
                next_action="start_new_adaptation",
            ) from error
        except ScriptAdaptationProviderError:
            raise
        except Exception as error:
            raise ScriptAdaptationProviderError(
                outcome="unknown",
                code="codex_local_unavailable",
                summary="Local Codex script adaptation is unavailable",
                retryable=True,
                next_action="retry",
            ) from error

    async def aclose(self) -> None:
        await _close_model(self._model)


class CodexLocalStoryboardDrafter:
    def __init__(
        self,
        *,
        codex_cli_path: str | None = None,
        codex_model: str | None = None,
        max_concurrency: int = 2,
        model: StructuredSkillModel | None = None,
        checkpoint_store: StoryboardCheckpointStore | None = None,
        verify_skill: bool = True,
    ) -> None:
        if verify_skill:
            verify_storyboard_skills()
        if model is not None:
            models = StoryboardAgentModels(
                source_analysis=model,
                scene_plan=model,
                shot_draft=model,
                review=model,
                repair=model,
            )
            self._owned_models = (model,)
        else:
            source_analysis = CodexLocalStructuredModel(
                output_model=SceneAnalysis,
                service_name="lanverse-storyboard-source-analysis",
                skill_name="storyboard-source-analysis",
                codex_cli_path=codex_cli_path,
                model=codex_model,
                max_concurrency=max_concurrency,
                timeout_seconds=165,
                validation_attempts=2,
            )
            scene_plan = CodexLocalStructuredModel(
                output_model=ScenePlan,
                service_name="lanverse-storyboard-scene-plan",
                skill_name="storyboard-scene-plan",
                codex_cli_path=codex_cli_path,
                model=codex_model,
                max_concurrency=max_concurrency,
                timeout_seconds=165,
                validation_attempts=2,
            )
            shot_draft = CodexLocalStructuredModel(
                output_model=StoryboardProviderResult,
                service_name="lanverse-storyboard-shot-draft",
                skill_name="storyboard-shot-draft",
                codex_cli_path=codex_cli_path,
                model=codex_model,
                max_concurrency=max_concurrency,
                timeout_seconds=165,
                validation_attempts=2,
            )
            review = CodexLocalStructuredModel(
                output_model=StoryboardReview,
                service_name="lanverse-storyboard-review",
                skill_name="storyboard-review",
                codex_cli_path=codex_cli_path,
                model=codex_model,
                max_concurrency=max_concurrency,
                timeout_seconds=165,
                validation_attempts=2,
            )
            repair = CodexLocalStructuredModel(
                output_model=StoryboardProviderResult,
                service_name="lanverse-storyboard-repair",
                skill_name="storyboard-repair",
                codex_cli_path=codex_cli_path,
                model=codex_model,
                max_concurrency=max_concurrency,
                timeout_seconds=165,
                validation_attempts=2,
            )
            models = StoryboardAgentModels(
                source_analysis=source_analysis,
                scene_plan=scene_plan,
                shot_draft=shot_draft,
                review=review,
                repair=repair,
            )
            self._owned_models = (
                source_analysis,
                scene_plan,
                shot_draft,
                review,
                repair,
            )
        self._harness = StoryboardAgentHarness(
            models=models,
            checkpoint_store=checkpoint_store,
        )

    async def draft(self, value: StoryboardDraftInput) -> dict[str, object]:
        try:
            result = await self._harness.run(value)
            if result.status != "needs_review" or result.candidate is None:
                issue_codes = ", ".join(issue.code for issue in result.issues[:5])
                raise StoryboardDraftProviderError(
                    outcome="failed",
                    code="storyboard_hard_gate_failed",
                    summary=(
                        "Storyboard candidate did not pass deterministic gates"
                        + (f": {issue_codes}" if issue_codes else "")
                    ),
                    retryable=False,
                    next_action="create_new_storyboard_draft_batch",
                )
            return expand_provider_result(result.candidate, value).model_dump(mode="json")
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
                code="codex_output_invalid",
                summary="Local Codex returned an invalid storyboard draft",
                retryable=False,
                next_action="create_new_storyboard_draft_batch",
            ) from error
        except StoryboardDraftProviderError:
            raise
        except Exception as error:
            raise StoryboardDraftProviderError(
                outcome="unknown",
                code="codex_local_unavailable",
                summary="Local Codex storyboard drafting is unavailable",
                retryable=True,
                next_action="create_new_storyboard_draft_batch",
            ) from error

    async def aclose(self) -> None:
        for model in self._owned_models:
            await _close_model(model)
