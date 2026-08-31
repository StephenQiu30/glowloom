from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time
from pathlib import Path

import httpx
import pytest

from app.candidate_runtime.scene_analysis_schemas import (
    SceneAnalysisAttemptResult,
    SceneAnalysisInvocation,
)
from app.modules.storygraph.harness import CodexRuntimeUnavailable
from app.modules.storygraph.scene_analysis_bundle import SCENE_ANALYSIS_SKILL_BUNDLE_HASH
from app.modules.storygraph.scene_analysis_candidates import ScriptSpanCandidate
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-scene-analysis-wire.json"
SECRET = "a-secure-agent-execution-secret-value"
DISPATCH_AUTHORIZATION_DOMAIN = "lanverse.scene-analysis.dispatch-authorization.production"


@pytest.mark.asyncio
async def test_health_reports_the_installed_scene_analysis_bundle(
    client: httpx.AsyncClient,
) -> None:
    response = await client.get("/healthz")

    assert response.status_code == 200
    assert response.json()["scene_analysis_skill_bundle_hash"] == (SCENE_ANALYSIS_SKILL_BUNDLE_HASH)


def fixture() -> dict[str, object]:
    return json.loads(WIRE_FIXTURE.read_text(encoding="utf-8"))


def invocation() -> SceneAnalysisInvocation:
    return SceneAnalysisInvocation.model_validate(fixture()["valid_invocation"])


def token(
    request: SceneAnalysisInvocation,
    *,
    attempt_id: str | None = None,
    domain_separated: bool = True,
) -> str:
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
    payload = (
        base64.urlsafe_b64encode(json.dumps(claims, separators=(",", ":")).encode())
        .decode()
        .rstrip("=")
    )
    signature_input = payload.encode()
    if domain_separated:
        signature_input = hashlib.sha256(
            DISPATCH_AUTHORIZATION_DOMAIN.encode()
            + b"\0"
            + base64.urlsafe_b64decode(payload + "=" * (-len(payload) % 4))
        ).digest()
    signature = (
        base64.urlsafe_b64encode(
            hmac.new(SECRET.encode(), signature_input, hashlib.sha256).digest()
        )
        .decode()
        .rstrip("=")
    )
    return payload + "." + signature


@pytest.mark.asyncio
async def test_scene_analysis_api_returns_an_immutable_accepted_candidate_result(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    candidate = ScriptSpanCandidate.model_validate(fixture()["valid_script_span_candidate"])

    async def execute(_: SceneAnalysisHarness) -> ScriptSpanCandidate:
        return candidate

    monkeypatch.setattr(SceneAnalysisHarness, "execute", execute)
    request = invocation()
    authorization = token(request)
    response = await client.post(
        "/internal/storygraph/scene-analysis/invocations",
        json=request.model_dump(mode="json"),
        headers={"X-Lanverse-Dispatch-Authorization": authorization},
    )
    assert response.status_code == 200
    result = response.json()
    assert result["status"] == "accepted"
    assert result["attempt_id"] == str(request.attempt_id)
    assert result["candidate_type"] == "script_span_candidate"
    assert result["output_hash"]
    assert result["result_hash"]
    assert (
        SceneAnalysisAttemptResult.model_validate(result).compute_result_hash()
        == result["result_hash"]
    )
    assert result["diagnostic_hash"]
    assert result["claim_version"] == 1
    assert (
        result["dispatch_authorization_hash"] == hashlib.sha256(authorization.encode()).hexdigest()
    )
    assert result["error"] is None

    unauthorized = await client.post(
        "/internal/storygraph/scene-analysis/invocations",
        json=request.model_dump(mode="json"),
        headers={
            "X-Lanverse-Dispatch-Authorization": token(
                request, attempt_id="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
            )
        },
    )
    assert unauthorized.status_code == 401

    legacy_authorization = await client.post(
        "/internal/storygraph/scene-analysis/invocations",
        json=request.model_dump(mode="json"),
        headers={
            "X-Lanverse-Dispatch-Authorization": token(
                request,
                domain_separated=False,
            )
        },
    )
    assert legacy_authorization.status_code == 401


@pytest.mark.asyncio
async def test_scene_analysis_api_rejects_malformed_dispatch_authorization(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)

    response = await client.post(
        "/internal/storygraph/scene-analysis/invocations",
        json=invocation().model_dump(mode="json"),
        headers={"X-Lanverse-Dispatch-Authorization": "a.signature"},
    )

    assert response.status_code == 401
    assert response.json() == {"detail": "invalid dispatch authorization"}


@pytest.mark.asyncio
async def test_scene_analysis_api_never_turns_runtime_uncertainty_into_empty_acceptance(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)

    async def execute(_: SceneAnalysisHarness) -> ScriptSpanCandidate:
        raise CodexRuntimeUnavailable("Codex CLI could not be started")

    monkeypatch.setattr(SceneAnalysisHarness, "execute", execute)
    request = invocation()
    authorization = token(request)
    response = await client.post(
        "/internal/storygraph/scene-analysis/invocations",
        json=request.model_dump(mode="json"),
        headers={"X-Lanverse-Dispatch-Authorization": authorization},
    )
    assert response.status_code == 200
    result = response.json()
    assert result["status"] == "outcome_unknown"
    assert result["candidate"] is None
    assert result["output_hash"] is None
    assert result["result_hash"]
    assert (
        SceneAnalysisAttemptResult.model_validate(result).compute_result_hash()
        == result["result_hash"]
    )
    assert result["claim_version"] == 1
    assert (
        result["dispatch_authorization_hash"] == hashlib.sha256(authorization.encode()).hexdigest()
    )
    assert result["error"]["retry_class"] == "same_release"
