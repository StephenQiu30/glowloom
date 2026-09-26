"""Single charging point for each structured model completion."""

from collections.abc import Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any, Protocol

from app.harness.budget import Budget, Usage


@dataclass(frozen=True)
class Price:
    input_micros_per_token: int
    output_micros_per_token: int

    def __post_init__(self) -> None:
        if min(self.input_micros_per_token, self.output_micros_per_token) < 0:
            raise ValueError("token price must be non-negative")


@dataclass(frozen=True)
class ModelResponse:
    output: dict[str, Any]
    input_tokens: int
    output_tokens: int
    latency_ms: int


class ModelClient(Protocol):
    async def complete(
        self,
        model_key: str,
        messages: list[dict[str, str]],
        output_schema: dict[str, Any],
        max_output_tokens: int,
    ) -> ModelResponse: ...


class ModelUnavailable(Exception):
    pass


class ContextTooLarge(Exception):
    pass


class ModelRouter:
    def __init__(self, client: ModelClient, prices: Mapping[str, Price]) -> None:
        self._client = client
        self._prices = dict(prices)

    async def complete(
        self,
        *,
        call_id: str,
        model_key: str,
        messages: list[dict[str, str]],
        output_schema: dict[str, Any],
        max_input_tokens: int,
        max_output_tokens: int,
        budget: Budget,
    ) -> tuple[ModelResponse, Usage]:
        price = self._prices.get(model_key)
        if price is None:
            raise ModelUnavailable(f"model unavailable: {model_key}")
        # Character estimate is only used for the zero-cost mock model.
        prompt_tokens = sum(len(message["content"]) for message in messages)
        if prompt_tokens > max_input_tokens:
            raise ContextTooLarge("context_too_large")
        budget.before_call(
            prompt_tokens=prompt_tokens,
            max_output_tokens=max_output_tokens,
            max_cost_micros=(
                prompt_tokens * price.input_micros_per_token
                + max_output_tokens * price.output_micros_per_token
            ),
            now=datetime.now(UTC),
        )
        response = await self._client.complete(
            model_key, messages, output_schema, max_output_tokens
        )
        usage = Usage(
            input_tokens=response.input_tokens,
            output_tokens=response.output_tokens,
            cost_micros=(
                response.input_tokens * price.input_micros_per_token
                + response.output_tokens * price.output_micros_per_token
            ),
        )
        budget.charge(call_id, usage)
        return response, usage
