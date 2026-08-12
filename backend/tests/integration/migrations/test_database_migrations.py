import logging
from collections.abc import AsyncIterator
from uuid import uuid4

import pytest
from sqlalchemy import inspect, text
from sqlalchemy.ext.asyncio import AsyncEngine

from app.core.database import Base, create_engine
from app.core.migrations import (
    DatabaseRevisionError,
    DatabaseSchemaMismatchError,
    adopt_existing_database,
    assert_database_at_head,
    assert_database_matches_metadata,
    downgrade_database,
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
    runtime_logger = logging.getLogger("lanverse.migration-contract")
    runtime_logger.disabled = False

    await upgrade_database(migration_engine)

    assert runtime_logger.disabled is False
    assert await get_database_heads(migration_engine) == get_script_heads()
    await assert_database_matches_metadata(migration_engine)
    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
    assert table_names == {*Base.metadata.tables, "alembic_version"}


@pytest.mark.asyncio
async def test_matching_pre_alembic_database_requires_explicit_verified_adoption(
    migration_engine: AsyncEngine,
) -> None:
    account_id = uuid4()
    async with migration_engine.begin() as connection:
        await connection.run_sync(Base.metadata.create_all)
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :id, 'migration@example.test', 'test-hash', 1,
                    'Migration Fixture', 'active', now(), now()
                )
                """
            ),
            {"id": account_id},
        )

    with pytest.raises(DatabaseRevisionError, match="not at migration head"):
        await assert_database_at_head(migration_engine)

    await adopt_existing_database(
        migration_engine,
        backup_reference="test-backup-before-baseline",
    )

    await assert_database_at_head(migration_engine)
    await assert_database_matches_metadata(migration_engine)
    assert await get_database_heads(migration_engine) == get_script_heads()
    async with migration_engine.connect() as connection:
        account = (
            await connection.execute(
                text("SELECT email_normalized, display_name FROM idn_user_accounts WHERE id = :id"),
                {"id": account_id},
            )
        ).one()
    assert account == ("migration@example.test", "Migration Fixture")


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "schema_drift",
    [
        "CREATE TABLE migration_unknown_table (id integer)",
        "DROP INDEX ix_idn_user_accounts_status",
        "ALTER TABLE ast_assets DROP CONSTRAINT fk_ast_asset_project_workspace",
    ],
)
async def test_existing_schema_drift_is_rejected_without_stamping(
    migration_engine: AsyncEngine,
    schema_drift: str,
) -> None:
    async with migration_engine.begin() as connection:
        await connection.run_sync(Base.metadata.create_all)
        await connection.execute(text(schema_drift))

    with pytest.raises(DatabaseSchemaMismatchError, match="schema differs from baseline"):
        await adopt_existing_database(
            migration_engine,
            backup_reference="test-backup-before-baseline",
        )

    assert await get_database_heads(migration_engine) == ()


@pytest.mark.asyncio
async def test_baseline_schema_can_downgrade_to_empty_database(
    migration_engine: AsyncEngine,
) -> None:
    await upgrade_database(migration_engine)

    await downgrade_database(migration_engine, "base")

    assert await get_database_heads(migration_engine) == ()
    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
    assert table_names <= {"alembic_version"}
