from __future__ import annotations

from typing import Any

from fastapi import Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

from lanverse.shared_kernel.errors import ApplicationError
from lanverse.shared_kernel.http_contracts import Problem, ProblemFieldError
from lanverse.shared_kernel.ids import new_id


class HttpProblem(ApplicationError):
    def __init__(
        self,
        *,
        status: int,
        title: str,
        code: str,
        retryable: bool = False,
        detail: str | None = None,
        errors: tuple[ProblemFieldError, ...] | None = None,
        metadata: dict[str, str | int | bool | None] | None = None,
    ) -> None:
        super().__init__(code)
        self.status = status
        self.title = title
        self.code = code
        self.retryable = retryable
        self.detail = detail
        self.errors = errors
        self.metadata = metadata


async def http_problem_handler(request: Request, error: Exception) -> JSONResponse:
    if not isinstance(error, HttpProblem):
        raise error
    problem = Problem(
        type=f"https://lanverse.local/problems/{error.code.lower().replace('_', '-')}",
        title=error.title,
        status=error.status,
        code=error.code,
        retryable=error.retryable,
        request_id=new_id(),
        detail=error.detail,
        errors=error.errors,
        metadata=error.metadata,
    )
    content: dict[str, Any] = problem.model_dump(mode="json", exclude_none=True)
    return JSONResponse(
        status_code=error.status,
        content=content,
        media_type="application/problem+json",
    )


async def request_validation_problem_handler(
    request: Request, error: Exception
) -> JSONResponse:
    if not isinstance(error, RequestValidationError):
        raise error
    errors = tuple(
        ProblemFieldError(
            field=".".join(str(part) for part in item["loc"] if part not in {"body", "header"}),
            code=str(item["type"]).upper(),
            message="The request field is invalid.",
        )
        for item in error.errors()
    )
    return await http_problem_handler(
        request,
        HttpProblem(
            status=422,
            title="Request validation failed",
            code="VALIDATION_ERROR",
            errors=errors,
        ),
    )
