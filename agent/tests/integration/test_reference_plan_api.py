from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time

import httpx
import pytest

from app.harness.reference_plan_schemas import ReferencePlanAttemptResult, ReferencePlanInvocation
from app.modules.storygraph.reference_plan_contract import ReferencePlanCandidate
from app.modules.storygraph.reference_plan_harness import ReferencePlanHarness
from app.reasoning.codex import CodexRuntimeUnavailable
from tests.contract.test_reference_plan_contract import valid_candidate
from tests.contract.test_reference_plan_wire_contract import valid_invocation

SECRET = "a-secure-agent-execution-secret-value"
DISPATCH_AUTHORIZATION_DOMAIN = "lanverse.scene-analysis.dispatch-authorization.production"


def token(request: ReferencePlanInvocation, *, attempt_id: str | None = None) -> str:
    claims = {
        "invocation_id": str(request.invocation_id),
        "attempt_id": attempt_id or str(request.attempt_id),
        "input_hash": request.input_hash,
        "skill_release_id": str(request.stage_release.skill_release_id),
        "skill_release_hash": request.stage_release.skill_release_hash,
        "stage_release_hash": request.stage_release.stage_release_hash,
        "bundle_content_hash": request.stage_release.bundle_content_hash,
        "control_hash": request.control.control_hash,
        "release_fence": request.control.release_fence,
        "claim_version": 1,
        "agent_image_digest": request.stage_release.agent_image_digest,
        "expires_at": int(time.time()) + 60,
    }
    raw = json.dumps(claims, separators=(",", ":")).encode()
    payload = base64.urlsafe_b64encode(raw).decode().rstrip("=")
    signature_input = hashlib.sha256(DISPATCH_AUTHORIZATION_DOMAIN.encode() + b"\0" + raw).digest()
    signature = (
        base64.urlsafe_b64encode(
            hmac.new(SECRET.encode(), signature_input, hashlib.sha256).digest()
        )
        .decode()
        .rstrip("=")
    )
    return payload + "." + signature


@pytest.mark.asyncio
async def test_reference_plan_api_returns_a_validated_candidate(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request = valid_invocation()
    candidate = ReferencePlanCandidate.model_validate(valid_candidate(request.payload.stage_input))

    async def execute(_: ReferencePlanHarness) -> ReferencePlanCandidate:
        return candidate

    monkeypatch.setattr(ReferencePlanHarness, "execute", execute)
    authorization = token(request)
    response = await client.post(
        "/internal/storygraph/reference-plan/invocations",
        json=request.model_dump(mode="json"),
        headers={"X-Lanverse-Dispatch-Authorization": authorization},
    )

    assert response.status_code == 200
    result = ReferencePlanAttemptResult.model_validate(response.json())
    assert result.status == "accepted"
    assert result.candidate_type == "reference_plan_candidate"
    result.validate_for(
        request,
        claim_version=1,
        dispatch_authorization_hash=hashlib.sha256(authorization.encode()).hexdigest(),
    )

    unauthorized = await client.post(
        "/internal/storygraph/reference-plan/invocations",
        json=request.model_dump(mode="json"),
        headers={
            "X-Lanverse-Dispatch-Authorization": token(
                request,
                attempt_id="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            )
        },
    )
    assert unauthorized.status_code == 401


@pytest.mark.asyncio
async def test_reference_plan_api_preserves_runtime_uncertainty(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)

    async def execute(_: ReferencePlanHarness) -> ReferencePlanCandidate:
        raise CodexRuntimeUnavailable("Codex CLI could not be started")

    monkeypatch.setattr(ReferencePlanHarness, "execute", execute)
    request = valid_invocation()
    response = await client.post(
        "/internal/storygraph/reference-plan/invocations",
        json=request.model_dump(mode="json"),
        headers={"X-Lanverse-Dispatch-Authorization": token(request)},
    )

    assert response.status_code == 200
    result = ReferencePlanAttemptResult.model_validate(response.json())
    assert result.status == "outcome_unknown"
    assert result.candidate is None
    assert result.error is not None
    assert result.error.retry_class == "same_release"
