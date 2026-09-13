from __future__ import annotations

import asyncio
import base64
import hashlib
import hmac
import io
import json
import os
import time
from collections.abc import AsyncIterator
from pathlib import Path
from tempfile import SpooledTemporaryFile
from typing import Any

import httpx
import pytest
from PIL import Image
from starlette.datastructures import UploadFile

from app.api.routes import vision_review
from app.harness.vision_review_schemas import VisionReviewAttemptResult, VisionReviewInvocation
from app.modules.storygraph.vision_review_contract import VisionReviewCandidate
from app.modules.storygraph.vision_review_harness import (
    VisionReviewHarness,
)
from app.protocol.canonical import production_canonical_hash
from app.reasoning.codex import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexExecutionError,
    CodexSchemaInvalid,
    CodexToolPolicyViolation,
)
from tests.contract.test_vision_review_wire import accepted_result, resign, valid_invocation
from tests.unit.test_codex_image_inputs import executable

ROUTE = "/internal/storygraph/vision-review/invocations"
SECRET = "synthetic-vision-review-api-secret-not-a-credential"
DISPATCH_AUTHORIZATION_DOMAIN = "lanverse.scene-analysis.dispatch-authorization.production"


def token(request: VisionReviewInvocation, *, attempt_id: str | None = None) -> str:
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


def invocation_with_images() -> tuple[VisionReviewInvocation, list[bytes]]:
    value = valid_invocation().model_dump(mode="json")
    stage = value["payload"]["stage_input"]
    images: list[bytes] = []
    for index, (slot, attachment) in enumerate(
        zip(stage["subject"]["slots"], stage["attachments"], strict=True)
    ):
        output = io.BytesIO()
        Image.new("RGB", (2, 2), (index * 70, 30, 40)).save(output, format="PNG")
        image = output.getvalue()
        images.append(image)
        slot["sha256"] = hashlib.sha256(image).hexdigest()
        attachment["slot"] = dict(slot)
        attachment.update(byte_length=len(image), pixel_width=2, pixel_height=2)
    material = {
        **stage,
        "subject": {k: v for k, v in stage["subject"].items() if k != "input_hash"},
    }
    stage["subject"]["input_hash"] = production_canonical_hash(material)
    resign(value, "input_hash")
    return VisionReviewInvocation.model_validate(value), images


def media_parts(
    request: VisionReviewInvocation, images: list[bytes]
) -> list[tuple[str, tuple[str, bytes, str]]]:
    return [
        ("media", (attachment.slot.slot_key, image, "image/png"))
        for attachment, image in zip(request.payload.stage_input.attachments, images, strict=True)
    ]


async def test_review_api_passes_complete_readonly_images_to_one_strict_call(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request, images = invocation_with_images()
    expected = VisionReviewCandidate.model_validate(accepted_result(request).candidate)
    captured: list[dict[str, Any]] = []
    paths: list[Path] = []

    async def execute(**kwargs: Any) -> VisionReviewCandidate:
        captured.append(kwargs)
        inputs = kwargs["image_inputs"]
        paths.extend(item.path for item in inputs)
        assert [item.path.read_bytes() for item in inputs] == images
        assert all(item.path.stat().st_mode & 0o222 == 0 for item in inputs)
        return expected

    monkeypatch.setattr("app.modules.storygraph.vision_review_harness.run_codex_process", execute)
    authorization = token(request)
    response = await client.post(
        ROUTE,
        data={"invocation": request.model_dump_json()},
        files=media_parts(request, images),
        headers={"X-Lanverse-Dispatch-Authorization": authorization},
    )
    assert response.status_code == 200, response.text
    result = VisionReviewAttemptResult.model_validate(response.json())
    result.validate_for(request, 1, hashlib.sha256(authorization.encode()).hexdigest())
    assert result.status == "accepted"
    assert len(captured) == 1
    assert captured[0]["strict_output_schema"] is True
    assert captured[0]["timeout_seconds"] == 120
    assert captured[0]["max_output_bytes"] == 131072
    assert "references/vision-review.md" in captured[0]["guidance"]
    assert not any(path.exists() for path in paths)


@pytest.mark.parametrize("interruption", ["over_limit", "cancelled"])
async def test_review_closes_partially_spooled_uploads(
    interruption: str, client: httpx.AsyncClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    opened: list[Any] = []

    def spool(*args: Any, **kwargs: Any) -> Any:
        value = SpooledTemporaryFile(*args, **kwargs)
        opened.append(value)
        return value

    monkeypatch.setattr("starlette.formparsers.SpooledTemporaryFile", spool)
    first = (
        b"--review-boundary\r\n"
        b'Content-Disposition: form-data; name="media"; filename="image"\r\n'
        b"Content-Type: image/png\r\n\r\n" + b"x" * 1024
    )

    async def stream() -> AsyncIterator[bytes]:
        yield first
        assert opened and not opened[0].closed
        if interruption == "cancelled":
            raise asyncio.CancelledError()
        yield b"more than declared"

    headers = {
        "Content-Type": "multipart/form-data; boundary=review-boundary",
        "Content-Length": str(len(first) + 1),
        "X-Lanverse-Dispatch-Authorization": "synthetic-invalid",
    }
    if interruption == "cancelled":
        with pytest.raises(asyncio.CancelledError):
            await client.post(ROUTE, content=stream(), headers=headers)
    else:
        response = await client.post(ROUTE, content=stream(), headers=headers)
        assert response.status_code == 413, response.text
    assert opened and all(value.closed for value in opened)


async def test_review_rechecks_authorization_after_upload_materialization(
    tmp_path: Path, client: httpx.AsyncClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request, images = invocation_with_images()
    authorization = token(request)
    now = time.time()
    read = UploadFile.read
    monkeypatch.setattr(vision_review.tempfile, "tempdir", str(tmp_path))

    async def expire_after_reading(self: UploadFile, size: int = -1) -> bytes:
        data = await read(self, size)
        if not data:
            monkeypatch.setattr("app.harness.grants.time.time", lambda: now + 120)
        return data

    async def forbidden(_: VisionReviewHarness) -> VisionReviewCandidate:
        pytest.fail("expired authorization must not launch the model")

    monkeypatch.setattr(UploadFile, "read", expire_after_reading)
    monkeypatch.setattr(VisionReviewHarness, "execute", forbidden)
    response = await client.post(
        ROUTE,
        data={"invocation": request.model_dump_json()},
        files=media_parts(request, images),
        headers={"X-Lanverse-Dispatch-Authorization": authorization},
    )
    assert response.status_code == 401, response.text
    assert not await asyncio.to_thread(os.listdir, tmp_path)


async def test_review_honors_reduced_budget_and_rejects_stale_candidate(
    client: httpx.AsyncClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request, images = invocation_with_images()
    stale = VisionReviewCandidate.model_validate(accepted_result(request).candidate)
    stale.subject.input_hash = "e" * 64
    value = request.model_dump(mode="json")
    value["budget"]["max_execution_seconds"] = 5
    value["budget"]["max_output_bytes"] = 8192
    resign(value, "input_hash")
    request = VisionReviewInvocation.model_validate(value)
    calls = 0

    async def execute(**kwargs: Any) -> VisionReviewCandidate:
        nonlocal calls
        calls += 1
        assert kwargs["timeout_seconds"] == 5
        assert kwargs["max_output_bytes"] == 8192
        return stale

    monkeypatch.setattr("app.modules.storygraph.vision_review_harness.run_codex_process", execute)
    response = await client.post(
        ROUTE,
        data={"invocation": request.model_dump_json()},
        files=media_parts(request, images),
        headers={"X-Lanverse-Dispatch-Authorization": token(request)},
    )
    assert response.status_code == 200, response.text
    assert response.json()["error"]["code"] == "candidate_schema_invalid"
    assert calls == 1


@pytest.mark.parametrize(
    "mutation", ["missing", "swapped", "filename", "bytes", "unknown_part", "authorization"]
)
async def test_review_api_rejects_invalid_group_before_model(
    mutation: str,
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request, images = invocation_with_images()
    parts = media_parts(request, images)
    authorization = token(request)
    calls = 0

    async def execute(_: VisionReviewHarness) -> VisionReviewCandidate:
        nonlocal calls
        calls += 1
        raise AssertionError("invalid group must not call the model")

    monkeypatch.setattr(VisionReviewHarness, "execute", execute)
    if mutation == "missing":
        parts.pop()
    elif mutation == "swapped":
        parts.reverse()
    elif mutation == "filename":
        parts[0] = ("media", ("../image.png", images[0], "image/png"))
    elif mutation == "bytes":
        parts[0] = ("media", (parts[0][1][0], b"invalid", "image/png"))
    elif mutation == "unknown_part":
        parts.append(("unexpected", ("unexpected", b"x", "image/png")))
    else:
        authorization += "invalid"
    response = await client.post(
        ROUTE,
        data={"invocation": request.model_dump_json()},
        files=parts,
        headers={"X-Lanverse-Dispatch-Authorization": authorization},
    )
    assert response.status_code in {200, 400, 401, 422}, response.text
    if response.status_code == 200:
        assert response.json()["status"] == "rejected"
        assert response.json()["error"]["code"] == "media_invalid"
    assert calls == 0


@pytest.mark.parametrize("mutation", ["tail", "truncated", "crc", "metadata", "animated"])
async def test_review_rejects_invalid_png_even_with_matching_content_hash(
    mutation: str, client: httpx.AsyncClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request, images = invocation_with_images()
    value = request.model_dump(mode="json")
    stage = value["payload"]["stage_input"]
    if mutation == "tail":
        images[0] += b"untrusted-tail"
    elif mutation == "truncated":
        images[0] = images[0][:-12]
    elif mutation == "crc":
        images[0] = images[0][:-1] + b"\0"
    elif mutation == "metadata":
        stage["attachments"][0]["pixel_width"] = 3
    else:
        output = io.BytesIO()
        first = Image.new("RGB", (2, 2), "red")
        first.save(
            output,
            format="PNG",
            save_all=True,
            append_images=[Image.new("RGB", (2, 2), "blue")],
            duration=100,
            loop=0,
        )
        images[0] = output.getvalue()
    slot = stage["subject"]["slots"][0]
    slot["sha256"] = hashlib.sha256(images[0]).hexdigest()
    stage["attachments"][0].update(slot=dict(slot), byte_length=len(images[0]))
    stage["subject"]["input_hash"] = production_canonical_hash(
        {**stage, "subject": {k: v for k, v in stage["subject"].items() if k != "input_hash"}}
    )
    resign(value, "input_hash")
    request = VisionReviewInvocation.model_validate(value)

    async def forbidden(_: VisionReviewHarness) -> VisionReviewCandidate:
        pytest.fail("invalid PNG must never launch the model")

    monkeypatch.setattr(VisionReviewHarness, "execute", forbidden)
    response = await client.post(
        ROUTE,
        data={"invocation": request.model_dump_json()},
        files=media_parts(request, images),
        headers={"X-Lanverse-Dispatch-Authorization": token(request)},
    )
    assert response.status_code == 200, response.text
    assert response.json()["error"]["code"] == "media_invalid"


@pytest.mark.parametrize("declared", [None, "invalid", "1", str(34 << 20)])
async def test_review_bounds_actual_and_declared_request_size(
    declared: str | None, client: httpx.AsyncClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request, images = invocation_with_images()
    outbound = client.build_request(
        "POST",
        ROUTE,
        data={"invocation": request.model_dump_json()},
        files=media_parts(request, images),
        headers={"X-Lanverse-Dispatch-Authorization": token(request)},
    )
    if declared is None:
        del outbound.headers["Content-Length"]
    else:
        outbound.headers["Content-Length"] = declared
    response = await client.send(outbound)
    assert response.status_code == 413, response.text


@pytest.mark.parametrize(
    ("error_type", "status", "code"),
    [
        (CodexBudgetExceeded, "rejected", "execution_budget_exceeded"),
        (CodexDeadlineExceeded, "rejected", "execution_deadline_exceeded"),
        (CodexToolPolicyViolation, "rejected", "tool_not_allowed"),
        (CodexSchemaInvalid, "rejected", "candidate_schema_invalid"),
        (CodexExecutionError, "outcome_unknown", "agent_execution_unknown"),
        (RuntimeError, "outcome_unknown", "agent_execution_unknown"),
    ],
)
async def test_review_terminal_failures_are_single_call_and_remove_private_files(
    error_type: type[Exception],
    status: str,
    code: str,
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request, images = invocation_with_images()
    paths: list[Path] = []
    calls = 0

    async def execute(**kwargs: Any) -> VisionReviewCandidate:
        nonlocal calls
        calls += 1
        paths.extend(value.path for value in kwargs["image_inputs"])
        raise error_type("private-path-and-prompt-must-not-leak")

    monkeypatch.setattr("app.modules.storygraph.vision_review_harness.run_codex_process", execute)
    response = await client.post(
        ROUTE,
        data={"invocation": request.model_dump_json()},
        files=media_parts(request, images),
        headers={"X-Lanverse-Dispatch-Authorization": token(request)},
    )
    assert response.status_code == 200, response.text
    assert response.json()["status"] == status
    assert response.json()["error"]["code"] == code
    assert "private-path-and-prompt" not in response.text
    assert calls == 1 and len(paths) == 3
    assert not any(path.exists() for path in paths)


async def test_review_cancellation_cleans_private_files(
    client: httpx.AsyncClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request, images = invocation_with_images()
    paths: list[Path] = []

    async def execute(**kwargs: Any) -> VisionReviewCandidate:
        paths.extend(value.path for value in kwargs["image_inputs"])
        raise asyncio.CancelledError()

    monkeypatch.setattr("app.modules.storygraph.vision_review_harness.run_codex_process", execute)
    with pytest.raises(asyncio.CancelledError):
        await client.post(
            ROUTE,
            data={"invocation": request.model_dump_json()},
            files=media_parts(request, images),
            headers={"X-Lanverse-Dispatch-Authorization": token(request)},
        )
    assert len(paths) == 3 and not any(path.exists() for path in paths)


async def test_review_executes_one_real_subprocess_with_complete_images_and_strict_output(
    tmp_path: Path, client: httpx.AsyncClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    request, images = invocation_with_images()
    candidate = accepted_result(request).candidate
    capture = tmp_path / "subprocess-observation.json"
    binary = executable(
        tmp_path / "synthetic-codex",
        "import hashlib, json, pathlib, sys\n"
        "images = [pathlib.Path(sys.argv[i + 1])\n"
        "          for i, v in enumerate(sys.argv) if v == '--image']\n"
        "schema_path = pathlib.Path(sys.argv[sys.argv.index('--output-schema') + 1])\n"
        "schema = json.loads(schema_path.read_text())\n"
        "assert schema['additionalProperties'] is False\n"
        "assert '--ephemeral' in sys.argv and '--ignore-user-config' in sys.argv\n"
        "assert sys.argv[sys.argv.index('--sandbox') + 1] == 'read-only'\n"
        "assert all(p.stat().st_mode & 0o222 == 0 for p in images)\n"
        f"capture = pathlib.Path({str(capture)!r})\n"
        "assert not capture.exists(), 'automatic second call'\n"
        "capture.write_text(json.dumps({'paths': [str(p) for p in images], "
        "'hashes': [hashlib.sha256(p.read_bytes()).hexdigest() for p in images]}))\n"
        "sys.stdin.buffer.read()\n"
        "target = pathlib.Path(sys.argv[sys.argv.index('--output-last-message') + 1])\n"
        f"target.write_text({json.dumps(candidate)!r})\n",
    )
    monkeypatch.setenv("CODEX_BIN", binary)
    response = await client.post(
        ROUTE,
        data={"invocation": request.model_dump_json()},
        files=media_parts(request, images),
        headers={"X-Lanverse-Dispatch-Authorization": token(request)},
    )
    assert response.status_code == 200, response.text
    assert response.json()["status"] == "accepted", response.text
    observation = json.loads(capture.read_text())
    assert observation["hashes"] == [hashlib.sha256(image).hexdigest() for image in images]
    paths = [Path(path) for path in observation["paths"]]
    assert not any(path.exists() for path in paths)
