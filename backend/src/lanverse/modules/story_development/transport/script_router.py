from __future__ import annotations

from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, Header, Request, Response, status

from lanverse.modules.production_jobs.transport.schemas import task_accepted
from lanverse.modules.story_development.application.generate import (
    GenerateScriptCommand,
    GenerateScriptHandler,
)
from lanverse.modules.story_development.application.scripts import (
    ConfirmScriptCommand,
    ConfirmScriptHandler,
    DeriveScriptDraftCommand,
    DeriveScriptDraftHandler,
    GetCurrentScriptHandler,
    GetScriptVersionHandler,
    ListScriptVersionsHandler,
    SaveScriptCommand,
    SaveScriptHandler,
)
from lanverse.modules.story_development.transport.script_schemas import (
    SaveScriptRequest,
    ScriptVersionListResponse,
    ScriptVersionResponse,
    script_response,
)
from lanverse.shared_kernel.http_contracts import Problem, TaskAccepted
from lanverse.shared_kernel.http_dependencies import database_from_request
from lanverse.shared_kernel.http_headers import (
    parse_if_match,
    strong_etag,
    validate_idempotency_key,
)

router = APIRouter(prefix="/v1")
ERROR_RESPONSES: dict[int | str, dict[str, Any]] = {
    404: {"model": Problem},
    409: {"model": Problem},
    412: {"model": Problem},
    422: {"model": Problem},
}


@router.post(
    "/episodes/{episode_id}/script-generations",
    operation_id="generateScript",
    response_model=TaskAccepted,
    status_code=status.HTTP_202_ACCEPTED,
    responses=ERROR_RESPONSES,
)
async def generate_script(
    episode_id: UUID,
    request: Request,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> TaskAccepted:
    value = await GenerateScriptHandler(
        database_from_request(request), release_version="0.1.0"
    ).execute(
        GenerateScriptCommand(episode_id, validate_idempotency_key(idempotency_key))
    )
    return task_accepted(value)


@router.get(
    "/episodes/{episode_id}/script",
    operation_id="getCurrentScript",
    response_model=ScriptVersionResponse,
    responses=ERROR_RESPONSES,
)
async def get_current_script(
    episode_id: UUID, request: Request, response: Response
) -> ScriptVersionResponse:
    value = await GetCurrentScriptHandler(database_from_request(request)).execute(episode_id)
    response.headers["ETag"] = strong_etag(value.resource_version)
    return script_response(value)


@router.put(
    "/script-versions/{version_id}",
    operation_id="saveScript",
    response_model=ScriptVersionResponse,
    responses=ERROR_RESPONSES,
)
async def save_script(
    version_id: UUID,
    body: SaveScriptRequest,
    request: Request,
    response: Response,
    if_match: Annotated[str, Header(alias="If-Match")],
) -> ScriptVersionResponse:
    value = await SaveScriptHandler(database_from_request(request)).execute(
        SaveScriptCommand(version_id, parse_if_match(if_match), body.content)
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return script_response(value)


@router.post(
    "/script-versions/{version_id}:confirm",
    operation_id="confirmScript",
    response_model=ScriptVersionResponse,
    responses=ERROR_RESPONSES,
)
async def confirm_script(
    version_id: UUID,
    request: Request,
    response: Response,
    if_match: Annotated[str, Header(alias="If-Match")],
) -> ScriptVersionResponse:
    value = await ConfirmScriptHandler(database_from_request(request)).execute(
        ConfirmScriptCommand(version_id, parse_if_match(if_match))
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return script_response(value)


@router.post(
    "/script-versions/{version_id}/drafts",
    operation_id="deriveScriptDraft",
    response_model=ScriptVersionResponse,
    status_code=status.HTTP_201_CREATED,
    responses=ERROR_RESPONSES,
)
async def derive_script_draft(
    version_id: UUID,
    request: Request,
    response: Response,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> ScriptVersionResponse:
    value = await DeriveScriptDraftHandler(database_from_request(request)).execute(
        DeriveScriptDraftCommand(version_id, validate_idempotency_key(idempotency_key))
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return script_response(value)


@router.get(
    "/episodes/{episode_id}/script-versions",
    operation_id="listScriptVersions",
    response_model=ScriptVersionListResponse,
    responses=ERROR_RESPONSES,
)
async def list_script_versions(
    episode_id: UUID, request: Request
) -> ScriptVersionListResponse:
    values = await ListScriptVersionsHandler(database_from_request(request)).execute(episode_id)
    return ScriptVersionListResponse(items=tuple(script_response(item) for item in values))


@router.get(
    "/script-versions/{version_id}",
    operation_id="getScriptVersion",
    response_model=ScriptVersionResponse,
    responses=ERROR_RESPONSES,
)
async def get_script_version(
    version_id: UUID, request: Request, response: Response
) -> ScriptVersionResponse:
    value = await GetScriptVersionHandler(database_from_request(request)).execute(version_id)
    response.headers["ETag"] = strong_etag(value.resource_version)
    return script_response(value)
