"""Shared local/HTTP text execution semantics, without transport or storage access."""

from collections.abc import Awaitable, Callable
from typing import Any, TypedDict

from app.modules.text_storyboard.harness import (
    CandidateContractInvalid,
    ContextInsufficient,
    InputContractInvalid,
    SkillReleaseInvalid,
    TextResult,
    TextTask,
)
from app.protocol.canonical import canonical_hash
from app.reasoning.codex import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexExecutionError,
    CodexSchemaInvalid,
)
from app.text_contract.failure import HarnessFailed, InvocationFailure


class FailureIdentity(TypedDict):
    invocation_id: str
    input_hash: str
    release_hash: str


async def execute_text(
    task: TextTask, execute: Callable[[TextTask], Awaitable[TextResult]]
) -> dict[str, Any]:
    identity: FailureIdentity = {
        "invocation_id": task.invocation_id,
        "input_hash": canonical_hash(task.model_dump(mode="json")),
        "release_hash": task.release_hash,
    }
    try:
        return (await execute(task)).model_dump(mode="json")
    except SkillReleaseInvalid:
        failure = InvocationFailure(**identity, phase="preflight", code="skill_release_unavailable")
    except ContextInsufficient:
        failure = InvocationFailure(**identity, phase="preflight", code="context_insufficient")
    except CandidateContractInvalid as error:
        failure = InvocationFailure(
            **identity,
            phase="validation",
            code="candidate_contract_invalid",
            diagnostic=error.diagnostic,
            candidate=error.candidate,
        )
    except CodexSchemaInvalid as error:
        failure = InvocationFailure(
            **identity,
            phase="generation",
            code="structured_output_invalid",
            raw_output=error.raw_output,
        )
    except CodexDeadlineExceeded:
        failure = InvocationFailure(
            **identity, phase="generation", code="execution_deadline_exceeded"
        )
    except CodexBudgetExceeded:
        failure = InvocationFailure(
            **identity, phase="generation", code="execution_output_budget_exceeded"
        )
    except CodexExecutionError as error:
        failure = InvocationFailure(
            **identity,
            phase="generation",
            code="reasoning_execution_failed_or_unknown",
            diagnostic=str(error)[:500],
        )
    except InputContractInvalid:
        failure = InvocationFailure(**identity, phase="preflight", code="input_contract_invalid")
    raise HarnessFailed(failure)
