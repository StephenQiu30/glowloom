from __future__ import annotations

import asyncio
import os
import sys
import time
from pathlib import Path
from typing import Any

import pytest
from pydantic import BaseModel

from app.reasoning.codex import (
    CodexBudgetExceeded,
    codex_output_schema,
    run_codex_process,
)
from app.text_contract.schemas import EpisodeAnalysis


class Output(BaseModel):
    value: str


class PendingProcess:
    returncode: int | None = None
    killed = False
    waited = False

    async def communicate(self, _: bytes) -> tuple[bytes, bytes]:
        await asyncio.Event().wait()
        return b"", b""

    def kill(self) -> None:
        self.killed = True
        self.returncode = -9

    async def wait(self) -> int:
        self.waited = True
        return -9


async def test_cancel_terminates_and_waits_without_inheriting_platform_secrets(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    process = PendingProcess()
    started = asyncio.Event()
    captured: dict[str, Any] = {}

    async def create(*_: str, **kwargs: Any) -> PendingProcess:
        captured.update(kwargs)
        started.set()
        return process

    monkeypatch.setenv("CREATION_DATABASE_URL", "test-only-do-not-inherit")
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", "test-only-do-not-inherit")
    monkeypatch.setattr(asyncio, "create_subprocess_exec", create)
    task = asyncio.create_task(
        run_codex_process(
            codex_bin="codex", guidance="test", prompt="{}", output_model=Output, timeout_seconds=30
        )
    )
    await started.wait()
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task
    assert process.killed and process.waited
    assert "CREATION_DATABASE_URL" not in captured["env"]
    assert "AGENT_EXECUTION_SECRET" not in captured["env"]


def test_strict_model_schema_requires_explicit_optional_evidence_position() -> None:
    schema = codex_output_schema(EpisodeAnalysis)
    evidence = schema["$defs"]["Evidence"]
    assert set(evidence["required"]) == set(evidence["properties"])
    assert "occurrence" in evidence["required"]
    assert "default" not in evidence["properties"]["occurrence"]


async def test_output_overflow_kills_real_child_and_waits(tmp_path: Path) -> None:
    program = tmp_path / "synthetic-codex"
    pid_path = tmp_path / "child.pid"
    program.write_text(
        f"#!{sys.executable}\n"
        "import os, pathlib, sys, time\n"
        f"pathlib.Path({str(pid_path)!r}).write_text(str(os.getpid()))\n"
        "if os.fork() == 0:\n"
        "    time.sleep(30)\n"
        "    os._exit(0)\n"
        "sys.stdin.buffer.read()\n"
        "sys.stdout.buffer.write(b'x' * 200000)\n"
        "sys.stdout.flush()\n"
        "time.sleep(30)\n"
    )
    program.chmod(0o700)
    started = time.monotonic()
    with pytest.raises(CodexBudgetExceeded):
        await run_codex_process(
            codex_bin=str(program),
            guidance="test",
            prompt="{}",
            output_model=Output,
            timeout_seconds=5,
            max_output_bytes=1024,
        )
    assert time.monotonic() - started < 5
    with pytest.raises(ProcessLookupError):
        os.kill(int(pid_path.read_text()), 0)
