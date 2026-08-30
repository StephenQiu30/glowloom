from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time
from pathlib import Path
from typing import Any, cast

import pytest

from app.candidate_runtime.grants import InvalidExecutionGrant, verify_execution_grant
from app.candidate_runtime.schemas import StoryGraphStageInvocation

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-stage-wire.json"
SECRET = "a-secure-agent-execution-secret-value"


def invocation() -> StoryGraphStageInvocation:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    return StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])


def token(request: StoryGraphStageInvocation, *, attempt: int = 1, fencing: int = 7) -> str:
    claims = {
        "invocation_id": str(request.invocation_id),
        "input_hash": request.input_hash,
        "execution_policy_hash": request.execution_policy.canonical_hash(),
        "expires_at": int(time.time()) + 60,
        "attempt": attempt,
        "fencing_token": fencing,
    }
    payload = (
        base64.urlsafe_b64encode(json.dumps(claims, separators=(",", ":")).encode("utf-8"))
        .decode("ascii")
        .rstrip("=")
    )
    signature = (
        base64.urlsafe_b64encode(
            hmac.new(SECRET.encode(), payload.encode("ascii"), hashlib.sha256).digest()
        )
        .decode("ascii")
        .rstrip("=")
    )
    return payload + "." + signature


def test_execution_grant_binds_storygraph_policy_attempt_and_fencing() -> None:
    request = invocation()
    verify_execution_grant(token(request), SECRET, request)
    with pytest.raises(InvalidExecutionGrant):
        verify_execution_grant(token(request) + "tampered", SECRET, request)
    with pytest.raises(InvalidExecutionGrant):
        verify_execution_grant(token(request, attempt=0), SECRET, request)
    with pytest.raises(InvalidExecutionGrant):
        verify_execution_grant(token(request, fencing=0), SECRET, request)
