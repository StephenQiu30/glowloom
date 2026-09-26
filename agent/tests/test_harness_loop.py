import asyncio
from datetime import UTC, datetime, timedelta
from typing import Any

import pytest

from app.harness.budget import Budget, BudgetExceeded, Usage
from app.harness.loop import Harness, InputValidationFailed, ValidationExhausted
from app.harness.router import ModelResponse, ModelRouter, Price
from app.harness.skills import SkillDefinition, SkillMetadata, SkillRegistry


class SequencedClient:
    def __init__(self, outputs: list[dict[str, Any]]) -> None:
        self.outputs = outputs
        self.calls = 0

    async def complete(
        self,
        model_key: str,
        messages: list[dict[str, str]],
        output_schema: dict[str, Any],
        max_output_tokens: int,
    ) -> ModelResponse:
        output = self.outputs[self.calls]
        self.calls += 1
        return ModelResponse(output, input_tokens=10, output_tokens=5, latency_ms=1)


def registry(max_repair_rounds: int = 2) -> SkillRegistry:
    schema: dict[str, Any] = {
        "type": "object",
        "required": ["value"],
        "properties": {"value": {"type": "string"}},
    }
    skill = SkillDefinition(
        metadata=SkillMetadata(
            key="mock.echo",
            version="1.0.0",
            description="test",
            default_model="mock.structured",
            max_input_tokens=1000,
            max_output_tokens=40,
            max_repair_rounds=max_repair_rounds,
            timeout_s=30,
            tools=[],
            validators=["schema"],
        ),
        content_hash="a" * 64,
        instruction="Only output JSON with the requested value.",
        input_schema=schema,
        output_schema=schema,
    )
    return SkillRegistry({("mock.echo", "1.0.0"): skill})


def budget() -> Budget:
    return Budget(
        max_tokens=1000,
        max_cost_micros=1000,
        deadline=datetime.now(UTC) + timedelta(minutes=1),
    )


def test_harness_repairs_once_and_charges_each_model_call() -> None:
    async def run() -> None:
        client = SequencedClient([{"invalid": "private output"}, {"value": "done"}])
        router = ModelRouter(client, {"mock.structured": Price(1, 1)})
        heartbeats: list[int] = []
        harness = Harness(registry(), router, heartbeat=lambda: heartbeats.append(1))
        result = await harness.run(
            skill_key="mock.echo",
            skill_version="1.0.0",
            model_key="mock.structured",
            inputs={"value": "original private source"},
            budget=budget(),
        )

        assert result.output == {"value": "done"}
        assert result.usage == Usage(input_tokens=20, output_tokens=10, cost_micros=30)
        assert client.calls == 2 and len(heartbeats) == 2
        assert len(result.trace["steps"]) == 2
        assert result.trace["steps"][0]["validation_errors"][0]["code"] == "schema"
        assert "original private source" not in str(result.trace)
        assert "private output" not in str(result.trace)

    asyncio.run(run())


def test_harness_stops_after_bounded_repair_and_preserves_raw_output() -> None:
    async def run() -> None:
        client = SequencedClient([{"invalid": "a"}] * 3)
        harness = Harness(registry(), ModelRouter(client, {"mock.structured": Price(0, 0)}))
        with pytest.raises(ValidationExhausted) as exc:
            await harness.run(
                skill_key="mock.echo",
                skill_version="1.0.0",
                model_key="mock.structured",
                inputs={"value": "source"},
                budget=budget(),
            )
        assert client.calls == 3
        assert exc.value.raw_output == {"invalid": "a"}
        assert len(exc.value.trace["steps"]) == 3
        assert exc.value.trace["usage"] == {"input_tokens": 30, "output_tokens": 15}

    asyncio.run(run())


def test_harness_rejects_invalid_input_and_budget_before_model_call() -> None:
    async def run() -> None:
        client = SequencedClient([{"value": "done"}])
        router = ModelRouter(client, {"mock.structured": Price(1, 1)})
        harness = Harness(registry(), router)
        with pytest.raises(InputValidationFailed):
            await harness.run(
                skill_key="mock.echo",
                skill_version="1.0.0",
                model_key="mock.structured",
                inputs={"value": 123},
                budget=budget(),
            )
        with pytest.raises(BudgetExceeded):
            await harness.run(
                skill_key="mock.echo",
                skill_version="1.0.0",
                model_key="mock.structured",
                inputs={"value": "source"},
                budget=Budget(
                    max_tokens=1000,
                    max_cost_micros=1,
                    deadline=datetime.now(UTC) + timedelta(minutes=1),
                ),
            )
        assert client.calls == 0

    asyncio.run(run())
