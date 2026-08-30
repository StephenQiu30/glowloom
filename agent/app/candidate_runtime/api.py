from __future__ import annotations

import os
from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager
from datetime import UTC, datetime
from typing import Literal

from fastapi import FastAPI, Header, HTTPException
from pydantic import ValidationError

from app.candidate_runtime.canonical import canonical_hash
from app.candidate_runtime.grants import (
    InvalidExecutionGrant,
    verify_execution_grant,
    verify_v2_execution_grant,
)
from app.candidate_runtime.schemas import (
    Executor,
    ResultError,
    StoryGraphStageInvocation,
    StoryGraphStageResult,
)
from app.candidate_runtime.v2_schemas import (
    StoryGraphV2AttemptResult,
    StoryGraphV2Diagnostic,
    StoryGraphV2Executor,
    StoryGraphV2Invocation,
    StoryGraphV2ResultError,
)
from app.modules.storygraph.bundle import BundleInvalid, StoryGraphBundle
from app.modules.storygraph.harness import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexExecutionError,
    CodexRuntimeUnavailable,
    CodexSchemaInvalid,
    CodexToolPolicyViolation,
    InvocationPolicyInvalid,
    SkillBundleUnavailable,
    StoryGraphHarness,
)
from app.modules.storygraph.skill_registry import stage_spec
from app.modules.storygraph.v2_bundle import StoryGraphV2Bundle
from app.modules.storygraph.v2_harness import StoryGraphV2Harness
from app.modules.storygraph.v2_registry import stage_spec_v2


@asynccontextmanager
async def lifespan(_: FastAPI) -> AsyncGenerator[None, None]:
    StoryGraphBundle().verify_installed_bundle()
    StoryGraphV2Bundle().verify_installed_bundle()
    yield


app = FastAPI(
    title="Lanverse Candidate Runtime",
    docs_url=None,
    redoc_url=None,
    openapi_url=None,
    lifespan=lifespan,
)


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    bundle_hash = StoryGraphBundle().verify_installed_bundle()
    v2_bundle_hash = StoryGraphV2Bundle().verify_installed_bundle()
    return {
        "status": "ok",
        "service": "lanverse-agent-runtime",
        "skill_bundle_hash": bundle_hash,
        "storygraph_v2_skill_bundle_hash": v2_bundle_hash,
    }


@app.post("/internal/v1/invocations", response_model=StoryGraphStageResult)
async def invoke(
    invocation: StoryGraphStageInvocation,
    execution_grant: str = Header(alias="X-Lanverse-Execution-Grant"),
) -> StoryGraphStageResult:
    secret = os.getenv("AGENT_EXECUTION_SECRET", "")
    try:
        verify_execution_grant(execution_grant, secret, invocation)
    except InvalidExecutionGrant as error:
        raise HTTPException(status_code=401, detail="invalid execution grant") from error
    try:
        harness = StoryGraphHarness(invocation)
        try:
            value = await harness.execute()
            model = harness.model_name
        finally:
            await harness.aclose()
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
            executor=Executor(name="codex-cli", version="storygraph-stage-harness-v1", model=model),
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
        executor=Executor(name="codex-cli", version="storygraph-stage-harness-v1", model="unknown"),
        error=ResultError(code=code, summary=summary[:800], retryable=retryable),
    )


@app.post("/internal/v2/invocations", response_model=StoryGraphV2AttemptResult)
async def invoke_v2(
    invocation: StoryGraphV2Invocation,
    execution_grant: str = Header(alias="X-Lanverse-Execution-Grant"),
) -> StoryGraphV2AttemptResult:
    secret = os.getenv("AGENT_EXECUTION_SECRET", "")
    try:
        verify_v2_execution_grant(execution_grant, secret, invocation)
    except InvalidExecutionGrant as error:
        raise HTTPException(status_code=401, detail="invalid execution grant") from error
    try:
        harness = StoryGraphV2Harness(invocation)
        try:
            value = await harness.execute()
            model = harness.model_name
        finally:
            await harness.aclose()
        candidate = value.model_dump(mode="json")
        result = StoryGraphV2AttemptResult(
            invocation_id=invocation.invocation_id,
            attempt_id=invocation.attempt_id,
            kind="storygraph_stage",
            wire_schema_version=invocation.wire_schema_version,
            variant=invocation.payload.variant,
            stage_release=invocation.stage_release,
            control=invocation.control,
            status="accepted",
            candidate_type=stage_spec_v2(invocation.payload.variant.stage_key).candidate_type,
            candidate=candidate,
            input_hash=invocation.input_hash,
            output_hash=canonical_hash(candidate),
            diagnostics=[],
            diagnostic_hash=canonical_hash([]),
            completed_at=datetime.now(UTC),
            executor=StoryGraphV2Executor(
                runtime_class="text",
                runtime_image_digest=invocation.stage_release.agent_image_digest,
                harness_version="storygraph-stage-harness-v2",
                model=model,
            ),
            error=None,
        )
        result.validate_for(invocation)
        return result
    except SkillBundleUnavailable as error:
        return _v2_failure(invocation, "outcome_unknown", "skill_bundle_unavailable", str(error))
    except BundleInvalid as error:
        return _v2_failure(invocation, "rejected", "skill_bundle_invalid", str(error))
    except InvocationPolicyInvalid as error:
        return _v2_failure(invocation, "rejected", "invocation_policy_invalid", str(error))
    except (ValidationError, CodexSchemaInvalid, ValueError) as error:
        return _v2_failure(invocation, "rejected", "candidate_schema_invalid", str(error))
    except CodexBudgetExceeded as error:
        return _v2_failure(invocation, "rejected", "execution_budget_exceeded", str(error))
    except CodexDeadlineExceeded as error:
        return _v2_failure(invocation, "rejected", "execution_deadline_exceeded", str(error))
    except CodexToolPolicyViolation as error:
        return _v2_failure(invocation, "rejected", "tool_not_allowed", str(error))
    except CodexRuntimeUnavailable as error:
        return _v2_failure(invocation, "outcome_unknown", "runtime_unavailable", str(error))
    except CodexExecutionError as error:
        return _v2_failure(invocation, "outcome_unknown", "agent_execution_unknown", str(error))
    except Exception:
        return _v2_failure(
            invocation,
            "outcome_unknown",
            "agent_execution_unknown",
            "Candidate execution ended without a trustworthy result",
        )


def _v2_failure(
    invocation: StoryGraphV2Invocation,
    status: Literal["rejected", "outcome_unknown"],
    code: str,
    summary: str,
) -> StoryGraphV2AttemptResult:
    retry_class: Literal["never", "same_release"] = (
        "never" if status == "rejected" else "same_release"
    )
    diagnostics = [StoryGraphV2Diagnostic(code=code, summary=summary[:800])]
    result = StoryGraphV2AttemptResult(
        invocation_id=invocation.invocation_id,
        attempt_id=invocation.attempt_id,
        kind="storygraph_stage",
        wire_schema_version=invocation.wire_schema_version,
        variant=invocation.payload.variant,
        stage_release=invocation.stage_release,
        control=invocation.control,
        status=status,
        candidate_type=stage_spec_v2(invocation.payload.variant.stage_key).candidate_type,
        candidate=None,
        input_hash=invocation.input_hash,
        output_hash=None,
        diagnostics=diagnostics,
        diagnostic_hash=canonical_hash([value.model_dump(mode="json") for value in diagnostics]),
        completed_at=datetime.now(UTC),
        executor=StoryGraphV2Executor(
            runtime_class="text",
            runtime_image_digest=invocation.stage_release.agent_image_digest,
            harness_version="storygraph-stage-harness-v2",
            model="unknown",
        ),
        error=StoryGraphV2ResultError(
            code=code,
            safe_summary=summary[:800],
            retry_class=retry_class,
        ),
    )
    result.validate_for(invocation)
    return result
