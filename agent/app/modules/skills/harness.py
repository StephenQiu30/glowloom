from __future__ import annotations

import asyncio
import json
import os
import shutil
import tempfile
import time
from pathlib import Path
from typing import Any, TypeVar, cast

from pydantic import BaseModel

from app.candidate_runtime.schemas import ExecutionPolicy

OutputModel = TypeVar("OutputModel", bound=BaseModel)


class CodexExecutionError(RuntimeError):
    pass


class CodexBudgetExceeded(CodexExecutionError):
    pass


class CodexDeadlineExceeded(CodexExecutionError):
    pass


class CodexToolPolicyViolation(CodexExecutionError):
    pass


class CodexSchemaInvalid(CodexExecutionError):
    pass


class CodexRuntimeUnavailable(CodexExecutionError):
    pass


_DISABLED_FEATURES = (
    "apps",
    "browser_use",
    "browser_use_external",
    "browser_use_full_cdp_access",
    "computer_use",
    "image_generation",
    "in_app_browser",
    "multi_agent",
    "multi_agent_v2",
    "plugins",
    "shell_tool",
    "skill_search",
    "standalone_web_search",
    "unified_exec",
    "view_image",
    "web_search_cached",
    "web_search_request",
    "workspace_dependencies",
)

_SAFE_ITEM_TYPES = {"agent_message", "error", "reasoning"}


def structured_diagnostic(stdout: bytes, stderr: bytes) -> str:
    messages: list[str] = []
    for line in stdout.decode("utf-8", errors="replace").splitlines():
        try:
            decoded: Any = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(decoded, dict):
            continue
        event = cast(dict[str, Any], decoded)
        event_type = str(event.get("type", ""))
        if event_type == "error":
            message = event.get("message")
            if isinstance(message, str) and message.strip():
                messages.append(message.strip())
        error_value = event.get("error")
        if isinstance(error_value, dict):
            error = cast(dict[str, Any], error_value)
            message = error.get("message")
            if isinstance(message, str) and message.strip():
                messages.append(message.strip())
        elif (
            event_type.endswith(".failed") and isinstance(error_value, str) and error_value.strip()
        ):
            messages.append(error_value.strip())
    if messages:
        return messages[-1][:400]

    fallback = [
        line.strip()
        for line in stderr.decode("utf-8", errors="replace").splitlines()
        if line.strip() not in {"", "{", "}", "[", "]"}
    ]
    return fallback[-1][:400] if fallback else "no diagnostic output"


class CodexSchemaRunner:
    def __init__(
        self,
        *,
        execution_policy: ExecutionPolicy,
        repository_root: Path | None = None,
    ) -> None:
        self._repository_root = repository_root or Path(__file__).resolve().parents[4]
        self._execution_policy = execution_policy
        self._model_calls = 0
        self._deadline_at = time.monotonic() + execution_policy.max_execution_seconds
        configured = os.getenv("CODEX_BIN", "").strip()
        self._codex_bin = configured or shutil.which("codex") or "codex"
        self.model_name = os.getenv("CODEX_MODEL", "").strip() or "inherited-local-config"

    async def run(
        self,
        prompt: str,
        output_model: type[OutputModel],
        *,
        skill_name: str,
    ) -> OutputModel:
        guidance = self._skill_guidance(skill_name)
        remaining_seconds = self._deadline_at - time.monotonic()
        if remaining_seconds <= 0:
            raise CodexDeadlineExceeded("Agent execution deadline is exhausted")
        if self._model_calls >= self._execution_policy.max_model_calls:
            raise CodexBudgetExceeded("Agent model-call budget is exhausted")
        self._model_calls += 1
        with tempfile.TemporaryDirectory(prefix="lanverse-codex-") as temporary:
            root = Path(temporary)
            schema_path = root / "output-schema.json"
            response_path = root / "response.json"
            schema_path.write_text(
                json.dumps(output_model.model_json_schema(), ensure_ascii=False),
                encoding="utf-8",
            )
            command = [
                self._codex_bin,
                "exec",
                "--ephemeral",
                "--sandbox",
                "read-only",
                "--cd",
                str(root),
                "--skip-git-repo-check",
                "--ignore-user-config",
                "--output-schema",
                str(schema_path),
                "--output-last-message",
                str(response_path),
                "--json",
                "--color",
                "never",
            ]
            for feature in _DISABLED_FEATURES:
                command.extend(["--disable", feature])
            configured_model = os.getenv("CODEX_MODEL", "").strip()
            if configured_model:
                command.extend(["--model", configured_model])
            command.append("-")
            try:
                process = await asyncio.create_subprocess_exec(
                    *command,
                    stdin=asyncio.subprocess.PIPE,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.PIPE,
                )
            except OSError as error:
                raise CodexRuntimeUnavailable("Codex CLI could not be started") from error
            try:
                stdout, stderr = await asyncio.wait_for(
                    process.communicate(_prompt_with_guidance(guidance, prompt).encode("utf-8")),
                    timeout=remaining_seconds,
                )
            except TimeoutError as error:
                try:
                    process.kill()
                except ProcessLookupError:
                    pass
                await process.wait()
                raise CodexDeadlineExceeded("Agent execution deadline is exhausted") from error
            if process.returncode != 0 or not response_path.is_file():
                raise CodexRuntimeUnavailable(
                    f"Codex CLI exited {process.returncode}: "
                    f"{structured_diagnostic(stdout, stderr)}"
                )
            unauthorized_item = _unauthorized_item_type(stdout)
            if unauthorized_item is not None:
                raise CodexToolPolicyViolation(
                    f"Codex CLI attempted disallowed item type: {unauthorized_item}"
                )
            try:
                value: Any = json.loads(response_path.read_text(encoding="utf-8"))
                return output_model.model_validate(value)
            except (OSError, json.JSONDecodeError, ValueError) as error:
                raise CodexSchemaInvalid(
                    "Codex CLI returned an invalid structured result"
                ) from error

    def _skill_guidance(self, skill_name: str) -> str:
        if not skill_name or any(
            character not in "abcdefghijklmnopqrstuvwxyz0123456789-" for character in skill_name
        ):
            raise CodexRuntimeUnavailable("Agent skill name is invalid")
        skills_root = (self._repository_root / ".agents" / "skills").resolve()
        skill_root = (skills_root / skill_name).resolve()
        if not skill_root.is_relative_to(skills_root) or not (skill_root / "SKILL.md").is_file():
            raise CodexRuntimeUnavailable("Agent skill bundle is unavailable")
        paths = [skill_root / "SKILL.md"]
        paths.extend(
            path
            for path in sorted(skill_root.rglob("*.md"))
            if path != skill_root / "SKILL.md" and path.resolve().is_relative_to(skill_root)
        )
        try:
            return "\n\n".join(
                f"## {path.relative_to(skill_root)}\n{path.read_text(encoding='utf-8')}"
                for path in paths
            )
        except OSError as error:
            raise CodexRuntimeUnavailable("Agent skill bundle could not be read") from error

    async def aclose(self) -> None:
        return None


def _prompt_with_guidance(guidance: str, prompt: str) -> str:
    return (
        "You are a restricted structured-text executor. No tools are authorized or available. "
        "Use only the immutable task input, project guidance, and output schema supplied by the "
        "harness. Never read files, run commands, call networks, or perform side effects.\n\n"
        f"# Project guidance\n{guidance}\n\n# Task\n{prompt}"
    )


def _unauthorized_item_type(stdout: bytes) -> str | None:
    for line in stdout.decode("utf-8", errors="replace").splitlines():
        try:
            decoded: Any = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(decoded, dict):
            continue
        event = cast(dict[str, Any], decoded)
        item_value = event.get("item")
        if not isinstance(item_value, dict):
            continue
        item = cast(dict[str, Any], item_value)
        item_type = item.get("type")
        if isinstance(item_type, str) and item_type not in _SAFE_ITEM_TYPES:
            return item_type[:80]
    return None
