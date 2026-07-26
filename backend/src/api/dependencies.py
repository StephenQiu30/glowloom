from __future__ import annotations

from typing import cast

from fastapi import Request

from api.problems import HttpProblem
from core.clock import Clock
from db.pool import DatabasePool
from integrations.object_storage import ObjectStore


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


def object_store_from_request(request: Request) -> ObjectStore:
    object_store = request.app.state.runtime.object_store
    if object_store is None:
        raise HttpProblem(
            status=503,
            title="Object storage unavailable",
            code="OBJECT_STORAGE_NOT_CONFIGURED",
            retryable=True,
        )
    return cast(ObjectStore, object_store)


def clock_from_request(request: Request) -> Clock:
    return cast(Clock, request.app.state.runtime.clock)
