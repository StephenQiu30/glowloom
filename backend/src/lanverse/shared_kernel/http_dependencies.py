from __future__ import annotations

from fastapi import Request

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.shared_kernel.http_errors import HttpProblem


def database_from_request(request: Request) -> DatabasePool:
    database = request.app.state.container.database
    if not isinstance(database, DatabasePool):
        raise HttpProblem(
            status=503,
            title="Database unavailable",
            code="DATABASE_NOT_CONFIGURED",
            retryable=True,
        )
    return database
