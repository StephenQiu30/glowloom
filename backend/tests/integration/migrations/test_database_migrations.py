import logging
from collections.abc import AsyncIterator
from uuid import uuid4

import pytest
from sqlalchemy import Connection, inspect, text
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

BASELINE_REVISION = "95c0d24572c5"
PROVIDER_REVISION = "8d9f2a6c4b71"
SCRIPT_DOCUMENT_REVISION = "4c8e2f7a9b31"
EPISODE_PLANNING_REVISION = "7f3a9c1d2e84"
ADAPTATION_REVISION = "9a4d6e2f1b73"
PROVIDER_TABLE_NAMES = {
    "prod_provider_bindings",
    "prod_provider_connections",
    "prod_provider_credential_versions",
    "prod_provider_health_checks",
}
SCRIPT_DOCUMENT_TABLE_NAMES = {
    "scr_script_documents",
    "scr_document_revisions",
    "scr_narrative_blocks",
    "scr_format_issues",
}
EPISODE_PLANNING_TABLE_NAMES = {
    "scr_episode_plans",
    "scr_episode_proposals",
    "scr_import_commits",
    "scr_episode_segment_origins",
}
ADAPTATION_TABLE_NAMES = {"scr_adaptation_runs"}
PROVIDER_CAPABILITY_UNIQUE = "uq_prod_capability_id_version"


def _create_historical_pre_provider_schema(sync_connection: Connection) -> None:
    legacy_tables = [
        table
        for name, table in Base.metadata.tables.items()
        if name
        not in (
            PROVIDER_TABLE_NAMES
            | SCRIPT_DOCUMENT_TABLE_NAMES
            | EPISODE_PLANNING_TABLE_NAMES
            | ADAPTATION_TABLE_NAMES
        )
    ]
    Base.metadata.create_all(sync_connection, tables=legacy_tables)
    sync_connection.execute(
        text(f"ALTER TABLE prod_model_capabilities DROP CONSTRAINT {PROVIDER_CAPABILITY_UNIQUE}")
    )


def _create_provider_era_schema(sync_connection: Connection) -> None:
    provider_era_tables = [
        table
        for name, table in Base.metadata.tables.items()
        if name
        not in SCRIPT_DOCUMENT_TABLE_NAMES | EPISODE_PLANNING_TABLE_NAMES | ADAPTATION_TABLE_NAMES
    ]
    Base.metadata.create_all(sync_connection, tables=provider_era_tables)


async def _drop_migration_test_schema(target_engine: AsyncEngine) -> None:
    async with target_engine.begin() as connection:
        await connection.execute(text("DROP TABLE IF EXISTS migration_unknown_table CASCADE"))
        await connection.run_sync(Base.metadata.drop_all)
        await connection.execute(text("DROP TABLE IF EXISTS alembic_version CASCADE"))


def _create_document_era_schema(sync_connection: Connection) -> None:
    document_era_tables = [
        table
        for name, table in Base.metadata.tables.items()
        if name not in EPISODE_PLANNING_TABLE_NAMES | ADAPTATION_TABLE_NAMES
    ]
    Base.metadata.create_all(sync_connection, tables=document_era_tables)


def _create_episode_planning_era_schema(sync_connection: Connection) -> None:
    episode_planning_era_tables = [
        table for name, table in Base.metadata.tables.items() if name not in ADAPTATION_TABLE_NAMES
    ]
    Base.metadata.create_all(sync_connection, tables=episode_planning_era_tables)


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
async def test_historical_pre_provider_database_is_adopted_and_upgraded_without_data_loss(
    migration_engine: AsyncEngine,
) -> None:
    account_id = uuid4()
    async with migration_engine.begin() as connection:
        await connection.run_sync(_create_historical_pre_provider_schema)
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :id, 'legacy@example.test', 'legacy-hash', 1,
                    'Legacy Fixture', 'active', now(), now()
                )
                """
            ),
            {"id": account_id},
        )

    with pytest.raises(DatabaseRevisionError, match="not at migration head"):
        await assert_database_at_head(migration_engine)

    await adopt_existing_database(
        migration_engine,
        backup_reference="test-backup-before-legacy-adoption",
    )

    await assert_database_at_head(migration_engine)
    await assert_database_matches_metadata(migration_engine)
    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
        capability_unique_constraints = await connection.run_sync(
            lambda sync: {
                constraint["name"]
                for constraint in inspect(sync).get_unique_constraints("prod_model_capabilities")
            }
        )
        account = (
            await connection.execute(
                text("SELECT email_normalized, display_name FROM idn_user_accounts WHERE id = :id"),
                {"id": account_id},
            )
        ).one()
    assert PROVIDER_TABLE_NAMES <= table_names
    assert PROVIDER_CAPABILITY_UNIQUE in capability_unique_constraints
    assert account == ("legacy@example.test", "Legacy Fixture")


@pytest.mark.asyncio
async def test_partial_historical_provider_schema_is_rejected_without_stamping(
    migration_engine: AsyncEngine,
) -> None:
    async with migration_engine.begin() as connection:
        await connection.run_sync(_create_historical_pre_provider_schema)
        await connection.execute(
            text(
                "ALTER TABLE prod_model_capabilities "
                f"ADD CONSTRAINT {PROVIDER_CAPABILITY_UNIQUE} "
                "UNIQUE (id, config_version)"
            )
        )

    with pytest.raises(
        DatabaseSchemaMismatchError,
        match="partial historical Provider schema",
    ):
        await adopt_existing_database(
            migration_engine,
            backup_reference="test-backup-before-partial-adoption",
        )

    assert await get_database_heads(migration_engine) == ()


@pytest.mark.asyncio
async def test_previously_shipped_full_baseline_upgrades_without_duplicate_provider_tables(
    migration_engine: AsyncEngine,
) -> None:
    async with migration_engine.begin() as connection:
        await connection.run_sync(_create_provider_era_schema)
        await connection.execute(
            text(
                """
                CREATE TABLE alembic_version (
                    version_num VARCHAR(32) NOT NULL,
                    CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num)
                )
                """
            )
        )
        await connection.execute(
            text("INSERT INTO alembic_version (version_num) VALUES (:revision)"),
            {"revision": BASELINE_REVISION},
        )

    await upgrade_database(migration_engine)

    await assert_database_at_head(migration_engine)
    await assert_database_matches_metadata(migration_engine)
    assert await get_database_heads(migration_engine) == get_script_heads()


@pytest.mark.asyncio
async def test_baseline_revision_represents_the_historical_thirty_eight_table_schema(
    migration_engine: AsyncEngine,
) -> None:
    await upgrade_database(migration_engine, revision=BASELINE_REVISION)

    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
        capability_unique_constraints = await connection.run_sync(
            lambda sync: {
                constraint["name"]
                for constraint in inspect(sync).get_unique_constraints("prod_model_capabilities")
            }
        )

    assert table_names == {
        *(
            set(Base.metadata.tables)
            - PROVIDER_TABLE_NAMES
            - SCRIPT_DOCUMENT_TABLE_NAMES
            - EPISODE_PLANNING_TABLE_NAMES
            - ADAPTATION_TABLE_NAMES
        ),
        "alembic_version",
    }
    assert PROVIDER_CAPABILITY_UNIQUE not in capability_unique_constraints


@pytest.mark.asyncio
async def test_provider_revision_downgrades_to_historical_baseline_without_losing_legacy_rows(
    migration_engine: AsyncEngine,
) -> None:
    account_id = uuid4()
    await upgrade_database(migration_engine)
    async with migration_engine.begin() as connection:
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :id, 'downgrade@example.test', 'downgrade-hash', 1,
                    'Downgrade Fixture', 'active', now(), now()
                )
                """
            ),
            {"id": account_id},
        )

    await downgrade_database(migration_engine, BASELINE_REVISION)

    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
        capability_unique_constraints = await connection.run_sync(
            lambda sync: {
                constraint["name"]
                for constraint in inspect(sync).get_unique_constraints("prod_model_capabilities")
            }
        )
        account = await connection.scalar(
            text("SELECT email_normalized FROM idn_user_accounts WHERE id = :id"),
            {"id": account_id},
        )
    assert PROVIDER_TABLE_NAMES.isdisjoint(table_names)
    assert PROVIDER_CAPABILITY_UNIQUE not in capability_unique_constraints
    assert account == "downgrade@example.test"


@pytest.mark.asyncio
async def test_provider_revision_is_the_pre_document_forty_two_table_schema(
    migration_engine: AsyncEngine,
) -> None:
    await upgrade_database(migration_engine, revision=PROVIDER_REVISION)

    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))

    assert await get_database_heads(migration_engine) == (PROVIDER_REVISION,)
    assert table_names == {
        *(
            set(Base.metadata.tables)
            - SCRIPT_DOCUMENT_TABLE_NAMES
            - EPISODE_PLANNING_TABLE_NAMES
            - ADAPTATION_TABLE_NAMES
        ),
        "alembic_version",
    }


@pytest.mark.asyncio
async def test_document_revision_upgrades_provider_era_and_preserves_existing_rows(
    migration_engine: AsyncEngine,
) -> None:
    account_id = uuid4()
    await upgrade_database(migration_engine, revision=PROVIDER_REVISION)
    async with migration_engine.begin() as connection:
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :id, 'document-upgrade@example.test', 'test-hash', 1,
                    'Document Upgrade', 'active', now(), now()
                )
                """
            ),
            {"id": account_id},
        )

    await upgrade_database(migration_engine, revision=SCRIPT_DOCUMENT_REVISION)

    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
        account = await connection.scalar(
            text("SELECT email_normalized FROM idn_user_accounts WHERE id = :id"),
            {"id": account_id},
        )
    assert await get_database_heads(migration_engine) == (SCRIPT_DOCUMENT_REVISION,)
    assert SCRIPT_DOCUMENT_TABLE_NAMES <= table_names
    assert EPISODE_PLANNING_TABLE_NAMES.isdisjoint(table_names)
    assert ADAPTATION_TABLE_NAMES.isdisjoint(table_names)
    assert account == "document-upgrade@example.test"


@pytest.mark.asyncio
async def test_unversioned_provider_era_schema_is_adopted_then_upgraded(
    migration_engine: AsyncEngine,
) -> None:
    account_id = uuid4()
    async with migration_engine.begin() as connection:
        await connection.run_sync(_create_provider_era_schema)
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :id, 'provider-era@example.test', 'test-hash', 1,
                    'Provider Era', 'active', now(), now()
                )
                """
            ),
            {"id": account_id},
        )

    await adopt_existing_database(
        migration_engine,
        backup_reference="test-backup-before-document-adoption",
    )

    async with migration_engine.connect() as connection:
        account = await connection.scalar(
            text("SELECT email_normalized FROM idn_user_accounts WHERE id = :id"),
            {"id": account_id},
        )
    assert await get_database_heads(migration_engine) == (ADAPTATION_REVISION,)
    assert account == "provider-era@example.test"
    await assert_database_matches_metadata(migration_engine)


@pytest.mark.asyncio
async def test_partial_document_schema_is_rejected_without_stamping(
    migration_engine: AsyncEngine,
) -> None:
    async with migration_engine.begin() as connection:
        await connection.run_sync(_create_provider_era_schema)
        await connection.run_sync(
            lambda sync: Base.metadata.tables["scr_script_documents"].create(sync)
        )

    with pytest.raises(
        DatabaseSchemaMismatchError,
        match="partial ScriptDocument schema",
    ):
        await adopt_existing_database(
            migration_engine,
            backup_reference="test-backup-before-partial-document-adoption",
        )

    assert await get_database_heads(migration_engine) == ()


@pytest.mark.asyncio
async def test_document_revision_downgrades_to_provider_revision_without_legacy_loss(
    migration_engine: AsyncEngine,
) -> None:
    account_id = uuid4()
    await upgrade_database(migration_engine)
    async with migration_engine.begin() as connection:
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :id, 'document-downgrade@example.test', 'test-hash', 1,
                    'Document Downgrade', 'active', now(), now()
                )
                """
            ),
            {"id": account_id},
        )

    await downgrade_database(migration_engine, PROVIDER_REVISION)

    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
        account = await connection.scalar(
            text("SELECT email_normalized FROM idn_user_accounts WHERE id = :id"),
            {"id": account_id},
        )
    assert SCRIPT_DOCUMENT_TABLE_NAMES.isdisjoint(table_names)
    assert EPISODE_PLANNING_TABLE_NAMES.isdisjoint(table_names)
    assert ADAPTATION_TABLE_NAMES.isdisjoint(table_names)
    assert account == "document-downgrade@example.test"


@pytest.mark.asyncio
async def test_episode_planning_revision_upgrades_document_era_and_preserves_rows(
    migration_engine: AsyncEngine,
) -> None:
    account_id = uuid4()
    await upgrade_database(migration_engine, revision=SCRIPT_DOCUMENT_REVISION)
    async with migration_engine.begin() as connection:
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :id, 'planning-upgrade@example.test', 'test-hash', 1,
                    'Planning Upgrade', 'active', now(), now()
                )
                """
            ),
            {"id": account_id},
        )

    await upgrade_database(migration_engine, revision=EPISODE_PLANNING_REVISION)

    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
        account = await connection.scalar(
            text("SELECT email_normalized FROM idn_user_accounts WHERE id = :id"),
            {"id": account_id},
        )
    assert await get_database_heads(migration_engine) == (EPISODE_PLANNING_REVISION,)
    assert EPISODE_PLANNING_TABLE_NAMES <= table_names
    assert ADAPTATION_TABLE_NAMES.isdisjoint(table_names)
    assert account == "planning-upgrade@example.test"


@pytest.mark.asyncio
async def test_adaptation_revision_upgrades_episode_planning_era_and_preserves_rows(
    migration_engine: AsyncEngine,
) -> None:
    account_id = uuid4()
    await upgrade_database(migration_engine, revision=EPISODE_PLANNING_REVISION)
    async with migration_engine.begin() as connection:
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :id, 'adaptation-upgrade@example.test', 'test-hash', 1,
                    'Adaptation Upgrade', 'active', now(), now()
                )
                """
            ),
            {"id": account_id},
        )

    await upgrade_database(migration_engine)

    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
        account = await connection.scalar(
            text("SELECT email_normalized FROM idn_user_accounts WHERE id = :id"),
            {"id": account_id},
        )
    assert await get_database_heads(migration_engine) == (ADAPTATION_REVISION,)
    assert ADAPTATION_TABLE_NAMES <= table_names
    assert account == "adaptation-upgrade@example.test"
    await assert_database_matches_metadata(migration_engine)


@pytest.mark.asyncio
async def test_unversioned_episode_planning_era_is_adopted_then_upgraded(
    migration_engine: AsyncEngine,
) -> None:
    async with migration_engine.begin() as connection:
        await connection.run_sync(_create_episode_planning_era_schema)

    await adopt_existing_database(
        migration_engine,
        backup_reference="test-backup-before-adaptation-adoption",
    )

    assert await get_database_heads(migration_engine) == (ADAPTATION_REVISION,)
    await assert_database_matches_metadata(migration_engine)


@pytest.mark.asyncio
async def test_partial_episode_planning_schema_is_rejected_without_stamping(
    migration_engine: AsyncEngine,
) -> None:
    async with migration_engine.begin() as connection:
        await connection.run_sync(_create_document_era_schema)
        await connection.run_sync(
            lambda sync: Base.metadata.tables["scr_episode_plans"].create(sync)
        )

    with pytest.raises(
        DatabaseSchemaMismatchError,
        match="partial EpisodePlan schema",
    ):
        await adopt_existing_database(
            migration_engine,
            backup_reference="test-backup-before-partial-episode-plan-adoption",
        )

    assert await get_database_heads(migration_engine) == ()


@pytest.mark.asyncio
async def test_episode_planning_revision_downgrades_to_document_revision_without_legacy_loss(
    migration_engine: AsyncEngine,
) -> None:
    account_id = uuid4()
    await upgrade_database(migration_engine)
    async with migration_engine.begin() as connection:
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :id, 'planning-downgrade@example.test', 'test-hash', 1,
                    'Planning Downgrade', 'active', now(), now()
                )
                """
            ),
            {"id": account_id},
        )

    await downgrade_database(migration_engine, SCRIPT_DOCUMENT_REVISION)

    async with migration_engine.connect() as connection:
        table_names = set(await connection.run_sync(lambda sync: inspect(sync).get_table_names()))
        account = await connection.scalar(
            text("SELECT email_normalized FROM idn_user_accounts WHERE id = :id"),
            {"id": account_id},
        )
    assert EPISODE_PLANNING_TABLE_NAMES.isdisjoint(table_names)
    assert ADAPTATION_TABLE_NAMES.isdisjoint(table_names)
    assert SCRIPT_DOCUMENT_TABLE_NAMES <= table_names
    assert account == "planning-downgrade@example.test"


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
