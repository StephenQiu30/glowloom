"""StoryGraph Harness invocation routes."""

from __future__ import annotations

import os
from typing import Annotated, Literal

from fastapi import APIRouter, Depends, Header, HTTPException
from pydantic import ValidationError

from app.api.dependencies import get_harness_service
from app.harness.grants import InvalidExecutionGrant, verify_execution_grant
from app.harness.schemas import (
    Executor,
    ResultError,
    StoryGraphStageInvocation,
    StoryGraphStageResult,
)
from app.harness.service import HarnessService
from app.modules.storygraph.bundle import BundleInvalid
from app.modules.storygraph.harness import InvocationPolicyInvalid, SkillBundleUnavailable
from app.modules.storygraph.skill_registry import stage_spec
from app.protocol.canonical import canonical_hash
from app.reasoning.codex import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexExecutionError,
    CodexRuntimeUnavailable,
    CodexSchemaInvalid,
    CodexToolPolicyViolation,
)

router = APIRouter(tags=["storygraph"])
Service = Annotated[HarnessService, Depends(get_harness_service)]


@router.post("/internal/storygraph/invocations", response_model=StoryGraphStageResult)
async def invoke_storygraph(
    invocation: StoryGraphStageInvocation,
    service: Service,
    execution_grant: str = Header(alias="X-Lanverse-Execution-Grant"),
) -> StoryGraphStageResult:
    try:
        verify_execution_grant(execution_grant, os.getenv("AGENT_EXECUTION_SECRET", ""), invocation)
    except InvalidExecutionGrant as error:
        raise HTTPException(status_code=401, detail="invalid execution grant") from error
    try:
        value, model = await service.storygraph(invocation)
        candidate = value.model_dump(mode="json")
        return StoryGraphStageResult(
            invocation_id=invocation.invocation_id,
            kind="storygraph_stage",
            wire_schema_version=invocation.wire_schema_version,
            stage=invocation.payload.stage,
            shard_key=invocation.payload.shard_key,
            status="succeeded",
            candidate_type=stage_spec(invocation.payload.stage).candidate_type,
            candidate=candidate,
            input_hash=invocation.input_hash,
            result_hash=canonical_hash(candidate),
            issues=[],
            executor=Executor(name="codex-cli", version="storygraph-stage-harness", model=model),
            error=None,
        )
    except SkillBundleUnavailable as error:
        return _failure(invocation, "unknown", "skill_bundle_unavailable", str(error), True)
    except BundleInvalid as error:
        return _failure(invocation, "failed", "skill_bundle_invalid", str(error), False)
    except InvocationPolicyInvalid as error:
        return _failure(invocation, "failed", "invocation_policy_invalid", str(error), False)
    except (ValidationError, CodexSchemaInvalid) as error:
        return _failure(invocation, "failed", "candidate_schema_invalid", str(error), False)
    except CodexBudgetExceeded as error:
        return _failure(invocation, "failed", "execution_budget_exceeded", str(error), False)
    except CodexDeadlineExceeded as error:
        return _failure(invocation, "failed", "execution_deadline_exceeded", str(error), False)
    except CodexToolPolicyViolation as error:
        return _failure(invocation, "failed", "tool_not_allowed", str(error), False)
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


def _failure(
    invocation: StoryGraphStageInvocation,
    status: Literal["failed", "unknown"],
    code: str,
    summary: str,
    retryable: bool,
) -> StoryGraphStageResult:
    return StoryGraphStageResult(
        invocation_id=invocation.invocation_id,
        kind="storygraph_stage",
        wire_schema_version=invocation.wire_schema_version,
        stage=invocation.payload.stage,
        shard_key=invocation.payload.shard_key,
        status=status,
        candidate_type=stage_spec(invocation.payload.stage).candidate_type,
        candidate=None,
        input_hash=invocation.input_hash,
        result_hash=None,
        issues=[],
        executor=Executor(name="codex-cli", version="storygraph-stage-harness", model="unknown"),
        error=ResultError(code=code, summary=summary[:800], retryable=retryable),
    )
