from collections.abc import AsyncIterator
from pathlib import Path

import pytest
from sqlalchemy import inspect, text
from sqlalchemy.ext.asyncio import AsyncEngine

from app.core.database import Base, create_engine
from app.core.schema import (
    DatabaseSchemaMismatchError,
    adopt_database_baseline,
    assert_database_schema,
    initialize_development_database,
)
from app.runtime.model_registry import register_implemented_models
from tests.conftest import TEST_DATABASE_URL

register_implemented_models()

BASELINE_MIGRATION = (
    Path(__file__).resolve().parents[4]
    / "backend"
    / "migrations"
    / "000001_compatibility_runtime_baseline.sql"
)


async def _drop_schema(target_engine: AsyncEngine) -> None:
    async with target_engine.begin() as connection:
        await connection.execute(text("DROP SCHEMA IF EXISTS lanverse_migration CASCADE"))
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
async def test_initialize_development_database_creates_compatible_baseline(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_development_database(schema_engine)

    async with schema_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
    assert table_names == set(Base.metadata.tables)
    await assert_database_schema(schema_engine)


@pytest.mark.asyncio
async def test_static_baseline_migration_matches_the_accepted_runtime_schema(
    schema_engine: AsyncEngine,
) -> None:
    baseline_sql = BASELINE_MIGRATION.read_text(encoding="utf-8")
    async with schema_engine.begin() as connection:
        for statement in baseline_sql.split(";\n\n"):
            if statement := statement.strip().removesuffix(";"):
                await connection.exec_driver_sql(statement)

    await adopt_database_baseline(schema_engine)
    await assert_database_schema(schema_engine)


@pytest.mark.asyncio
async def test_initialize_development_database_adopts_an_exact_legacy_schema(
    schema_engine: AsyncEngine,
) -> None:
    async with schema_engine.begin() as connection:
        await connection.run_sync(Base.metadata.create_all)
        await connection.execute(
            text('CREATE TABLE "alembic_version" (version_num VARCHAR(32) NOT NULL)')
        )
        await connection.execute(
            text("INSERT INTO \"alembic_version\" (version_num) VALUES ('legacy')")
        )

    await initialize_development_database(schema_engine)

    async with schema_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
    assert "alembic_version" not in table_names
    assert table_names == set(Base.metadata.tables)


@pytest.mark.asyncio
async def test_schema_guard_accepts_target_tables_and_columns(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_development_database(schema_engine)
    async with schema_engine.begin() as connection:
        await connection.execute(text("CREATE TABLE evt_audit_records (id UUID PRIMARY KEY)"))
        await connection.execute(
            text("ALTER TABLE sys_outbox_events ADD COLUMN envelope_version INTEGER")
        )

    await assert_database_schema(schema_engine)


@pytest.mark.asyncio
async def test_schema_guard_rejects_column_drift(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_development_database(schema_engine)
    async with schema_engine.begin() as connection:
        await connection.execute(text("ALTER TABLE idn_workspaces DROP COLUMN revision"))

    with pytest.raises(DatabaseSchemaMismatchError, match="missing column"):
        await assert_database_schema(schema_engine)


@pytest.mark.asyncio
async def test_schema_guard_accepts_the_preapproved_audit_foundation_version(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_development_database(schema_engine)
    async with schema_engine.begin() as connection:
        await connection.execute(
            text(
                "INSERT INTO lanverse_migration.schema_migrations "
                "(version, name, checksum, source) "
                "VALUES (2, 'audit_projection_foundation', NULL, 'migration')"
            )
        )

    await assert_database_schema(schema_engine)


@pytest.mark.asyncio
async def test_schema_guard_rejects_unknown_or_out_of_window_migrations(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_development_database(schema_engine)
    async with schema_engine.begin() as connection:
        await connection.execute(
            text(
                "INSERT INTO lanverse_migration.schema_migrations "
                "(version, name, checksum, source) VALUES "
                "(2, 'audit_projection_foundation', NULL, 'migration'), "
                "(3, 'future_migration', NULL, 'migration')"
            )
        )

    with pytest.raises(DatabaseSchemaMismatchError, match="migration version 3"):
        await assert_database_schema(schema_engine)


@pytest.mark.asyncio
async def test_schema_guard_rejects_a_forged_baseline_identity(
    schema_engine: AsyncEngine,
) -> None:
    await initialize_development_database(schema_engine)
    async with schema_engine.begin() as connection:
        await connection.execute(
            text(
                "UPDATE lanverse_migration.schema_migrations SET name = 'forged' WHERE version = 1"
            )
        )

    with pytest.raises(DatabaseSchemaMismatchError, match="migration 1 identity"):
        await assert_database_schema(schema_engine)
