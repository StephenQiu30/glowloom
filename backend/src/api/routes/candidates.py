from __future__ import annotations

from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, Query, Request

from api.dependencies import (
    clock_from_request,
    database_from_request,
    object_store_from_request,
)
from schemas.candidate_api import (
    CandidateListResponse,
    PreviewAuthorizationRequest,
    PreviewAuthorizationResponse,
    candidate_response,
    preview_response,
)
from schemas.common import Problem
from schemas.media_registration import UsageType
from services.candidates import CandidateQueryService, PreviewAuthorizationService

router = APIRouter(prefix="/v1")
ERROR_RESPONSES: dict[int | str, dict[str, Any]] = {
    404: {"model": Problem},
    422: {"model": Problem},
    503: {"model": Problem},
}


@router.get(
    "/episodes/{episode_id}/candidates",
    operation_id="listCandidates",
    response_model=CandidateListResponse,
    responses=ERROR_RESPONSES,
)
async def list_candidates(
    episode_id: UUID,
    request: Request,
    usage_type: Annotated[UsageType, Query()],
    usage_id: Annotated[UUID, Query()],
    input_version_id: Annotated[UUID, Query()],
    input_hash: Annotated[str, Query(pattern=r"^[0-9a-f]{64}$")],
) -> CandidateListResponse:
    values = await CandidateQueryService(database_from_request(request)).list(
        episode_id=episode_id,
        usage_type=usage_type,
        usage_id=usage_id,
        input_version_id=input_version_id,
        input_hash=input_hash,
    )
    return CandidateListResponse(items=tuple(candidate_response(item) for item in values))


@router.post(
    "/media-versions/{media_version_id}/preview-authorizations",
    operation_id="authorizeCandidatePreview",
    response_model=PreviewAuthorizationResponse,
    responses=ERROR_RESPONSES,
)
async def authorize_candidate_preview(
    media_version_id: UUID,
    body: PreviewAuthorizationRequest,
    request: Request,
) -> PreviewAuthorizationResponse:
    value = await PreviewAuthorizationService(
        database_from_request(request),
        object_store_from_request(request),
        clock_from_request(request),
    ).authorize(episode_id=body.episode_id, media_version_id=media_version_id)
    return preview_response(value)
