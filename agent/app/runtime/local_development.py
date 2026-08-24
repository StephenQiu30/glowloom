import asyncio
import os
import re

from sqlalchemy import text
from sqlalchemy.engine import URL, make_url
from sqlalchemy.ext.asyncio import create_async_engine

from app.core.config import Settings, get_settings

SAFE_DATABASE_NAME = re.compile(r"^[a-z][a-z0-9_]{0,62}$")


def resolve_local_database_url(
    settings: Settings,
    *,
    requested_name: str | None,
) -> URL:
    if settings.environment != "development":
        raise ValueError("the local runtime is restricted to the development environment")

    source = make_url(settings.database_url)
    if not source.drivername.startswith("postgresql+") or source.database is None:
        raise ValueError("the local runtime requires an async PostgreSQL database URL")

    database_name = requested_name or f"{source.database}_development"
    if SAFE_DATABASE_NAME.fullmatch(database_name) is None:
        raise ValueError("local database name must use lowercase letters, numbers, and underscores")
    if database_name == source.database:
        raise ValueError("local database name must not overwrite the configured database")
    return source.set(database=database_name)


async def ensure_local_database(target: URL) -> None:
    database_name = target.database
    if database_name is None or SAFE_DATABASE_NAME.fullmatch(database_name) is None:
        raise ValueError("local database name is invalid")

    admin_engine = create_async_engine(
        target.set(database="postgres"),
        isolation_level="AUTOCOMMIT",
    )
    try:
        async with admin_engine.connect() as connection:
            exists = await connection.scalar(
                text("SELECT 1 FROM pg_database WHERE datname = :database_name"),
                {"database_name": database_name},
            )
            if exists is None:
                await connection.execute(text(f'CREATE DATABASE "{database_name}"'))
    finally:
        await admin_engine.dispose()


async def run_local_development() -> None:
    initial_settings = Settings()
    database_url = resolve_local_database_url(
        initial_settings,
        requested_name=os.getenv("LOCAL_DATABASE_NAME") or None,
    )
    await ensure_local_database(database_url)

    os.environ["DATABASE_URL"] = database_url.render_as_string(hide_password=False)
    get_settings.cache_clear()

    from app.runtime.api import run_api
    from app.runtime.commands.database import prepare_database

    active_settings = get_settings()
    await prepare_database(active_settings)
    await run_api(active_settings)


def main() -> None:
    asyncio.run(run_local_development())


if __name__ == "__main__":
    main()
