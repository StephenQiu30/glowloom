from __future__ import annotations

from copy import deepcopy
from datetime import UTC, datetime
from typing import Any, Literal
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.harness.reference_plan_schemas import (
    ReferencePlanAttemptResult,
    ReferencePlanExecutor,
    ReferencePlanInvocation,
    ReferencePlanPayload,
    ReferencePlanScope,
    ReferencePlanShard,
    ReferencePlanStageVariant,
    validate_reference_plan_dispatch_authorization,
)
from app.harness.scene_analysis_schemas import (
    SceneAnalysisControlProof,
    SceneAnalysisDispatchAuthorizationClaims,
    SceneAnalysisExecutionBudget,
    SceneAnalysisReleaseIdentity,
    SceneAnalysisResultError,
)
from app.modules.storygraph.bundle import SKILL_BUNDLE_HASH
from app.modules.storygraph.reference_plan_contract import ReferencePlanInput
from app.protocol.canonical import production_canonical_hash
from tests.contract.test_reference_plan_contract import digest, valid_candidate, valid_input


def valid_invocation() -> ReferencePlanInvocation:
    stage_input = ReferencePlanInput.model_validate(valid_input())
    return ReferencePlanInvocation.build(
        invocation_id=UUID("00000000-0000-0000-0000-000000000580"),
        attempt_id=UUID("00000000-0000-0000-0000-000000000590"),
        stage_release=SceneAnalysisReleaseIdentity(
            skill_release_id=UUID("66666666-6666-4666-8666-666666666666"),
            skill_release_hash="1" * 64,
            stage_release_hash="2" * 64,
            bundle_content_hash=SKILL_BUNDLE_HASH,
            agent_image_digest="sha256:" + "4" * 64,
        ),
        control=SceneAnalysisControlProof(
            control_record_id=UUID("77777777-7777-4777-8777-777777777777"),
            control_revision=1,
            status="approved",
            control_hash="5" * 64,
            release_fence=0,
        ),
        budget=SceneAnalysisExecutionBudget(
            max_attempts=3,
            max_model_calls=1,
            max_execution_seconds=120,
            max_output_bytes=131072,
        ),
        payload=ReferencePlanPayload(
            variant=ReferencePlanStageVariant(
                stage_key="plan_reference_assets",
                profile_key="default",
                lane_key="primary",
                output_schema_version="reference-plan-candidate-production",
            ),
            scope=ReferencePlanScope(
                workspace_id=stage_input.workspace_id,
                project_id=stage_input.project_id,
            ),
            shard=ReferencePlanShard(
                manifest_id=UUID("00000000-0000-0000-0000-000000000540"),
                manifest_hash=digest("reference-plan-shard"),
                shard_key=f"project:{stage_input.project_id}",
                impact_closure_hash=digest("reference-plan-impact"),
            ),
            stage_input=stage_input,
        ),
    )


def valid_dispatch_claims(
    invocation: ReferencePlanInvocation,
) -> SceneAnalysisDispatchAuthorizationClaims:
    return SceneAnalysisDispatchAuthorizationClaims(
        invocation_id=invocation.invocation_id,
        attempt_id=invocation.attempt_id,
        input_hash=invocation.input_hash,
        skill_release_id=invocation.stage_release.skill_release_id,
        skill_release_hash=invocation.stage_release.skill_release_hash,
        stage_release_hash=invocation.stage_release.stage_release_hash,
        bundle_content_hash=invocation.stage_release.bundle_content_hash,
        control_hash=invocation.control.control_hash,
        release_fence=invocation.control.release_fence,
        claim_version=1,
        agent_image_digest=invocation.stage_release.agent_image_digest,
        expires_at=200,
    )


def valid_accepted_result(invocation: ReferencePlanInvocation) -> ReferencePlanAttemptResult:
    candidate = valid_candidate(invocation.payload.stage_input)
    return ReferencePlanAttemptResult.build(
        invocation_id=invocation.invocation_id,
        attempt_id=invocation.attempt_id,
        kind="storygraph_stage",
        wire_schema_version="storygraph-stage-wire-production",
        variant=invocation.payload.variant,
        stage_release=invocation.stage_release,
        control=invocation.control,
        claim_version=1,
        dispatch_authorization_hash=digest("reference-plan-dispatch"),
        status="accepted",
        candidate_type="reference_plan_candidate",
        candidate=candidate,
        input_hash=invocation.input_hash,
        output_hash=production_canonical_hash(candidate),
        diagnostics=[],
        diagnostic_hash=production_canonical_hash([]),
        completed_at=datetime(2026, 9, 12, tzinfo=UTC),
        executor=ReferencePlanExecutor(
            runtime_class="text",
            runtime_image_digest=invocation.stage_release.agent_image_digest,
            harness_version="reference-plan-harness",
            model="codex-cli-default",
        ),
        error=None,
    )


def test_reference_plan_invocation_freezes_project_input_and_identity() -> None:
    invocation = valid_invocation()

    assert invocation.input_hash == invocation.compute_input_hash()
    assert invocation.input_hash == (
        "8a403c1be5eef541b6b04f129e7e8de59c1d0ccb7a14cb551a4880b4f2f5dc67"
    )
    assert invocation.stage_instance_key() == (
        "ab5bb1bd80460d1a7b17cc88cdb6f841936773f1cbd2b49a6c1dbcaef1d2c014"
    )
    assert invocation.payload.scope.workspace_id == invocation.payload.stage_input.workspace_id
    assert invocation.payload.scope.project_id == invocation.payload.stage_input.project_id


@pytest.mark.parametrize(
    ("path", "value"),
    [
        (("unexpected",), True),
        (("payload", "scope", "project_id"), "00000000-0000-0000-0000-000000000099"),
        (("payload", "shard", "shard_key"), "project:00000000-0000-0000-0000-000000000099"),
        (("payload", "stage_input", "visual_foundation_candidate_revision_hash"), "9" * 64),
    ],
)
def test_reference_plan_invocation_rejects_unknown_or_drifting_input(
    path: tuple[str, ...],
    value: object,
) -> None:
    payload: Any = deepcopy(valid_invocation().model_dump(mode="json"))
    target = payload
    for key in path[:-1]:
        target = target[key]
    target[path[-1]] = value

    with pytest.raises(ValidationError):
        ReferencePlanInvocation.model_validate(payload)


def test_reference_plan_dispatch_authorization_binds_attempt_and_expiry() -> None:
    invocation = valid_invocation()
    claims = valid_dispatch_claims(invocation)

    validate_reference_plan_dispatch_authorization(
        claims, invocation, claim_version=1, now_unix=100
    )
    with pytest.raises(ValueError):
        validate_reference_plan_dispatch_authorization(
            claims, invocation, claim_version=1, now_unix=200
        )


def test_reference_plan_attempt_result_validates_candidate_and_terminal_states() -> None:
    invocation = valid_invocation()
    authorization_hash = digest("reference-plan-dispatch")
    accepted = valid_accepted_result(invocation)

    accepted.validate_for(invocation, 1, authorization_hash)
    assert accepted.output_hash == (
        "9176e76d8c146a3e92899322b88e4a92e87002c3f17f94527f8cc174371d3ba1"
    )
    assert accepted.result_hash == (
        "291097ce9c77cc606450e28664f9db30f7b9a441175eeb03dc097d17eec5f039"
    )
    assert accepted.result_hash == accepted.compute_result_hash()

    unsafe = accepted.model_dump(mode="json")
    unsafe["candidate"]["provider"] = "seedream"
    unsafe["output_hash"] = production_canonical_hash(unsafe["candidate"])
    unsafe["result_hash"] = production_canonical_hash(
        {key: value for key, value in unsafe.items() if key != "result_hash"}
    )
    with pytest.raises((ValidationError, ValueError)):
        ReferencePlanAttemptResult.model_validate(unsafe)

    terminal_states: tuple[
        tuple[
            Literal["rejected", "outcome_unknown"],
            Literal["never", "same_release"],
        ],
        ...,
    ] = (("rejected", "never"), ("outcome_unknown", "same_release"))
    for status, retry_class in terminal_states:
        result = ReferencePlanAttemptResult.build(
            invocation_id=accepted.invocation_id,
            attempt_id=accepted.attempt_id,
            kind=accepted.kind,
            wire_schema_version=accepted.wire_schema_version,
            variant=accepted.variant,
            stage_release=accepted.stage_release,
            control=accepted.control,
            claim_version=accepted.claim_version,
            dispatch_authorization_hash=accepted.dispatch_authorization_hash,
            status=status,
            candidate_type=accepted.candidate_type,
            candidate=None,
            input_hash=accepted.input_hash,
            output_hash=None,
            diagnostics=accepted.diagnostics,
            diagnostic_hash=accepted.diagnostic_hash,
            completed_at=accepted.completed_at,
            executor=accepted.executor,
            error=SceneAnalysisResultError(
                code="model_failed",
                safe_summary="参考规划候选未生成。",
                retry_class=retry_class,
            ),
        )
        result.validate_for(invocation, 1, authorization_hash)
