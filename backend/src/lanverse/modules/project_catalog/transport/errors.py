from __future__ import annotations

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from lanverse.infrastructure.idempotency.repository import IdempotencyKeyReused
from lanverse.modules.project_catalog.application.queries import ProjectNotFound
from lanverse.modules.project_catalog.application.sources import (
    EpisodeNotFound,
    InvalidRightsBasis,
    SourceParentNotFound,
    SourceRevisionNotFound,
    VersionConflict,
    VersionImmutable,
)
from lanverse.modules.project_catalog.domain.values import ProjectCatalogValidationError
from lanverse.shared_kernel.http_errors import HttpProblem, http_problem_handler


def to_problem(error: Exception) -> HttpProblem:
    if isinstance(error, ProjectCatalogValidationError):
        field = "title" if error.code.startswith("PROJECT_TITLE") else "content"
        metadata: dict[str, str | int | bool | None] = dict(error.metadata)
        return HttpProblem(
            status=422,
            title="Invalid project input",
            code=error.code,
            metadata=metadata,
            detail=f"The {field} field does not satisfy the accepted contract.",
        )
    if isinstance(error, InvalidRightsBasis):
        return HttpProblem(status=422, title="Invalid rights basis", code="RIGHTS_BASIS_INVALID")
    if isinstance(error, IdempotencyKeyReused):
        return HttpProblem(
            status=409, title="Idempotency key reused", code="IDEMPOTENCY_KEY_REUSED"
        )
    if isinstance(error, VersionConflict):
        return HttpProblem(status=412, title="Version conflict", code="VERSION_CONFLICT")
    if isinstance(error, VersionImmutable):
        return HttpProblem(status=409, title="Version is immutable", code="VERSION_IMMUTABLE")
    if isinstance(error, SourceParentNotFound):
        return HttpProblem(status=422, title="Invalid source parent", code="SOURCE_PARENT_INVALID")
    if isinstance(error, (ProjectNotFound, EpisodeNotFound, SourceRevisionNotFound)):
        return HttpProblem(status=404, title="Resource not found", code="RESOURCE_NOT_FOUND")
    raise error


async def project_catalog_error_handler(request: Request, error: Exception) -> JSONResponse:
    return await http_problem_handler(request, to_problem(error))


def register_project_catalog_errors(app: FastAPI) -> None:
    for error_type in (
        ProjectCatalogValidationError,
        InvalidRightsBasis,
        IdempotencyKeyReused,
        VersionConflict,
        VersionImmutable,
        SourceParentNotFound,
        ProjectNotFound,
        EpisodeNotFound,
        SourceRevisionNotFound,
    ):
        app.add_exception_handler(error_type, project_catalog_error_handler)
