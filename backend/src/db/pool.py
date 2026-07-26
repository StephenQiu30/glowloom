from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import asyncpg  # type: ignore[import-untyped]

from core.config import ApplicationSettings


class DatabasePool:
    def __init__(self, dsn: str, *, min_size: int = 1, max_size: int = 10) -> None:
        if min_size > max_size:
            raise ValueError("min_size cannot exceed max_size")
        self._dsn = dsn
        self._min_size = min_size
        self._max_size = max_size
        self._pool: asyncpg.Pool[asyncpg.Record] | None = None

    @classmethod
    def from_settings(cls, settings: ApplicationSettings) -> DatabasePool:
        return cls(
            settings.require_database_url(),
            min_size=settings.database_pool_min_size,
            max_size=settings.database_pool_max_size,
        )

    async def start(self) -> None:
        if self._pool is not None:
            return
        self._pool = await asyncpg.create_pool(
            dsn=self._dsn,
            min_size=self._min_size,
            max_size=self._max_size,
            command_timeout=30,
            server_settings={"application_name": "lanverse", "timezone": "UTC"},
        )

    async def close(self) -> None:
        if self._pool is None:
            return
        pool, self._pool = self._pool, None
        await pool.close()

    @asynccontextmanager
    async def transaction(self) -> AsyncIterator[asyncpg.Connection[asyncpg.Record]]:
        if self._pool is None:
            raise RuntimeError("database pool is not started")
        async with self._pool.acquire() as connection, connection.transaction(
            isolation="read_committed"
        ):
            yield connection
