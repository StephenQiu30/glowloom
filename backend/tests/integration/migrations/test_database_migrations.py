import json
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

BASELINE_REVISION = "95c0d24572c5"
PROVIDER_REVISION = "8d9f2a6c4b71"
SCRIPT_DOCUMENT_REVISION = "4c8e2f7a9b31"
EPISODE_PLANNING_REVISION = "7f3a9c1d2e84"
ADAPTATION_REVISION = "9a4d6e2f1b73"
NARRATIVE_REVISION = "2b7e4c9a1d63"
ASSET_STATE_REVISION = "6c1f8d4a7e20"
ASSET_CHANGE_REVISION = "36bf151da189"
STORYBOARD_DRAFT_REVISION = "ecdbb9f876f8"
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
NARRATIVE_TABLE_NAMES = {
    "scr_narrative_structures",
    "scr_narrative_units",
    "scr_narrative_unit_versions",
    "scr_narrative_impacts",
}
ASSET_STATE_TABLE_NAMES = {
    "ast_asset_states",
    "ast_asset_occurrences",
}
ASSET_CHANGE_TABLE_NAMES = {"ast_asset_name_revisions"}
STORYBOARD_DRAFT_TABLE_NAMES = {
    "sbd_draft_batches",
    "sbd_draft_input_units",
    "sbd_draft_input_assets",
    "sbd_draft_shots",
    "sbd_draft_shot_units",
    "sbd_draft_asset_refs",
    "sbd_draft_decisions",
}
PROVIDER_CAPABILITY_UNIQUE = "uq_prod_capability_id_version"


async def _drop_migration_test_schema(target_engine: AsyncEngine) -> None:
    async with target_engine.begin() as connection:
        table_names = await connection.run_sync(lambda sync: inspect(sync).get_table_names())
        for table_name in table_names:
            quoted = connection.dialect.identifier_preparer.quote(table_name)
            await connection.execute(text(f"DROP TABLE {quoted} CASCADE"))


async def _create_unversioned_revision(
    target_engine: AsyncEngine,
    revision: str,
) -> None:
    """Build a historical schema from its migration, then remove only its stamp."""
    await upgrade_database(target_engine, revision=revision)
    async with target_engine.begin() as connection:
        await connection.execute(text("DROP TABLE alembic_version"))


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
    assert await get_database_heads(migration_engine) == (STORYBOARD_DRAFT_REVISION,)
    assert ASSET_STATE_TABLE_NAMES <= table_names


@pytest.mark.asyncio
async def test_head_upgrade_rejects_stale_revision_marker(
    migration_engine: AsyncEngine,
) -> None:
    await upgrade_database(migration_engine)
    async with migration_engine.begin() as connection:
        await connection.execute(text("DROP TABLE sys_schedules CASCADE"))

    with pytest.raises(DatabaseSchemaMismatchError, match="schema differs from baseline"):
        await upgrade_database(migration_engine)


@pytest.mark.asyncio
async def test_asset_state_revision_moves_asset_current_without_dual_write(
    migration_engine: AsyncEngine,
) -> None:
    actor_id = uuid4()
    workspace_id = uuid4()
    project_id = uuid4()
    asset_id = uuid4()
    version_id = uuid4()
    source_id = uuid4()
    await upgrade_database(migration_engine, revision=NARRATIVE_REVISION)
    async with migration_engine.begin() as connection:
        values = {
            "actor_id": actor_id,
            "workspace_id": workspace_id,
            "project_id": project_id,
            "asset_id": asset_id,
            "version_id": version_id,
            "source_id": source_id,
            "spec": json.dumps(
                {
                    "kind": "character",
                    "identity": "沈岚",
                    "appearance": "常服",
                    "age_impression": "31 岁",
                    "temperament": ["坚定"],
                },
                ensure_ascii=False,
            ),
            "content_hash": "a" * 64,
        }
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :actor_id, 'asset-state-migration@example.test', 'hash', 1,
                    'Asset State Migration', 'active', now(), now()
                )
                """
            ),
            values,
        )
        await connection.execute(
            text(
                """
                INSERT INTO idn_workspaces (
                    id, name, status, revision, created_at, updated_at
                ) VALUES (
                    :workspace_id, 'Migration Workspace', 'active', 1, now(), now()
                )
                """
            ),
            values,
        )
        await connection.execute(
            text(
                """
                INSERT INTO prj_projects (
                    id, workspace_id, name, aspect_ratio, language,
                    target_duration_ms, budget_limit, currency, status, revision,
                    created_at, updated_at
                ) VALUES (
                    :project_id, :workspace_id, 'Migration Project', '9:16', 'zh-CN',
                    90000, 0, 'CNY', 'active', 1, now(), now()
                )
                """
            ),
            values,
        )
        await connection.execute(
            text(
                """
                INSERT INTO ast_assets (
                    id, workspace_id, project_id, kind, name, normalized_name,
                    aliases, tags, status, current_version_id, revision, created_by,
                    created_at, updated_at
                ) VALUES (
                    :asset_id, :workspace_id, :project_id, 'character', '沈岚', '沈岚',
                    ARRAY[]::text[], ARRAY[]::text[], 'active', NULL, 1, :actor_id,
                    now(), now()
                )
                """
            ),
            values,
        )
        await connection.execute(
            text(
                """
                INSERT INTO ast_asset_versions (
                    id, workspace_id, asset_id, version_no, schema_version, spec,
                    prompt_description, source_type, source_id, content_hash,
                    created_by, created_at
                ) VALUES (
                    :version_id, :workspace_id, :asset_id, 1, 1, CAST(:spec AS jsonb),
                    '迁移前版本', 'candidate', :source_id, :content_hash,
                    :actor_id, now()
                )
                """
            ),
            values,
        )
        await connection.execute(
            text("UPDATE ast_assets SET current_version_id = :version_id WHERE id = :asset_id"),
            values,
        )

    await upgrade_database(migration_engine)

    async with migration_engine.connect() as connection:
        asset_columns = {
            column["name"]
            for column in await connection.run_sync(
                lambda sync: inspect(sync).get_columns("ast_assets")
            )
        }
        state = (
            await connection.execute(
                text(
                    "SELECT id, state_key, current_version_id FROM ast_asset_states "
                    "WHERE asset_id = :asset_id"
                ),
                {"asset_id": asset_id},
            )
        ).one()
        migrated_version = (
            await connection.execute(
                text(
                    "SELECT asset_state_id, source_type FROM ast_asset_versions "
                    "WHERE id = :version_id"
                ),
                {"version_id": version_id},
            )
        ).one()
    assert "current_version_id" not in asset_columns
    assert state.state_key == "base"
    assert state.current_version_id == version_id
    assert migrated_version.asset_state_id == state.id
    assert migrated_version.source_type == "script_extraction_candidate"

    await downgrade_database(migration_engine, NARRATIVE_REVISION)
    async with migration_engine.connect() as connection:
        restored = (
            await connection.execute(
                text(
                    "SELECT a.current_version_id, v.source_type "
                    "FROM ast_assets AS a "
                    "JOIN ast_asset_versions AS v ON v.asset_id = a.id "
                    "WHERE a.id = :asset_id"
                ),
                {"asset_id": asset_id},
            )
        ).one()
    assert restored.current_version_id == version_id
    assert restored.source_type == "candidate"


@pytest.mark.asyncio
async def test_asset_change_revision_backfills_name_and_availability(
    migration_engine: AsyncEngine,
) -> None:
    actor_id = uuid4()
    workspace_id = uuid4()
    project_id = uuid4()
    asset_id = uuid4()
    await upgrade_database(migration_engine, revision=ASSET_STATE_REVISION)
    values = {
        "actor_id": actor_id,
        "workspace_id": workspace_id,
        "project_id": project_id,
        "asset_id": asset_id,
    }
    async with migration_engine.begin() as connection:
        await connection.execute(
            text(
                """
                INSERT INTO idn_user_accounts (
                    id, email_normalized, password_hash, token_version,
                    display_name, status, created_at, updated_at
                ) VALUES (
                    :actor_id, 'asset-change-migration@example.test', 'hash', 1,
                    'Asset Change Migration', 'active', now(), now()
                )
                """
            ),
            values,
        )
        await connection.execute(
            text(
                """
                INSERT INTO idn_workspaces (
                    id, name, status, revision, created_at, updated_at
                ) VALUES (
                    :workspace_id, 'Asset Change Workspace', 'active', 1,
                    now(), now()
                )
                """
            ),
            values,
        )
        await connection.execute(
            text(
                """
                INSERT INTO prj_projects (
                    id, workspace_id, name, aspect_ratio, language,
                    target_duration_ms, budget_limit, currency, status,
                    revision, created_at, updated_at
                ) VALUES (
                    :project_id, :workspace_id, 'Asset Change Project',
                    '9:16', 'zh-CN', 90000, 0, 'CNY', 'active', 1,
                    now(), now()
                )
                """
            ),
            values,
        )
        await connection.execute(
            text(
                """
                INSERT INTO ast_assets (
                    id, workspace_id, project_id, kind, name,
                    normalized_name, aliases, tags, status, revision,
                    created_by, created_at, updated_at
                ) VALUES (
                    :asset_id, :workspace_id, :project_id, 'location',
                    '旧泵站', '旧泵站', ARRAY['泵站']::text[],
                    ARRAY[]::text[], 'active', 3, :actor_id, now(), now()
                )
                """
            ),
            values,
        )

    await upgrade_database(migration_engine)

    async with migration_engine.connect() as connection:
        asset = (
            await connection.execute(
                text(
                    "SELECT availability, name_revision, command_receipts "
                    "FROM ast_assets WHERE id = :asset_id"
                ),
                values,
            )
        ).one()
        name_revision = (
            await connection.execute(
                text(
                    "SELECT revision_no, name_snapshot, normalized_name "
                    "FROM ast_asset_name_revisions WHERE asset_id = :asset_id"
                ),
                values,
            )
        ).one()
    assert asset == ("enabled", 1, {})
    assert name_revision == (1, "旧泵站", "旧泵站")
    await assert_database_matches_metadata(migration_engine)

    await downgrade_database(migration_engine, ASSET_STATE_REVISION)
    async with migration_engine.connect() as connection:
        preserved = (
            await connection.execute(
                text("SELECT name, aliases, revision FROM ast_assets WHERE id = :asset_id"),
                values,
            )
        ).one()
    assert preserved == ("旧泵站", ["泵站"], 3)


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
    await _create_unversioned_revision(migration_engine, BASELINE_REVISION)
    async with migration_engine.begin() as connection:
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
    await _create_unversioned_revision(migration_engine, BASELINE_REVISION)
    async with migration_engine.begin() as connection:
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
    await upgrade_database(migration_engine, revision=PROVIDER_REVISION)
    async with migration_engine.begin() as connection:
        await connection.execute(
            text("UPDATE alembic_version SET version_num = :revision"),
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
            - NARRATIVE_TABLE_NAMES
            - ASSET_STATE_TABLE_NAMES
            - ASSET_CHANGE_TABLE_NAMES
            - STORYBOARD_DRAFT_TABLE_NAMES
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
            - NARRATIVE_TABLE_NAMES
            - ASSET_STATE_TABLE_NAMES
            - ASSET_CHANGE_TABLE_NAMES
            - STORYBOARD_DRAFT_TABLE_NAMES
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
    await _create_unversioned_revision(migration_engine, PROVIDER_REVISION)
    async with migration_engine.begin() as connection:
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
    assert await get_database_heads(migration_engine) == (STORYBOARD_DRAFT_REVISION,)
    assert account == "provider-era@example.test"
    await assert_database_matches_metadata(migration_engine)


@pytest.mark.asyncio
async def test_partial_document_schema_is_rejected_without_stamping(
    migration_engine: AsyncEngine,
) -> None:
    await _create_unversioned_revision(migration_engine, PROVIDER_REVISION)
    async with migration_engine.begin() as connection:
        await connection.execute(text("CREATE TABLE scr_script_documents (id UUID PRIMARY KEY)"))

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
async def test_head_upgrades_episode_planning_era_and_preserves_rows(
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
    assert await get_database_heads(migration_engine) == (STORYBOARD_DRAFT_REVISION,)
    assert ADAPTATION_TABLE_NAMES <= table_names
    assert NARRATIVE_TABLE_NAMES <= table_names
    assert account == "adaptation-upgrade@example.test"
    await assert_database_matches_metadata(migration_engine)


@pytest.mark.asyncio
async def test_unversioned_episode_planning_era_is_adopted_then_upgraded(
    migration_engine: AsyncEngine,
) -> None:
    await _create_unversioned_revision(migration_engine, EPISODE_PLANNING_REVISION)

    await adopt_existing_database(
        migration_engine,
        backup_reference="test-backup-before-adaptation-adoption",
    )

    assert await get_database_heads(migration_engine) == (STORYBOARD_DRAFT_REVISION,)
    await assert_database_matches_metadata(migration_engine)


@pytest.mark.asyncio
async def test_unversioned_adaptation_era_is_adopted_then_upgraded(
    migration_engine: AsyncEngine,
) -> None:
    await _create_unversioned_revision(migration_engine, ADAPTATION_REVISION)

    await adopt_existing_database(
        migration_engine,
        backup_reference="test-backup-before-narrative-adoption",
    )

    assert await get_database_heads(migration_engine) == (STORYBOARD_DRAFT_REVISION,)
    await assert_database_matches_metadata(migration_engine)


@pytest.mark.asyncio
async def test_partial_narrative_schema_is_rejected_without_stamping(
    migration_engine: AsyncEngine,
) -> None:
    await _create_unversioned_revision(migration_engine, ADAPTATION_REVISION)
    async with migration_engine.begin() as connection:
        await connection.execute(
            text("CREATE TABLE scr_narrative_structures (id UUID PRIMARY KEY)")
        )

    with pytest.raises(
        DatabaseSchemaMismatchError,
        match="partial NarrativeUnit schema",
    ):
        await adopt_existing_database(
            migration_engine,
            backup_reference="test-backup-before-partial-narrative-adoption",
        )

    assert await get_database_heads(migration_engine) == ()


@pytest.mark.asyncio
async def test_partial_episode_planning_schema_is_rejected_without_stamping(
    migration_engine: AsyncEngine,
) -> None:
    await _create_unversioned_revision(migration_engine, SCRIPT_DOCUMENT_REVISION)
    async with migration_engine.begin() as connection:
        await connection.execute(text("CREATE TABLE scr_episode_plans (id UUID PRIMARY KEY)"))

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

    with pytest.raises(DatabaseSchemaMismatchError, match="schema differs from"):
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
