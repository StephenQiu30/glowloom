from __future__ import annotations

from collections.abc import Iterable
from typing import Any, cast

from sqlalchemy import inspect, text
from sqlalchemy.engine import Connection, Dialect
from sqlalchemy.engine.reflection import Inspector
from sqlalchemy.ext.asyncio import AsyncEngine
from sqlalchemy.schema import (
    CheckConstraint,
    Column,
    DefaultClause,
    ForeignKeyConstraint,
    PrimaryKeyConstraint,
    Table,
    UniqueConstraint,
)
from sqlalchemy.sql.type_api import TypeEngine

from app.core.database import Base, engine

LEGACY_TABLE_NAMES = frozenset({"alembic_version"})
SCHEMA_LOCK_KEY = "lanverse.final-schema"


class DatabaseSchemaMismatchError(RuntimeError):
    pass


def _normalize(value: object | None) -> str | None:
    if value is None:
        return None
    return " ".join(str(value).replace("::character varying", "").split()).lower()


def _type_signature(value: TypeEngine[Any], dialect: Dialect) -> str | None:
    return _normalize(value.compile(dialect=dialect))


def _columns_signature(columns: Iterable[Column[Any]]) -> tuple[str, ...]:
    return tuple(column.name for column in columns)


def _expected_unique_constraints(table: Table) -> dict[tuple[str, ...], str | None]:
    constraints = table.constraints
    return {
        _columns_signature(constraint.columns): cast(str | None, constraint.name)
        for constraint in constraints
        if isinstance(constraint, UniqueConstraint)
    }


def _actual_unique_constraints(
    inspector: Inspector,
    table_name: str,
) -> dict[tuple[str, ...], str | None]:
    return {
        tuple(item.get("column_names") or ()): item.get("name")
        for item in inspector.get_unique_constraints(table_name)
    }


def _expected_foreign_keys(table: Table) -> dict[tuple[object, ...], str | None]:
    result: dict[tuple[object, ...], str | None] = {}
    for constraint in table.constraints:
        if not isinstance(constraint, ForeignKeyConstraint):
            continue
        key = (
            _columns_signature(constraint.columns),
            tuple(element.target_fullname for element in constraint.elements),
            constraint.ondelete,
            constraint.onupdate,
        )
        result[key] = cast(str | None, constraint.name)
    return result


def _actual_foreign_keys(
    inspector: Inspector,
    table_name: str,
) -> dict[tuple[object, ...], str | None]:
    result: dict[tuple[object, ...], str | None] = {}
    for item in inspector.get_foreign_keys(table_name):
        options = item.get("options") or {}
        key = (
            tuple(item.get("constrained_columns") or ()),
            tuple(
                f"{item.get('referred_table')}.{column}"
                for column in (item.get("referred_columns") or ())
            ),
            options.get("ondelete"),
            options.get("onupdate"),
        )
        result[key] = item.get("name")
    return result


def _check_signature(name: str | None, expression: object | None) -> tuple[str, str | None]:
    if name:
        return (f"name:{name}", None)
    return ("expression", _normalize(expression))


def _expected_checks(table: Table) -> set[tuple[str, str | None]]:
    return {
        _check_signature(cast(str | None, constraint.name), constraint.sqltext)
        for constraint in table.constraints
        if isinstance(constraint, CheckConstraint)
    }


def _actual_checks(inspector: Inspector, table_name: str) -> set[tuple[str, str | None]]:
    return {
        _check_signature(item.get("name"), item.get("sqltext"))
        for item in inspector.get_check_constraints(table_name)
    }


def _expected_indexes(table: Table) -> set[tuple[str | None, tuple[str, ...], bool]]:
    return {
        (cast(str | None, index.name), _columns_signature(index.columns), bool(index.unique))
        for index in table.indexes
    }


def _actual_indexes(
    inspector: Inspector,
    table_name: str,
) -> set[tuple[str | None, tuple[str, ...], bool]]:
    unique_constraint_names = {
        item.get("name") for item in inspector.get_unique_constraints(table_name)
    }
    return {
        (
            item.get("name"),
            tuple(column for column in (item.get("column_names") or ()) if column is not None),
            bool(item.get("unique")),
        )
        for item in inspector.get_indexes(table_name)
        if item.get("name") not in unique_constraint_names
    }


def _schema_differences(connection: Connection) -> list[str]:
    inspector = inspect(connection)
    expected_tables = set(Base.metadata.tables)
    actual_tables = set(inspector.get_table_names()) - LEGACY_TABLE_NAMES
    differences: list[str] = []

    differences.extend(f"missing table {name}" for name in sorted(expected_tables - actual_tables))
    differences.extend(
        f"unexpected table {name}" for name in sorted(actual_tables - expected_tables)
    )

    for table_name in sorted(expected_tables & actual_tables):
        expected_table = Base.metadata.tables[table_name]
        expected_columns = {column.name: column for column in expected_table.columns}
        actual_columns = {item["name"]: item for item in inspector.get_columns(table_name)}

        differences.extend(
            f"{table_name}: missing column {name}"
            for name in sorted(set(expected_columns) - set(actual_columns))
        )
        differences.extend(
            f"{table_name}: unexpected column {name}"
            for name in sorted(set(actual_columns) - set(expected_columns))
        )
        for column_name in sorted(set(expected_columns) & set(actual_columns)):
            expected_column = expected_columns[column_name]
            actual_column = actual_columns[column_name]
            expected_default = (
                _normalize(expected_column.server_default.arg)
                if isinstance(expected_column.server_default, DefaultClause)
                else None
            )
            actual_default = _normalize(actual_column.get("default"))
            expected_signature = (
                _type_signature(expected_column.type, connection.dialect),
                expected_column.nullable,
                expected_default,
            )
            actual_signature = (
                _type_signature(actual_column["type"], connection.dialect),
                actual_column["nullable"],
                actual_default,
            )
            if expected_signature != actual_signature:
                differences.append(f"{table_name}.{column_name}: column definition differs")

        expected_primary_key = next(
            constraint
            for constraint in expected_table.constraints
            if isinstance(constraint, PrimaryKeyConstraint)
        )
        actual_primary_key = inspector.get_pk_constraint(table_name)
        if _columns_signature(expected_primary_key.columns) != tuple(
            actual_primary_key.get("constrained_columns") or ()
        ):
            differences.append(f"{table_name}: primary key differs")

        expected_unique = _expected_unique_constraints(expected_table)
        actual_unique = _actual_unique_constraints(inspector, table_name)
        if set(expected_unique) != set(actual_unique):
            differences.append(f"{table_name}: unique constraints differ")
        else:
            for columns, expected_name in expected_unique.items():
                actual_name = actual_unique[columns]
                if expected_name is not None and expected_name != actual_name:
                    differences.append(f"{table_name}: unique constraint name differs")

        expected_foreign_keys = _expected_foreign_keys(expected_table)
        actual_foreign_keys = _actual_foreign_keys(inspector, table_name)
        if set(expected_foreign_keys) != set(actual_foreign_keys):
            differences.append(f"{table_name}: foreign keys differ")
        else:
            for key, expected_name in expected_foreign_keys.items():
                actual_name = actual_foreign_keys[key]
                if expected_name is not None and expected_name != actual_name:
                    differences.append(f"{table_name}: foreign key name differs")

        if _expected_checks(expected_table) != _actual_checks(inspector, table_name):
            differences.append(f"{table_name}: check constraints differ")
        if _expected_indexes(expected_table) != _actual_indexes(inspector, table_name):
            differences.append(f"{table_name}: indexes differ")

    return differences


def _assert_database_schema(connection: Connection) -> None:
    differences = _schema_differences(connection)
    if differences:
        summary = "; ".join(differences[:8])
        raise DatabaseSchemaMismatchError(
            "database schema does not match the final ORM metadata; "
            f"first differences: {summary}"
        )


def _drop_legacy_revision_marker(connection: Connection) -> None:
    connection.execute(text('DROP TABLE IF EXISTS "alembic_version"'))


def _initialize_schema(connection: Connection) -> None:
    connection.execute(
        text("SELECT pg_advisory_xact_lock(hashtext(:lock_key))"),
        {"lock_key": SCHEMA_LOCK_KEY},
    )
    table_names = set(inspect(connection).get_table_names())
    application_tables = table_names - LEGACY_TABLE_NAMES
    if not application_tables:
        _drop_legacy_revision_marker(connection)
        Base.metadata.create_all(connection)
        return

    _assert_database_schema(connection)
    _drop_legacy_revision_marker(connection)


async def initialize_database(target_engine: AsyncEngine = engine) -> None:
    async with target_engine.begin() as connection:
        await connection.run_sync(_initialize_schema)
    await assert_database_schema(target_engine)


async def assert_database_schema(target_engine: AsyncEngine = engine) -> None:
    async with target_engine.connect() as connection:
        await connection.run_sync(_assert_database_schema)
