from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest
from anyio import Path as AsyncPath

from app.candidate_runtime.scene_analysis_schemas import SceneAnalysisInvocation
from app.modules.storygraph.scene_analysis_candidates import ScriptSpanCandidate
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-scene-analysis-wire.json"


def fixture() -> dict[str, Any]:
    return json.loads(WIRE_FIXTURE.read_text(encoding="utf-8"))


class FakeProcess:
    def __init__(self, command: tuple[str, ...], output: dict[str, Any]) -> None:
        self.command = command
        self.output = output
        self.returncode = 0

    async def communicate(self, prompt: bytes) -> tuple[bytes, bytes]:
        assert b"references/script-spans.md" in prompt
        assert b"references/scene-facts.md" not in prompt
        response_index = self.command.index("--output-last-message") + 1
        await AsyncPath(self.command[response_index]).write_text(
            json.dumps(self.output, ensure_ascii=False), encoding="utf-8"
        )
        return b"", b""

    def kill(self) -> None:
        self.returncode = -9

    async def wait(self) -> int:
        return self.returncode


@pytest.mark.asyncio
async def test_scene_analysis_harness_runs_in_read_only_isolation_and_validates_the_frozen_source(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    data = fixture()
    request = SceneAnalysisInvocation.model_validate(data["valid_invocation"])
    seen: dict[str, tuple[str, ...]] = {}

    async def create_process(*command: str, **_: Any) -> FakeProcess:
        seen["command"] = command
        return FakeProcess(command, data["valid_script_span_candidate"])

    monkeypatch.setattr("asyncio.create_subprocess_exec", create_process)
    harness = SceneAnalysisHarness(request, repository_root=REPOSITORY_ROOT)
    candidate = await harness.execute()
    assert isinstance(candidate, ScriptSpanCandidate)
    command = seen["command"]
    assert command[command.index("--sandbox") + 1] == "read-only"
    assert "--ignore-user-config" in command
    assert not await AsyncPath(command[command.index("--cd") + 1]).exists()
