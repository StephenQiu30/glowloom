from __future__ import annotations

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from lanverse.infrastructure.idempotency.repository import IdempotencyKeyReused
from lanverse.modules.project_catalog.public import ConfirmedSourceNotFound
from lanverse.modules.story_development.application.asset_versions import (
    CreativeAssetIdentityInvalid,
    CreativeAssetVersionNotFound,
)
from lanverse.modules.story_development.application.scripts import (
    ScriptVersionNotFound,
    VersionConflict,
    VersionImmutable,
)
from lanverse.modules.story_development.application.storyboard_versions import (
    StoryboardVersionNotFound,
    StoryReferenceInvalid,
)
from lanverse.shared_kernel.http_errors import HttpProblem, http_problem_handler


def to_problem(error: Exception) -> HttpProblem:
    if isinstance(error, IdempotencyKeyReused):
        return HttpProblem(
            status=409,
            title="Idempotency key reused",
            code="IDEMPOTENCY_KEY_REUSED",
        )
    if isinstance(error, VersionConflict):
        return HttpProblem(status=412, title="Version conflict", code="VERSION_CONFLICT")
    if isinstance(error, VersionImmutable):
        return HttpProblem(
            status=409, title="Version is immutable", code="VERSION_IMMUTABLE"
        )
    if isinstance(error, (StoryReferenceInvalid, CreativeAssetIdentityInvalid)):
        return HttpProblem(
            status=422,
            title="Invalid story reference",
            code="STORY_REFERENCE_INVALID",
            detail=str(error),
        )
    if isinstance(
        error,
        (
            ConfirmedSourceNotFound,
            ScriptVersionNotFound,
            CreativeAssetVersionNotFound,
            StoryboardVersionNotFound,
        ),
    ):
        return HttpProblem(status=404, title="Story resource not found", code="RESOURCE_NOT_FOUND")
    raise error


async def story_development_error_handler(
    request: Request, error: Exception
) -> JSONResponse:
    return await http_problem_handler(request, to_problem(error))


def register_story_development_errors(app: FastAPI) -> None:
    for error_type in (
        IdempotencyKeyReused,
        VersionConflict,
        VersionImmutable,
        StoryReferenceInvalid,
        CreativeAssetIdentityInvalid,
        ConfirmedSourceNotFound,
        ScriptVersionNotFound,
        CreativeAssetVersionNotFound,
        StoryboardVersionNotFound,
    ):
        app.add_exception_handler(error_type, story_development_error_handler)
