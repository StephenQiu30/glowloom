from __future__ import annotations

from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Header, Request, Response, status

from api.dependencies import database_from_request
from api.headers import parse_if_match, strong_etag, validate_idempotency_key
from api.responses import RESOURCE_API_ERRORS
from schemas.subtitle_api import (
    SaveSubtitleRequest,
    SubtitleVersionListResponse,
    SubtitleVersionResponse,
    subtitle_response,
)
from services.subtitles import (
    ConfirmSubtitleCommand,
    ConfirmSubtitleHandler,
    CreateSubtitlesCommand,
    CreateSubtitlesHandler,
    DeriveSubtitleDraftCommand,
    DeriveSubtitleDraftHandler,
    GetCurrentSubtitleHandler,
    GetSubtitleVersionHandler,
    ListSubtitleVersionsHandler,
    SaveSubtitleCommand,
    SaveSubtitleHandler,
)

router = APIRouter(prefix="/v1")


@router.post(
    "/episodes/{episode_id}/subtitle-versions",
    operation_id="createSubtitles",
    response_model=SubtitleVersionResponse,
    status_code=status.HTTP_201_CREATED,
    responses=RESOURCE_API_ERRORS,
)
async def create_subtitles(
    episode_id: UUID,
    request: Request,
    response: Response,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> SubtitleVersionResponse:
    value = await CreateSubtitlesHandler(database_from_request(request)).execute(
        CreateSubtitlesCommand(episode_id, validate_idempotency_key(idempotency_key))
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return subtitle_response(value)


@router.get(
    "/episodes/{episode_id}/subtitles",
    operation_id="getSubtitles",
    response_model=SubtitleVersionResponse,
    responses=RESOURCE_API_ERRORS,
)
async def get_subtitles(
    episode_id: UUID, request: Request, response: Response
) -> SubtitleVersionResponse:
    value = await GetCurrentSubtitleHandler(database_from_request(request)).execute(
        episode_id
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return subtitle_response(value)


@router.put(
    "/subtitle-versions/{version_id}",
    operation_id="saveSubtitles",
    response_model=SubtitleVersionResponse,
    responses=RESOURCE_API_ERRORS,
)
async def save_subtitles(
    version_id: UUID,
    body: SaveSubtitleRequest,
    request: Request,
    response: Response,
    if_match: Annotated[str, Header(alias="If-Match")],
) -> SubtitleVersionResponse:
    value = await SaveSubtitleHandler(database_from_request(request)).execute(
        SaveSubtitleCommand(version_id, parse_if_match(if_match), body.content)
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return subtitle_response(value)


@router.get(
    "/episodes/{episode_id}/subtitle-versions",
    operation_id="listSubtitleVersions",
    response_model=SubtitleVersionListResponse,
    responses=RESOURCE_API_ERRORS,
)
async def list_subtitle_versions(
    episode_id: UUID, request: Request
) -> SubtitleVersionListResponse:
    values = await ListSubtitleVersionsHandler(database_from_request(request)).execute(
        episode_id
    )
    return SubtitleVersionListResponse(
        items=tuple(subtitle_response(value) for value in values)
    )


@router.get(
    "/subtitle-versions/{version_id}",
    operation_id="getSubtitleVersion",
    response_model=SubtitleVersionResponse,
    responses=RESOURCE_API_ERRORS,
)
async def get_subtitle_version(
    version_id: UUID, request: Request, response: Response
) -> SubtitleVersionResponse:
    value = await GetSubtitleVersionHandler(database_from_request(request)).execute(
        version_id
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return subtitle_response(value)


@router.post(
    "/subtitle-versions/{version_id}/drafts",
    operation_id="deriveSubtitleDraft",
    response_model=SubtitleVersionResponse,
    status_code=status.HTTP_201_CREATED,
    responses=RESOURCE_API_ERRORS,
)
async def derive_subtitle_draft(
    version_id: UUID,
    request: Request,
    response: Response,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> SubtitleVersionResponse:
    value = await DeriveSubtitleDraftHandler(database_from_request(request)).execute(
        DeriveSubtitleDraftCommand(
            version_id, validate_idempotency_key(idempotency_key)
        )
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return subtitle_response(value)


@router.post(
    "/subtitle-versions/{version_id}:confirm",
    operation_id="confirmSubtitles",
    response_model=SubtitleVersionResponse,
    responses=RESOURCE_API_ERRORS,
)
async def confirm_subtitles(
    version_id: UUID,
    request: Request,
    response: Response,
    if_match: Annotated[str, Header(alias="If-Match")],
) -> SubtitleVersionResponse:
    value = await ConfirmSubtitleHandler(database_from_request(request)).execute(
        ConfirmSubtitleCommand(version_id, parse_if_match(if_match))
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return subtitle_response(value)
