"""Temporal JSON contracts for the mock provider."""

from typing import Any, Literal

from pydantic import BaseModel, Field, ValidationError
from temporalio import activity
from temporalio.client import Client
from temporalio.exceptions import ApplicationError
from temporalio.worker import Worker

from app.providers.mock import MockProvider, SubmitRequest, TaskRef


class ProviderSubmitInput(BaseModel):
    operation_id: str = Field(min_length=1)
    provider_request_key: str = Field(min_length=1)
    adapter_key: Literal["mock"]
    provider_model_id: str = Field(min_length=1)
    capability: str = Field(min_length=1)
    mode: str = Field(min_length=1)
    params: dict[str, object] = Field(default_factory=dict)
    inputs: list[dict[str, object]] = Field(default_factory=list)
    output_count: int = Field(default=1, ge=1)


class ProviderQueryInput(BaseModel):
    provider_request_key: str | None = None
    provider_task_id: str | None = None


class ProviderCancelInput(BaseModel):
    provider_task_id: str = Field(min_length=1)


def _validate_input[InputModel: BaseModel](
    model: type[InputModel], payload: dict[str, Any]
) -> InputModel:
    try:
        return model.model_validate(payload)
    except ValidationError:
        raise ApplicationError(
            "invalid provider activity input",
            type="invalid_activity_input",
            non_retryable=True,
        ) from None


class MockProviderActivities:
    def __init__(self, provider: MockProvider) -> None:
        self._provider = provider

    @activity.defn(name="provider.submit")
    async def submit(self, payload: dict[str, Any]) -> dict[str, Any]:
        data = _validate_input(ProviderSubmitInput, payload)
        result = await self._provider.submit(
            SubmitRequest(
                operation_id=data.operation_id,
                request_key=data.provider_request_key,
                provider_model_id=data.provider_model_id,
                capability=data.capability,
                mode=data.mode,
                params=data.params,
                inputs=data.inputs,
                output_count=data.output_count,
            )
        )
        error = (
            {
                "code": result.error_code,
                "retryable": False,
                "message": "mock provider did not return an accepted task",
            }
            if result.error_code
            else None
        )
        return {
            "outcome": result.outcome,
            "provider_task_id": result.provider_task_id,
            "error": error,
        }

    @activity.defn(name="provider.query")
    async def query(self, payload: dict[str, Any]) -> dict[str, Any]:
        data = _validate_input(ProviderQueryInput, payload)
        try:
            ref = TaskRef(
                request_key=data.provider_request_key,
                provider_task_id=data.provider_task_id,
            )
        except ValidationError:
            raise ApplicationError(
                "provide exactly one provider task reference",
                type="invalid_activity_input",
                non_retryable=True,
            ) from None
        result = await self._provider.query(ref)
        return {
            "state": result.state,
            "result_urls": result.result_urls,
            "usage": None,
            "error": (
                {"code": result.error_code, "retryable": False, "message": "mock task failed"}
                if result.error_code
                else None
            ),
        }

    @activity.defn(name="provider.cancel")
    async def cancel(self, payload: dict[str, Any]) -> dict[str, Any]:
        data = _validate_input(ProviderCancelInput, payload)
        result = await self._provider.cancel(TaskRef(provider_task_id=data.provider_task_id))
        return {"outcome": "cancelled" if result.cancelled else "not_cancelled"}


def create_mock_worker(
    client: Client, provider: MockProvider, task_queue: str = "agent.mock"
) -> Worker:
    activities = MockProviderActivities(provider)
    return Worker(
        client,
        task_queue=task_queue,
        workflows=[],
        activities=[activities.submit, activities.query, activities.cancel],
    )
