"""Canonical ASGI entry point for the single Lanverse Agent service."""

from __future__ import annotations

from collections.abc import Callable

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from psycopg import Error as DatabaseError
from temporalio.client import Client

from app.api.router import router
from app.core.config import Settings
from app.core.container import AgentRuntime
from app.core.lifespan import lifespan
from app.creation.execution import ExecutionConflict
from app.creation.repository import Repository, SchemaMismatch
from app.skills.catalog import SkillCatalog
from app.skills.runtime import SkillRuntime


def create_app(
    repository: Repository | None = None,
    secret: str | None = None,
    task_queue: str | None = None,
    temporal: Client | None = None,
    runtime_ready: Callable[[], bool] | None = None,
    settings: Settings | None = None,
    skill_catalog: SkillCatalog | None = None,
) -> FastAPI:
    """Build one FastAPI application with explicit runtime dependencies.

    Production calls this factory without overrides and receives validated
    environment settings. Tests pass repositories, clients and catalogs directly
    so no infrastructure is started by module import.
    """

    if settings is None and repository is None:
        settings = Settings.from_environment()
    if repository is None:
        if settings is None:
            raise ValueError("settings are required when repository is omitted")
        repository = Repository(settings.database_url)
    if secret is None:
        if settings is None:
            raise ValueError("secret is required when settings are omitted")
        secret = settings.secret
    if task_queue is None:
        task_queue = settings.task_queue if settings is not None else "lanverse-creation-text"
    runtime = AgentRuntime(
        repository=repository,
        secret=secret,
        task_queue=task_queue,
        settings=settings,
        temporal=temporal,
        skill_runtime=SkillRuntime(skill_catalog or SkillCatalog()),
        runtime_ready=runtime_ready,
    )
    application = FastAPI(
        title="Lanverse Agent",
        docs_url=None,
        redoc_url=None,
        openapi_url=None,
        lifespan=lifespan,
    )
    application.state.runtime = runtime
    application.include_router(router)
    application.add_exception_handler(DatabaseError, _storage_unavailable)
    application.add_exception_handler(SchemaMismatch, _storage_unavailable)
    application.add_exception_handler(ExecutionConflict, _execution_conflict)
    return application


async def _storage_unavailable(_: Request, __: Exception) -> JSONResponse:
    return JSONResponse({"detail": "creation_storage_unavailable"}, status_code=503)


async def _execution_conflict(_: Request, __: Exception) -> JSONResponse:
    return JSONResponse({"detail": "creation_execution_conflict"}, status_code=409)


__all__ = ["create_app"]
