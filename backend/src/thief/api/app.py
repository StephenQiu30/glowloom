from __future__ import annotations

import secrets
from datetime import UTC, datetime, timedelta
from uuid import uuid4

from fastapi import FastAPI
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker

from thief import __version__
from thief.api.session_routes import SessionService, session_router
from thief.identity.passwords import Argon2PasswordHasher
from thief.identity.sessions import IdentitySessions
from thief.identity.unit_of_work import SqlAlchemyIdentityUnitOfWork
from thief.settings import database_url


def create_app(*, sessions: SessionService | None = None) -> FastAPI:
    app = FastAPI(title="Thief API", version=__version__)
    app.include_router(session_router(sessions or _sessions_from_env()))

    @app.get("/health/live")
    async def live() -> dict[str, str]:
        return {"service": "api", "status": "ok"}

    @app.get("/health/ready")
    async def ready() -> dict[str, str]:
        return {"service": "api", "status": "ok"}

    return app


def _sessions_from_env() -> IdentitySessions:
    engine = create_engine(database_url(), pool_pre_ping=True)
    factory = sessionmaker(engine, expire_on_commit=False)
    return IdentitySessions(
        unit_of_work=lambda: SqlAlchemyIdentityUnitOfWork(factory),
        passwords=Argon2PasswordHasher(),
        now=lambda: datetime.now(UTC),
        new_id=uuid4,
        new_secret=lambda: secrets.token_urlsafe(32),
        session_ttl=timedelta(hours=8),
    )


app = create_app()
