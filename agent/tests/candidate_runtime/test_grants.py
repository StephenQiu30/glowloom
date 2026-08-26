from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time
from uuid import uuid4

import pytest

from app.candidate_runtime.grants import InvalidExecutionGrant, verify_execution_grant
from app.candidate_runtime.schemas import Invocation, execution_policy_for


@pytest.mark.parametrize(
    ("kind", "expected_hash"),
    [
        (
            "production_bible",
            "6f2a808344083bdcdc0d542d94861bb25511f8373a48958c3e0c02f46c3f15a2",
        ),
        (
            "storyboard_draft",
            "a36be6c82351d8628536721d842316817495c8c43ff5f34662cef2516aa09a0b",
        ),
    ],
)
def test_execution_policy_hash_matches_backend_contract(kind: str, expected_hash: str) -> None:
    assert execution_policy_for(kind).canonical_hash() == expected_hash


def _grant(secret: str, invocation: Invocation, *, expires_at: int) -> str:
    payload = json.dumps(
        {
            "invocation_id": str(invocation.invocation_id),
            "kind": invocation.kind,
            "input_hash": invocation.input_hash,
            "execution_policy_hash": invocation.execution_policy.canonical_hash(),
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
        execution_policy=execution_policy_for("production_bible"),
        payload={},
    )
    value = _grant(secret, invocation, expires_at=int(time.time()) + 120)

    verify_execution_grant(value, secret, invocation)

    changed = invocation.model_copy(update={"input_hash": "b" * 64})
    with pytest.raises(InvalidExecutionGrant):
        verify_execution_grant(value, secret, changed)

    changed_policy = invocation.execution_policy.model_copy(update={"max_model_calls": 2})
    changed = invocation.model_copy(update={"execution_policy": changed_policy})
    with pytest.raises(InvalidExecutionGrant):
        verify_execution_grant(value, secret, changed)

    changed_policy = invocation.execution_policy.model_copy(
        update={"max_execution_seconds": invocation.execution_policy.max_execution_seconds - 1}
    )
    changed = invocation.model_copy(update={"execution_policy": changed_policy})
    with pytest.raises(InvalidExecutionGrant):
        verify_execution_grant(value, secret, changed)


def test_execution_grant_rejects_excessive_ttl() -> None:
    secret = "a-secure-agent-execution-secret-value"
    invocation = Invocation(
        invocation_id=uuid4(),
        kind="production_bible",
        input_hash="a" * 64,
        schema_version="agent-candidate-v1",
        execution_policy=execution_policy_for("production_bible"),
        payload={},
    )
    value = _grant(secret, invocation, expires_at=int(time.time()) + 301)

    with pytest.raises(InvalidExecutionGrant):
        verify_execution_grant(value, secret, invocation)
