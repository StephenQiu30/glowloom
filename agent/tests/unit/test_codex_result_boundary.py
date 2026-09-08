from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import Any

import pytest
from pydantic import BaseModel

from app.reasoning.codex import (
    CodexBudgetExceeded,
    CodexSchemaInvalid,
    run_codex_process,
)


class Output(BaseModel):
    value: Any


def executable(path: Path, body: str) -> str:
    path.write_text(f"#!{sys.executable}\n" + body)
    path.chmod(0o700)
    return str(path)


async def test_output_limit_interrupts_a_blocked_prompt_writer(tmp_path: Path) -> None:
    pid = tmp_path / "child.pid"
    program = executable(
        tmp_path / "synthetic-codex",
        "import os, pathlib, sys, time\n"
        f"pathlib.Path({str(pid)!r}).write_text(str(os.getpid()))\n"
        "sys.stdout.buffer.write(b'x' * 200000)\n"
        "sys.stdout.flush()\n"
        "time.sleep(30)\n",
    )
    with pytest.raises(CodexBudgetExceeded):
        await run_codex_process(
            codex_bin=program,
            guidance="synthetic",
            prompt="x" * 4_000_000,
            output_model=Output,
            timeout_seconds=2,
            max_output_bytes=1024,
        )
    with pytest.raises(ProcessLookupError):
        os.kill(int(pid.read_text()), 0)


@pytest.mark.parametrize(
    "raw",
    [
        '{"value":"first","value":"second"}',
        '{"value":{"a":1,"a":2}}',
        '{"value":NaN}',
        '{"value":Infinity}',
        '{"value":1e999}',
    ],
)
async def test_structured_output_rejects_ambiguous_json(tmp_path: Path, raw: str) -> None:
    program = executable(
        tmp_path / "synthetic-codex",
        "import pathlib, sys\n"
        "sys.stdin.buffer.read()\n"
        "target = pathlib.Path(sys.argv[sys.argv.index('--output-last-message') + 1])\n"
        f"target.write_text({raw!r})\n",
    )
    with pytest.raises(CodexSchemaInvalid):
        await run_codex_process(
            codex_bin=program,
            guidance="synthetic",
            prompt="{}",
            output_model=Output,
            timeout_seconds=5,
            max_output_bytes=1024,
        )


async def test_structured_output_rejects_symbolic_link(tmp_path: Path) -> None:
    fixture = tmp_path / "synthetic-output.json"
    fixture.write_text('{"value":"synthetic"}')
    program = executable(
        tmp_path / "synthetic-codex",
        "import pathlib, sys\n"
        "sys.stdin.buffer.read()\n"
        "target = pathlib.Path(sys.argv[sys.argv.index('--output-last-message') + 1])\n"
        f"target.symlink_to({str(fixture)!r})\n",
    )
    with pytest.raises(CodexSchemaInvalid):
        await run_codex_process(
            codex_bin=program,
            guidance="synthetic",
            prompt="{}",
            output_model=Output,
            timeout_seconds=5,
            max_output_bytes=1024,
        )
