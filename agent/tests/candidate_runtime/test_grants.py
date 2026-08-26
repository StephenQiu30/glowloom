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
            "66f9586bc95df6f1714735b115e09d9122658bf720fb712a5ef36d2b9ba78b99",
        ),
        (
            "storyboard_draft",
            "bc6030d16816445388853a7ae4dd0f65dea8de4e57a4c969e611afd67f3ed5b8",
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
