from __future__ import annotations

from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Header, Request, Response, status

from api.dependencies import database_from_request
from api.headers import (
    parse_if_match,
    strong_etag,
    validate_idempotency_key,
)
from api.responses import RESOURCE_API_ERRORS
from schemas.project_api import (
    CreateProjectRequest,
    CreateSourceRevisionRequest,
    EpisodeResponse,
    ProjectDetailResponse,
    ProjectListResponse,
    SourceRevisionListResponse,
    SourceRevisionResponse,
    episode_response,
    project_detail_response,
    source_response,
)
from services.project_queries import (
    GetEpisodeHandler,
    GetProjectHandler,
    ListProjectsHandler,
)
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.sources import (
    ConfirmSourceCommand,
    ConfirmSourceHandler,
    CreateSourceRevisionCommand,
    CreateSourceRevisionHandler,
    GetSourceRevisionHandler,
    ListSourceRevisionsHandler,
)

router = APIRouter(prefix="/v1")


@router.post(
    "/projects",
    operation_id="createProject",
    response_model=ProjectDetailResponse,
    status_code=status.HTTP_201_CREATED,
    responses=RESOURCE_API_ERRORS,
)
async def create_project(
    body: CreateProjectRequest,
    request: Request,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> ProjectDetailResponse:
    database = database_from_request(request)
    result = await CreateProjectHandler(database).execute(
        CreateProjectCommand(
            title=body.title,
            idempotency_key=validate_idempotency_key(idempotency_key),
        )
    )
    return project_detail_response(result)


@router.get("/projects", operation_id="listProjects", response_model=ProjectListResponse)
async def list_projects(request: Request) -> ProjectListResponse:
    values = await ListProjectsHandler(database_from_request(request)).execute()
    return ProjectListResponse(items=tuple(project_detail_response(value) for value in values))


@router.get(
    "/projects/{project_id}",
    operation_id="getProject",
    response_model=ProjectDetailResponse,
    responses=RESOURCE_API_ERRORS,
)
async def get_project(project_id: UUID, request: Request) -> ProjectDetailResponse:
    value = await GetProjectHandler(database_from_request(request)).execute(project_id)
    return project_detail_response(value)


@router.get(
    "/episodes/{episode_id}",
    operation_id="getEpisode",
    response_model=EpisodeResponse,
    responses=RESOURCE_API_ERRORS,
)
async def get_episode(episode_id: UUID, request: Request) -> EpisodeResponse:
    value = await GetEpisodeHandler(database_from_request(request)).execute(episode_id)
    return episode_response(value)


@router.post(
    "/episodes/{episode_id}/sources",
    operation_id="createSourceRevision",
    response_model=SourceRevisionResponse,
    status_code=status.HTTP_201_CREATED,
    responses=RESOURCE_API_ERRORS,
)
async def create_source_revision(
    episode_id: UUID,
    body: CreateSourceRevisionRequest,
    request: Request,
    response: Response,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> SourceRevisionResponse:
    value = await CreateSourceRevisionHandler(database_from_request(request)).execute(
        CreateSourceRevisionCommand(
            episode_id=episode_id,
            content=body.content,
            rights_basis=body.rights_basis,
            parent_id=body.parent_id,
            idempotency_key=validate_idempotency_key(idempotency_key),
        )
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return source_response(value)


@router.post(
    "/source-revisions/{version_id}:confirm",
    operation_id="confirmSource",
    response_model=SourceRevisionResponse,
    responses=RESOURCE_API_ERRORS,
)
async def confirm_source(
    version_id: UUID,
    request: Request,
    response: Response,
    if_match: Annotated[str, Header(alias="If-Match")],
) -> SourceRevisionResponse:
    value = await ConfirmSourceHandler(database_from_request(request)).execute(
        ConfirmSourceCommand(version_id, parse_if_match(if_match))
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return source_response(value)


@router.get(
    "/episodes/{episode_id}/source-revisions",
    operation_id="listSourceRevisions",
    response_model=SourceRevisionListResponse,
    responses=RESOURCE_API_ERRORS,
)
async def list_source_revisions(
    episode_id: UUID, request: Request
) -> SourceRevisionListResponse:
    values = await ListSourceRevisionsHandler(database_from_request(request)).execute(episode_id)
    return SourceRevisionListResponse(items=tuple(source_response(value) for value in values))


@router.get(
    "/source-revisions/{version_id}",
    operation_id="getSourceRevision",
    response_model=SourceRevisionResponse,
    responses=RESOURCE_API_ERRORS,
)
async def get_source_revision(
    version_id: UUID, request: Request, response: Response
) -> SourceRevisionResponse:
    value = await GetSourceRevisionHandler(database_from_request(request)).execute(version_id)
    response.headers["ETag"] = strong_etag(value.resource_version)
    return source_response(value)
