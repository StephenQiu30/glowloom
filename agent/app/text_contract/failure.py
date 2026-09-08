"""Bounded, task-bound failure receipts shared by the private HTTP peers."""

from typing import Any, Literal, Self

from pydantic import Field, model_validator

from app.text_contract.schemas import Record
from app.text_contract.source import Digest

FailureCode = Literal[
    "context_insufficient",
    "skill_release_unavailable",
    "input_contract_invalid",
    "candidate_contract_invalid",
    "execution_output_budget_exceeded",
    "execution_deadline_exceeded",
    "structured_output_invalid",
    "reasoning_execution_failed_or_unknown",
]


class InvocationFailure(Record):
    invocation_id: str = Field(min_length=1, max_length=128)
    input_hash: Digest
    release_hash: Digest
    phase: Literal["preflight", "generation", "validation"]
    code: FailureCode
    diagnostic: str = Field(default="", max_length=500)
    candidate: dict[str, Any] | None = None
    raw_output: str | None = Field(default=None, max_length=2000000)

    @model_validator(mode="after")
    def phase_matches_code(self) -> Self:
        phases = {
            "context_insufficient": "preflight",
            "skill_release_unavailable": "preflight",
            "input_contract_invalid": "preflight",
            "candidate_contract_invalid": "validation",
            "execution_output_budget_exceeded": "generation",
            "execution_deadline_exceeded": "generation",
            "structured_output_invalid": "generation",
            "reasoning_execution_failed_or_unknown": "generation",
        }
        if (
            self.phase != phases[self.code]
            or (self.candidate is not None and self.phase != "validation")
            or (self.raw_output is not None and self.code != "structured_output_invalid")
        ):
            raise ValueError("failure phase mismatch")
        return self

    @property
    def state(self) -> Literal["unknown", "failed"]:
        return "unknown" if self.code == "reasoning_execution_failed_or_unknown" else "failed"

    @property
    def error_code(self) -> str:
        return "harness_response_unknown" if self.state == "unknown" else self.code


class HarnessFailed(RuntimeError):
    def __init__(self, failure: InvocationFailure) -> None:
        super().__init__(failure.code)
        self.failure = failure
