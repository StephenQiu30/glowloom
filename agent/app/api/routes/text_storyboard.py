"""Text Storyboard Harness invocation routes."""

from __future__ import annotations

import os
import time
from typing import Annotated, TypedDict

from fastapi import APIRouter, Depends, Header, HTTPException

from app.api.dependencies import get_harness_service
from app.harness.service import HarnessService
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
from app.text_contract.authorization import verify_task
from app.text_contract.failure import InvocationFailure

router = APIRouter(tags=["text-storyboard"])
Service = Annotated[HarnessService, Depends(get_harness_service)]


class FailureIdentity(TypedDict):
    invocation_id: str
    input_hash: str
    release_hash: str


@router.post("/internal/text-storyboard/invocations", response_model=TextResult)
async def invoke_text(
    task: TextTask,
    service: Service,
    authorization: str = Header(alias="X-Lanverse-Text-Authorization"),
) -> TextResult:
    try:
        verify_task(task, authorization, os.getenv("AGENT_EXECUTION_SECRET", ""), int(time.time()))
    except ValueError as error:
        raise HTTPException(status_code=401, detail="invalid text task authorization") from error
    identity: FailureIdentity = {
        "invocation_id": task.invocation_id,
        "input_hash": canonical_hash(task.model_dump(mode="json")),
        "release_hash": task.release_hash,
    }
    try:
        return await service.text_storyboard(task)
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
    status = (
        502
        if failure.state == "unknown"
        else (409 if failure.code == "skill_release_unavailable" else 422)
    )
    raise HTTPException(status, failure.model_dump(mode="json"))
