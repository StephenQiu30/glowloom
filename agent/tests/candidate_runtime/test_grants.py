from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time
from uuid import uuid4

import pytest

from app.candidate_runtime.grants import InvalidExecutionGrant, verify_execution_grant
from app.candidate_runtime.schemas import Invocation


def _grant(secret: str, invocation: Invocation, *, expires_at: int) -> str:
    payload = json.dumps(
        {
            "invocation_id": str(invocation.invocation_id),
            "kind": invocation.kind,
            "input_hash": invocation.input_hash,
            "expires_at": expires_at,
        },
        separators=(",", ":"),
    ).encode()
    encoded = base64.urlsafe_b64encode(payload).decode().rstrip("=")
    signature = (
        base64.urlsafe_b64encode(
            hmac.new(secret.encode(), encoded.encode("ascii"), hashlib.sha256).digest()
        )
        .decode()
        .rstrip("=")
    )
    return f"{encoded}.{signature}"


def test_execution_grant_binds_invocation_kind_and_hash() -> None:
    secret = "a-secure-agent-execution-secret-value"
    invocation = Invocation(
        invocation_id=uuid4(),
        kind="production_bible",
        input_hash="a" * 64,
        schema_version="agent-candidate-v1",
        payload={},
    )
    value = _grant(secret, invocation, expires_at=int(time.time()) + 120)

    verify_execution_grant(value, secret, invocation)

    changed = invocation.model_copy(update={"input_hash": "b" * 64})
    with pytest.raises(InvalidExecutionGrant):
        verify_execution_grant(value, secret, changed)


def test_execution_grant_rejects_excessive_ttl() -> None:
    secret = "a-secure-agent-execution-secret-value"
    invocation = Invocation(
        invocation_id=uuid4(),
        kind="production_bible",
        input_hash="a" * 64,
        schema_version="agent-candidate-v1",
        payload={},
    )
    value = _grant(secret, invocation, expires_at=int(time.time()) + 301)

    with pytest.raises(InvalidExecutionGrant):
        verify_execution_grant(value, secret, invocation)
