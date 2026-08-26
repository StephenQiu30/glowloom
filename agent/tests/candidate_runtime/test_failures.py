from __future__ import annotations

from uuid import uuid4

import pytest

from app.candidate_runtime import api
from app.candidate_runtime.schemas import Invocation, execution_policy_for
from app.modules.skills.harness import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexRuntimeUnavailable,
    CodexSchemaInvalid,
    CodexToolPolicyViolation,
)


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("error", "status", "code", "retryable"),
    [
        (CodexBudgetExceeded("budget exhausted"), "failed", "execution_budget_exceeded", False),
        (
            CodexDeadlineExceeded("deadline exhausted"),
            "failed",
            "execution_deadline_exceeded",
            False,
        ),
        (CodexToolPolicyViolation("shell is not allowed"), "failed", "tool_not_allowed", False),
        (CodexSchemaInvalid("invalid output"), "failed", "candidate_schema_invalid", False),
        (CodexRuntimeUnavailable("runtime unavailable"), "unknown", "runtime_unavailable", True),
    ],
)
async def test_runtime_failures_have_independent_result_contracts(
    monkeypatch: pytest.MonkeyPatch,
    error: Exception,
    status: str,
    code: str,
    retryable: bool,
) -> None:
    async def fail(_: Invocation) -> tuple[dict[str, object], object]:
        raise error

    def allow_grant(_: str, __: str, ___: Invocation) -> None:
        return None

    monkeypatch.setattr(api, "_candidate", fail)
    monkeypatch.setattr(api, "verify_execution_grant", allow_grant)
    invocation = Invocation(
        invocation_id=uuid4(),
        kind="storyboard_draft",
        input_hash="a" * 64,
        schema_version="agent-candidate-v1",
        execution_policy=execution_policy_for("storyboard_draft"),
        payload={},
    )

    result = await api.invoke(invocation, "test-grant")

    assert result.status == status
    assert result.error is not None
    assert result.error.code == code
    assert result.error.retryable is retryable
