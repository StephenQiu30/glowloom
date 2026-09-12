from __future__ import annotations

from copy import deepcopy
from datetime import UTC, datetime
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.harness.reference_brief_schemas import (
    ReferenceBriefAttemptResult,
    ReferenceBriefExecutor,
    ReferenceBriefInvocation,
    ReferenceBriefPayload,
    ReferenceBriefScope,
    ReferenceBriefShard,
    ReferenceBriefStageVariant,
    validate_reference_brief_dispatch_authorization,
)
from app.harness.scene_analysis_schemas import (
    SceneAnalysisControlProof,
    SceneAnalysisDispatchAuthorizationClaims,
    SceneAnalysisExecutionBudget,
    SceneAnalysisReleaseIdentity,
)
from app.modules.storygraph.bundle import SKILL_BUNDLE_HASH
from app.modules.storygraph.reference_brief_contract import ReferenceBriefInput
from app.protocol.canonical import production_canonical_hash
from tests.contract.test_reference_brief_contract import (
    digest,
    reference_brief_candidate,
    reference_brief_input,
)


def valid_invocation() -> ReferenceBriefInvocation:
    stage_input = ReferenceBriefInput.model_validate(
        reference_brief_input(reference_brief_candidate("character_appearance"))
    )
    return ReferenceBriefInvocation.build(
        invocation_id=UUID("00000000-0000-0000-0000-000000000680"),
        attempt_id=UUID("00000000-0000-0000-0000-000000000690"),
        stage_release=SceneAnalysisReleaseIdentity(
            skill_release_id=UUID("66666666-6666-4666-8666-666666666666"),
            skill_release_hash="1" * 64,
            stage_release_hash=stage_input.stage_release.stage_release_hash,
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
        payload=ReferenceBriefPayload(
            variant=ReferenceBriefStageVariant(
                stage_key="compile_reference_brief",
                profile_key="default",
                lane_key="primary",
                output_schema_version="reference-brief-candidate-production",
            ),
            scope=ReferenceBriefScope(
                workspace_id=stage_input.workspace_id,
                project_id=stage_input.project_id,
                target_business_key=stage_input.target_business_key,
            ),
            shard=ReferenceBriefShard(
                manifest_id=UUID("00000000-0000-0000-0000-000000000640"),
                manifest_hash="6" * 64,
                shard_key=f"reference_target:{stage_input.target_business_key}",
                impact_closure_hash="7" * 64,
            ),
            stage_input=stage_input,
        ),
    )


def test_reference_brief_invocation_freezes_target_input_and_release() -> None:
    invocation = valid_invocation()
    assert invocation.input_hash == invocation.compute_input_hash()
    assert invocation.stage_instance_key()

    drifted = invocation.model_dump(mode="json")
    drifted["payload"]["stage_input"]["stage_release"]["stage_release_hash"] = "9" * 64
    with pytest.raises(ValidationError):
        ReferenceBriefInvocation.model_validate(drifted)


def test_reference_brief_invocation_rejects_unknown_or_drifting_target() -> None:
    payload = valid_invocation().model_dump(mode="json")
    payload["provider"] = "seedream"
    with pytest.raises(ValidationError):
        ReferenceBriefInvocation.model_validate(payload)

    payload = valid_invocation().model_dump(mode="json")
    payload["payload"]["scope"]["target_business_key"] = "wrong-target"
    with pytest.raises(ValidationError):
        ReferenceBriefInvocation.model_validate(payload)


def test_reference_brief_dispatch_authorization_binds_attempt_and_expiry() -> None:
    invocation = valid_invocation()
    claims = SceneAnalysisDispatchAuthorizationClaims(
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
    validate_reference_brief_dispatch_authorization(
        claims, invocation, claim_version=1, now_unix=100
    )
    with pytest.raises(ValueError):
        validate_reference_brief_dispatch_authorization(
            claims, invocation, claim_version=1, now_unix=200
        )


def test_reference_brief_attempt_result_revalidates_frozen_candidate() -> None:
    invocation = valid_invocation()
    candidate = reference_brief_candidate("character_appearance")
    authorization_hash = digest("reference-brief-dispatch")
    accepted = ReferenceBriefAttemptResult.build(
        invocation_id=invocation.invocation_id,
        attempt_id=invocation.attempt_id,
        kind="storygraph_stage",
        wire_schema_version="storygraph-stage-wire-production",
        variant=invocation.payload.variant,
        stage_release=invocation.stage_release,
        control=invocation.control,
        claim_version=1,
        dispatch_authorization_hash=authorization_hash,
        status="accepted",
        candidate_type="reference_brief_candidate",
        candidate=candidate,
        input_hash=invocation.input_hash,
        output_hash=production_canonical_hash(candidate),
        diagnostics=[],
        diagnostic_hash=production_canonical_hash([]),
        completed_at=datetime(2026, 9, 12, tzinfo=UTC),
        executor=ReferenceBriefExecutor(
            runtime_class="text",
            runtime_image_digest=invocation.stage_release.agent_image_digest,
            harness_version="reference-brief-harness",
            model="codex-cli-default",
        ),
        error=None,
    )
    accepted.validate_for(invocation, 1, authorization_hash)

    stale = deepcopy(candidate)
    stale["typed_read_set_root"] = "f" * 64
    payload = accepted.model_dump(mode="json")
    payload["candidate"] = stale
    payload["output_hash"] = production_canonical_hash(stale)
    payload["result_hash"] = production_canonical_hash(
        {key: value for key, value in payload.items() if key != "result_hash"}
    )
    stale_result = ReferenceBriefAttemptResult.model_validate(payload)
    with pytest.raises(ValueError):
        stale_result.validate_for(invocation, 1, authorization_hash)
