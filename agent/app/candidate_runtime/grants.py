from __future__ import annotations

import base64
import hashlib
import hmac
import time

from pydantic import ValidationError

from app.candidate_runtime.scene_analysis_schemas import (
    SceneAnalysisExecutionGrantClaims,
    SceneAnalysisInvocation,
)
from app.candidate_runtime.schemas import (
    StoryGraphExecutionGrantClaims,
    StoryGraphStageInvocation,
)

MAX_TTL_SECONDS = 300


class InvalidExecutionGrant(ValueError):
    pass


def _decode(value: str) -> bytes:
    try:
        return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))
    except (ValueError, TypeError) as error:
        raise InvalidExecutionGrant("invalid execution grant encoding") from error


def verify_execution_grant(
    value: str,
    secret: str,
    invocation: StoryGraphStageInvocation,
) -> None:
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
        claims = StoryGraphExecutionGrantClaims.model_validate_json(_decode(parts[0]))
    except ValidationError as error:
        raise InvalidExecutionGrant("invalid execution grant payload") from error
    now = int(time.time())
    try:
        claims.validate_for(invocation, now_unix=now)
    except ValueError as error:
        raise InvalidExecutionGrant("execution grant does not authorize invocation") from error
    if claims.expires_at > now + MAX_TTL_SECONDS:
        raise InvalidExecutionGrant("execution grant expiry exceeds the maximum TTL")


def verify_scene_analysis_execution_grant(
    value: str,
    secret: str,
    invocation: SceneAnalysisInvocation,
) -> None:
    claims = _verify_signed_claims(value, secret, SceneAnalysisExecutionGrantClaims)
    now = int(time.time())
    try:
        claims.validate_for(invocation, now_unix=now)
    except ValueError as error:
        raise InvalidExecutionGrant("execution grant does not authorize invocation") from error
    if claims.expires_at > now + MAX_TTL_SECONDS:
        raise InvalidExecutionGrant("execution grant expiry exceeds the maximum TTL")


def _verify_signed_claims(
    value: str,
    secret: str,
    claims_model: type[SceneAnalysisExecutionGrantClaims],
) -> SceneAnalysisExecutionGrantClaims:
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
        return claims_model.model_validate_json(_decode(parts[0]))
    except ValidationError as error:
        raise InvalidExecutionGrant("invalid execution grant payload") from error
