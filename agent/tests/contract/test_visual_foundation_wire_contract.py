from __future__ import annotations

import json
from copy import deepcopy
from datetime import UTC, datetime
from pathlib import Path
from typing import Any
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.harness.scene_analysis_schemas import (
    SceneAnalysisControlProof,
    SceneAnalysisDispatchAuthorizationClaims,
    SceneAnalysisExecutionBudget,
    SceneAnalysisReleaseIdentity,
    SceneAnalysisResultError,
)
from app.harness.visual_foundation_schemas import (
    VisualFoundationAttemptResult,
    VisualFoundationExecutor,
    VisualFoundationInvocation,
    VisualFoundationMediaAttachment,
    VisualFoundationPayload,
    VisualFoundationScope,
    VisualFoundationShard,
    VisualFoundationStageVariant,
    validate_visual_foundation_dispatch_authorization,
)
from app.modules.storygraph.bundle import SKILL_BUNDLE_HASH
from app.modules.storygraph.visual_foundation_contract import VisualFoundationInput
from app.protocol.canonical import production_canonical_hash
from tests.contract.test_visual_foundation_contract import digest, valid_candidate, valid_input

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
INVOCATION_FIXTURE = REPOSITORY_ROOT.joinpath(
    "backend",
    "tests",
    "fixtures",
    "agent",
    "storygraph-visual-foundation-invocation.json",
)
RESULT_FIXTURE = REPOSITORY_ROOT.joinpath(
    "backend",
    "tests",
    "fixtures",
    "agent",
    "storygraph-visual-foundation-result.json",
)


def valid_invocation() -> VisualFoundationInvocation:
    stage_input = VisualFoundationInput.model_validate(valid_input())
    return VisualFoundationInvocation.build(
        invocation_id=UUID("00000000-0000-0000-0000-000000000080"),
        attempt_id=UUID("00000000-0000-0000-0000-000000000090"),
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
        payload=VisualFoundationPayload(
            variant=VisualFoundationStageVariant(
                stage_key="resolve_visual_foundation",
                profile_key="default",
                lane_key="primary",
                output_schema_version="visual-foundation-candidate-production",
            ),
            scope=VisualFoundationScope(
                workspace_id=stage_input.workspace_id,
                project_id=stage_input.project_id,
            ),
            shard=VisualFoundationShard(
                manifest_id=UUID("00000000-0000-0000-0000-000000000040"),
                manifest_hash=digest("visual-shard"),
                shard_key=f"project:{stage_input.project_id}",
                impact_closure_hash=digest("visual-impact"),
            ),
            media_attachments=[
                VisualFoundationMediaAttachment(
                    attachment_id=stage_input.reference_attachments[0].attachment_id,
                    media_object_id=UUID("00000000-0000-0000-0000-000000000031"),
                    version_no=1,
                    purpose="style_reference",
                    object_key=stage_input.reference_attachments[0].object_key,
                    content_hash=stage_input.reference_attachments[0].content_hash,
                    media_type=stage_input.reference_attachments[0].media_type,
                    byte_length=1024,
                    pixel_width=1024,
                    pixel_height=1024,
                    page_count=1,
                    frame_count=1,
                    rights_basis=stage_input.reference_attachments[0].rights_basis,
                    rights_ref_hash=stage_input.reference_attachments[0].rights_ref_hash,
                    lineage_ref_hash=digest("visual-lineage"),
                )
            ],
            stage_input=stage_input,
        ),
    )


def valid_dispatch_claims(
    invocation: VisualFoundationInvocation,
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


def valid_accepted_result(
    invocation: VisualFoundationInvocation,
) -> VisualFoundationAttemptResult:
    candidate = valid_candidate(invocation.payload.stage_input)
    return VisualFoundationAttemptResult.build(
        invocation_id=invocation.invocation_id,
        attempt_id=invocation.attempt_id,
        kind="storygraph_stage",
        wire_schema_version="storygraph-stage-wire-production",
        variant=invocation.payload.variant,
        stage_release=invocation.stage_release,
        control=invocation.control,
        claim_version=1,
        dispatch_authorization_hash=digest("visual-dispatch"),
        status="accepted",
        candidate_type="visual_foundation_candidate",
        candidate=candidate,
        input_hash=invocation.input_hash,
        output_hash=production_canonical_hash(candidate),
        diagnostics=[],
        diagnostic_hash=production_canonical_hash([]),
        completed_at=datetime(2026, 9, 12, tzinfo=UTC),
        executor=VisualFoundationExecutor(
            runtime_class="vision",
            runtime_image_digest=invocation.stage_release.agent_image_digest,
            harness_version="visual-foundation-harness",
            model="codex-cli-default",
        ),
        error=None,
    )


def test_visual_foundation_invocation_freezes_project_media_and_input_hash() -> None:
    invocation = valid_invocation()
    fixture = json.loads(INVOCATION_FIXTURE.read_text(encoding="utf-8"))

    assert invocation.model_dump(mode="json") == fixture
    assert VisualFoundationInvocation.model_validate(fixture) == invocation
    assert invocation.input_hash == invocation.compute_input_hash()
    assert (
        invocation.input_hash == "4a766671538a285c6b11db1b39aade8b305d9423b82c084aa5c3e4216607f689"
    )
    assert invocation.stage_instance_key() == (
        "42a3b13d2d7f349a361fbdeaee2ac618cefb386b022073fca7c172859b0843aa"
    )
    assert invocation.payload.media_attachments[0].attachment_id == (
        invocation.payload.stage_input.reference_attachments[0].attachment_id
    )


@pytest.mark.parametrize(
    ("path", "value"),
    [
        (("unexpected",), True),
        (("payload", "scope", "project_id"), "00000000-0000-0000-0000-000000000099"),
        (("payload", "shard", "shard_key"), "project:00000000-0000-0000-0000-000000000099"),
        (("payload", "media_attachments", 0, "content_hash"), "9" * 64),
        (("payload", "media_attachments", 0, "byte_length"), 10 * 1024 * 1024 + 1),
    ],
)
def test_visual_foundation_invocation_rejects_unknown_or_drifting_input(
    path: tuple[str | int, ...],
    value: object,
) -> None:
    payload: Any = deepcopy(valid_invocation().model_dump(mode="json"))
    target = payload
    for key in path[:-1]:
        target = target[key]
    target[path[-1]] = value

    with pytest.raises(ValidationError):
        VisualFoundationInvocation.model_validate(payload)


def test_visual_foundation_dispatch_authorization_binds_attempt_and_expiry() -> None:
    invocation = valid_invocation()
    claims = valid_dispatch_claims(invocation)

    validate_visual_foundation_dispatch_authorization(
        claims, invocation, claim_version=1, now_unix=100
    )
    with pytest.raises(ValueError):
        validate_visual_foundation_dispatch_authorization(
            claims, invocation, claim_version=1, now_unix=200
        )


def test_visual_foundation_attempt_result_validates_candidate_and_terminal_states() -> None:
    invocation = valid_invocation()
    authorization_hash = digest("visual-dispatch")
    accepted = valid_accepted_result(invocation)
    fixture = json.loads(RESULT_FIXTURE.read_text(encoding="utf-8"))

    assert fixture == {
        "authorization_claims": valid_dispatch_claims(invocation).model_dump(mode="json"),
        "attempt_result": accepted.model_dump(mode="json"),
    }
    assert accepted.output_hash == (
        "3c89f9503800ac661d25bc41ee15127ee392ad32601329442c04177ac192c7eb"
    )
    assert accepted.result_hash == (
        "f773fbe1841f02d41cd9fea167360c32666418623f11f0644348997f8938fd0c"
    )
    accepted.validate_for(invocation, 1, authorization_hash)
    assert accepted.result_hash == accepted.compute_result_hash()

    unsafe = accepted.model_dump(mode="json")
    unsafe["candidate"]["approved"] = True
    unsafe["output_hash"] = production_canonical_hash(unsafe["candidate"])
    unsafe["result_hash"] = production_canonical_hash(
        {key: value for key, value in unsafe.items() if key != "result_hash"}
    )
    with pytest.raises((ValidationError, ValueError)):
        result = VisualFoundationAttemptResult.model_validate(unsafe)
        result.validate_for(invocation, 1, authorization_hash)

    terminal_states = (
        (
            "rejected",
            SceneAnalysisResultError(
                code="model_failed",
                safe_summary="视觉候选未生成。",
                retry_class="never",
            ),
        ),
        (
            "outcome_unknown",
            SceneAnalysisResultError(
                code="model_failed",
                safe_summary="视觉候选未生成。",
                retry_class="same_release",
            ),
        ),
    )
    for status, result_error in terminal_states:
        result = VisualFoundationAttemptResult.build(
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
            error=result_error,
        )
        result.validate_for(invocation, 1, authorization_hash)


def test_visual_foundation_attempt_result_rejects_unknown_fields() -> None:
    payload: Any = valid_accepted_result(valid_invocation()).model_dump(mode="json")
    payload["unexpected"] = True
    with pytest.raises(ValidationError):
        VisualFoundationAttemptResult.model_validate(payload)
