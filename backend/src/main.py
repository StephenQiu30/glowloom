from __future__ import annotations

import uvicorn
from fastapi import FastAPI
from fastapi.exceptions import RequestValidationError
from fastapi.middleware.cors import CORSMiddleware

from api.problems import (
    HttpProblem,
    http_problem_handler,
    request_validation_problem_handler,
)
from api.project_errors import register_project_catalog_errors
from api.router import router
from api.story_errors import register_story_development_errors
from api.task_errors import register_production_job_errors
from core.config import ApplicationSettings, load_settings
from core.lifespan import create_lifespan
from core.logging import configure_logging
from core.runtime import create_runtime


def create_app(settings: ApplicationSettings | None = None) -> FastAPI:
    resolved = settings or ApplicationSettings()
    runtime = create_runtime(resolved)
    app = FastAPI(
        title="Lanverse API",
        version="0.1.0",
        docs_url="/docs" if resolved.expose_docs else None,
        redoc_url=None,
        openapi_url="/openapi.json" if resolved.expose_docs else None,
        lifespan=create_lifespan(runtime),
    )
    app.add_exception_handler(HttpProblem, http_problem_handler)
    app.add_exception_handler(RequestValidationError, request_validation_problem_handler)
    register_project_catalog_errors(app)
    register_production_job_errors(app)
    register_story_development_errors(app)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["http://127.0.0.1:3000"],
        allow_credentials=False,
        allow_methods=["GET", "POST", "PUT"],
        allow_headers=["Content-Type", "Idempotency-Key", "If-Match"],
        expose_headers=["ETag"],
    )
    app.include_router(router)
    return app


def run() -> None:
    settings = load_settings()
    configure_logging(settings.log_level)
    uvicorn.run(
        create_app(settings),
        host=settings.api_host,
        port=settings.api_port,
        log_level=settings.log_level.lower(),
    )
