from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time
from typing import Any

from app.candidate_runtime.schemas import Invocation

MAX_TTL_SECONDS = 300


class InvalidExecutionGrant(ValueError):
    pass


def _decode(value: str) -> bytes:
    try:
        return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))
    except (ValueError, TypeError) as error:
        raise InvalidExecutionGrant("invalid execution grant encoding") from error


def verify_execution_grant(value: str, secret: str, invocation: Invocation) -> None:
    if len(secret.encode("utf-8")) < 32:
        raise InvalidExecutionGrant("agent execution secret must contain at least 32 bytes")
    parts = value.split(".")
    if len(parts) != 2:
        raise InvalidExecutionGrant("invalid execution grant format")
    expected = (
        base64.urlsafe_b64encode(
            hmac.new(secret.encode("utf-8"), parts[0].encode("ascii"), hashlib.sha256).digest()
        )
        .decode("ascii")
        .rstrip("=")
    )
    if not hmac.compare_digest(parts[1], expected):
        raise InvalidExecutionGrant("invalid execution grant signature")
    try:
        claims: dict[str, Any] = json.loads(_decode(parts[0]))
        expires_at = int(claims["expires_at"])
    except (KeyError, TypeError, ValueError, json.JSONDecodeError) as error:
        raise InvalidExecutionGrant("invalid execution grant payload") from error
    now = int(time.time())
    if (
        claims.get("invocation_id") != str(invocation.invocation_id)
        or claims.get("kind") != invocation.kind
        or claims.get("input_hash") != invocation.input_hash
        or expires_at <= now
        or expires_at > now + MAX_TTL_SECONDS
    ):
        raise InvalidExecutionGrant("execution grant does not authorize invocation")
