"""Per-run token, cost, and deadline limits for model calls."""

from dataclasses import dataclass
from datetime import datetime


@dataclass(frozen=True)
class Usage:
    input_tokens: int = 0
    output_tokens: int = 0
    cost_micros: int = 0

    def __post_init__(self) -> None:
        if min(self.input_tokens, self.output_tokens, self.cost_micros) < 0:
            raise ValueError("usage values must be non-negative")


class BudgetExceeded(Exception):
    def __init__(self, reason: str, usage: Usage) -> None:
        super().__init__(f"budget {reason} exceeded")
        self.reason = reason
        self.usage = usage


class Budget:
    def __init__(self, *, max_tokens: int, max_cost_micros: int, deadline: datetime) -> None:
        if max_tokens < 1 or max_cost_micros < 0:
            raise ValueError("invalid budget limits")
        if deadline.tzinfo is None or deadline.utcoffset() is None:
            raise ValueError("deadline must have a timezone")
        self.max_tokens = max_tokens
        self.max_cost_micros = max_cost_micros
        self.deadline = deadline
        self._usage = Usage()
        self._charges: dict[str, Usage] = {}

    @property
    def usage(self) -> Usage:
        return self._usage

    def before_call(
        self,
        *,
        prompt_tokens: int,
        max_output_tokens: int,
        max_cost_micros: int,
        now: datetime,
    ) -> None:
        if min(prompt_tokens, max_output_tokens, max_cost_micros) < 0:
            raise ValueError("call estimate must be non-negative")
        if now >= self.deadline:
            raise BudgetExceeded("deadline", self._usage)
        if (
            self._usage.input_tokens + self._usage.output_tokens + prompt_tokens + max_output_tokens
            > self.max_tokens
        ):
            raise BudgetExceeded("tokens", self._usage)
        if self._usage.cost_micros + max_cost_micros > self.max_cost_micros:
            raise BudgetExceeded("cost", self._usage)

    def charge(self, call_id: str, usage: Usage) -> None:
        if not call_id:
            raise ValueError("call_id is required")
        previous = self._charges.get(call_id)
        if previous is not None:
            if previous != usage:
                raise ValueError("call_id has different usage")
            return
        self._charges[call_id] = usage
        self._usage = Usage(
            input_tokens=self._usage.input_tokens + usage.input_tokens,
            output_tokens=self._usage.output_tokens + usage.output_tokens,
            cost_micros=self._usage.cost_micros + usage.cost_micros,
        )
        if self._usage.input_tokens + self._usage.output_tokens > self.max_tokens:
            raise BudgetExceeded("tokens", self._usage)
        if self._usage.cost_micros > self.max_cost_micros:
            raise BudgetExceeded("cost", self._usage)
