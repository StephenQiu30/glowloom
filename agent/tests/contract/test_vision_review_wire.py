from __future__ import annotations

import base64
import hashlib
import hmac
import json
from datetime import UTC, datetime
from pathlib import Path
from typing import Any
from uuid import UUID

import pytest

from app.harness.grants import (
    InvalidSceneAnalysisDispatchAuthorization,
    verify_vision_review_dispatch_authorization,
)
from app.harness.vision_review_schemas import (
    VisionReviewAttemptResult,
    VisionReviewExecutor,
    VisionReviewInvocation,
    VisionReviewPayload,
    VisionReviewScope,
    VisionReviewShard,
    VisionReviewStageVariant,
    decode_vision_review_attempt_result,
    decode_vision_review_invocation,
)
from app.modules.storygraph.vision_review_input import decode_vision_review_input
from app.protocol.canonical import production_canonical_hash
from tests.contract.test_reference_brief_wire_contract import valid_invocation as brief_invocation
from tests.contract.test_vision_review_contract import review_document

FIXTURES = Path(__file__).resolve().parents[3] / "backend/tests/agent/testdata"


def wire_fixture() -> dict[str, Any]:
    fixture = json.loads((FIXTURES / "vision_review_wire.json").read_text())
    authorization = fixture["authorization"]
    payload = json.dumps(authorization["claims"], separators=(",", ":")).encode()
    signature = bytes.fromhex(authorization["signature_hex"])
    authorization["value"] = (
        base64.urlsafe_b64encode(payload).decode().rstrip("=")
        + "."
        + base64.urlsafe_b64encode(signature).decode().rstrip("=")
    )
    return fixture


def resign(value: dict[str, Any], field: str) -> None:
    if field == "input_hash":
        material = {
            key: value[key]
            for key in ("wire_schema_version", "stage_release", "control", "budget", "payload")
        }
    else:
        material = {key: item for key, item in value.items() if key != field}
    value[field] = production_canonical_hash(material)


def valid_invocation() -> VisionReviewInvocation:
    root = Path(__file__).resolve().parents[3]
    stage_input = decode_vision_review_input(
        (root / "backend/tests/agent/testdata/vision_review_input.json").read_bytes()
    )
    brief = brief_invocation()
    return VisionReviewInvocation.build(
        invocation_id=brief.invocation_id,
        attempt_id=brief.attempt_id,
        stage_release=brief.stage_release.model_copy(
            update={"stage_release_hash": stage_input.subject.stage_release_hash}
        ),
        control=brief.control,
        budget=brief.budget,
        payload=VisionReviewPayload(
            variant=VisionReviewStageVariant(
                stage_key="review_reference_artifact",
                profile_key="default",
                lane_key="primary",
                output_schema_version="vision-review-candidate-production",
            ),
            scope=VisionReviewScope(
                workspace_id=UUID(stage_input.subject.workspace_id),
                project_id=UUID(stage_input.subject.project_id),
                bundle_input_id=UUID(stage_input.subject.bundle_input_ref.id),
            ),
            shard=VisionReviewShard(
                manifest_id=brief.payload.shard.manifest_id,
                manifest_hash=brief.payload.shard.manifest_hash,
                shard_key=f"vision_bundle:{stage_input.subject.bundle_input_ref.id}",
                impact_closure_hash=brief.payload.shard.impact_closure_hash,
            ),
            stage_input=stage_input,
        ),
    )


def test_vision_invocation_separates_content_and_envelope_hashes() -> None:
    value = valid_invocation()
    assert value.input_hash != value.payload.stage_input.subject.input_hash
    assert VisionReviewInvocation.model_validate_json(value.model_dump_json()) == value
    assert value.stage_instance_key()
    assert json.loads(value.model_dump_json())["payload"]["stage_input"]["subject"]["input_hash"]


@pytest.mark.parametrize(
    "path,replacement",
    [
        (("invocation_id",), "00000000-0000-0000-0000-000000000000"),
        (("attempt_id",), "00000000000000000000000000000690"),
        (("payload", "scope", "bundle_input_id"), "00000000-0000-4000-8000-000000000099"),
        (("payload", "shard", "manifest_id"), "00000000-0000-0000-0000-000000000000"),
        (("payload", "shard", "shard_key"), "vision_bundle:other"),
        (("payload", "variant", "stage_key"), "compile_reference_brief"),
        (("stage_release", "stage_release_hash"), "e" * 64),
        (("budget", "max_model_calls"), 2),
        (("budget", "max_execution_seconds"), 121),
        (("budget", "max_output_bytes"), 131073),
        (("payload", "stage_input", "subject", "input_hash"), "f" * 64),
        (
            (
                "payload",
                "stage_input",
                "attachments",
            ),
            [],
        ),
    ],
)
def test_vision_invocation_rejects_drift_even_after_rehash(
    path: tuple[str, ...],
    replacement: Any,
) -> None:
    value = valid_invocation().model_dump(mode="json")
    target = value
    for key in path[:-1]:
        target = target[key]
    target[path[-1]] = replacement
    resign(value, "input_hash")
    with pytest.raises(ValueError):
        VisionReviewInvocation.model_validate(value)


def accepted_result(invocation: VisionReviewInvocation) -> VisionReviewAttemptResult:
    candidate = review_document()
    candidate["subject"] = invocation.payload.stage_input.subject.model_dump(mode="json")
    return VisionReviewAttemptResult.build(
        invocation_id=invocation.invocation_id,
        attempt_id=invocation.attempt_id,
        kind="storygraph_stage",
        wire_schema_version=invocation.wire_schema_version,
        variant=invocation.payload.variant,
        stage_release=invocation.stage_release,
        control=invocation.control,
        claim_version=1,
        dispatch_authorization_hash="8" * 64,
        status="accepted",
        candidate_type="vision_review_candidate",
        candidate=candidate,
        input_hash=invocation.input_hash,
        output_hash=production_canonical_hash(candidate),
        diagnostics=[],
        diagnostic_hash=production_canonical_hash([]),
        completed_at=datetime(2026, 9, 13, tzinfo=UTC),
        executor=VisionReviewExecutor(
            runtime_class="vision",
            runtime_image_digest=invocation.stage_release.agent_image_digest,
            harness_version="vision-review-harness",
            model="codex-cli-default",
        ),
        error=None,
    )


@pytest.mark.parametrize(
    "field",
    [
        "attempt_id",
        "input_hash",
        "stage_release",
        "subject",
        "claim_version",
        "dispatch_authorization_hash",
    ],
)
def test_vision_result_rejects_other_execution_even_after_rehash(field: str) -> None:
    invocation = valid_invocation()
    result = accepted_result(invocation)
    result.validate_for(invocation, 1, "8" * 64)
    value = result.model_dump(mode="json")
    if field == "subject":
        value["candidate"]["subject"]["input_hash"] = "f" * 64
        value["output_hash"] = production_canonical_hash(value["candidate"])
    elif field == "stage_release":
        value[field]["stage_release_hash"] = "e" * 64
    elif field == "attempt_id":
        value[field] = "00000000-0000-4000-8000-000000000099"
    else:
        value[field] = 2 if field == "claim_version" else "f" * 64
    resign(value, "result_hash")
    with pytest.raises(ValueError):
        VisionReviewAttemptResult.model_validate(value).validate_for(invocation, 1, "8" * 64)


def test_vision_dispatch_authorization_accepts_shared_signed_fixture(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fixture = wire_fixture()
    invocation = VisionReviewInvocation.model_validate(fixture["invocation"])
    monkeypatch.setattr("app.harness.grants.time.time", lambda: 100)
    evidence = verify_vision_review_dispatch_authorization(
        fixture["authorization"]["value"],
        fixture["test_secret"],
        invocation,
    )
    assert evidence.authorization_hash == fixture["authorization"]["hash"]
    assert evidence.claim_version == 1
    assert evidence.expires_at == 400
    assert invocation.input_hash == valid_invocation().input_hash
    assert invocation.stage_instance_key() == fixture["stage_instance_key"]
    result = VisionReviewAttemptResult.model_validate(fixture["accepted_result"])
    result.validate_for(invocation, 1, evidence.authorization_hash)
    monkeypatch.setattr("app.harness.grants.time.time", lambda: 400)
    with pytest.raises(InvalidSceneAnalysisDispatchAuthorization):
        verify_vision_review_dispatch_authorization(
            fixture["authorization"]["value"],
            fixture["test_secret"],
            invocation,
        )


@pytest.mark.parametrize("field", ["output_hash", "error", "diagnostics", "claim_version"])
def test_vision_result_requires_explicit_nullable_and_zero_fields(field: str) -> None:
    value = accepted_result(valid_invocation()).model_dump(mode="json")
    del value[field]
    with pytest.raises(ValueError):
        decode_vision_review_attempt_result(json.dumps(value))


@pytest.mark.parametrize("mutation", ["unknown", "missing_zero", "null_zero", "duplicate"])
def test_vision_invocation_requires_closed_canonical_wire(mutation: str) -> None:
    value = valid_invocation().model_dump(mode="json")
    if mutation == "unknown":
        value["provider"] = "unexpected"
    elif mutation == "missing_zero":
        del value["control"]["release_fence"]
    elif mutation == "null_zero":
        value["control"]["release_fence"] = None
    raw = json.dumps(value)
    if mutation == "duplicate":
        raw = '{"input_hash":"' + value["input_hash"] + '",' + raw[1:]
    with pytest.raises(ValueError):
        decode_vision_review_invocation(raw)


@pytest.mark.parametrize("status", ["rejected", "outcome_unknown"])
def test_vision_failure_preserves_unknown_without_candidate(status: str) -> None:
    invocation = valid_invocation()
    value = accepted_result(invocation).model_dump(mode="json")
    value.update(
        status=status,
        candidate=None,
        output_hash=None,
        error={
            "code": "execution_failed",
            "safe_summary": "执行未取得可信结果",
            "retry_class": "never" if status == "rejected" else "same_release",
        },
    )
    resign(value, "result_hash")
    result = decode_vision_review_attempt_result(json.dumps(value))
    result.validate_for(invocation, 1, "8" * 64)
    del value["output_hash"]
    with pytest.raises(ValueError):
        decode_vision_review_attempt_result(json.dumps(value))


@pytest.mark.parametrize(
    "mutation",
    [
        "signature",
        "attempt",
        "claim",
        "image",
        "control",
        "budget",
        "inner_hash",
        "ttl",
        "short_secret",
    ],
)
def test_vision_dispatch_rejects_reuse_or_invalid_authority(
    mutation: str,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fixture = wire_fixture()
    request = fixture["invocation"]
    auth = fixture["authorization"]["value"]
    secret = fixture["test_secret"]
    monkeypatch.setattr("app.harness.grants.time.time", lambda: 100)
    if mutation == "signature":
        auth += "x"
    elif mutation == "attempt":
        request["attempt_id"] = "00000000-0000-4000-8000-000000000099"
    elif mutation == "image":
        request["stage_release"]["agent_image_digest"] = "sha256:" + "f" * 64
    elif mutation == "control":
        request["control"]["release_fence"] += 1
    elif mutation == "budget":
        request["budget"]["max_execution_seconds"] -= 1
    elif mutation == "ttl":
        monkeypatch.setattr("app.harness.grants.time.time", lambda: 99)
    elif mutation == "short_secret":
        secret = "short"
    elif mutation == "claim":
        # Re-sign a changed claim, so this exercises semantic validation, not HMAC failure.
        claims = json.loads(base64.urlsafe_b64decode(auth.split(".")[0] + "=="))
        claims["claim_version"] = 0
        payload = json.dumps(claims, separators=(",", ":")).encode()
        encoded = base64.urlsafe_b64encode(payload).decode().rstrip("=")
        domain = b"lanverse.scene-analysis.dispatch-authorization.production\0"
        signature = hmac.new(
            secret.encode(), hashlib.sha256(domain + payload).digest(), hashlib.sha256
        ).digest()
        auth = encoded + "." + base64.urlsafe_b64encode(signature).decode().rstrip("=")
    resign(request, "input_hash")
    invocation = VisionReviewInvocation.model_validate(request)
    if mutation == "inner_hash":
        invocation.input_hash = invocation.payload.stage_input.subject.input_hash
    with pytest.raises(InvalidSceneAnalysisDispatchAuthorization):
        verify_vision_review_dispatch_authorization(auth, secret, invocation)
