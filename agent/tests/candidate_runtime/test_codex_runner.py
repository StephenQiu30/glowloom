from __future__ import annotations

import asyncio
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import BaseModel, ConfigDict

from app.candidate_runtime.schemas import execution_policy_for
from app.modules.scripts.production_bibles.contracts import ProductionBibleCandidate
from app.modules.skills.harness import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexRuntimeUnavailable,
    CodexSchemaInvalid,
    CodexSchemaRunner,
    CodexToolPolicyViolation,
    structured_diagnostic,
)
from app.modules.storyboards.contracts import StoryboardCandidate


class ProbeResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    value: str


class FakeProcess:
    def __init__(
        self,
        command: tuple[str, ...],
        *,
        stdout: bytes = b'{"type":"turn.completed"}\n',
        response: str = '{"value":"ok"}',
        returncode: int = 0,
    ) -> None:
        self.command = command
        self.stdout = stdout
        self.response = response
        self.returncode = returncode
        self.killed = False

    async def communicate(self, _: bytes) -> tuple[bytes, bytes]:
        output_path = Path(self.command[self.command.index("--output-last-message") + 1])
        await asyncio.to_thread(output_path.write_text, self.response, encoding="utf-8")
        return self.stdout, b""

    def kill(self) -> None:
        self.killed = True

    async def wait(self) -> int:
        return self.returncode


def _repository(tmp_path: Path) -> Path:
    root = tmp_path / "repository"
    skill = root / "agent" / "skills" / "draft-shots"
    skill.mkdir(parents=True)
    (skill / "SKILL.md").write_text("# Draft Shots\nReturn schema-valid JSON.", encoding="utf-8")
    return root


def _assert_strict_objects(value: Any) -> None:
    if isinstance(value, dict):
        mapping = cast(dict[str, Any], value)
        if mapping.get("type") == "object":
            properties = cast(dict[str, Any], mapping.get("properties", {}))
            required = cast(list[str], mapping.get("required", []))
            assert mapping.get("additionalProperties") is False
            assert set(required) == set(properties)
        for child in mapping.values():
            _assert_strict_objects(child)
    elif isinstance(value, list):
        for child in cast(list[Any], value):
            _assert_strict_objects(child)


def test_structured_diagnostic_prefers_codex_error_event() -> None:
    stdout = b'{"type":"thread.started","thread_id":"test"}\n'
    stdout += b'{"type":"error","message":"model request failed"}\n'

    result = structured_diagnostic(stdout, b"details {\n}\n")

    assert result == "model request failed"


def test_structured_diagnostic_falls_back_without_exposing_multiline_payload() -> None:
    result = structured_diagnostic(b"", b"request failed\ninternal diagnostic\n}\n")

    assert result == "internal diagnostic"


def test_candidate_output_schemas_are_strict_structured_outputs() -> None:
    _assert_strict_objects(ProductionBibleCandidate.model_json_schema())
    _assert_strict_objects(StoryboardCandidate.model_json_schema())


@pytest.mark.asyncio
async def test_runner_isolates_codex_and_enforces_model_call_budget(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    commands: list[tuple[str, ...]] = []

    async def create_process(*command: str, **_: object) -> FakeProcess:
        commands.append(command)
        return FakeProcess(command)

    monkeypatch.setattr("asyncio.create_subprocess_exec", create_process)
    runner = CodexSchemaRunner(
        repository_root=_repository(tmp_path),
        execution_policy=execution_policy_for("storyboard_draft"),
    )

    assert await runner.run("Draft one shot", ProbeResult, skill_name="draft-shots") == ProbeResult(
        value="ok"
    )
    with pytest.raises(CodexBudgetExceeded):
        await runner.run("Draft another shot", ProbeResult, skill_name="draft-shots")

    command = commands[0]
    working_root = Path(command[command.index("--cd") + 1])
    assert working_root != tmp_path / "repository"
    assert "--ignore-user-config" in command
    assert "--skip-git-repo-check" in command
    for feature in ("shell_tool", "unified_exec", "apps", "plugins", "browser_use"):
        assert ("--disable", feature) in zip(command, command[1:], strict=False)


@pytest.mark.asyncio
async def test_runner_rejects_any_codex_tool_event(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    async def create_process(*command: str, **_: object) -> FakeProcess:
        event = b'{"type":"item.completed","item":{"type":"command_execution"}}\n'
        return FakeProcess(command, stdout=event)

    monkeypatch.setattr("asyncio.create_subprocess_exec", create_process)
    runner = CodexSchemaRunner(
        repository_root=_repository(tmp_path),
        execution_policy=execution_policy_for("storyboard_draft"),
    )

    with pytest.raises(CodexToolPolicyViolation):
        await runner.run("Draft one shot", ProbeResult, skill_name="draft-shots")


@pytest.mark.asyncio
async def test_runner_kills_codex_when_invocation_deadline_is_exhausted(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    process: FakeProcess | None = None

    async def create_process(*command: str, **_: object) -> FakeProcess:
        nonlocal process
        process = FakeProcess(command)

        async def hang(_: bytes) -> tuple[bytes, bytes]:
            await asyncio.Event().wait()
            return b"", b""

        process.communicate = hang  # type: ignore[method-assign]
        return process

    monkeypatch.setattr("asyncio.create_subprocess_exec", create_process)
    policy = execution_policy_for("storyboard_draft").model_copy(
        update={"max_execution_seconds": 1}
    )
    runner = CodexSchemaRunner(
        repository_root=_repository(tmp_path),
        execution_policy=policy,
    )

    with pytest.raises(CodexDeadlineExceeded):
        await runner.run("Draft one shot", ProbeResult, skill_name="draft-shots")

    assert process is not None
    assert process.killed is True


@pytest.mark.asyncio
async def test_runner_does_not_misclassify_codex_error_item_as_a_tool(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    async def create_process(*command: str, **_: object) -> FakeProcess:
        event = b'{"type":"item.completed","item":{"type":"error"}}\n'
        return FakeProcess(command, stdout=event)

    monkeypatch.setattr("asyncio.create_subprocess_exec", create_process)
    runner = CodexSchemaRunner(
        repository_root=_repository(tmp_path),
        execution_policy=execution_policy_for("storyboard_draft"),
    )

    result = await runner.run("Draft one shot", ProbeResult, skill_name="draft-shots")

    assert result.value == "ok"


@pytest.mark.asyncio
async def test_runner_distinguishes_schema_and_runtime_failures(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    repository = _repository(tmp_path)

    async def invalid_schema(*command: str, **_: object) -> FakeProcess:
        return FakeProcess(command, response='{"unknown":"value"}')

    monkeypatch.setattr("asyncio.create_subprocess_exec", invalid_schema)
    runner = CodexSchemaRunner(
        repository_root=repository,
        execution_policy=execution_policy_for("storyboard_draft"),
    )
    with pytest.raises(CodexSchemaInvalid):
        await runner.run("Draft one shot", ProbeResult, skill_name="draft-shots")

    async def unavailable(*_: str, **__: object) -> FakeProcess:
        raise OSError("codex missing")

    monkeypatch.setattr("asyncio.create_subprocess_exec", unavailable)
    runner = CodexSchemaRunner(
        repository_root=repository,
        execution_policy=execution_policy_for("storyboard_draft"),
    )
    with pytest.raises(CodexRuntimeUnavailable):
        await runner.run("Draft one shot", ProbeResult, skill_name="draft-shots")


@pytest.mark.asyncio
async def test_runner_does_not_fall_back_to_legacy_skill_path(tmp_path: Path) -> None:
    repository = tmp_path / "repository"
    legacy_skill = repository / ".agents" / "skills" / "draft-shots"
    legacy_skill.mkdir(parents=True)
    (legacy_skill / "SKILL.md").write_text("# Legacy", encoding="utf-8")
    runner = CodexSchemaRunner(
        repository_root=repository,
        execution_policy=execution_policy_for("storyboard_draft"),
    )

    with pytest.raises(CodexRuntimeUnavailable):
        await runner.run("Draft one shot", ProbeResult, skill_name="draft-shots")
