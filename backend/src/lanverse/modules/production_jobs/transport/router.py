from __future__ import annotations

from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, Header, Query, Request, Response, status

from lanverse.modules.production_jobs.application.tasks import (
    CancelTaskHandler,
    RetryTaskHandler,
    TaskQueryService,
)
from lanverse.modules.production_jobs.transport.schemas import (
    TaskListResponse,
    task_accepted,
    task_response,
)
from lanverse.shared_kernel.http_contracts import Problem, TaskAccepted, TaskResponse
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


@router.get("/tasks", operation_id="listTasks", response_model=TaskListResponse)
async def list_tasks(
    request: Request, episode_id: Annotated[UUID, Query()]
) -> TaskListResponse:
    values = await TaskQueryService(database_from_request(request)).list(episode_id)
    return TaskListResponse(items=tuple(task_response(value) for value in values))


@router.get(
    "/tasks/{task_id}",
    operation_id="getTask",
    response_model=TaskResponse,
    responses=ERROR_RESPONSES,
)
async def get_task(task_id: UUID, request: Request, response: Response) -> TaskResponse:
    value = await TaskQueryService(database_from_request(request)).get(task_id)
    response.headers["ETag"] = strong_etag(value.resource_version)
    return task_response(value)


@router.post(
    "/tasks/{task_id}:cancel",
    operation_id="cancelTask",
    response_model=TaskResponse,
    responses=ERROR_RESPONSES,
)
async def cancel_task(
    task_id: UUID,
    request: Request,
    response: Response,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
    if_match: Annotated[str, Header(alias="If-Match")],
) -> TaskResponse:
    value = await CancelTaskHandler(database_from_request(request)).execute(
        task_id,
        parse_if_match(if_match),
        validate_idempotency_key(idempotency_key),
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return task_response(value)


@router.post(
    "/tasks/{task_id}:retry",
    operation_id="retryTask",
    response_model=TaskAccepted,
    status_code=status.HTTP_202_ACCEPTED,
    responses=ERROR_RESPONSES,
)
async def retry_task(
    task_id: UUID,
    request: Request,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
    if_match: Annotated[str, Header(alias="If-Match")],
) -> TaskAccepted:
    value = await RetryTaskHandler(
        database_from_request(request), release_version="0.1.0"
    ).execute(
        task_id,
        parse_if_match(if_match),
        validate_idempotency_key(idempotency_key),
    )
    return task_accepted(value)
