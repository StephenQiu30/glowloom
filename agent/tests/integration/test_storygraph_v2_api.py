from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time
from pathlib import Path

import httpx
import pytest

from app.candidate_runtime.v2_schemas import StoryGraphV2Invocation
from app.modules.storygraph.harness import CodexRuntimeUnavailable
from app.modules.storygraph.v2_candidate_schemas import ScriptSpanCandidateV2
from app.modules.storygraph.v2_harness import StoryGraphV2Harness

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-stage-wire-v2.json"
SECRET = "a-secure-agent-execution-secret-value"


def fixture() -> dict[str, object]:
    return json.loads(WIRE_FIXTURE.read_text(encoding="utf-8"))


def invocation() -> StoryGraphV2Invocation:
    return StoryGraphV2Invocation.model_validate(fixture()["valid_invocation"])


def token(request: StoryGraphV2Invocation, *, attempt_id: str | None = None) -> str:
    claims = {
        "invocation_id": str(request.invocation_id),
        "attempt_id": attempt_id or str(request.attempt_id),
        "input_hash": request.input_hash,
        "stage_release_id": str(request.stage_release.release_id),
        "agent_image_digest": request.stage_release.agent_image_digest,
        "expires_at": int(time.time()) + 60,
    }
    payload = (
        base64.urlsafe_b64encode(json.dumps(claims, separators=(",", ":")).encode())
        .decode()
        .rstrip("=")
    )
    signature = (
        base64.urlsafe_b64encode(
            hmac.new(SECRET.encode(), payload.encode(), hashlib.sha256).digest()
        )
        .decode()
        .rstrip("=")
    )
    return payload + "." + signature


@pytest.mark.asyncio
async def test_v2_api_returns_an_immutable_accepted_candidate_result(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    candidate = ScriptSpanCandidateV2.model_validate(fixture()["valid_script_span_candidate"])

    async def execute(_: StoryGraphV2Harness) -> ScriptSpanCandidateV2:
        return candidate

    monkeypatch.setattr(StoryGraphV2Harness, "execute", execute)
    request = invocation()
    response = await client.post(
        "/internal/v2/invocations",
        json=request.model_dump(mode="json"),
        headers={"X-Lanverse-Execution-Grant": token(request)},
    )
    assert response.status_code == 200
    result = response.json()
    assert result["status"] == "accepted"
    assert result["attempt_id"] == str(request.attempt_id)
    assert result["candidate_type"] == "script_span_candidate_v2"
    assert result["output_hash"]
    assert result["diagnostic_hash"]
    assert result["error"] is None

    unauthorized = await client.post(
        "/internal/v2/invocations",
        json=request.model_dump(mode="json"),
        headers={
            "X-Lanverse-Execution-Grant": token(
                request, attempt_id="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
            )
        },
    )
    assert unauthorized.status_code == 401


@pytest.mark.asyncio
async def test_v2_api_never_turns_runtime_uncertainty_into_empty_acceptance(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)

    async def execute(_: StoryGraphV2Harness) -> ScriptSpanCandidateV2:
        raise CodexRuntimeUnavailable("Codex CLI could not be started")

    monkeypatch.setattr(StoryGraphV2Harness, "execute", execute)
    request = invocation()
    response = await client.post(
        "/internal/v2/invocations",
        json=request.model_dump(mode="json"),
        headers={"X-Lanverse-Execution-Grant": token(request)},
    )
    assert response.status_code == 200
    result = response.json()
    assert result["status"] == "outcome_unknown"
    assert result["candidate"] is None
    assert result["output_hash"] is None
    assert result["error"]["retry_class"] == "same_release"
