from __future__ import annotations

import base64
import hashlib
import hmac
import time
from dataclasses import dataclass

from pydantic import ValidationError

from app.harness.reference_plan_schemas import (
    ReferencePlanInvocation,
    validate_reference_plan_dispatch_authorization,
)
from app.harness.scene_analysis_schemas import (
    SceneAnalysisDispatchAuthorizationClaims,
    SceneAnalysisInvocation,
)
from app.harness.schemas import (
    StoryGraphExecutionGrantClaims,
    StoryGraphStageInvocation,
)
from app.harness.visual_foundation_schemas import (
    VisualFoundationInvocation,
    validate_visual_foundation_dispatch_authorization,
)

MAX_TTL_SECONDS = 300
SCENE_ANALYSIS_DISPATCH_AUTHORIZATION_DOMAIN = (
    "lanverse.scene-analysis.dispatch-authorization.production"
)


class InvalidExecutionGrant(ValueError):
    pass


class InvalidSceneAnalysisDispatchAuthorization(ValueError):
    pass


@dataclass(frozen=True)
class SceneAnalysisDispatchAuthorizationEvidence:
    claim_version: int
    authorization_hash: str
    expires_at: int


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


def verify_scene_analysis_dispatch_authorization(
    value: str,
    secret: str,
    invocation: SceneAnalysisInvocation,
) -> SceneAnalysisDispatchAuthorizationEvidence:
    claims, evidence, now = _verify_dispatch_authorization(value, secret)
    try:
        claims.validate_for(
            invocation,
            now_unix=now,
        )
    except ValueError as error:
        raise InvalidSceneAnalysisDispatchAuthorization(
            "dispatch authorization does not authorize invocation"
        ) from error
    return evidence


def verify_visual_foundation_dispatch_authorization(
    value: str,
    secret: str,
    invocation: VisualFoundationInvocation,
) -> SceneAnalysisDispatchAuthorizationEvidence:
    claims, evidence, now = _verify_dispatch_authorization(value, secret)
    try:
        validate_visual_foundation_dispatch_authorization(
            claims,
            invocation,
            claim_version=claims.claim_version,
            now_unix=now,
        )
    except ValueError as error:
        raise InvalidSceneAnalysisDispatchAuthorization(
            "dispatch authorization does not authorize invocation"
        ) from error
    return evidence


def verify_reference_plan_dispatch_authorization(
    value: str,
    secret: str,
    invocation: ReferencePlanInvocation,
) -> SceneAnalysisDispatchAuthorizationEvidence:
    claims, evidence, now = _verify_dispatch_authorization(value, secret)
    try:
        validate_reference_plan_dispatch_authorization(
            claims,
            invocation,
            claim_version=claims.claim_version,
            now_unix=now,
        )
    except ValueError as error:
        raise InvalidSceneAnalysisDispatchAuthorization(
            "dispatch authorization does not authorize invocation"
        ) from error
    return evidence


def _verify_dispatch_authorization(
    value: str,
    secret: str,
) -> tuple[
    SceneAnalysisDispatchAuthorizationClaims,
    SceneAnalysisDispatchAuthorizationEvidence,
    int,
]:
    if len(secret.encode("utf-8")) < 32:
        raise InvalidSceneAnalysisDispatchAuthorization(
            "agent execution secret must contain at least 32 bytes"
        )
    parts = value.split(".")
    if len(parts) != 2:
        raise InvalidSceneAnalysisDispatchAuthorization("invalid dispatch authorization format")
    try:
        payload = _decode(parts[0])
    except InvalidExecutionGrant as error:
        raise InvalidSceneAnalysisDispatchAuthorization(
            "invalid dispatch authorization encoding"
        ) from error
    expected = (
        base64.urlsafe_b64encode(
            hmac.new(
                secret.encode("utf-8"),
                hashlib.sha256(
                    SCENE_ANALYSIS_DISPATCH_AUTHORIZATION_DOMAIN.encode("ascii") + b"\0" + payload
                ).digest(),
                hashlib.sha256,
            ).digest()
        )
        .decode("ascii")
        .rstrip("=")
    )
    if not hmac.compare_digest(parts[1], expected):
        raise InvalidSceneAnalysisDispatchAuthorization("invalid dispatch authorization signature")
    try:
        claims = SceneAnalysisDispatchAuthorizationClaims.model_validate_json(payload)
    except ValidationError as error:
        raise InvalidSceneAnalysisDispatchAuthorization(
            "invalid dispatch authorization payload"
        ) from error
    now = int(time.time())
    if claims.expires_at > now + MAX_TTL_SECONDS:
        raise InvalidSceneAnalysisDispatchAuthorization(
            "dispatch authorization expiry exceeds the maximum TTL"
        )
    return (
        claims,
        SceneAnalysisDispatchAuthorizationEvidence(
            claim_version=claims.claim_version,
            authorization_hash=hashlib.sha256(value.encode("utf-8")).hexdigest(),
            expires_at=claims.expires_at,
        ),
        now,
    )
