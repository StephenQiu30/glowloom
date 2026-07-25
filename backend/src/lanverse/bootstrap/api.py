from __future__ import annotations

from fastapi import FastAPI

from lanverse.bootstrap.container import create_container
from lanverse.bootstrap.lifespan import create_lifespan
from lanverse.shared_kernel.config import ApplicationSettings


def create_app(settings: ApplicationSettings | None = None) -> FastAPI:
    resolved = settings or ApplicationSettings()
    container = create_container(resolved)
    openapi_url = "/openapi.json" if resolved.expose_docs else None
    docs_url = "/docs" if resolved.expose_docs else None

    return FastAPI(
        title="Lanverse API",
        version="0.1.0",
        docs_url=docs_url,
        redoc_url=None,
        openapi_url=openapi_url,
        lifespan=create_lifespan(container),
    )
