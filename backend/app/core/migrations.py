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


def _baseline_differences(connection: Connection) -> list[object]:
    context = MigrationContext.configure(
        connection,
        opts={
            "compare_type": True,
            "compare_server_default": True,
            "include_object": _include_baseline_object,
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
    table_names = set(inspect(connection).get_table_names()) - {"alembic_version"}
    if not table_names:
        raise DatabaseSchemaMismatchError(
            "database schema differs from baseline; an empty database must use upgrade"
        )

    current = _database_heads(connection)
    if current:
        ensure_expected_heads(current, get_script_heads())
        return

    _assert_database_matches_metadata(connection)

    configuration = create_alembic_config()
    configuration.attributes["connection"] = connection
    command.stamp(configuration, "head")


async def adopt_existing_database(
    target_engine: AsyncEngine = engine,
    *,
    backup_reference: str,
) -> None:
    validate_backup_reference(backup_reference)
    async with target_engine.begin() as connection:
        await connection.run_sync(_adopt_existing_database)
