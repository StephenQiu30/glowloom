import asyncio
from collections.abc import Sequence
from typing import Any

import pytest
from langgraph.checkpoint.memory import InMemorySaver
from pydantic import BaseModel

from app.modules.skills import (
    SkillDefinition,
    SkillExecutionContext,
    SkillExecutionError,
    SkillHarness,
)


class _Output(BaseModel):
    value: str


class _Model:
    def __init__(self, result: Any, *, delay: float = 0) -> None:
        self.result = result
        self.delay = delay
        self.messages: Sequence[Any] = []

    async def ainvoke(self, messages: Sequence[Any]) -> object:
        self.messages = messages
        if self.delay:
            await asyncio.sleep(self.delay)
        return self.result


def _skill(*, max_input_chars: int = 100) -> SkillDefinition:
    return SkillDefinition(
        name="test.skill",
        version="v1",
        max_input_chars=max_input_chars,
    )


@pytest.mark.asyncio
async def test_harness_validates_output_and_records_input_snapshot() -> None:
    model = _Model({"value": "ok"})
    run = await SkillHarness().run(
        skill=_skill(),
        model=model,
        system_prompt="system",
        user_payload="payload",
        output_model=_Output,
        context=SkillExecutionContext(
            skill_name="test.skill",
            skill_version="v1",
            trace_id="trace-1",
        ),
    )

    assert run.output == _Output(value="ok")
    assert run.skill_name == "test.skill"
    assert run.skill_version == "v1"
    assert run.trace_id == "trace-1"
    assert len(run.input_hash) == 64
    assert model.messages[0].content == "system"
    assert model.messages[1].content == "payload"


@pytest.mark.asyncio
async def test_harness_accepts_a_langgraph_checkpointer_for_resumable_runs() -> None:
    run = await SkillHarness(checkpointer=InMemorySaver()).run(
        skill=_skill(),
        model=_Model({"value": "checkpointed"}),
        system_prompt="system",
        user_payload="payload",
        output_model=_Output,
        context=SkillExecutionContext(
            skill_name="test.skill",
            skill_version="v1",
            task_id="task-1",
        ),
    )

    assert run.output == _Output(value="checkpointed")


@pytest.mark.asyncio
async def test_harness_rejects_invalid_structured_output() -> None:
    with pytest.raises(SkillExecutionError, match="invalid structured result") as error:
        await SkillHarness().run(
            skill=_skill(),
            model=_Model({"unexpected": True}),
            system_prompt="system",
            user_payload="payload",
            output_model=_Output,
        )

    assert error.value.code == "skill_output_invalid"
    assert error.value.outcome == "failed"


@pytest.mark.asyncio
async def test_harness_does_not_impose_a_wall_clock_timeout() -> None:
    run = await SkillHarness().run(
        skill=_skill(),
        model=_Model({"value": "complete"}, delay=0.01),
        system_prompt="system",
        user_payload="payload",
        output_model=_Output,
    )

    assert run.output == _Output(value="complete")


def test_harness_denies_tools_outside_skill_allowlist() -> None:
    with pytest.raises(SkillExecutionError) as error:
        SkillHarness().require_tools(_skill(), ["database.write"])

    assert error.value.code == "skill_tool_denied"
