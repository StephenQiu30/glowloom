from __future__ import annotations

import secrets
from datetime import UTC, datetime, timedelta
from uuid import uuid4

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from sqlalchemy import create_engine
from sqlalchemy.orm import Session, sessionmaker

from thief import __version__
from thief.api.catalog_routes import catalog_router
from thief.api.errors import error_response
from thief.api.session_routes import SessionService, session_router
from thief.catalog.query import CatalogQueries
from thief.catalog.query_repository import SqlAlchemyCatalogQueryRepository
from thief.identity.passwords import Argon2PasswordHasher
from thief.identity.sessions import IdentitySessions
from thief.identity.unit_of_work import SqlAlchemyIdentityUnitOfWork
from thief.settings import database_url


def create_app(
    *,
    sessions: SessionService | None = None,
    catalog: CatalogQueries | None = None,
) -> FastAPI:
    factory = _session_factory() if sessions is None or catalog is None else None
    app = FastAPI(title="Thief API", version=__version__)
    app.include_router(session_router(sessions or _sessions(factory)))
    app.include_router(
        catalog_router(catalog or SqlAlchemyCatalogQueryRepository(_require(factory)))
    )

    @app.exception_handler(RequestValidationError)
    async def invalid_request(
        request: Request,
        error: RequestValidationError,
    ) -> JSONResponse:
        del error
        catalog_path = request.url.path.startswith("/v1/templates") or (
            request.url.path in {"/v1/search", "/v1/categories"}
        )
        code = "invalid_query" if catalog_path else "invalid_request"
        return error_response(400, code, "Request is invalid.")

    @app.get("/health/live")
    async def live() -> dict[str, str]:
        return {"service": "api", "status": "ok"}

    @app.get("/health/ready")
    async def ready() -> dict[str, str]:
        return {"service": "api", "status": "ok"}

    return app


def _session_factory() -> sessionmaker[Session]:
    engine = create_engine(database_url(), pool_pre_ping=True)
    return sessionmaker(engine, expire_on_commit=False)


def _sessions(factory: sessionmaker[Session] | None) -> IdentitySessions:
    return IdentitySessions(
        unit_of_work=lambda: SqlAlchemyIdentityUnitOfWork(_require(factory)),
        passwords=Argon2PasswordHasher(),
        now=lambda: datetime.now(UTC),
        new_id=uuid4,
        new_secret=lambda: secrets.token_urlsafe(32),
        session_ttl=timedelta(hours=8),
    )


def _require(factory: sessionmaker[Session] | None) -> sessionmaker[Session]:
    if factory is None:
        raise RuntimeError("database session factory is required")
    return factory


app = create_app()
