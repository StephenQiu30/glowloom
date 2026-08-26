from __future__ import annotations

import os
from typing import Any

from fastapi import FastAPI, Header, HTTPException
from pydantic import ValidationError

from app.candidate_runtime.canonical import canonical_hash
from app.candidate_runtime.grants import InvalidExecutionGrant, verify_execution_grant
from app.candidate_runtime.schemas import Executor, Invocation, Result, ResultError
from app.modules.scripts.production_bibles import CodexLocalProductionBibleGenerator
from app.modules.scripts.production_bibles.contracts import ProductionBibleInput
from app.modules.skills.harness import (
    CodexBudgetExceeded,
    CodexExecutionError,
    CodexRuntimeUnavailable,
    CodexSchemaInvalid,
    CodexToolPolicyViolation,
)
from app.modules.storyboards import CodexLocalStoryboardDrafter
from app.modules.storyboards.contracts import StoryboardDraftInput

app = FastAPI(
    title="Lanverse Candidate Runtime",
    docs_url=None,
    redoc_url=None,
    openapi_url=None,
)


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    return {"status": "ok", "service": "lanverse-agent-runtime"}


@app.post("/internal/v1/invocations", response_model=Result)
async def invoke(
    invocation: Invocation,
    execution_grant: str = Header(alias="X-Lanverse-Execution-Grant"),
) -> Result:
    secret = os.getenv("AGENT_EXECUTION_SECRET", "")
    try:
        verify_execution_grant(execution_grant, secret, invocation)
    except InvalidExecutionGrant as error:
        raise HTTPException(status_code=401, detail="invalid execution grant") from error
    try:
        candidate, executor = await _candidate(invocation)
        return Result(
            invocation_id=invocation.invocation_id,
            kind=invocation.kind,
            input_hash=invocation.input_hash,
            status="succeeded",
            candidate=candidate,
            result_hash=canonical_hash(candidate),
            executor=executor,
            error=None,
        )
    except (ValidationError, ValueError) as error:
        return _failure(invocation, "failed", "candidate_validation_failed", str(error), False)
    except CodexBudgetExceeded as error:
        return _failure(invocation, "failed", "execution_budget_exceeded", str(error), False)
    except CodexToolPolicyViolation as error:
        return _failure(invocation, "failed", "tool_not_allowed", str(error), False)
    except CodexSchemaInvalid as error:
        return _failure(invocation, "failed", "candidate_schema_invalid", str(error), False)
    except CodexRuntimeUnavailable as error:
        return _failure(invocation, "unknown", "runtime_unavailable", str(error), True)
    except CodexExecutionError as error:
        return _failure(invocation, "unknown", "agent_execution_unknown", str(error), True)
    except Exception:
        return _failure(
            invocation,
            "unknown",
            "agent_execution_unknown",
            "Candidate execution ended without a trustworthy result",
            True,
        )


async def _candidate(invocation: Invocation) -> tuple[dict[str, Any], Executor]:
    if invocation.kind == "production_bible":
        generator = CodexLocalProductionBibleGenerator(execution_policy=invocation.execution_policy)
        try:
            candidate = await generator.generate(
                ProductionBibleInput.model_validate(invocation.payload)
            )
            model = str(getattr(generator, "model_name", "inherited-local-config"))
        finally:
            await generator.aclose()
        return candidate, Executor(
            name="codex-cli", version="production-bible-harness-v1", model=model
        )
    if invocation.kind == "storyboard_draft":
        generator = CodexLocalStoryboardDrafter(execution_policy=invocation.execution_policy)
        try:
            candidate = await generator.draft(
                StoryboardDraftInput.model_validate(invocation.payload)
            )
            model = str(getattr(generator, "model_name", "inherited-local-config"))
        finally:
            await generator.aclose()
        return candidate, Executor(name="codex-cli", version="storyboard-harness-v1", model=model)
    raise ValueError("script structure is owned by the deterministic Go Backend")


def _failure(
    invocation: Invocation,
    status: str,
    code: str,
    summary: str,
    retryable: bool,
) -> Result:
    return Result(
        invocation_id=invocation.invocation_id,
        kind=invocation.kind,
        input_hash=invocation.input_hash,
        status=status,  # type: ignore[arg-type]
        candidate=None,
        result_hash=None,
        executor=Executor(name="codex-cli", version="candidate-runtime-v1", model="unknown"),
        error=ResultError(code=code, summary=summary[:800], retryable=retryable),
    )
