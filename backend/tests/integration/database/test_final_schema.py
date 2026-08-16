from collections.abc import AsyncIterator

import pytest
from sqlalchemy import inspect, text
from sqlalchemy.ext.asyncio import AsyncEngine

from app.core.database import Base, create_engine
from app.core.schema import (
    DatabaseSchemaMismatchError,
    assert_database_schema,
    initialize_database,
)
from app.model_registry import register_implemented_models
from tests.conftest import TEST_DATABASE_URL

register_implemented_models()


async def _drop_schema(target_engine: AsyncEngine) -> None:
    async with target_engine.begin() as connection:
        table_names = await connection.run_sync(lambda sync: inspect(sync).get_table_names())
        for table_name in sorted(table_names):
            quoted = connection.dialect.identifier_preparer.quote(table_name)
            await connection.execute(text(f"DROP TABLE {quoted} CASCADE"))


@pytest.fixture
async def schema_engine() -> AsyncIterator[AsyncEngine]:
    target_engine = create_engine(TEST_DATABASE_URL)
    await _drop_schema(target_engine)
    try:
        yield target_engine
    finally:
        await _drop_schema(target_engine)
        await target_engine.dispose()


@pytest.mark.asyncio
async def test_initialize_database_creates_only_the_final_schema(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_database(schema_engine)

    async with schema_engine.connect() as connection:
        table_names = set(
            await connection.run_sync(lambda sync: inspect(sync).get_table_names())
        )
    assert table_names == set(Base.metadata.tables)
    await assert_database_schema(schema_engine)


@pytest.mark.asyncio
async def test_initialize_database_removes_only_the_legacy_revision_marker(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_database(schema_engine)
    async with schema_engine.begin() as connection:
        await connection.execute(
            text('CREATE TABLE "alembic_version" (version_num VARCHAR(32) NOT NULL)')
        )
        await connection.execute(
            text('INSERT INTO "alembic_version" (version_num) VALUES (\'legacy\')')
        )

    await initialize_database(schema_engine)

    async with schema_engine.connect() as connection:
        table_names = set(
            await connection.run_sync(lambda sync: inspect(sync).get_table_names())
        )
    assert "alembic_version" not in table_names
    assert table_names == set(Base.metadata.tables)


@pytest.mark.asyncio
async def test_schema_guard_rejects_unknown_tables(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_database(schema_engine)
    async with schema_engine.begin() as connection:
        await connection.execute(text("CREATE TABLE unknown_schema_table (id UUID PRIMARY KEY)"))

    with pytest.raises(DatabaseSchemaMismatchError, match="unexpected table"):
        await assert_database_schema(schema_engine)


@pytest.mark.asyncio
async def test_schema_guard_rejects_column_drift(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_database(schema_engine)
    async with schema_engine.begin() as connection:
        await connection.execute(text("ALTER TABLE idn_workspaces ADD COLUMN drift_marker TEXT"))

    with pytest.raises(DatabaseSchemaMismatchError, match="unexpected column"):
        await assert_database_schema(schema_engine)
