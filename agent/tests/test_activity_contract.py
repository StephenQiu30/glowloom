"""Shared Go/Python JSON examples for provider Temporal activities."""

import json
from pathlib import Path

import pytest
from pydantic import BaseModel, ValidationError

from app.worker.provider_activities import (
    ProviderCancelInput,
    ProviderCancelOutput,
    ProviderQueryInput,
    ProviderQueryOutput,
    ProviderSubmitInput,
    ProviderSubmitOutput,
)

EXAMPLES = Path(__file__).resolve().parents[2] / "contracts" / "activities"


@pytest.mark.parametrize(
    ("name", "input_model", "output_model"),
    [
        ("provider_submit", ProviderSubmitInput, ProviderSubmitOutput),
        ("provider_query", ProviderQueryInput, ProviderQueryOutput),
        ("provider_cancel", ProviderCancelInput, ProviderCancelOutput),
    ],
)
def test_provider_activity_json_contract(
    name: str, input_model: type[BaseModel], output_model: type[BaseModel]
) -> None:
    example = json.loads((EXAMPLES / f"{name}.json").read_text(encoding="utf-8"))
    assert set(example) == {"input", "output"}

    for side, model_type in (("input", input_model), ("output", output_model)):
        payload = example[side]
        model = model_type.model_validate(payload, strict=True)
        assert model.model_dump(mode="json", exclude_unset=True) == payload
        with pytest.raises(ValidationError):
            model_type.model_validate({**payload, "unknown_contract_field": True}, strict=True)

    if name == "provider_submit":
        assert example["input"]["adapter_key"] == "mock"
        assert example["input"]["operation_id"]
        assert example["input"]["provider_request_key"]
        assert example["input"]["params"]
        assert example["input"]["inputs"]
        assert example["input"]["output_count"] == 1
        assert example["output"]["outcome"] == "accepted"
        assert example["output"]["provider_task_id"]
    elif name == "provider_query":
        assert ("provider_task_id" in example["input"]) != (
            "provider_request_key" in example["input"]
        )
        assert example["output"]["state"] == "pending"
        assert example["output"]["result_urls"] == []
        assert "usage" in example["output"]
        assert "error" in example["output"]
    else:
        assert example["input"]["provider_task_id"]
        assert example["output"]["outcome"] == "cancelled"


@pytest.mark.parametrize(
    "task_ref",
    [{}, {"provider_task_id": "task-1", "provider_request_key": "request-1"}],
)
def test_query_input_requires_exactly_one_task_reference(task_ref: dict[str, str]) -> None:
    with pytest.raises(ValidationError):
        ProviderQueryInput.model_validate(task_ref, strict=True)
