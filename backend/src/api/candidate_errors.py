from __future__ import annotations

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from api.problems import HttpProblem, http_problem_handler
from integrations.object_storage import ObjectStoreUnavailable
from services.candidates import CandidateNotFound, CandidateQueryInvalid


async def candidate_error_handler(request: Request, error: Exception) -> JSONResponse:
    if isinstance(error, CandidateNotFound):
        problem = HttpProblem(
            status=404,
            title="Candidate not found",
            code="CANDIDATE_NOT_FOUND",
        )
    elif isinstance(error, CandidateQueryInvalid):
        problem = HttpProblem(
            status=422,
            title="Candidate query is invalid",
            code="CANDIDATE_QUERY_INVALID",
        )
    elif isinstance(error, ObjectStoreUnavailable):
        problem = HttpProblem(
            status=503,
            title="Object storage unavailable",
            code="OBJECT_STORAGE_UNAVAILABLE",
            retryable=True,
        )
    else:
        raise error
    return await http_problem_handler(request, problem)


def register_candidate_errors(app: FastAPI) -> None:
    for error_type in (
        CandidateNotFound,
        CandidateQueryInvalid,
        ObjectStoreUnavailable,
    ):
        app.add_exception_handler(error_type, candidate_error_handler)
