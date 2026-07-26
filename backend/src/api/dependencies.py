from __future__ import annotations

from fastapi import Request

from api.problems import HttpProblem
from db.pool import DatabasePool


def database_from_request(request: Request) -> DatabasePool:
    database = request.app.state.runtime.database
    if not isinstance(database, DatabasePool):
        raise HttpProblem(
            status=503,
            title="Database unavailable",
            code="DATABASE_NOT_CONFIGURED",
            retryable=True,
        )
    return database
