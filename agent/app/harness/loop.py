"""Bounded Skill execution with schema repair and redacted trace data."""

import json
from collections.abc import Callable
from dataclasses import dataclass
from hashlib import sha256
from typing import Any

from jsonschema import Draft202012Validator

from app.harness.budget import Budget, Usage
from app.harness.router import ModelRouter
from app.harness.skills import SkillRegistry


class InputValidationFailed(Exception):
    pass


class HarnessConfigurationError(Exception):
    pass


class ValidationExhausted(Exception):
    def __init__(self, raw_output: dict[str, Any], trace: dict[str, Any]) -> None:
        super().__init__("skill_validation_failed")
        self.raw_output = raw_output
        self.trace = trace


@dataclass(frozen=True)
class HarnessResult:
    output: dict[str, Any]
    usage: Usage
    trace: dict[str, Any]


def _canonical_json(value: object) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def _digest(value: object) -> str:
    return sha256(_canonical_json(value).encode("utf-8")).hexdigest()


class Harness:
    def __init__(
        self,
        registry: SkillRegistry,
        router: ModelRouter,
        *,
        heartbeat: Callable[[], None] = lambda: None,
    ) -> None:
        self._registry = registry
        self._router = router
        self._heartbeat = heartbeat

    async def run(
        self,
        *,
        skill_key: str,
        skill_version: str,
        model_key: str,
        inputs: dict[str, Any],
        budget: Budget,
    ) -> HarnessResult:
        skill = self._registry.get(skill_key, skill_version)
        if skill.metadata.tools or set(skill.metadata.validators) != {"schema"}:
            raise HarnessConfigurationError("skill requires unavailable tools or validators")
        input_errors = list(Draft202012Validator(skill.input_schema).iter_errors(inputs))
        if input_errors:
            raise InputValidationFailed("skill input does not match schema")

        messages = [
            {
                "role": "system",
                "content": (
                    skill.instruction
                    + "\nOutput JSON schema: "
                    + _canonical_json(skill.output_schema)
                ),
            },
            {"role": "user", "content": _canonical_json(inputs)},
        ]
        trace: dict[str, Any] = {
            "skill": f"{skill_key}@{skill_version}#{skill.content_hash}",
            "model": model_key,
            "input_hash": _digest(inputs),
            "steps": [],
        }
        validator = Draft202012Validator(skill.output_schema)
        for round_number in range(skill.metadata.max_repair_rounds + 1):
            self._heartbeat()
            response, usage = await self._router.complete(
                call_id=f"{skill_key}@{skill_version}:{round_number}",
                model_key=model_key,
                messages=messages,
                output_schema=skill.output_schema,
                max_input_tokens=skill.metadata.max_input_tokens,
                max_output_tokens=skill.metadata.max_output_tokens,
                budget=budget,
            )
            errors = [
                {"code": "schema", "path": ".".join(map(str, error.path))}
                for error in validator.iter_errors(response.output)
            ]
            trace["steps"].append(
                {
                    "round": round_number,
                    "messages_digest": _digest(messages),
                    "output_digest": _digest(response.output),
                    "prompt_tokens": usage.input_tokens,
                    "completion_tokens": usage.output_tokens,
                    "latency_ms": response.latency_ms,
                    "validation_errors": errors,
                }
            )
            if not errors:
                trace["usage"] = {
                    "input_tokens": budget.usage.input_tokens,
                    "output_tokens": budget.usage.output_tokens,
                }
                trace["cost_micros"] = budget.usage.cost_micros
                return HarnessResult(response.output, budget.usage, trace)
            if round_number == skill.metadata.max_repair_rounds:
                trace["usage"] = {
                    "input_tokens": budget.usage.input_tokens,
                    "output_tokens": budget.usage.output_tokens,
                }
                trace["cost_micros"] = budget.usage.cost_micros
                raise ValidationExhausted(response.output, trace)
            messages.append({"role": "assistant", "content": _canonical_json(response.output)})
            messages.append(
                {
                    "role": "user",
                    "content": "Repair the JSON output to satisfy the schema errors: "
                    + _canonical_json(errors),
                }
            )
        raise AssertionError("unreachable repair round")
