"""Temporal Activity boundary for versioned Harness runs."""

from __future__ import annotations

from datetime import datetime
from typing import TYPE_CHECKING, Any

from pydantic import BaseModel, Field, ValidationError
from temporalio import activity
from temporalio.client import Client
from temporalio.exceptions import ApplicationError
from temporalio.worker import Worker

from app.harness.budget import Budget, BudgetExceeded
from app.harness.loop import (
    Harness,
    HarnessConfigurationError,
    InputValidationFailed,
    ValidationExhausted,
)
from app.harness.router import ContextTooLarge, ModelRouter, ModelUnavailable
from app.harness.skills import SkillRegistry, SkillVersionUnavailable
from app.moderation.mock import MockModerationActivities

if TYPE_CHECKING:
    from app.worker.credential_activities import CredentialTestActivities


class BudgetInput(BaseModel):
    max_tokens: int = Field(gt=0)
    max_cost_micros: int = Field(ge=0)
    deadline: datetime


class RunSkillInput(BaseModel):
    skill_key: str = Field(min_length=1)
    skill_version: str = Field(min_length=1)
    model_key: str = Field(min_length=1)
    inputs: dict[str, Any]
    budget: BudgetInput


class SkillActivities:
    def __init__(self, registry: SkillRegistry, router: ModelRouter) -> None:
        self._harness = Harness(registry, router, heartbeat=activity.heartbeat)

    @activity.defn(name="llm.run_skill")
    async def run(self, payload: dict[str, Any]) -> dict[str, Any]:
        try:
            data = RunSkillInput.model_validate(payload)
            budget = Budget(
                max_tokens=data.budget.max_tokens,
                max_cost_micros=data.budget.max_cost_micros,
                deadline=data.budget.deadline,
            )
        except (ValidationError, ValueError):
            raise ApplicationError(
                "invalid skill activity input",
                type="invalid_activity_input",
                non_retryable=True,
            ) from None
        try:
            result = await self._harness.run(
                skill_key=data.skill_key,
                skill_version=data.skill_version,
                model_key=data.model_key,
                inputs=data.inputs,
                budget=budget,
            )
        except SkillVersionUnavailable:
            raise ApplicationError(
                "skill version unavailable", type="skill_version_unavailable", non_retryable=True
            ) from None
        except InputValidationFailed:
            raise ApplicationError(
                "skill input invalid", type="skill_input_invalid", non_retryable=True
            ) from None
        except HarnessConfigurationError:
            raise ApplicationError(
                "skill configuration unavailable",
                type="skill_configuration_unavailable",
                non_retryable=True,
            ) from None
        except ModelUnavailable:
            raise ApplicationError(
                "model unavailable", type="model_unavailable", non_retryable=True
            ) from None
        except ContextTooLarge:
            raise ApplicationError(
                "context too large", type="context_too_large", non_retryable=True
            ) from None
        except BudgetExceeded as exc:
            raise ApplicationError(
                "skill budget exceeded",
                {
                    "reason": exc.reason,
                    "input_tokens": exc.usage.input_tokens,
                    "output_tokens": exc.usage.output_tokens,
                    "cost_micros": exc.usage.cost_micros,
                },
                type="budget_exceeded",
                non_retryable=True,
            ) from None
        except ValidationExhausted as exc:
            raise ApplicationError(
                "skill validation failed",
                exc.trace,
                type="skill_validation_failed",
                non_retryable=True,
            ) from None
        return {
            "result": result.output,
            "trace_steps": result.trace["steps"],
            "trace": result.trace,
            "usage": {
                "input_tokens": result.usage.input_tokens,
                "output_tokens": result.usage.output_tokens,
            },
            "cost_micros": result.usage.cost_micros,
        }


def create_skill_worker(
    client: Client,
    registry: SkillRegistry,
    router: ModelRouter,
    task_queue: str = "agent",
    credential_tests: CredentialTestActivities | None = None,
) -> Worker:
    activities = SkillActivities(registry, router)
    moderation = MockModerationActivities()
    handlers = [activities.run, moderation.check]
    if credential_tests is not None:
        handlers.append(credential_tests.test_credential)
    return Worker(client, task_queue=task_queue, workflows=[], activities=handlers)
