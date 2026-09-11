"""Restricted Codex process adapter; independent of business tasks and trusted storage."""

from __future__ import annotations

import asyncio
import hashlib
import json
import math
import os
import signal
import stat
import tempfile
from collections.abc import Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any, cast

from pydantic import BaseModel


class CodexExecutionError(RuntimeError):
    pass


class CodexBudgetExceeded(CodexExecutionError):
    pass


class CodexDeadlineExceeded(CodexExecutionError):
    pass


class CodexToolPolicyViolation(CodexExecutionError):
    pass


class CodexSchemaInvalid(CodexExecutionError):
    def __init__(self, message: str, raw_output: str | None = None) -> None:
        super().__init__(message)
        self.raw_output = raw_output


class CodexRuntimeUnavailable(CodexExecutionError):
    pass


class CodexMediaInvalid(CodexExecutionError):
    pass


@dataclass(frozen=True)
class CodexImageInput:
    sort_key: str
    path: Path
    content_hash: str
    byte_length: int
    media_type: str


_CODEX_DELEGATED_AGENT_FEATURE = "multi_agent_" + "v" + "2"

_DISABLED_FEATURES = (
    "apps",
    "browser_use",
    "browser_use_external",
    "browser_use_full_cdp_access",
    "computer_use",
    "image_generation",
    "in_app_browser",
    "multi_agent",
    _CODEX_DELEGATED_AGENT_FEATURE,
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

_IMAGE_EXTENSIONS = {
    "image/jpeg": ".jpg",
    "image/png": ".png",
    "image/webp": ".webp",
}


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
        elif event_type.endswith(".failed") and isinstance(error_value, str):
            if error_value.strip():
                messages.append(error_value.strip())
    if messages:
        return messages[-1][:400]
    fallback = [
        line.strip()
        for line in stderr.decode("utf-8", errors="replace").splitlines()
        if line.strip() not in {"", "{", "}", "[", "]"}
    ]
    return fallback[-1][:400] if fallback else "no diagnostic output"


async def run_codex_process(
    *,
    codex_bin: str,
    guidance: str,
    prompt: str,
    output_model: type[BaseModel],
    timeout_seconds: float,
    max_output_bytes: int | None = None,
    strict_output_schema: bool = False,
    image_inputs: Sequence[CodexImageInput] = (),
) -> BaseModel:
    if timeout_seconds <= 0:
        raise CodexDeadlineExceeded("Agent execution deadline is exhausted")
    with tempfile.TemporaryDirectory(prefix="lanverse-codex-") as temporary:
        root = Path(temporary)
        schema_path = root / "output-schema.json"
        response_path = root / "response.json"
        schema_path.write_text(
            json.dumps(
                codex_output_schema(output_model)
                if strict_output_schema
                else output_model.model_json_schema(),
                ensure_ascii=False,
            ),
            encoding="utf-8",
        )
        staged_images = _stage_image_inputs(root, image_inputs)
        command = [
            codex_bin,
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
        for staged_image in staged_images:
            command.extend(["--image", str(staged_image)])
        command.append("-")
        try:
            process = await asyncio.create_subprocess_exec(
                *command,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                env=codex_environment(),
                start_new_session=os.name == "posix",
            )
        except OSError as error:
            raise CodexRuntimeUnavailable("Codex CLI could not be started") from error
        try:
            stdout, stderr = await asyncio.wait_for(
                (
                    process.communicate(
                        _prompt_with_guidance(
                            guidance,
                            prompt,
                            has_images=bool(staged_images),
                        ).encode("utf-8")
                    )
                    if max_output_bytes is None
                    else communicate_bounded(
                        process,
                        _prompt_with_guidance(
                            guidance,
                            prompt,
                            has_images=bool(staged_images),
                        ).encode("utf-8"),
                        max_output_bytes,
                    )
                ),
                timeout=timeout_seconds,
            )
        except BaseException as error:
            try:
                pid = getattr(process, "pid", None)
                if os.name == "posix" and isinstance(pid, int):
                    os.killpg(pid, signal.SIGKILL)
                else:
                    process.kill()
            except ProcessLookupError:
                pass
            await drain_and_wait(process)
            if isinstance(error, TimeoutError):
                raise CodexDeadlineExceeded("Agent execution deadline is exhausted") from error
            if isinstance(error, OSError):
                raise CodexRuntimeUnavailable("Codex transport was interrupted") from error
            raise
        unauthorized_item = unauthorized_item_type(stdout)
        if unauthorized_item is not None:
            raise CodexToolPolicyViolation(
                f"Codex CLI attempted disallowed item type: {unauthorized_item}"
            )
        if process.returncode != 0 or not response_path.is_file():
            raise CodexRuntimeUnavailable(
                f"Codex CLI exited {process.returncode}: {structured_diagnostic(stdout, stderr)}"
            )
        raw = b""
        try:
            # Check the opened descriptor, not a path that could change between stat and read.
            flags = os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW
            with os.fdopen(os.open(response_path, flags), "rb") as output:
                info = os.fstat(output.fileno())
                if not stat.S_ISREG(info.st_mode):
                    raise ValueError("structured output must be a regular file")
                if max_output_bytes is not None and info.st_size > max_output_bytes:
                    raise CodexBudgetExceeded("Codex structured output exceeds the byte budget")
                raw = output.read(-1 if max_output_bytes is None else max_output_bytes + 1)
            if max_output_bytes is not None and len(raw) > max_output_bytes:
                raise CodexBudgetExceeded("Codex structured output exceeds the byte budget")
            value = decode_structured_output(raw.decode("utf-8"))
            return output_model.model_validate(value)
        except (OSError, json.JSONDecodeError, ValueError) as error:
            raise CodexSchemaInvalid(
                "Codex CLI returned an invalid structured result",
                raw.decode("utf-8", errors="replace")[:2000000],
            ) from error


def codex_environment() -> dict[str, str]:
    # Model authentication is allowed; platform/storage/provider credentials are not.
    names = (
        "PATH",
        "HOME",
        "CODEX_HOME",
        "XDG_CONFIG_HOME",
        "TMPDIR",
        "LANG",
        "LC_ALL",
        "TERM",
        "SSL_CERT_FILE",
        "SSL_CERT_DIR",
        "OPENAI_API_KEY",
    )
    return {name: os.environ[name] for name in names if name in os.environ}


async def drain_and_wait(process: asyncio.subprocess.Process) -> None:
    async def drain(stream: asyncio.StreamReader) -> None:
        while await stream.read(65536):
            pass

    # wait() can otherwise hang after kill when a full pipe has paused its transport.
    readers = [
        drain(stream)
        for stream in (getattr(process, "stdout", None), getattr(process, "stderr", None))
        if isinstance(stream, asyncio.StreamReader)
    ]
    await asyncio.gather(process.wait(), *readers)


def codex_output_schema(model: type[BaseModel]) -> dict[str, Any]:
    """Require explicit nulls for optional fields in strict structured output."""
    schema = model.model_json_schema()

    def visit(value: Any) -> None:
        if isinstance(value, dict):
            node = cast(dict[str, Any], value)
            node.pop("default", None)
            properties = node.get("properties")
            if isinstance(properties, dict):
                node["required"] = list(cast(dict[str, Any], properties))
            for child in node.values():
                visit(child)
        elif isinstance(value, list):
            for child in cast(list[Any], value):
                visit(child)

    visit(schema)
    return schema


async def communicate_bounded(
    process: asyncio.subprocess.Process, prompt: bytes, limit: int
) -> tuple[bytes, bytes]:
    if process.stdin is None or process.stdout is None or process.stderr is None:
        raise CodexExecutionError("Codex pipes are unavailable")

    async def read(stream: asyncio.StreamReader) -> bytes:
        value = bytearray()
        while chunk := await stream.read(65536):
            value.extend(chunk)
            if len(value) > limit:
                raise CodexBudgetExceeded("Codex diagnostic output exceeds the byte budget")
        return bytes(value)

    writer = process.stdin

    async def write() -> None:
        try:
            writer.write(prompt)
            await writer.drain()
        finally:
            writer.close()

    # Readers must be able to fail while the child refuses to consume stdin.
    stdout = asyncio.create_task(read(process.stdout))
    stderr = asyncio.create_task(read(process.stderr))
    sender = asyncio.create_task(write())
    try:
        await asyncio.gather(stdout, stderr, sender)
        await process.wait()
        return stdout.result(), stderr.result()
    finally:
        for task in (stdout, stderr, sender):
            if not task.done():
                task.cancel()
        await asyncio.gather(stdout, stderr, sender, return_exceptions=True)


def decode_structured_output(raw: str) -> Any:
    def object_pairs(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in pairs:
            if key in result:
                raise ValueError("structured output contains duplicate keys")
            result[key] = value
        return result

    def finite_float(value: str) -> float:
        number = float(value)
        if not math.isfinite(number):
            raise ValueError("structured output contains a non-finite number")
        return number

    def invalid_constant(_: str) -> Any:
        raise ValueError("structured output contains a non-JSON constant")

    return json.loads(
        raw,
        object_pairs_hook=object_pairs,
        parse_float=finite_float,
        parse_constant=invalid_constant,
    )


def _prompt_with_guidance(guidance: str, prompt: str, *, has_images: bool = False) -> str:
    executor = "structured vision" if has_images else "structured-text"
    allowed_input = " and attached image inputs" if has_images else ""
    return (
        f"You are a restricted {executor} executor. No tools are authorized or available. "
        "Use only the immutable task input, explicit project guidance, output schema supplied by "
        f"the harness{allowed_input}. Never read files, run commands, call networks, or perform "
        "side effects."
        f"\n\n# Project guidance\n{guidance}\n\n# Frozen stage input\n{prompt}"
    )


def _stage_image_inputs(root: Path, image_inputs: Sequence[CodexImageInput]) -> tuple[Path, ...]:
    keys = [value.sort_key for value in image_inputs]
    if keys != sorted(set(keys)) or any(not value or value != value.strip() for value in keys):
        raise CodexMediaInvalid("Codex image inputs must have canonical sorted keys")

    staged: list[Path] = []
    for index, image in enumerate(image_inputs):
        extension = _IMAGE_EXTENSIONS.get(image.media_type)
        if (
            extension is None
            or image.byte_length < 1
            or len(image.content_hash) != 64
            or any(character not in "0123456789abcdef" for character in image.content_hash)
            or not image.path.is_absolute()
        ):
            raise CodexMediaInvalid("Codex image declaration is invalid")
        destination = root / f"input-{index:03d}{extension}"
        try:
            flags = os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW
            with os.fdopen(os.open(image.path, flags), "rb") as source:
                info = os.fstat(source.fileno())
                if not stat.S_ISREG(info.st_mode) or info.st_size != image.byte_length:
                    raise CodexMediaInvalid("Codex image file identity drifted")
                digest = hashlib.sha256()
                prefix = b""
                with destination.open("xb") as output:
                    while chunk := source.read(65536):
                        if len(prefix) < 12:
                            prefix = (prefix + chunk)[:12]
                        digest.update(chunk)
                        output.write(chunk)
            if not _matches_media_type(prefix, image.media_type):
                raise CodexMediaInvalid("Codex image media type does not match its bytes")
            if digest.hexdigest() != image.content_hash:
                raise CodexMediaInvalid("Codex image content hash drifted")
            destination.chmod(0o400)
        except CodexMediaInvalid:
            raise
        except OSError as error:
            raise CodexMediaInvalid("Codex image input is unavailable") from error
        staged.append(destination)
    return tuple(staged)


def _matches_media_type(prefix: bytes, media_type: str) -> bool:
    if media_type == "image/jpeg":
        return prefix.startswith(b"\xff\xd8\xff")
    if media_type == "image/png":
        return prefix.startswith(b"\x89PNG\r\n\x1a\n")
    if media_type == "image/webp":
        return prefix.startswith(b"RIFF") and prefix[8:12] == b"WEBP"
    return False


def unauthorized_item_type(stdout: bytes) -> str | None:
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
