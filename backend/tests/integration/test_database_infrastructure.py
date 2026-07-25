from __future__ import annotations

import asyncpg
import pytest

from lanverse.infrastructure.database.errors import (
    DatabaseConflictError,
    DatabaseUnavailableError,
    translate_database_error,
)
from lanverse.infrastructure.database.pool import DatabasePool


@pytest.mark.asyncio
async def test_pool_opens_read_committed_utc_transactions() -> None:
    database = DatabasePool("postgresql:///postgres", min_size=1, max_size=2)
    await database.start()
    try:
        async with database.transaction() as connection:
            isolation = await connection.fetchval("SHOW transaction_isolation")
            timezone = await connection.fetchval("SHOW timezone")

        assert isolation == "read committed"
        assert timezone == "UTC"
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_pool_lifecycle_is_explicit_and_idempotent() -> None:
    database = DatabasePool("postgresql:///postgres", min_size=1, max_size=1)

    with pytest.raises(RuntimeError, match="not started"):
        async with database.transaction():
            pass

    await database.start()
    await database.start()
    await database.close()
    await database.close()


def test_database_errors_are_translated_without_driver_details() -> None:
    conflict = translate_database_error(asyncpg.UniqueViolationError("sensitive row"))
    unavailable = translate_database_error(asyncpg.CannotConnectNowError("private host"))

    assert isinstance(conflict, DatabaseConflictError)
    assert conflict.code == "database_conflict"
    assert "sensitive row" not in str(conflict)
    assert isinstance(unavailable, DatabaseUnavailableError)
    assert unavailable.code == "database_unavailable"
    assert "private host" not in str(unavailable)
