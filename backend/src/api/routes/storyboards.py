from __future__ import annotations

from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, Header, Request, Response, status

from api.dependencies import database_from_request
from api.headers import (
    parse_if_match,
    strong_etag,
    validate_idempotency_key,
)
from schemas.common import Problem, TaskAccepted
from schemas.storyboard_api import (
    SaveStoryboardRequest,
    StoryboardGenerationResponse,
    StoryboardVersionListResponse,
    StoryboardVersionResponse,
    generation_response,
    storyboard_response,
)
from schemas.task_accepted import task_accepted
from services.story_generation import (
    GenerateStoryboardCommand,
    GenerateStoryboardHandler,
)
from services.storyboards import (
    ConfirmStoryboardCommand,
    ConfirmStoryboardHandler,
    DeriveStoryboardDraftCommand,
    DeriveStoryboardDraftHandler,
    GetStoryboardHandler,
    GetStoryboardVersionHandler,
    ListStoryboardVersionsHandler,
    SaveStoryboardCommand,
    SaveStoryboardHandler,
)

router = APIRouter(prefix="/v1")
ERROR_RESPONSES: dict[int | str, dict[str, Any]] = {
    404: {"model": Problem},
    409: {"model": Problem},
    412: {"model": Problem},
    422: {"model": Problem},
}


@router.post(
    "/episodes/{episode_id}/storyboard-generations",
    operation_id="generateStoryboard",
    response_model=TaskAccepted,
    status_code=status.HTTP_202_ACCEPTED,
    responses=ERROR_RESPONSES,
)
async def generate_storyboard(
    episode_id: UUID,
    request: Request,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> TaskAccepted:
    value = await GenerateStoryboardHandler(
        database_from_request(request), release_version="0.1.0"
    ).execute(
        GenerateStoryboardCommand(episode_id, validate_idempotency_key(idempotency_key))
    )
    return task_accepted(value)


@router.get(
    "/episodes/{episode_id}/storyboard",
    operation_id="getStoryboard",
    response_model=StoryboardVersionResponse,
    responses=ERROR_RESPONSES,
)
async def get_storyboard(
    episode_id: UUID, request: Request, response: Response
) -> StoryboardVersionResponse:
    value = await GetStoryboardHandler(database_from_request(request)).execute(episode_id)
    response.headers["ETag"] = strong_etag(value.resource_version)
    return storyboard_response(value)


@router.put(
    "/shot-spec-versions/{version_id}",
    operation_id="saveStoryboard",
    response_model=StoryboardVersionResponse,
    responses=ERROR_RESPONSES,
)
async def save_storyboard(
    version_id: UUID,
    body: SaveStoryboardRequest,
    request: Request,
    response: Response,
    if_match: Annotated[str, Header(alias="If-Match")],
) -> StoryboardVersionResponse:
    value = await SaveStoryboardHandler(database_from_request(request)).execute(
        SaveStoryboardCommand(version_id, parse_if_match(if_match), body.content)
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return storyboard_response(value)


@router.post(
    "/shot-spec-versions/{version_id}:confirm",
    operation_id="confirmStoryboard",
    response_model=StoryboardGenerationResponse,
    responses=ERROR_RESPONSES,
)
async def confirm_storyboard(
    version_id: UUID,
    request: Request,
    response: Response,
    if_match: Annotated[str, Header(alias="If-Match")],
) -> StoryboardGenerationResponse:
    value = await ConfirmStoryboardHandler(database_from_request(request)).execute(
        ConfirmStoryboardCommand(version_id, parse_if_match(if_match))
    )
    response.headers["ETag"] = strong_etag(value.storyboard.resource_version)
    return generation_response(value)


@router.post(
    "/shot-spec-versions/{version_id}/drafts",
    operation_id="deriveStoryboardDraft",
    response_model=StoryboardGenerationResponse,
    status_code=status.HTTP_201_CREATED,
    responses=ERROR_RESPONSES,
)
async def derive_storyboard_draft(
    version_id: UUID,
    request: Request,
    response: Response,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> StoryboardGenerationResponse:
    value = await DeriveStoryboardDraftHandler(database_from_request(request)).execute(
        DeriveStoryboardDraftCommand(
            version_id, validate_idempotency_key(idempotency_key)
        )
    )
    response.headers["ETag"] = strong_etag(value.storyboard.resource_version)
    return generation_response(value)


@router.get(
    "/episodes/{episode_id}/shot-spec-versions",
    operation_id="listStoryboardVersions",
    response_model=StoryboardVersionListResponse,
    responses=ERROR_RESPONSES,
)
async def list_storyboard_versions(
    episode_id: UUID, request: Request
) -> StoryboardVersionListResponse:
    values = await ListStoryboardVersionsHandler(database_from_request(request)).execute(
        episode_id
    )
    return StoryboardVersionListResponse(
        items=tuple(storyboard_response(item) for item in values)
    )


@router.get(
    "/shot-spec-versions/{version_id}",
    operation_id="getStoryboardVersion",
    response_model=StoryboardVersionResponse,
    responses=ERROR_RESPONSES,
)
async def get_storyboard_version(
    version_id: UUID, request: Request, response: Response
) -> StoryboardVersionResponse:
    value = await GetStoryboardVersionHandler(database_from_request(request)).execute(version_id)
    response.headers["ETag"] = strong_etag(value.resource_version)
    return storyboard_response(value)
