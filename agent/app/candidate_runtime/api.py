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
    verify_scene_analysis_execution_grant,
)
from app.candidate_runtime.scene_analysis_schemas import (
    SceneAnalysisAttemptResult,
    SceneAnalysisDiagnostic,
    SceneAnalysisExecutor,
    SceneAnalysisInvocation,
    SceneAnalysisResultError,
)
from app.candidate_runtime.schemas import (
    Executor,
    ResultError,
    StoryGraphStageInvocation,
    StoryGraphStageResult,
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
from app.modules.storygraph.scene_analysis_bundle import SceneAnalysisBundle
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec
from app.modules.storygraph.skill_registry import stage_spec


@asynccontextmanager
async def lifespan(_: FastAPI) -> AsyncGenerator[None, None]:
    StoryGraphBundle().verify_installed_bundle()
    SceneAnalysisBundle().verify_installed_bundle()
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
    scene_analysis_bundle_hash = SceneAnalysisBundle().verify_installed_bundle()
    return {
        "status": "ok",
        "service": "lanverse-agent-runtime",
        "skill_bundle_hash": bundle_hash,
        "scene_analysis_skill_bundle_hash": scene_analysis_bundle_hash,
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


@app.post(
    "/internal/storygraph/scene-analysis/invocations", response_model=SceneAnalysisAttemptResult
)
async def invoke_scene_analysis(
    invocation: SceneAnalysisInvocation,
    execution_grant: str = Header(alias="X-Lanverse-Execution-Grant"),
) -> SceneAnalysisAttemptResult:
    secret = os.getenv("AGENT_EXECUTION_SECRET", "")
    try:
        verify_scene_analysis_execution_grant(execution_grant, secret, invocation)
    except InvalidExecutionGrant as error:
        raise HTTPException(status_code=401, detail="invalid execution grant") from error
    try:
        harness = SceneAnalysisHarness(invocation)
        try:
            value = await harness.execute()
            model = harness.model_name
        finally:
            await harness.aclose()
        candidate = value.model_dump(mode="json")
        result = SceneAnalysisAttemptResult(
            invocation_id=invocation.invocation_id,
            attempt_id=invocation.attempt_id,
            kind="storygraph_stage",
            wire_schema_version=invocation.wire_schema_version,
            variant=invocation.payload.variant,
            stage_release=invocation.stage_release,
            control=invocation.control,
            status="accepted",
            candidate_type=scene_analysis_stage_spec(
                invocation.payload.variant.stage_key
            ).candidate_type,
            candidate=candidate,
            input_hash=invocation.input_hash,
            output_hash=canonical_hash(candidate),
            diagnostics=[],
            diagnostic_hash=canonical_hash([]),
            completed_at=datetime.now(UTC),
            executor=SceneAnalysisExecutor(
                runtime_class="text",
                runtime_image_digest=invocation.stage_release.agent_image_digest,
                harness_version="scene-analysis-harness",
                model=model,
            ),
            error=None,
        )
        result.validate_for(invocation)
        return result
    except SkillBundleUnavailable as error:
        return _scene_analysis_failure(
            invocation, "outcome_unknown", "skill_bundle_unavailable", str(error)
        )
    except BundleInvalid as error:
        return _scene_analysis_failure(invocation, "rejected", "skill_bundle_invalid", str(error))
    except InvocationPolicyInvalid as error:
        return _scene_analysis_failure(
            invocation, "rejected", "invocation_policy_invalid", str(error)
        )
    except (ValidationError, CodexSchemaInvalid, ValueError) as error:
        return _scene_analysis_failure(
            invocation, "rejected", "candidate_schema_invalid", str(error)
        )
    except CodexBudgetExceeded as error:
        return _scene_analysis_failure(
            invocation, "rejected", "execution_budget_exceeded", str(error)
        )
    except CodexDeadlineExceeded as error:
        return _scene_analysis_failure(
            invocation, "rejected", "execution_deadline_exceeded", str(error)
        )
    except CodexToolPolicyViolation as error:
        return _scene_analysis_failure(invocation, "rejected", "tool_not_allowed", str(error))
    except CodexRuntimeUnavailable as error:
        return _scene_analysis_failure(
            invocation, "outcome_unknown", "runtime_unavailable", str(error)
        )
    except CodexExecutionError as error:
        return _scene_analysis_failure(
            invocation, "outcome_unknown", "agent_execution_unknown", str(error)
        )
    except Exception:
        return _scene_analysis_failure(
            invocation,
            "outcome_unknown",
            "agent_execution_unknown",
            "Candidate execution ended without a trustworthy result",
        )


def _scene_analysis_failure(
    invocation: SceneAnalysisInvocation,
    status: Literal["rejected", "outcome_unknown"],
    code: str,
    summary: str,
) -> SceneAnalysisAttemptResult:
    retry_class: Literal["never", "same_release"] = (
        "never" if status == "rejected" else "same_release"
    )
    diagnostics = [SceneAnalysisDiagnostic(code=code, summary=summary[:800])]
    result = SceneAnalysisAttemptResult(
        invocation_id=invocation.invocation_id,
        attempt_id=invocation.attempt_id,
        kind="storygraph_stage",
        wire_schema_version=invocation.wire_schema_version,
        variant=invocation.payload.variant,
        stage_release=invocation.stage_release,
        control=invocation.control,
        status=status,
        candidate_type=scene_analysis_stage_spec(
            invocation.payload.variant.stage_key
        ).candidate_type,
        candidate=None,
        input_hash=invocation.input_hash,
        output_hash=None,
        diagnostics=diagnostics,
        diagnostic_hash=canonical_hash([value.model_dump(mode="json") for value in diagnostics]),
        completed_at=datetime.now(UTC),
        executor=SceneAnalysisExecutor(
            runtime_class="text",
            runtime_image_digest=invocation.stage_release.agent_image_digest,
            harness_version="scene-analysis-harness",
            model="unknown",
        ),
        error=SceneAnalysisResultError(
            code=code,
            safe_summary=summary[:800],
            retry_class=retry_class,
        ),
    )
    result.validate_for(invocation)
    return result
