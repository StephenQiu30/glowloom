from __future__ import annotations

from fastapi import FastAPI
from fastapi.exceptions import RequestValidationError
from fastapi.middleware.cors import CORSMiddleware

from lanverse.bootstrap.container import create_container
from lanverse.bootstrap.lifespan import create_lifespan
from lanverse.modules.project_catalog.transport.errors import register_project_catalog_errors
from lanverse.modules.project_catalog.transport.router import router as project_catalog_router
from lanverse.shared_kernel.config import ApplicationSettings
from lanverse.shared_kernel.http_errors import (
    HttpProblem,
    http_problem_handler,
    request_validation_problem_handler,
)


def create_app(settings: ApplicationSettings | None = None) -> FastAPI:
    resolved = settings or ApplicationSettings()
    container = create_container(resolved)
    openapi_url = "/openapi.json" if resolved.expose_docs else None
    docs_url = "/docs" if resolved.expose_docs else None

    app = FastAPI(
        title="Lanverse API",
        version="0.1.0",
        docs_url=docs_url,
        redoc_url=None,
        openapi_url=openapi_url,
        lifespan=create_lifespan(container),
    )
    app.add_exception_handler(HttpProblem, http_problem_handler)
    app.add_exception_handler(RequestValidationError, request_validation_problem_handler)
    register_project_catalog_errors(app)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["http://127.0.0.1:3000"],
        allow_credentials=False,
        allow_methods=["GET", "POST", "PUT"],
        allow_headers=["Content-Type", "Idempotency-Key", "If-Match"],
        expose_headers=["ETag"],
    )
    app.include_router(project_catalog_router)
    return app
