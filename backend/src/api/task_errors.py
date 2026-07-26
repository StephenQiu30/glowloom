from __future__ import annotations

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from api.problems import HttpProblem, http_problem_handler
from domain.task_states import InvalidStateTransition
from services.tasks import (
    TaskNotFound,
    TaskNotRetryable,
    TaskVersionConflict,
)


def to_problem(error: Exception) -> HttpProblem:
    if isinstance(error, TaskNotFound):
        return HttpProblem(status=404, title="Task not found", code="TASK_NOT_FOUND")
    if isinstance(error, TaskVersionConflict):
        return HttpProblem(status=412, title="Version conflict", code="VERSION_CONFLICT")
    if isinstance(error, TaskNotRetryable):
        return HttpProblem(
            status=409, title="Task is not retryable", code="TASK_NOT_RETRYABLE"
        )
    if isinstance(error, InvalidStateTransition):
        return HttpProblem(status=409, title="Invalid task state", code="INVALID_TASK_STATE")
    raise error


async def production_job_error_handler(request: Request, error: Exception) -> JSONResponse:
    return await http_problem_handler(request, to_problem(error))


def register_production_job_errors(app: FastAPI) -> None:
    for error_type in (
        TaskNotFound,
        TaskVersionConflict,
        TaskNotRetryable,
        InvalidStateTransition,
    ):
        app.add_exception_handler(error_type, production_job_error_handler)
