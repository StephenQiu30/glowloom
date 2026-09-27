"""Temporal JSON contracts for the mock provider."""

from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, ValidationError, model_validator
from temporalio import activity
from temporalio.client import Client
from temporalio.exceptions import ApplicationError
from temporalio.worker import Worker

from app.providers.mock import MockProvider, SubmitRequest, TaskRef


class ProviderActivityModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class ProviderInput(ProviderActivityModel):
    role: str = Field(min_length=1)
    media_url: str | None = None
    media_type: str | None = None
    duration_ms: int | None = Field(default=None, ge=0)
    text: str | None = None


class ProviderCredential(ProviderActivityModel):
    id: str = Field(min_length=1)
    key_id: str = Field(min_length=1)
    ciphertext: str = Field(min_length=1)


class ProviderError(ProviderActivityModel):
    code: str = Field(min_length=1)
    retryable: bool
    message: str = Field(min_length=1)


class ProviderSubmitInput(ProviderActivityModel):
    operation_id: str = Field(min_length=1)
    provider_request_key: str = Field(min_length=1)
    adapter_key: str = Field(min_length=1)
    provider_model_id: str = Field(min_length=1)
    capability: str = Field(min_length=1)
    mode: str = Field(min_length=1)
    params: dict[str, object] = Field(default_factory=dict)
    inputs: list[ProviderInput] = Field(default_factory=list)
    output_count: int = Field(default=1, ge=1)
    credential: ProviderCredential | None = None


class ProviderSubmitOutput(ProviderActivityModel):
    outcome: Literal["accepted", "rejected", "not_submitted", "unknown"]
    provider_task_id: str | None = None
    error: ProviderError | None = None

    @model_validator(mode="after")
    def accepted_requires_task_id(self) -> "ProviderSubmitOutput":
        if self.outcome == "accepted" and not self.provider_task_id:
            raise ValueError("accepted submission requires provider_task_id")
        return self


class ProviderQueryInput(ProviderActivityModel):
    provider_request_key: str | None = None
    provider_task_id: str | None = None

    @model_validator(mode="after")
    def exactly_one_reference(self) -> "ProviderQueryInput":
        if bool(self.provider_request_key) == bool(self.provider_task_id):
            raise ValueError("provide exactly one provider task reference")
        return self


class ProviderQueryOutput(ProviderActivityModel):
    state: Literal["pending", "running", "succeeded", "failed", "not_found"]
    provider_task_id: str | None = None
    result_urls: list[str] = Field(default_factory=list)
    usage: dict[str, object] | None = None
    error: ProviderError | None = None


class ProviderCancelInput(ProviderActivityModel):
    provider_task_id: str = Field(min_length=1)


class ProviderCancelOutput(ProviderActivityModel):
    outcome: Literal["cancelled", "not_cancelled"]


def _validate_input[InputModel: BaseModel](
    model: type[InputModel], payload: dict[str, Any]
) -> InputModel:
    try:
        return model.model_validate(payload, strict=True)
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
        if data.adapter_key != self._provider.key:
            raise ApplicationError(
                "invalid provider activity input",
                type="invalid_activity_input",
                non_retryable=True,
            )
        result = await self._provider.submit(
            SubmitRequest(
                operation_id=data.operation_id,
                request_key=data.provider_request_key,
                provider_model_id=data.provider_model_id,
                capability=data.capability,
                mode=data.mode,
                params=data.params,
                inputs=[item.model_dump(exclude_none=True) for item in data.inputs],
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
        return ProviderSubmitOutput(
            outcome=result.outcome,
            provider_task_id=result.provider_task_id,
            error=ProviderError.model_validate(error) if error else None,
        ).model_dump(mode="json")

    @activity.defn(name="provider.query")
    async def query(self, payload: dict[str, Any]) -> dict[str, Any]:
        data = _validate_input(ProviderQueryInput, payload)
        ref = TaskRef(
            request_key=data.provider_request_key,
            provider_task_id=data.provider_task_id,
        )
        result = await self._provider.query(ref)
        return ProviderQueryOutput(
            state=result.state,
            provider_task_id=result.provider_task_id,
            result_urls=result.result_urls,
            usage=None,
            error=(
                ProviderError(code=result.error_code, retryable=False, message="mock task failed")
                if result.error_code
                else None
            ),
        ).model_dump(mode="json")

    @activity.defn(name="provider.cancel")
    async def cancel(self, payload: dict[str, Any]) -> dict[str, Any]:
        data = _validate_input(ProviderCancelInput, payload)
        result = await self._provider.cancel(TaskRef(provider_task_id=data.provider_task_id))
        return ProviderCancelOutput(
            outcome="cancelled" if result.cancelled else "not_cancelled"
        ).model_dump(mode="json")


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
