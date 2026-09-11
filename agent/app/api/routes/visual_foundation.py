"""Visual Foundation Harness invocation route with verified image transport."""

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

from app.api.dependencies import get_harness_service
from app.harness.grants import (
    InvalidSceneAnalysisDispatchAuthorization,
    SceneAnalysisDispatchAuthorizationEvidence,
    verify_visual_foundation_dispatch_authorization,
)
from app.harness.scene_analysis_schemas import SceneAnalysisDiagnostic, SceneAnalysisResultError
from app.harness.service import HarnessService
from app.harness.visual_foundation_schemas import (
    MAX_VISUAL_FOUNDATION_IMAGE_BYTES,
    MAX_VISUAL_FOUNDATION_IMAGES,
    MAX_VISUAL_FOUNDATION_TOTAL_IMAGE_BYTES,
    VisualFoundationAttemptResult,
    VisualFoundationExecutor,
    VisualFoundationInvocation,
)
from app.modules.storygraph.bundle import BundleInvalid
from app.modules.storygraph.harness import SkillBundleUnavailable
from app.modules.storygraph.visual_foundation_harness import VisualFoundationMediaBinding
from app.protocol.canonical import production_canonical_hash
from app.reasoning.codex import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexExecutionError,
    CodexMediaInvalid,
    CodexSchemaInvalid,
    CodexToolPolicyViolation,
)

router = APIRouter(tags=["visual-foundation"])
Service = Annotated[HarnessService, Depends(get_harness_service)]

_IMAGE_FORMATS = {
    "image/jpeg": "JPEG",
    "image/png": "PNG",
    "image/webp": "WEBP",
}
_MAX_INVOCATION_BYTES = 1 << 20
_MAX_MULTIPART_OVERHEAD_BYTES = 64 << 10
_MAX_REQUEST_BYTES = (
    MAX_VISUAL_FOUNDATION_TOTAL_IMAGE_BYTES + _MAX_INVOCATION_BYTES + _MAX_MULTIPART_OVERHEAD_BYTES
)


@router.post(
    "/internal/storygraph/visual-foundation/invocations",
    response_model=VisualFoundationAttemptResult,
)
async def invoke_visual_foundation(
    request: Request,
    service: Service,
    dispatch_authorization: str = Header(alias="X-Lanverse-Dispatch-Authorization"),
) -> VisualFoundationAttemptResult:
    _validate_request_size(request)
    try:
        async with request.form(
            max_files=MAX_VISUAL_FOUNDATION_IMAGES,
            max_fields=1,
            max_part_size=_MAX_INVOCATION_BYTES,
        ) as form:
            invocation_parts = form.getlist("invocation")
            media_parts = form.getlist("media")
            if len(invocation_parts) != 1 or not isinstance(invocation_parts[0], str):
                raise HTTPException(status_code=422, detail="invalid visual foundation invocation")
            try:
                invocation = VisualFoundationInvocation.model_validate_json(invocation_parts[0])
            except ValidationError as error:
                raise HTTPException(
                    status_code=422, detail="invalid visual foundation invocation"
                ) from error
            authorization = _authorize(dispatch_authorization, invocation)
            with tempfile.TemporaryDirectory(prefix="lanverse-visual-foundation-") as temporary:
                try:
                    bindings = await _materialize_media(
                        invocation,
                        media_parts,
                        Path(temporary),
                    )
                    value, model = await service.visual_foundation(invocation, bindings)
                    candidate = value.model_dump(mode="json")
                    result = VisualFoundationAttemptResult.build(
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
                        candidate_type="visual_foundation_candidate",
                        candidate=candidate,
                        input_hash=invocation.input_hash,
                        output_hash=production_canonical_hash(candidate),
                        diagnostics=[],
                        diagnostic_hash=production_canonical_hash([]),
                        completed_at=_utc_now(),
                        executor=VisualFoundationExecutor(
                            runtime_class="vision",
                            runtime_image_digest=invocation.stage_release.agent_image_digest,
                            harness_version="visual-foundation-harness",
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
                except CodexMediaInvalid:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "media_invalid",
                        "Visual Foundation media failed verification",
                    )
                except SkillBundleUnavailable:
                    return _failure(
                        invocation,
                        authorization,
                        "outcome_unknown",
                        "skill_bundle_unavailable",
                        "Frozen Visual Foundation skill bundle is unavailable",
                    )
                except BundleInvalid:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "skill_bundle_invalid",
                        "Visual Foundation skill bundle is invalid",
                    )
                except (ValidationError, CodexSchemaInvalid, ValueError):
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "candidate_schema_invalid",
                        "Visual Foundation Candidate failed strict validation",
                    )
                except CodexBudgetExceeded:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "execution_budget_exceeded",
                        "Visual Foundation execution budget was exceeded",
                    )
                except CodexDeadlineExceeded:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "execution_deadline_exceeded",
                        "Visual Foundation execution deadline was exceeded",
                    )
                except CodexToolPolicyViolation:
                    return _failure(
                        invocation,
                        authorization,
                        "rejected",
                        "tool_not_allowed",
                        "Visual Foundation attempted a disallowed tool",
                    )
                except CodexExecutionError:
                    return _failure(
                        invocation,
                        authorization,
                        "outcome_unknown",
                        "agent_execution_unknown",
                        "Visual Foundation execution ended without a trustworthy result",
                    )
                except Exception:
                    return _failure(
                        invocation,
                        authorization,
                        "outcome_unknown",
                        "agent_execution_unknown",
                        "Visual Foundation execution ended without a trustworthy result",
                    )
    except HTTPException:
        raise


def _validate_request_size(request: Request) -> None:
    declared = request.headers.get("content-length", "")
    try:
        size = int(declared)
    except ValueError as error:
        raise HTTPException(
            status_code=413, detail="visual foundation request too large"
        ) from error
    if size < 1 or size > _MAX_REQUEST_BYTES:
        raise HTTPException(status_code=413, detail="visual foundation request too large")


def _authorize(
    value: str,
    invocation: VisualFoundationInvocation,
) -> SceneAnalysisDispatchAuthorizationEvidence:
    try:
        return verify_visual_foundation_dispatch_authorization(
            value,
            os.getenv("AGENT_EXECUTION_SECRET", ""),
            invocation,
        )
    except InvalidSceneAnalysisDispatchAuthorization as error:
        raise HTTPException(status_code=401, detail="invalid dispatch authorization") from error


async def _materialize_media(
    invocation: VisualFoundationInvocation,
    parts: Sequence[UploadFile | str],
    root: Path,
) -> tuple[VisualFoundationMediaBinding, ...]:
    expected = invocation.payload.media_attachments
    if len(parts) != len(expected):
        raise CodexMediaInvalid("Visual Foundation media set is incomplete")
    bindings: list[VisualFoundationMediaBinding] = []
    total_bytes = 0
    for index, (part, attachment) in enumerate(zip(parts, expected, strict=True)):
        if not isinstance(part, UploadFile):
            raise CodexMediaInvalid("Visual Foundation media part is invalid")
        upload = part
        if (
            upload.filename != str(attachment.attachment_id)
            or upload.content_type != attachment.media_type
        ):
            raise CodexMediaInvalid("Visual Foundation media identity drifted")
        destination = root / f"media-{index:03d}"
        digest = hashlib.sha256()
        written = 0
        with destination.open("xb") as output:
            while chunk := await upload.read(65536):
                written += len(chunk)
                total_bytes += len(chunk)
                if (
                    written > attachment.byte_length
                    or written > MAX_VISUAL_FOUNDATION_IMAGE_BYTES
                    or total_bytes > MAX_VISUAL_FOUNDATION_TOTAL_IMAGE_BYTES
                ):
                    raise CodexMediaInvalid("Visual Foundation media exceeds its frozen budget")
                digest.update(chunk)
                output.write(chunk)
        if written != attachment.byte_length or digest.hexdigest() != attachment.content_hash:
            raise CodexMediaInvalid("Visual Foundation media content drifted")
        _verify_image(
            destination, attachment.media_type, attachment.pixel_width, attachment.pixel_height
        )
        destination.chmod(0o400)
        bindings.append(
            VisualFoundationMediaBinding(
                attachment_id=attachment.attachment_id,
                path=destination.resolve(strict=True),
                byte_length=written,
            )
        )
    return tuple(bindings)


def _verify_image(path: Path, media_type: str, width: int, height: int) -> None:
    try:
        with warnings.catch_warnings():
            warnings.simplefilter("error", Image.DecompressionBombWarning)
            with Image.open(path) as image:
                if (
                    image.format != _IMAGE_FORMATS[media_type]
                    or image.size != (width, height)
                    or getattr(image, "n_frames", 1) != 1
                ):
                    raise CodexMediaInvalid("Visual Foundation image metadata drifted")
                image.verify()
    except CodexMediaInvalid:
        raise
    except (
        KeyError,
        OSError,
        UnidentifiedImageError,
        Image.DecompressionBombError,
        Image.DecompressionBombWarning,
    ) as error:
        raise CodexMediaInvalid("Visual Foundation image bytes are invalid") from error


def _failure(
    invocation: VisualFoundationInvocation,
    authorization: SceneAnalysisDispatchAuthorizationEvidence,
    status: Literal["rejected", "outcome_unknown"],
    code: str,
    summary: str,
) -> VisualFoundationAttemptResult:
    retry_class: Literal["never", "same_release"] = (
        "never" if status == "rejected" else "same_release"
    )
    diagnostics = [SceneAnalysisDiagnostic(code=code, summary=summary)]
    result = VisualFoundationAttemptResult.build(
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
        candidate_type="visual_foundation_candidate",
        candidate=None,
        input_hash=invocation.input_hash,
        output_hash=None,
        diagnostics=diagnostics,
        diagnostic_hash=production_canonical_hash(
            [value.model_dump(mode="json") for value in diagnostics]
        ),
        completed_at=_utc_now(),
        executor=VisualFoundationExecutor(
            runtime_class="vision",
            runtime_image_digest=invocation.stage_release.agent_image_digest,
            harness_version="visual-foundation-harness",
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
