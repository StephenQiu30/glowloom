"""Vision Review Harness invocation route with verified image transport."""

from __future__ import annotations

import hashlib
import os
import tempfile
import warnings
from collections.abc import Sequence
from datetime import UTC, datetime
from pathlib import Path
from typing import Annotated, Literal

from fastapi import APIRouter, Depends, Header, HTTPException, Request
from PIL import Image, UnidentifiedImageError
from pydantic import ValidationError
from starlette.datastructures import UploadFile
from starlette.exceptions import HTTPException as StarletteHTTPException
from starlette.formparsers import MultiPartException
from starlette.types import Message

from app.api.dependencies import get_harness_service
from app.harness.grants import (
    InvalidSceneAnalysisDispatchAuthorization,
    SceneAnalysisDispatchAuthorizationEvidence,
    verify_vision_review_dispatch_authorization,
)
from app.harness.scene_analysis_schemas import SceneAnalysisDiagnostic, SceneAnalysisResultError
from app.harness.service import HarnessService
from app.harness.vision_review_schemas import (
    VisionReviewAttemptResult,
    VisionReviewExecutor,
    VisionReviewInvocation,
    decode_vision_review_invocation,
)
from app.modules.storygraph.bundle import BundleInvalid
from app.modules.storygraph.harness import SkillBundleUnavailable
from app.modules.storygraph.vision_review_harness import VisionReviewMediaBinding
from app.protocol.canonical import production_canonical_hash
from app.reasoning.codex import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexExecutionError,
    CodexMediaInvalid,
    CodexSchemaInvalid,
    CodexToolPolicyViolation,
)

router = APIRouter(tags=["vision-review"])
Service = Annotated[HarnessService, Depends(get_harness_service)]

MAX_VISION_REVIEW_IMAGE_BYTES = 10 << 20
MAX_VISION_REVIEW_IMAGES = 4
MAX_VISION_REVIEW_TOTAL_IMAGE_BYTES = 32 << 20
_LIMIT_DETAIL = "Vision Review request exceeds its byte budget"
_MAX_INVOCATION_BYTES = 1 << 20
_MAX_MULTIPART_OVERHEAD_BYTES = 64 << 10
_MAX_REQUEST_BYTES = (
    MAX_VISION_REVIEW_TOTAL_IMAGE_BYTES + _MAX_INVOCATION_BYTES + _MAX_MULTIPART_OVERHEAD_BYTES
)


@router.post(
    "/internal/storygraph/vision-review/invocations",
    response_model=VisionReviewAttemptResult,
)
async def invoke_vision_review(
    request: Request,
    service: Service,
    dispatch_authorization: str = Header(alias="X-Lanverse-Dispatch-Authorization"),
) -> VisionReviewAttemptResult:
    request = _bounded_request(request)
    try:
        async with request.form(
            max_files=MAX_VISION_REVIEW_IMAGES,
            max_fields=1,
            max_part_size=_MAX_INVOCATION_BYTES,
        ) as form:
            invocation_parts = form.getlist("invocation")
            media_parts = form.getlist("media")
            if (
                set(form.keys()) != {"invocation", "media"}
                or len(invocation_parts) != 1
                or not isinstance(invocation_parts[0], str)
            ):
                raise HTTPException(status_code=422, detail="invalid vision review invocation")
            try:
                invocation = decode_vision_review_invocation(invocation_parts[0])
            except ValueError as error:
                raise HTTPException(
                    status_code=422, detail="invalid vision review invocation"
                ) from error
            authorization = _authorize(dispatch_authorization, invocation)
            with tempfile.TemporaryDirectory(prefix="lanverse-vision-review-") as temporary:
                try:
                    bindings = await _materialize_media(
                        invocation,
                        media_parts,
                        Path(temporary),
                    )
                    authorization = _authorize(dispatch_authorization, invocation)
                    value, model = await service.vision_review(invocation, bindings)
                    candidate = value.model_dump(mode="json")
                    result = VisionReviewAttemptResult.build(
                        invocation_id=invocation.invocation_id,
                        attempt_id=invocation.attempt_id,
                        kind="storygraph_stage",
                        wire_schema_version=invocation.wire_schema_version,
                        variant=invocation.payload.variant,
                        stage_release=invocation.stage_release,
                        control=invocation.control,
                        claim_version=authorization.claim_version,
                        dispatch_authorization_hash=authorization.authorization_hash,
                        status="accepted",
                        candidate_type="vision_review_candidate",
                        candidate=candidate,
                        input_hash=invocation.input_hash,
                        output_hash=production_canonical_hash(candidate),
                        diagnostics=[],
                        diagnostic_hash=production_canonical_hash([]),
                        completed_at=_utc_now(),
                        executor=VisionReviewExecutor(
                            runtime_class="vision",
                            runtime_image_digest=invocation.stage_release.agent_image_digest,
                            harness_version="vision-review-harness",
                            model=model,
                        ),
                        error=None,
                    )
                    result.validate_for(
                        invocation,
                        authorization.claim_version,
                        authorization.authorization_hash,
                    )
                    return result
                except HTTPException:
                    raise
                except CodexMediaInvalid:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "media_invalid",
                        "Vision Review media failed verification",
                    )
                except SkillBundleUnavailable:
                    return _failure(
                        invocation,
                        authorization,
                        "outcome_unknown",
                        "skill_bundle_unavailable",
                        "Frozen Vision Review skill bundle is unavailable",
                    )
                except BundleInvalid:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "skill_bundle_invalid",
                        "Vision Review skill bundle is invalid",
                    )
                except (ValidationError, CodexSchemaInvalid, ValueError):
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "candidate_schema_invalid",
                        "Vision Review Candidate failed strict validation",
                    )
                except CodexBudgetExceeded:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "execution_budget_exceeded",
                        "Vision Review execution budget was exceeded",
                    )
                except CodexDeadlineExceeded:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "execution_deadline_exceeded",
                        "Vision Review execution deadline was exceeded",
                    )
                except CodexToolPolicyViolation:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "tool_not_allowed",
                        "Vision Review attempted a disallowed tool",
                    )
                except CodexExecutionError:
                    return _failure(
                        invocation,
                        authorization,
                        "outcome_unknown",
                        "agent_execution_unknown",
                        "Vision Review execution ended without a trustworthy result",
                    )
                except Exception:
                    return _failure(
                        invocation,
                        authorization,
                        "outcome_unknown",
                        "agent_execution_unknown",
                        "Vision Review execution ended without a trustworthy result",
                    )
    except StarletteHTTPException as error:
        if error.detail == _LIMIT_DETAIL:
            raise HTTPException(status_code=413, detail=_LIMIT_DETAIL) from error
        raise


def _bounded_request(request: Request) -> Request:
    declared = request.headers.get("content-length", "")
    try:
        size = int(declared)
    except ValueError as error:
        raise HTTPException(status_code=413, detail="vision review request too large") from error
    if size < 1 or size > _MAX_REQUEST_BYTES:
        raise HTTPException(status_code=413, detail=_LIMIT_DETAIL)
    received = 0

    async def receive() -> Message:
        nonlocal received
        message = await request.receive()
        if message["type"] == "http.request":
            received += len(message.get("body", b""))
            if (
                received > size
                or received > _MAX_REQUEST_BYTES
                or (not message.get("more_body", False) and received != size)
            ):
                # MultipartException lets Starlette close every partially spooled file.
                raise MultiPartException(_LIMIT_DETAIL)
        return message

    return Request(request.scope, receive=receive)


def _authorize(
    value: str,
    invocation: VisionReviewInvocation,
) -> SceneAnalysisDispatchAuthorizationEvidence:
    try:
        return verify_vision_review_dispatch_authorization(
            value,
            os.getenv("AGENT_EXECUTION_SECRET", ""),
            invocation,
        )
    except InvalidSceneAnalysisDispatchAuthorization as error:
        raise HTTPException(status_code=401, detail="invalid dispatch authorization") from error


async def _materialize_media(
    invocation: VisionReviewInvocation,
    parts: Sequence[UploadFile | str],
    root: Path,
) -> tuple[VisionReviewMediaBinding, ...]:
    expected = invocation.payload.stage_input.attachments
    if len(parts) != len(expected):
        raise CodexMediaInvalid("Vision Review media set is incomplete")
    bindings: list[VisionReviewMediaBinding] = []
    total_bytes = 0
    for index, (part, attachment) in enumerate(zip(parts, expected, strict=True)):
        if not isinstance(part, UploadFile):
            raise CodexMediaInvalid("Vision Review media part is invalid")
        upload = part
        if (
            upload.filename != attachment.slot.slot_key
            or upload.content_type != attachment.media_type
        ):
            raise CodexMediaInvalid("Vision Review media identity drifted")
        destination = root / f"media-{index:03d}"
        digest = hashlib.sha256()
        written = 0
        with destination.open("xb") as output:
            while chunk := await upload.read(65536):
                written += len(chunk)
                total_bytes += len(chunk)
                if (
                    written > attachment.byte_length
                    or written > MAX_VISION_REVIEW_IMAGE_BYTES
                    or total_bytes > MAX_VISION_REVIEW_TOTAL_IMAGE_BYTES
                ):
                    raise CodexMediaInvalid("Vision Review media exceeds its frozen budget")
                digest.update(chunk)
                output.write(chunk)
        if written != attachment.byte_length or digest.hexdigest() != attachment.slot.sha256:
            raise CodexMediaInvalid("Vision Review media content drifted")
        _verify_image(
            destination, attachment.media_type, attachment.pixel_width, attachment.pixel_height
        )
        destination.chmod(0o400)
        bindings.append(
            VisionReviewMediaBinding(
                slot_key=attachment.slot.slot_key,
                path=destination.resolve(strict=True),
                byte_length=written,
            )
        )
    return tuple(bindings)


def _verify_image(path: Path, media_type: str, width: int, height: int) -> None:
    try:
        with warnings.catch_warnings():
            warnings.simplefilter("error", Image.DecompressionBombWarning)
            with path.open("rb") as source:
                with Image.open(source) as image:
                    if (
                        media_type != "image/png"
                        or image.format != "PNG"
                        or image.size != (width, height)
                        or getattr(image, "n_frames", 1) != 1
                    ):
                        raise CodexMediaInvalid("Vision Review image metadata drifted")
                    image.verify()
                    # Pillow stops after IEND's header. Require its empty-payload CRC and EOF.
                    if source.read() != bytes.fromhex("ae426082"):
                        raise CodexMediaInvalid("Vision Review PNG end marker is invalid")
                source.seek(0)
                with Image.open(source) as image:
                    image.load()
    except CodexMediaInvalid:
        raise
    except (
        OSError,
        ValueError,
        SyntaxError,
        UnidentifiedImageError,
        Image.DecompressionBombError,
        Image.DecompressionBombWarning,
    ) as error:
        raise CodexMediaInvalid("Vision Review image bytes are invalid") from error


def _failure(
    invocation: VisionReviewInvocation,
    authorization: SceneAnalysisDispatchAuthorizationEvidence,
    status: Literal["rejected", "outcome_unknown"],
    code: str,
    summary: str,
) -> VisionReviewAttemptResult:
    retry_class: Literal["never", "same_release"] = (
        "never" if status == "rejected" else "same_release"
    )
    diagnostics = [SceneAnalysisDiagnostic(code=code, summary=summary)]
    result = VisionReviewAttemptResult.build(
        invocation_id=invocation.invocation_id,
        attempt_id=invocation.attempt_id,
        kind="storygraph_stage",
        wire_schema_version=invocation.wire_schema_version,
        variant=invocation.payload.variant,
        stage_release=invocation.stage_release,
        control=invocation.control,
        claim_version=authorization.claim_version,
        dispatch_authorization_hash=authorization.authorization_hash,
        status=status,
        candidate_type="vision_review_candidate",
        candidate=None,
        input_hash=invocation.input_hash,
        output_hash=None,
        diagnostics=diagnostics,
        diagnostic_hash=production_canonical_hash(
            [value.model_dump(mode="json") for value in diagnostics]
        ),
        completed_at=_utc_now(),
        executor=VisionReviewExecutor(
            runtime_class="vision",
            runtime_image_digest=invocation.stage_release.agent_image_digest,
            harness_version="vision-review-harness",
            model="unknown",
        ),
        error=SceneAnalysisResultError(
            code=code,
            safe_summary=summary,
            retry_class=retry_class,
        ),
    )
    result.validate_for(
        invocation,
        authorization.claim_version,
        authorization.authorization_hash,
    )
    return result


def _utc_now() -> datetime:
    return datetime.now(UTC)
