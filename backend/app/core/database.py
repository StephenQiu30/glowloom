from collections.abc import AsyncIterator
from urllib.parse import urlsplit

from sqlalchemy import inspect, text
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)
from sqlalchemy.orm import DeclarativeBase

from app.core.config import get_settings


class Base(DeclarativeBase):
    pass


def create_engine(database_url: str) -> AsyncEngine:
    return create_async_engine(database_url, pool_pre_ping=True)


engine = create_engine(get_settings().database_url)
session_factory = async_sessionmaker(engine, expire_on_commit=False)


async def get_async_session() -> AsyncIterator[AsyncSession]:
    async with session_factory() as session:
        yield session


def validate_test_database_url(test_url: str | None, application_url: str) -> str:
    if not test_url:
        raise ValueError("TEST_DATABASE_URL is required for database tests")
    test_name = urlsplit(test_url.replace("postgresql+asyncpg", "postgresql", 1)).path.lstrip("/")
    if not test_name.endswith("_test"):
        raise ValueError("TEST_DATABASE_URL database name must end with _test")
    if test_url == application_url:
        raise ValueError("TEST_DATABASE_URL must not equal DATABASE_URL")
    return test_url


async def database_ping(target_engine: AsyncEngine = engine) -> None:
    async with target_engine.connect() as connection:
        await connection.execute(text("SELECT 1"))


async def initialize_empty_database(target_engine: AsyncEngine = engine) -> None:
    async with target_engine.begin() as connection:
        existing = await connection.run_sync(lambda sync: inspect(sync).get_table_names())
        expected = set(Base.metadata.tables)
        unexpected = set(existing) - expected
        if unexpected:
            names = ", ".join(sorted(unexpected))
            raise RuntimeError(f"database is not empty; unexpected tables: {names}")
        await connection.run_sync(Base.metadata.create_all)
