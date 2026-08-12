from collections.abc import AsyncIterator

import pytest
from sqlalchemy import inspect, text
from sqlalchemy.ext.asyncio import AsyncEngine

from app.core.database import Base, create_engine
from app.core.migrations import (
    DatabaseRevisionError,
    DatabaseSchemaMismatchError,
    adopt_existing_database,
    assert_database_at_head,
    get_database_heads,
    get_script_heads,
    upgrade_database,
)
from app.model_registry import register_implemented_models
from tests.conftest import TEST_DATABASE_URL


async def _drop_migration_test_schema(target_engine: AsyncEngine) -> None:
    async with target_engine.begin() as connection:
        await connection.execute(text("DROP TABLE IF EXISTS migration_unknown_table CASCADE"))
        await connection.run_sync(Base.metadata.drop_all)
        await connection.execute(text("DROP TABLE IF EXISTS alembic_version CASCADE"))


@pytest.fixture
async def migration_engine() -> AsyncIterator[AsyncEngine]:
    register_implemented_models()
    target_engine = create_engine(TEST_DATABASE_URL)
    await _drop_migration_test_schema(target_engine)
    try:
        yield target_engine
    finally:
        await _drop_migration_test_schema(target_engine)
        await target_engine.dispose()


@pytest.mark.asyncio
async def test_empty_database_upgrades_to_registered_metadata_head(
    migration_engine: AsyncEngine,
) -> None:
    await upgrade_database(migration_engine)

    assert await get_database_heads(migration_engine) == get_script_heads()
    async with migration_engine.connect() as connection:
        table_names = set(
            await connection.run_sync(lambda sync: inspect(sync).get_table_names())
        )
    assert table_names == {*Base.metadata.tables, "alembic_version"}


@pytest.mark.asyncio
async def test_matching_pre_alembic_database_requires_explicit_verified_adoption(
    migration_engine: AsyncEngine,
) -> None:
    async with migration_engine.begin() as connection:
        await connection.run_sync(Base.metadata.create_all)

    with pytest.raises(DatabaseRevisionError, match="not at migration head"):
        await assert_database_at_head(migration_engine)

    await adopt_existing_database(migration_engine)

    await assert_database_at_head(migration_engine)
    assert await get_database_heads(migration_engine) == get_script_heads()


@pytest.mark.asyncio
async def test_unknown_existing_schema_is_rejected_without_stamping(
    migration_engine: AsyncEngine,
) -> None:
    async with migration_engine.begin() as connection:
        await connection.run_sync(Base.metadata.create_all)
        await connection.execute(text("CREATE TABLE migration_unknown_table (id integer)"))

    with pytest.raises(DatabaseSchemaMismatchError, match="schema differs from baseline"):
        await adopt_existing_database(migration_engine)

    assert await get_database_heads(migration_engine) == ()
