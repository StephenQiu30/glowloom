from __future__ import annotations

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

from api.problems import (
    HttpProblem,
    http_problem_handler,
    request_validation_problem_handler,
)
from domain.projects import ProjectCatalogValidationError
from domain.task_states import InvalidStateTransition
from integrations.object_storage import ObjectStoreUnavailable
from repositories.idempotency import IdempotencyKeyReused
from services.adoptions import (
    AdoptionCandidateNotFound,
    AdoptionInputOutdated,
    CandidateNotAdoptable,
)
from services.asset_versions import (
    CreativeAssetIdentityInvalid,
    CreativeAssetVersionNotFound,
)
from services.candidates import CandidateNotFound, CandidateQueryInvalid
from services.media_generation import (
    MediaInputNotFound,
    MediaInputOutdated,
    UnsupportedMediaUsage,
)
from services.project_queries import ProjectNotFound
from services.project_reader import ConfirmedSourceNotFound
from services.script_versions import (
    ScriptVersionNotFound,
)
from services.script_versions import (
    VersionConflict as ScriptVersionConflict,
)
from services.script_versions import (
    VersionImmutable as ScriptVersionImmutable,
)
from services.sources import (
    EpisodeNotFound,
    InvalidRightsBasis,
    SourceParentNotFound,
    SourceRevisionNotFound,
)
from services.sources import (
    VersionConflict as SourceVersionConflict,
)
from services.sources import (
    VersionImmutable as SourceVersionImmutable,
)
from services.storyboard_versions import (
    StoryboardVersionNotFound,
    StoryReferenceInvalid,
)
from services.tasks import TaskNotFound, TaskNotRetryable, TaskVersionConflict


def to_problem(error: Exception) -> HttpProblem:
    if isinstance(error, ProjectCatalogValidationError):
        field = "title" if error.code.startswith("PROJECT_TITLE") else "content"
        return HttpProblem(
            status=422,
            title="Invalid project input",
            code=error.code,
            metadata=dict(error.metadata),
            detail=f"The {field} field does not satisfy the accepted contract.",
        )
    if isinstance(error, InvalidRightsBasis):
        return HttpProblem(status=422, title="Invalid rights basis", code="RIGHTS_BASIS_INVALID")
    if isinstance(error, IdempotencyKeyReused):
        return HttpProblem(
            status=409,
            title="Idempotency key reused",
            code="IDEMPOTENCY_KEY_REUSED",
        )
    if isinstance(error, (SourceVersionConflict, ScriptVersionConflict, TaskVersionConflict)):
        return HttpProblem(status=412, title="Version conflict", code="VERSION_CONFLICT")
    if isinstance(error, (MediaInputOutdated, AdoptionInputOutdated)):
        return HttpProblem(
            status=412,
            title="Input is outdated",
            code="VERSION_CONFLICT",
            detail=str(error),
        )
    if isinstance(error, (SourceVersionImmutable, ScriptVersionImmutable)):
        return HttpProblem(status=409, title="Version is immutable", code="VERSION_IMMUTABLE")
    if isinstance(error, SourceParentNotFound):
        return HttpProblem(status=422, title="Invalid source parent", code="SOURCE_PARENT_INVALID")
    if isinstance(error, (StoryReferenceInvalid, CreativeAssetIdentityInvalid)):
        return HttpProblem(
            status=422,
            title="Invalid story reference",
            code="STORY_REFERENCE_INVALID",
            detail=str(error),
        )
    if isinstance(error, UnsupportedMediaUsage):
        return HttpProblem(
            status=422,
            title="Media usage is unsupported",
            code="MEDIA_USAGE_UNSUPPORTED",
            detail=str(error),
        )
    if isinstance(error, (CandidateNotFound, AdoptionCandidateNotFound)):
        return HttpProblem(status=404, title="Candidate not found", code="CANDIDATE_NOT_FOUND")
    if isinstance(error, CandidateNotAdoptable):
        return HttpProblem(
            status=422,
            title="Candidate is not adoptable",
            code="CANDIDATE_NOT_ADOPTABLE",
            detail=str(error),
        )
    if isinstance(error, CandidateQueryInvalid):
        return HttpProblem(
            status=422,
            title="Candidate query is invalid",
            code="CANDIDATE_QUERY_INVALID",
        )
    if isinstance(error, ObjectStoreUnavailable):
        return HttpProblem(
            status=503,
            title="Object storage unavailable",
            code="OBJECT_STORAGE_UNAVAILABLE",
            retryable=True,
        )
    if isinstance(error, TaskNotFound):
        return HttpProblem(status=404, title="Task not found", code="TASK_NOT_FOUND")
    if isinstance(error, TaskNotRetryable):
        return HttpProblem(status=409, title="Task is not retryable", code="TASK_NOT_RETRYABLE")
    if isinstance(error, InvalidStateTransition):
        return HttpProblem(status=409, title="Invalid task state", code="INVALID_TASK_STATE")
    if isinstance(
        error,
        (
            ProjectNotFound,
            EpisodeNotFound,
            SourceRevisionNotFound,
            ConfirmedSourceNotFound,
            ScriptVersionNotFound,
            CreativeAssetVersionNotFound,
            StoryboardVersionNotFound,
            MediaInputNotFound,
        ),
    ):
        return HttpProblem(status=404, title="Resource not found", code="RESOURCE_NOT_FOUND")
    raise error


async def api_error_handler(request: Request, error: Exception) -> JSONResponse:
    return await http_problem_handler(request, to_problem(error))


BUSINESS_ERRORS = (
    ProjectCatalogValidationError,
    InvalidRightsBasis,
    IdempotencyKeyReused,
    SourceVersionConflict,
    ScriptVersionConflict,
    TaskVersionConflict,
    MediaInputOutdated,
    AdoptionInputOutdated,
    SourceVersionImmutable,
    ScriptVersionImmutable,
    SourceParentNotFound,
    StoryReferenceInvalid,
    CreativeAssetIdentityInvalid,
    UnsupportedMediaUsage,
    CandidateNotFound,
    AdoptionCandidateNotFound,
    CandidateNotAdoptable,
    CandidateQueryInvalid,
    ObjectStoreUnavailable,
    TaskNotFound,
    TaskNotRetryable,
    InvalidStateTransition,
    ProjectNotFound,
    EpisodeNotFound,
    SourceRevisionNotFound,
    ConfirmedSourceNotFound,
    ScriptVersionNotFound,
    CreativeAssetVersionNotFound,
    StoryboardVersionNotFound,
    MediaInputNotFound,
)


def register_api_errors(app: FastAPI) -> None:
    app.add_exception_handler(HttpProblem, http_problem_handler)
    app.add_exception_handler(RequestValidationError, request_validation_problem_handler)
    for error_type in BUSINESS_ERRORS:
        app.add_exception_handler(error_type, api_error_handler)
