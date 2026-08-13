import re
from collections.abc import Sequence
from pathlib import Path

from alembic.autogenerate import compare_metadata
from alembic.config import Config
from alembic.runtime.migration import MigrationContext
from alembic.script import ScriptDirectory
from sqlalchemy import Connection, inspect
from sqlalchemy.ext.asyncio import AsyncEngine

from alembic import command
from app.core.database import Base, engine

BACKEND_ROOT = Path(__file__).resolve().parents[2]
BACKUP_REFERENCE_PATTERN = re.compile(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}\Z")
BASELINE_REVISION = "95c0d24572c5"
PROVIDER_REVISION = "8d9f2a6c4b71"
SCRIPT_DOCUMENT_REVISION = "4c8e2f7a9b31"
EPISODE_PLANNING_REVISION = "7f3a9c1d2e84"
ADAPTATION_REVISION = "9a4d6e2f1b73"
NARRATIVE_REVISION = "2b7e4c9a1d63"
ASSET_STATE_REVISION = "6c1f8d4a7e20"
PROVIDER_TABLE_NAMES = frozenset(
    {
        "prod_provider_bindings",
        "prod_provider_connections",
        "prod_provider_credential_versions",
        "prod_provider_health_checks",
    }
)
SCRIPT_DOCUMENT_TABLE_NAMES = frozenset(
    {
        "scr_script_documents",
        "scr_document_revisions",
        "scr_narrative_blocks",
        "scr_format_issues",
    }
)
EPISODE_PLANNING_TABLE_NAMES = frozenset(
    {
        "scr_episode_plans",
        "scr_episode_proposals",
        "scr_import_commits",
        "scr_episode_segment_origins",
    }
)
ADAPTATION_TABLE_NAMES = frozenset({"scr_adaptation_runs"})
NARRATIVE_TABLE_NAMES = frozenset(
    {
        "scr_narrative_structures",
        "scr_narrative_units",
        "scr_narrative_unit_versions",
        "scr_narrative_impacts",
    }
)
ASSET_STATE_TABLE_NAMES = frozenset(
    {
        "ast_asset_states",
        "ast_asset_occurrences",
    }
)
ASSET_CHANGE_TABLE_NAMES = frozenset({"ast_asset_name_revisions"})
ASSET_CHANGE_COLUMNS = frozenset(
    {
        "availability",
        "name_revision",
        "command_receipts",
    }
)
PROVIDER_CAPABILITY_UNIQUE = "uq_prod_capability_id_version"


class DatabaseRevisionError(RuntimeError):
    pass


class DatabaseSchemaMismatchError(RuntimeError):
    pass


def create_alembic_config() -> Config:
    return Config(str(BACKEND_ROOT / "alembic.ini"))


def get_script_heads() -> tuple[str, ...]:
    return tuple(ScriptDirectory.from_config(create_alembic_config()).get_heads())


def ensure_expected_heads(current: Sequence[str], expected: Sequence[str]) -> None:
    if set(current) == set(expected):
        return
    raise DatabaseRevisionError(
        "database is not at migration head; "
        f"current={sorted(current)!r}, expected={sorted(expected)!r}"
    )


def validate_backup_reference(value: str) -> str:
    reference = value.strip()
    if not BACKUP_REFERENCE_PATTERN.fullmatch(reference):
        raise ValueError(
            "backup reference is required and must be a 1-200 character operator reference"
        )
    return reference


def _database_heads(connection: Connection) -> tuple[str, ...]:
    context = MigrationContext.configure(connection)
    return tuple(context.get_current_heads())


async def get_database_heads(target_engine: AsyncEngine = engine) -> tuple[str, ...]:
    async with target_engine.connect() as connection:
        return await connection.run_sync(_database_heads)


async def assert_database_at_head(target_engine: AsyncEngine = engine) -> None:
    ensure_expected_heads(await get_database_heads(target_engine), get_script_heads())


def _upgrade(connection: Connection, revision: str) -> None:
    configuration = create_alembic_config()
    configuration.attributes["connection"] = connection
    command.upgrade(configuration, revision)


async def upgrade_database(target_engine: AsyncEngine = engine, revision: str = "head") -> None:
    async with target_engine.begin() as connection:
        await connection.run_sync(_upgrade, revision)
    if revision == "head":
        await assert_database_matches_metadata(target_engine)


def _downgrade(connection: Connection, revision: str) -> None:
    configuration = create_alembic_config()
    configuration.attributes["connection"] = connection
    command.downgrade(configuration, revision)


async def downgrade_database(target_engine: AsyncEngine, revision: str) -> None:
    async with target_engine.begin() as connection:
        await connection.run_sync(_downgrade, revision)


def _include_baseline_object(
    _object: object,
    name: str | None,
    type_: str,
    _reflected: bool,
    _compare_to: object | None,
) -> bool:
    return not (type_ == "table" and name == "alembic_version")


_PRE_ASSET_STATE_COLUMNS = {
    ("ast_assets", "current_version_id"),
    ("ast_asset_versions", "asset_state_id"),
    ("sbd_asset_references", "asset_state_id"),
    ("sbd_asset_references", "asset_id"),
    ("sbd_asset_references", "binding_source"),
}
_PRE_ASSET_STATE_OBJECTS = {
    "ck_sbd_asset_ref_binding_source",
    "fk_ast_asset_current_version_workspace",
    "fk_ast_state_current_version_scope",
    "fk_ast_version_asset_workspace",
    "fk_ast_version_state_scope",
    "fk_sbd_asset_ref_version_scope",
    "fk_sbd_asset_ref_version_workspace",
    "ix_ast_version_state_number",
    "ix_sbd_asset_ref_state",
    "uq_ast_version_scope",
    "uq_scr_narrative_version_unit_scope",
}

_PRE_ASSET_CHANGE_OBJECTS = {
    "ck_ast_asset_availability",
    "ck_ast_asset_name_revision",
    "ck_ast_name_revision_number",
    "fk_ast_asset_current_name",
    "fk_ast_name_revision_asset_workspace",
    "ix_ast_asset_project_availability",
    "ix_ast_name_revision_asset_created",
    "uq_ast_name_revision_scope",
}


def _include_pre_asset_change_object(
    object_: object,
    name: str | None,
    type_: str,
    reflected: bool,
    compare_to: object | None,
) -> bool:
    if type_ == "table" and name in ASSET_CHANGE_TABLE_NAMES:
        return False
    table = getattr(object_, "table", None)
    table_name = getattr(table, "name", None)
    if type_ == "column" and table_name == "ast_assets" and name in ASSET_CHANGE_COLUMNS:
        return False
    if name in _PRE_ASSET_CHANGE_OBJECTS:
        return False
    return _include_baseline_object(object_, name, type_, reflected, compare_to)


def _include_pre_asset_state_object(
    object_: object,
    name: str | None,
    type_: str,
    reflected: bool,
    compare_to: object | None,
) -> bool:
    if type_ == "table" and name in ASSET_STATE_TABLE_NAMES:
        return False
    table = getattr(object_, "table", None)
    table_name = getattr(table, "name", None)
    if type_ == "column" and (table_name, name) in _PRE_ASSET_STATE_COLUMNS:
        return False
    if name in _PRE_ASSET_STATE_OBJECTS:
        return False
    return _include_pre_asset_change_object(object_, name, type_, reflected, compare_to)


def _include_historical_pre_provider_object(
    object_: object,
    name: str | None,
    type_: str,
    reflected: bool,
    compare_to: object | None,
) -> bool:
    if type_ == "table" and name in (
        PROVIDER_TABLE_NAMES
        | SCRIPT_DOCUMENT_TABLE_NAMES
        | EPISODE_PLANNING_TABLE_NAMES
        | ADAPTATION_TABLE_NAMES
        | NARRATIVE_TABLE_NAMES
        | ASSET_STATE_TABLE_NAMES
    ):
        return False
    if type_ == "unique_constraint" and name == PROVIDER_CAPABILITY_UNIQUE:
        return False
    return _include_pre_asset_state_object(object_, name, type_, reflected, compare_to)


def _include_provider_era_object(
    object_: object,
    name: str | None,
    type_: str,
    reflected: bool,
    compare_to: object | None,
) -> bool:
    if type_ == "table" and name in (
        SCRIPT_DOCUMENT_TABLE_NAMES
        | EPISODE_PLANNING_TABLE_NAMES
        | ADAPTATION_TABLE_NAMES
        | NARRATIVE_TABLE_NAMES
        | ASSET_STATE_TABLE_NAMES
    ):
        return False
    return _include_pre_asset_state_object(object_, name, type_, reflected, compare_to)


def _include_document_era_object(
    object_: object,
    name: str | None,
    type_: str,
    reflected: bool,
    compare_to: object | None,
) -> bool:
    if type_ == "table" and name in (
        EPISODE_PLANNING_TABLE_NAMES
        | ADAPTATION_TABLE_NAMES
        | NARRATIVE_TABLE_NAMES
        | ASSET_STATE_TABLE_NAMES
    ):
        return False
    return _include_pre_asset_state_object(object_, name, type_, reflected, compare_to)


def _include_episode_planning_era_object(
    object_: object,
    name: str | None,
    type_: str,
    reflected: bool,
    compare_to: object | None,
) -> bool:
    if type_ == "table" and name in (
        ADAPTATION_TABLE_NAMES | NARRATIVE_TABLE_NAMES | ASSET_STATE_TABLE_NAMES
    ):
        return False
    return _include_pre_asset_state_object(object_, name, type_, reflected, compare_to)


def _include_adaptation_era_object(
    object_: object,
    name: str | None,
    type_: str,
    reflected: bool,
    compare_to: object | None,
) -> bool:
    if type_ == "table" and name in (NARRATIVE_TABLE_NAMES | ASSET_STATE_TABLE_NAMES):
        return False
    return _include_pre_asset_state_object(object_, name, type_, reflected, compare_to)


def _include_narrative_era_object(
    object_: object,
    name: str | None,
    type_: str,
    reflected: bool,
    compare_to: object | None,
) -> bool:
    return _include_pre_asset_state_object(
        object_,
        name,
        type_,
        reflected,
        compare_to,
    )


def _baseline_differences(
    connection: Connection,
    *,
    historical_pre_provider: bool = False,
    provider_era: bool = False,
    document_era: bool = False,
    episode_planning_era: bool = False,
    adaptation_era: bool = False,
    narrative_era: bool = False,
    asset_state_era: bool = False,
) -> list[object]:
    if (
        sum(
            (
                historical_pre_provider,
                provider_era,
                document_era,
                episode_planning_era,
                adaptation_era,
                narrative_era,
                asset_state_era,
            )
        )
        > 1
    ):
        raise ValueError("schema comparison mode must be singular")
    include_object = _include_baseline_object
    if historical_pre_provider:
        include_object = _include_historical_pre_provider_object
    elif provider_era:
        include_object = _include_provider_era_object
    elif document_era:
        include_object = _include_document_era_object
    elif episode_planning_era:
        include_object = _include_episode_planning_era_object
    elif adaptation_era:
        include_object = _include_adaptation_era_object
    elif narrative_era:
        include_object = _include_narrative_era_object
    elif asset_state_era:
        include_object = _include_pre_asset_change_object
    context = MigrationContext.configure(
        connection,
        opts={
            "compare_type": True,
            "compare_server_default": True,
            "include_object": include_object,
        },
    )
    return list(compare_metadata(context, Base.metadata))


def _assert_database_matches_metadata(connection: Connection) -> None:
    differences = _baseline_differences(connection)
    if not differences:
        return
    summary = "; ".join(repr(item) for item in differences[:3])
    raise DatabaseSchemaMismatchError(
        f"database schema differs from baseline; first differences: {summary}"
    )


async def assert_database_matches_metadata(target_engine: AsyncEngine = engine) -> None:
    async with target_engine.connect() as connection:
        await connection.run_sync(_assert_database_matches_metadata)


def _adopt_existing_database(connection: Connection) -> None:
    inspector = inspect(connection)
    table_names = set(inspector.get_table_names()) - {"alembic_version"}
    if not table_names:
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline; an empty database must use upgrade"
        )

    current = _database_heads(connection)
    if current:
        ensure_expected_heads(current, get_script_heads())
        return

    configuration = create_alembic_config()
    configuration.attributes["connection"] = connection
    current_differences = _baseline_differences(connection)
    if not current_differences:
        command.stamp(configuration, "head")
        return

    provider_tables = table_names & PROVIDER_TABLE_NAMES
    script_document_tables = table_names & SCRIPT_DOCUMENT_TABLE_NAMES
    episode_planning_tables = table_names & EPISODE_PLANNING_TABLE_NAMES
    adaptation_tables = table_names & ADAPTATION_TABLE_NAMES
    narrative_tables = table_names & NARRATIVE_TABLE_NAMES
    asset_state_tables = table_names & ASSET_STATE_TABLE_NAMES
    asset_change_tables = table_names & ASSET_CHANGE_TABLE_NAMES
    asset_columns = {column["name"] for column in inspector.get_columns("ast_assets")}
    asset_change_columns = asset_columns & ASSET_CHANGE_COLUMNS
    capability_unique_present = "prod_model_capabilities" in table_names and any(
        constraint["name"] == PROVIDER_CAPABILITY_UNIQUE
        for constraint in inspector.get_unique_constraints("prod_model_capabilities")
    )
    expected_script_document_tables = set(SCRIPT_DOCUMENT_TABLE_NAMES)
    expected_provider_tables = set(PROVIDER_TABLE_NAMES)
    expected_episode_planning_tables = set(EPISODE_PLANNING_TABLE_NAMES)
    expected_adaptation_tables = set(ADAPTATION_TABLE_NAMES)
    expected_narrative_tables = set(NARRATIVE_TABLE_NAMES)
    expected_asset_state_tables = set(ASSET_STATE_TABLE_NAMES)
    expected_asset_change_tables = set(ASSET_CHANGE_TABLE_NAMES)
    expected_asset_change_columns = set(ASSET_CHANGE_COLUMNS)
    if (
        (asset_change_tables and asset_change_tables != expected_asset_change_tables)
        or (asset_change_columns and asset_change_columns != expected_asset_change_columns)
        or bool(asset_change_tables) != bool(asset_change_columns)
    ):
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline; partial asset change schema; "
            f"tables={sorted(asset_change_tables)!r}, "
            f"columns={sorted(asset_change_columns)!r}"
        )
    if asset_state_tables and asset_state_tables != expected_asset_state_tables:
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline; partial AssetState schema; "
            f"tables={sorted(asset_state_tables)!r}"
        )
    if narrative_tables and narrative_tables != expected_narrative_tables:
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline; partial NarrativeUnit schema; "
            f"tables={sorted(narrative_tables)!r}"
        )
    if adaptation_tables and adaptation_tables != expected_adaptation_tables:
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline; partial AdaptationRun schema; "
            f"tables={sorted(adaptation_tables)!r}"
        )
    if episode_planning_tables and episode_planning_tables != expected_episode_planning_tables:
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline; partial EpisodePlan schema; "
            f"tables={sorted(episode_planning_tables)!r}"
        )
    if script_document_tables and script_document_tables != expected_script_document_tables:
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline; partial ScriptDocument schema; "
            f"tables={sorted(script_document_tables)!r}"
        )
    provider_complete = provider_tables == expected_provider_tables and capability_unique_present
    provider_absent = not provider_tables and not capability_unique_present
    if not provider_complete and not provider_absent:
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline; partial historical Provider schema; "
            f"tables={sorted(provider_tables)!r}, "
            f"capability_unique={capability_unique_present!r}"
        )

    if asset_change_tables:
        summary = "; ".join(repr(item) for item in current_differences[:3])
        raise DatabaseSchemaMismatchError(
            "database schema differs from current asset change schema; "
            f"first differences: {summary}"
        )

    if asset_state_tables:
        asset_state_era_differences = _baseline_differences(
            connection,
            asset_state_era=True,
        )
        if asset_state_era_differences:
            summary = "; ".join(repr(item) for item in asset_state_era_differences[:3])
            raise DatabaseSchemaMismatchError(
                f"database schema differs from AssetState-era schema; first differences: {summary}"
            )
        command.stamp(configuration, ASSET_STATE_REVISION)
        command.upgrade(configuration, "head")
        _assert_database_matches_metadata(connection)
        ensure_expected_heads(_database_heads(connection), get_script_heads())
        return

    if narrative_tables:
        narrative_era_differences = _baseline_differences(
            connection,
            narrative_era=True,
        )
        if narrative_era_differences:
            summary = "; ".join(repr(item) for item in narrative_era_differences[:3])
            raise DatabaseSchemaMismatchError(
                "database schema differs from NarrativeUnit-era schema; "
                f"first differences: {summary}"
            )
        command.stamp(configuration, NARRATIVE_REVISION)
        command.upgrade(configuration, "head")
        _assert_database_matches_metadata(connection)
        ensure_expected_heads(_database_heads(connection), get_script_heads())
        return

    if adaptation_tables:
        adaptation_era_differences = _baseline_differences(
            connection,
            adaptation_era=True,
        )
        if adaptation_era_differences:
            summary = "; ".join(repr(item) for item in adaptation_era_differences[:3])
            raise DatabaseSchemaMismatchError(
                "database schema differs from AdaptationRun-era schema; "
                f"first differences: {summary}"
            )
        command.stamp(configuration, ADAPTATION_REVISION)
        command.upgrade(configuration, "head")
        _assert_database_matches_metadata(connection)
        ensure_expected_heads(_database_heads(connection), get_script_heads())
        return

    if episode_planning_tables:
        episode_planning_era_differences = _baseline_differences(
            connection,
            episode_planning_era=True,
        )
        if episode_planning_era_differences:
            summary = "; ".join(repr(item) for item in episode_planning_era_differences[:3])
            raise DatabaseSchemaMismatchError(
                f"database schema differs from EpisodePlan-era schema; first differences: {summary}"
            )
        command.stamp(configuration, EPISODE_PLANNING_REVISION)
        command.upgrade(configuration, "head")
        _assert_database_matches_metadata(connection)
        ensure_expected_heads(_database_heads(connection), get_script_heads())
        return

    if script_document_tables:
        document_era_differences = _baseline_differences(
            connection,
            document_era=True,
        )
        if document_era_differences:
            summary = "; ".join(repr(item) for item in document_era_differences[:3])
            raise DatabaseSchemaMismatchError(
                "database schema differs from ScriptDocument-era schema; "
                f"first differences: {summary}"
            )
        command.stamp(configuration, SCRIPT_DOCUMENT_REVISION)
        command.upgrade(configuration, "head")
        _assert_database_matches_metadata(connection)
        ensure_expected_heads(_database_heads(connection), get_script_heads())
        return

    if provider_complete:
        provider_era_differences = _baseline_differences(
            connection,
            provider_era=True,
        )
        if provider_era_differences:
            summary = "; ".join(repr(item) for item in provider_era_differences[:3])
            raise DatabaseSchemaMismatchError(
                f"database schema differs from Provider-era schema; first differences: {summary}"
            )
        command.stamp(configuration, PROVIDER_REVISION)
        command.upgrade(configuration, "head")
        _assert_database_matches_metadata(connection)
        ensure_expected_heads(_database_heads(connection), get_script_heads())
        return

    historical_differences = _baseline_differences(
        connection,
        historical_pre_provider=True,
    )
    if historical_differences:
        summary = "; ".join(repr(item) for item in historical_differences[:3])
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline and historical pre-Provider schema; "
            f"first differences: {summary}"
        )

    command.stamp(configuration, BASELINE_REVISION)
    command.upgrade(configuration, "head")
    _assert_database_matches_metadata(connection)
    ensure_expected_heads(_database_heads(connection), get_script_heads())


async def adopt_existing_database(
    target_engine: AsyncEngine = engine,
    *,
    backup_reference: str,
) -> None:
    validate_backup_reference(backup_reference)
    async with target_engine.begin() as connection:
        await connection.run_sync(_adopt_existing_database)
