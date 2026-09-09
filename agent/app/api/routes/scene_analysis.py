"""Scene Analysis Harness invocation routes."""

from __future__ import annotations

import os
from datetime import UTC, datetime
from typing import Annotated, Literal

from fastapi import APIRouter, Depends, Header, HTTPException
from pydantic import ValidationError

from app.api.dependencies import get_harness_service
from app.harness.grants import (
    InvalidSceneAnalysisDispatchAuthorization,
    SceneAnalysisDispatchAuthorizationEvidence,
    verify_scene_analysis_dispatch_authorization,
)
from app.harness.scene_analysis_schemas import (
    SceneAnalysisAttemptResult,
    SceneAnalysisDiagnostic,
    SceneAnalysisExecutor,
    SceneAnalysisInvocation,
    SceneAnalysisResultError,
)
from app.harness.service import HarnessService
from app.modules.storygraph.bundle import BundleInvalid
from app.modules.storygraph.harness import InvocationPolicyInvalid, SkillBundleUnavailable
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec
from app.protocol.canonical import production_canonical_hash
from app.reasoning.codex import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexExecutionError,
    CodexRuntimeUnavailable,
    CodexSchemaInvalid,
    CodexToolPolicyViolation,
)

router = APIRouter(tags=["scene-analysis"])
Service = Annotated[HarnessService, Depends(get_harness_service)]


@router.post(
    "/internal/storygraph/scene-analysis/invocations", response_model=SceneAnalysisAttemptResult
)
async def invoke_scene_analysis(
    invocation: SceneAnalysisInvocation,
    service: Service,
    dispatch_authorization: str = Header(alias="X-Lanverse-Dispatch-Authorization"),
) -> SceneAnalysisAttemptResult:
    secret = os.getenv("AGENT_EXECUTION_SECRET", "")
    try:
        authorization = verify_scene_analysis_dispatch_authorization(
            dispatch_authorization,
            secret,
            invocation,
        )
    except InvalidSceneAnalysisDispatchAuthorization as error:
        raise HTTPException(status_code=401, detail="invalid dispatch authorization") from error
    try:
        value, model = await service.scene_analysis(invocation)
        candidate = value.model_dump(mode="json")
        result = SceneAnalysisAttemptResult.build(
            invocation_id=invocation.invocation_id,
            attempt_id=invocation.attempt_id,
            kind="storygraph_stage",
            wire_schema_version=invocation.wire_schema_version,
            variant=invocation.payload.variant,
            stage_release=invocation.stage_release,
            control=invocation.control,
            claim_version=authorization.claim_version,
            dispatch_authorization_hash=authorization.authorization_hash,
            status="accepted",
            candidate_type=scene_analysis_stage_spec(
                invocation.payload.variant.stage_key,
                invocation.payload.variant.profile_key,
            ).candidate_type,
            candidate=candidate,
            input_hash=invocation.input_hash,
            output_hash=production_canonical_hash(candidate),
            diagnostics=[],
            diagnostic_hash=production_canonical_hash([]),
            completed_at=datetime.now(UTC),
            executor=SceneAnalysisExecutor(
                runtime_class="text",
                runtime_image_digest=invocation.stage_release.agent_image_digest,
                harness_version="scene-analysis-harness",
                model=model,
            ),
            error=None,
        )
        result.validate_for(
            invocation,
            authorization.claim_version,
            authorization.authorization_hash,
        )
        return result
    except SkillBundleUnavailable as error:
        return _failure(
            invocation, authorization, "outcome_unknown", "skill_bundle_unavailable", str(error)
        )
    except BundleInvalid as error:
        return _failure(invocation, authorization, "rejected", "skill_bundle_invalid", str(error))
    except InvocationPolicyInvalid as error:
        return _failure(
            invocation, authorization, "rejected", "invocation_policy_invalid", str(error)
        )
    except (ValidationError, CodexSchemaInvalid, ValueError) as error:
        return _failure(
            invocation, authorization, "rejected", "candidate_schema_invalid", str(error)
        )
    except CodexBudgetExceeded as error:
        return _failure(
            invocation, authorization, "rejected", "execution_budget_exceeded", str(error)
        )
    except CodexDeadlineExceeded as error:
        return _failure(
            invocation, authorization, "rejected", "execution_deadline_exceeded", str(error)
        )
    except CodexToolPolicyViolation as error:
        return _failure(invocation, authorization, "rejected", "tool_not_allowed", str(error))
    except CodexRuntimeUnavailable as error:
        return _failure(
            invocation, authorization, "outcome_unknown", "runtime_unavailable", str(error)
        )
    except CodexExecutionError as error:
        return _failure(
            invocation, authorization, "outcome_unknown", "agent_execution_unknown", str(error)
        )
    except Exception:
        return _failure(
            invocation,
            authorization,
            "outcome_unknown",
            "agent_execution_unknown",
            "Candidate execution ended without a trustworthy result",
        )


def _failure(
    invocation: SceneAnalysisInvocation,
    authorization: SceneAnalysisDispatchAuthorizationEvidence,
    status: Literal["rejected", "outcome_unknown"],
    code: str,
    summary: str,
) -> SceneAnalysisAttemptResult:
    retry_class: Literal["never", "same_release"] = (
        "never" if status == "rejected" else "same_release"
    )
    diagnostics = [SceneAnalysisDiagnostic(code=code, summary=summary[:800])]
    result = SceneAnalysisAttemptResult.build(
        invocation_id=invocation.invocation_id,
        attempt_id=invocation.attempt_id,
        kind="storygraph_stage",
        wire_schema_version=invocation.wire_schema_version,
        variant=invocation.payload.variant,
        stage_release=invocation.stage_release,
        control=invocation.control,
        claim_version=authorization.claim_version,
        dispatch_authorization_hash=authorization.authorization_hash,
        status=status,
        candidate_type=scene_analysis_stage_spec(
            invocation.payload.variant.stage_key,
            invocation.payload.variant.profile_key,
        ).candidate_type,
        candidate=None,
        input_hash=invocation.input_hash,
        output_hash=None,
        diagnostics=diagnostics,
        diagnostic_hash=production_canonical_hash(
            [value.model_dump(mode="json") for value in diagnostics]
        ),
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
    result.validate_for(
        invocation,
        authorization.claim_version,
        authorization.authorization_hash,
    )
    return result
