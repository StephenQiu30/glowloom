from __future__ import annotations

import json
from pathlib import Path
from typing import Any, cast

import pytest
from anyio import Path as AsyncPath

from app.candidate_runtime.schemas import StoryGraphStageInvocation
from app.modules.storygraph.bundle import StoryGraphBundle
from app.modules.storygraph.candidate_schemas import SourceEvidenceCandidate
from app.modules.storygraph.harness import (
    CodexToolPolicyViolation,
    SkillBundleUnavailable,
    StoryGraphHarness,
    unauthorized_item_type,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-stage-wire-v1.json"


def invocation() -> StoryGraphStageInvocation:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    return StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])


def candidate() -> dict[str, Any]:
    return {
        "observations": [
            {
                "observation_key": "observation-1",
                "kind": "entity",
                "proposed_key": "character:lin-yi",
                "label": "林一",
                "facts": ["林一发出开始指令"],
                "evidence": [
                    {
                        "source_start": 0,
                        "source_end": 2,
                        "text_hash": "a" * 64,
                        "exact_anchor": "林一",
                        "episode_number": None,
                    }
                ],
                "ambiguities": [],
            }
        ],
        "review_issues": [],
    }


class FakeProcess:
    def __init__(
        self, command: tuple[str, ...], output: dict[str, Any], stdout: bytes = b""
    ) -> None:
        self.returncode = 0
        self.command = command
        self.output = output
        self.stdout = stdout

    async def communicate(self, prompt: bytes) -> tuple[bytes, bytes]:
        assert b"references/source-evidence.md" in prompt
        response_index = self.command.index("--output-last-message") + 1
        await AsyncPath(self.command[response_index]).write_text(
            json.dumps(self.output, ensure_ascii=False), encoding="utf-8"
        )
        return self.stdout, b""

    def kill(self) -> None:
        self.returncode = -9

    async def wait(self) -> int:
        return self.returncode


@pytest.mark.asyncio
async def test_harness_uses_ephemeral_read_only_empty_directory_and_declared_guidance(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    seen: dict[str, tuple[str, ...]] = {}

    async def create_process(*command: str, **_: Any) -> FakeProcess:
        seen["command"] = command
        return FakeProcess(command, candidate())

    monkeypatch.setattr("asyncio.create_subprocess_exec", create_process)
    harness = StoryGraphHarness(invocation(), repository_root=REPOSITORY_ROOT)
    result = await harness.execute()
    assert isinstance(result, SourceEvidenceCandidate)
    command = seen["command"]
    assert "--ephemeral" in command
    assert command[command.index("--sandbox") + 1] == "read-only"
    assert "--ignore-user-config" in command
    working_directory = Path(command[command.index("--cd") + 1])
    assert not await AsyncPath(working_directory).exists()


def test_harness_rejects_an_unroutable_bundle_hash_and_tool_event() -> None:
    request = invocation()
    changed_policy = request.execution_policy.model_copy(update={"skill_bundle_hash": "b" * 64})
    changed = request.model_copy(update={"execution_policy": changed_policy})
    with pytest.raises(SkillBundleUnavailable):
        StoryGraphHarness(changed, repository_root=REPOSITORY_ROOT)

    stdout = b'{"item":{"type":"command_execution"}}\n'
    assert unauthorized_item_type(stdout) == "command_execution"
    assert issubclass(CodexToolPolicyViolation, RuntimeError)


def test_bundle_only_loads_the_current_stage_files() -> None:
    guidance = StoryGraphBundle(REPOSITORY_ROOT).guidance("extract_source_evidence")
    assert "references/source-evidence.md" in guidance
    assert "references/story-analysis.md" not in guidance
    assert "references/shot-detail.md" not in guidance
