from __future__ import annotations

import asyncio
import json
import os
import shutil
import tempfile
from pathlib import Path
from typing import Any, TypeVar, cast

from pydantic import BaseModel

OutputModel = TypeVar("OutputModel", bound=BaseModel)


class CodexExecutionError(RuntimeError):
    pass


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
    def __init__(self, *, repository_root: Path | None = None) -> None:
        self._repository_root = repository_root or Path(__file__).resolve().parents[4]
        configured = os.getenv("CODEX_BIN", "").strip()
        self._codex_bin = configured or shutil.which("codex") or "codex"
        self.model_name = os.getenv("CODEX_MODEL", "").strip() or "inherited-local-config"

    async def run(self, prompt: str, output_model: type[OutputModel]) -> OutputModel:
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
                str(self._repository_root),
                "--output-schema",
                str(schema_path),
                "--output-last-message",
                str(response_path),
                "--json",
                "--color",
                "never",
            ]
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
                raise CodexExecutionError("Codex CLI could not be started") from error
            stdout, stderr = await process.communicate(prompt.encode("utf-8"))
            if process.returncode != 0 or not response_path.is_file():
                raise CodexExecutionError(
                    f"Codex CLI exited {process.returncode}: "
                    f"{structured_diagnostic(stdout, stderr)}"
                )
            try:
                value: Any = json.loads(response_path.read_text(encoding="utf-8"))
                return output_model.model_validate(value)
            except (OSError, json.JSONDecodeError, ValueError) as error:
                raise CodexExecutionError(
                    "Codex CLI returned an invalid structured result"
                ) from error

    async def aclose(self) -> None:
        return None
