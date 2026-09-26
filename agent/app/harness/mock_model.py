"""Deterministic structured model used only by mock Skills and local verification."""

import json
from typing import Any

from app.harness.router import ContextTooLarge, ModelResponse, ModelUnavailable


class MockStructuredClient:
    async def complete(
        self,
        model_key: str,
        messages: list[dict[str, str]],
        output_schema: dict[str, Any],
        max_output_tokens: int,
    ) -> ModelResponse:
        if model_key != "mock.structured":
            raise ModelUnavailable(f"model unavailable: {model_key}")
        inputs = json.loads(messages[1]["content"])
        output = {"value": inputs["value"]}
        output_tokens = len(json.dumps(output, ensure_ascii=False))
        if output_tokens > max_output_tokens:
            raise ContextTooLarge("mock output exceeds skill token limit")
        return ModelResponse(
            output=output,
            input_tokens=sum(len(message["content"]) for message in messages),
            output_tokens=output_tokens,
            latency_ms=0,
        )
