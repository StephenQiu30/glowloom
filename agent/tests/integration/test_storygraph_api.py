from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time
from pathlib import Path
from typing import Any, cast

import httpx
import pytest

from app.candidate_runtime.schemas import StoryGraphStageInvocation
from app.modules.storygraph.candidate_schemas import SourceEvidenceCandidate
from app.modules.storygraph.harness import CodexRuntimeUnavailable, StoryGraphHarness

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-stage-wire.json"
SECRET = "a-secure-agent-execution-secret-value"


def invocation() -> StoryGraphStageInvocation:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    return StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])


def token(request: StoryGraphStageInvocation) -> str:
    claims = {
        "invocation_id": str(request.invocation_id),
        "input_hash": request.input_hash,
        "execution_policy_hash": request.execution_policy.canonical_hash(),
        "expires_at": int(time.time()) + 60,
        "attempt": 1,
        "fencing_token": 7,
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


def source_candidate() -> SourceEvidenceCandidate:
    return SourceEvidenceCandidate.model_validate(
        {
            "observations": [
                {
                    "observation_key": "observation-1",
                    "kind": "entity",
                    "proposed_key": "character:lin-yi",
                    "label": "林一",
                    "facts": ["林一发出开始指令"],
                    "evidence": [
                        {
                            "source_start": 0,
                            "source_end": 2,
                            "text_hash": "a" * 64,
                            "exact_anchor": "林一",
                            "episode_number": None,
                        }
                    ],
                    "ambiguities": [],
                }
            ],
            "review_issues": [],
        }
    )


@pytest.mark.asyncio
async def test_private_api_only_accepts_storygraph_stage_and_returns_a_hashed_candidate(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)

    async def execute(_: StoryGraphHarness) -> SourceEvidenceCandidate:
        return source_candidate()

    monkeypatch.setattr(StoryGraphHarness, "execute", execute)
    request = invocation()
    response = await client.post(
        "/internal/storygraph/invocations",
        json=request.model_dump(mode="json", exclude_none=True),
        headers={"X-Lanverse-Execution-Grant": token(request)},
    )
    assert response.status_code == 200
    result = response.json()
    assert result["kind"] == "storygraph_stage"
    assert result["candidate_type"] == "source_evidence_candidate"
    assert result["result_hash"]

    removed = request.model_dump(mode="json", exclude_none=True)
    removed["kind"] = "production_bible"
    response = await client.post(
        "/internal/storygraph/invocations",
        json=removed,
        headers={"X-Lanverse-Execution-Grant": "invalid"},
    )
    assert response.status_code == 422


@pytest.mark.asyncio
async def test_runtime_unavailable_is_unknown_and_never_an_empty_success(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)

    async def execute(_: StoryGraphHarness) -> SourceEvidenceCandidate:
        raise CodexRuntimeUnavailable("Codex CLI could not be started")

    monkeypatch.setattr(StoryGraphHarness, "execute", execute)
    request = invocation()
    response = await client.post(
        "/internal/storygraph/invocations",
        json=request.model_dump(mode="json", exclude_none=True),
        headers={"X-Lanverse-Execution-Grant": token(request)},
    )
    assert response.status_code == 200
    result = response.json()
    assert result["status"] == "unknown"
    assert result["candidate"] is None
    assert result["result_hash"] is None
    assert result["error"] == {
        "code": "runtime_unavailable",
        "summary": "Codex CLI could not be started",
        "retryable": True,
    }
