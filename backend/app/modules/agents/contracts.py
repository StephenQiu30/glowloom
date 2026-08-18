from collections.abc import Sequence
from dataclasses import dataclass, field
from hashlib import sha256
from typing import Any, Generic, Literal, Protocol, TypeVar

from pydantic import BaseModel

AgentOutcome = Literal["failed", "unknown"]
T = TypeVar("T", bound=BaseModel)


class StructuredAgentModel(Protocol):
    async def ainvoke(self, messages: Sequence[Any]) -> object: ...


@dataclass(frozen=True, slots=True)
class SkillDefinition:
    name: str
    version: str
    max_input_chars: int
    timeout_seconds: float
    candidate_only: bool = True
    allowed_tools: frozenset[str] = field(default_factory=lambda: frozenset[str]())


@dataclass(frozen=True, slots=True)
class AgentExecutionContext:
    skill_name: str
    skill_version: str
    trace_id: str | None = None
    workspace_id: str | None = None
    task_id: str | None = None


@dataclass(frozen=True, slots=True)
class AgentRun(Generic[T]):
    output: T
    skill_name: str
    skill_version: str
    input_hash: str
    trace_id: str | None


class AgentExecutionError(Exception):
    outcome: AgentOutcome

    def __init__(
        self,
        *,
        outcome: AgentOutcome,
        code: str,
        summary: str,
        retryable: bool,
        next_action: str,
    ) -> None:
        super().__init__(summary)
        self.outcome = outcome
        self.code = code
        self.summary = summary
        self.retryable = retryable
        self.next_action = next_action


def input_hash(value: str) -> str:
    return sha256(value.encode("utf-8")).hexdigest()
