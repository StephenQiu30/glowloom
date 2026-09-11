from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time
from pathlib import Path
from typing import Any, cast
from uuid import UUID

import httpx
import pytest

from app.harness.scene_analysis_schemas import (
    SceneAnalysisControlProof,
    SceneAnalysisExecutionBudget,
    SceneAnalysisReleaseIdentity,
)
from app.harness.visual_foundation_schemas import (
    VisualFoundationAttemptResult,
    VisualFoundationInvocation,
    VisualFoundationPayload,
)
from app.modules.storygraph.visual_foundation_bundle import VISUAL_FOUNDATION_SKILL_BUNDLE_HASH
from app.modules.storygraph.visual_foundation_contract import VisualFoundationCandidate
from app.modules.storygraph.visual_foundation_harness import VisualFoundationHarness
from app.protocol.canonical import production_canonical_hash

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
INVOCATION_FIXTURE = REPOSITORY_ROOT.joinpath(
    "backend/tests/fixtures/agent/storygraph-visual-foundation-invocation.json"
)
RESULT_FIXTURE = REPOSITORY_ROOT.joinpath(
    "backend/tests/fixtures/agent/storygraph-visual-foundation-result.json"
)
SECRET = "a-secure-agent-execution-secret-value"
DISPATCH_AUTHORIZATION_DOMAIN = "lanverse.scene-analysis.dispatch-authorization.production"
PNG = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
)


def invocation(
    content: bytes = PNG,
    *,
    pixel_width: int = 1,
    pixel_height: int = 1,
) -> VisualFoundationInvocation:
    value = cast(dict[str, Any], json.loads(INVOCATION_FIXTURE.read_text(encoding="utf-8")))
    attachment = cast(dict[str, Any], value["payload"]["media_attachments"][0])
    reference = cast(dict[str, Any], value["payload"]["stage_input"]["reference_attachments"][0])
    content_hash = hashlib.sha256(content).hexdigest()
    object_key = "visual-references/project-1/courtyard.png"
    attachment.update(
        byte_length=len(content),
        content_hash=content_hash,
        media_type="image/png",
        object_key=object_key,
        pixel_width=pixel_width,
        pixel_height=pixel_height,
    )
    reference.update(content_hash=content_hash, media_type="image/png", object_key=object_key)
    stage_input = cast(dict[str, Any], value["payload"]["stage_input"])
    stage_input["reference_attachments_hash"] = production_canonical_hash(
        stage_input["reference_attachments"]
    )
    return VisualFoundationInvocation.build(
        invocation_id=UUID(value["invocation_id"]),
        attempt_id=UUID(value["attempt_id"]),
        stage_release=SceneAnalysisReleaseIdentity.model_validate(value["stage_release"]),
        control=SceneAnalysisControlProof.model_validate(value["control"]),
        budget=SceneAnalysisExecutionBudget.model_validate(value["budget"]),
        payload=VisualFoundationPayload.model_validate(value["payload"]),
    )


def candidate(request: VisualFoundationInvocation) -> VisualFoundationCandidate:
    fixture = cast(dict[str, Any], json.loads(RESULT_FIXTURE.read_text(encoding="utf-8")))
    value = cast(dict[str, Any], fixture["attempt_result"]["candidate"])
    value["reference_attachments_hash"] = request.payload.stage_input.reference_attachments_hash
    return VisualFoundationCandidate.model_validate(value)


def token(request: VisualFoundationInvocation, *, attempt_id: str | None = None) -> str:
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
    payload = json.dumps(claims, separators=(",", ":")).encode()
    encoded = base64.urlsafe_b64encode(payload).decode().rstrip("=")
    signature_input = hashlib.sha256(
        DISPATCH_AUTHORIZATION_DOMAIN.encode("ascii") + b"\0" + payload
    ).digest()
    signature = (
        base64.urlsafe_b64encode(
            hmac.new(SECRET.encode(), signature_input, hashlib.sha256).digest()
        )
        .decode()
        .rstrip("=")
    )
    return encoded + "." + signature


@pytest.mark.asyncio
async def test_visual_foundation_api_returns_a_verified_candidate_result(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request = invocation()
    expected = candidate(request)
    staged_paths: list[Path] = []

    async def execute(harness: VisualFoundationHarness) -> VisualFoundationCandidate:
        staged_paths.extend(value.path for value in harness.image_inputs)
        assert all(value.path.is_file() for value in harness.image_inputs)
        return expected

    monkeypatch.setattr(VisualFoundationHarness, "execute", execute)
    authorization = token(request)
    response = await client.post(
        "/internal/storygraph/visual-foundation/invocations",
        data={"invocation": request.model_dump_json()},
        files={
            "media": (str(request.payload.media_attachments[0].attachment_id), PNG, "image/png")
        },
        headers={"X-Lanverse-Dispatch-Authorization": authorization},
    )

    assert response.status_code == 200
    result = VisualFoundationAttemptResult.model_validate(response.json())
    result.validate_for(
        request,
        claim_version=1,
        dispatch_authorization_hash=hashlib.sha256(authorization.encode()).hexdigest(),
    )
    assert result.status == "accepted"
    assert result.candidate == expected.model_dump(mode="json")
    assert result.executor.runtime_class == "vision"
    assert result.executor.harness_version == "visual-foundation-harness"
    assert staged_paths and all(not path.exists() for path in staged_paths)

    health = await client.get("/healthz")
    assert health.status_code == 200
    assert health.json()["visual_foundation_skill_bundle_hash"] == (
        VISUAL_FOUNDATION_SKILL_BUNDLE_HASH
    )


@pytest.mark.asyncio
async def test_visual_foundation_api_rejects_authorization_or_media_drift(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request = invocation()
    unauthorized = await client.post(
        "/internal/storygraph/visual-foundation/invocations",
        data={"invocation": request.model_dump_json()},
        files={
            "media": (str(request.payload.media_attachments[0].attachment_id), PNG, "image/png")
        },
        headers={
            "X-Lanverse-Dispatch-Authorization": token(
                request, attempt_id="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
            )
        },
    )
    assert unauthorized.status_code == 401

    authorization = token(request)
    missing = await client.post(
        "/internal/storygraph/visual-foundation/invocations",
        data={"invocation": request.model_dump_json()},
        headers={"X-Lanverse-Dispatch-Authorization": authorization},
    )
    assert missing.status_code == 200
    assert missing.json()["error"]["code"] == "media_invalid"

    drifted = await client.post(
        "/internal/storygraph/visual-foundation/invocations",
        data={"invocation": request.model_dump_json()},
        files={
            "media": (
                str(request.payload.media_attachments[0].attachment_id),
                PNG[:-1] + b"x",
                "image/png",
            )
        },
        headers={"X-Lanverse-Dispatch-Authorization": authorization},
    )
    assert drifted.status_code == 200
    result = VisualFoundationAttemptResult.model_validate(drifted.json())
    assert result.status == "rejected"
    assert result.candidate is None
    assert result.error is not None
    assert result.error.code == "media_invalid"
    assert result.error.retry_class == "never"

    wrong_dimensions = invocation(pixel_width=2)
    dimensions = await client.post(
        "/internal/storygraph/visual-foundation/invocations",
        data={"invocation": wrong_dimensions.model_dump_json()},
        files={
            "media": (
                str(wrong_dimensions.payload.media_attachments[0].attachment_id),
                PNG,
                "image/png",
            )
        },
        headers={"X-Lanverse-Dispatch-Authorization": token(wrong_dimensions)},
    )
    assert dimensions.status_code == 200
    assert dimensions.json()["error"]["code"] == "media_invalid"
