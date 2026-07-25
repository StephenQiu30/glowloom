from __future__ import annotations

import re
from pathlib import Path

TYPE_MAP = {
    "u": "uuid",
    "t": "text",
    "i": "integer",
    "b": "bigint",
    "j": "jsonb",
    "z": "timestamptz",
    "h": "varchar(64)",
}
FORBIDDEN = re.compile(
    r"\b(CREATE\s+(?:DATABASE|SCHEMA|USER|ROLE|EXTENSION)|DROP|TRUNCATE)\b",
    re.IGNORECASE,
)


def _fail(message: str) -> None:
    raise AssertionError(message)


def _normalize(value: str) -> str:
    return re.sub(r"\s+", "", value)


def _sql_type(token: str) -> str:
    nullable = token.endswith("?")
    token = token.removesuffix("?")
    if token.startswith("v("):
        value = f"varchar({token[2:-1]})"
    else:
        value = TYPE_MAP[token]
    return value + ("?" if nullable else "")


def _table_columns(sql: str) -> list[tuple[str, str]]:
    result: list[tuple[str, str]] = []
    pattern = re.compile(
        r"^    ([a-z][a-z0-9_]*) "
        r"(uuid|text|varchar\([1-9][0-9]*\)|integer|bigint|jsonb|timestamptz)"
        r"([^\n]*)$",
        re.MULTILINE,
    )
    for name, field_type, suffix in pattern.findall(sql):
        nullable = "NOT NULL" not in suffix
        result.append((name, field_type + ("?" if nullable else "")))
    return result


def _manifest_fks(
    manifest: dict,
) -> set[tuple[str, tuple[str, ...], str, tuple[str, ...]]]:
    return {
        (child, tuple(local), parent, tuple(remote))
        for child, local, parent, remote in manifest["foreign_keys"]
    }


def _sql_fks(sql: str) -> set[tuple[str, tuple[str, ...], str, tuple[str, ...]]]:
    pattern = re.compile(
        r"ALTER TABLE public\.([a-z_]+) ADD CONSTRAINT ([a-z0-9_]+) "
        r"FOREIGN KEY \(([^)]+)\) REFERENCES public\.([a-z_]+) \(([^)]+)\) "
        r"MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT "
        r"NOT DEFERRABLE INITIALLY IMMEDIATE;"
    )
    result = set()
    names: set[str] = set()
    for child, name, local, parent, remote in pattern.findall(sql):
        if name in names or len(name.encode()) > 63:
            _fail(f"invalid or duplicate FK name {name}")
        names.add(name)
        result.add(
            (
                child,
                tuple(item.strip() for item in local.split(",")),
                parent,
                tuple(item.strip() for item in remote.split(",")),
            )
        )
    if sql.count("ALTER TABLE") != len(result):
        _fail("foreign-key file contains an unrecognized or duplicate statement")
    return result


def _sql_partials(sql: str, unique: bool) -> set[tuple[str, str, tuple[str, ...], str]]:
    keyword = "CREATE UNIQUE INDEX" if unique else "CREATE INDEX"
    pattern = re.compile(
        rf"{keyword} ([a-z0-9_]+) ON public\.([a-z_]+) "
        r"\(([^)]+)\) WHERE ([^;]+);"
    )
    return {
        (
            name,
            table,
            tuple(item.strip() for item in columns.split(",")),
            _normalize(predicate),
        )
        for name, table, columns, predicate in pattern.findall(sql)
    }


def _manifest_partials(
    manifest: dict, key: str
) -> set[tuple[str, str, tuple[str, ...], str]]:
    return {
        (name, table, tuple(columns), _normalize(predicate))
        for name, table, columns, predicate in manifest[key]
    }


def validate_sql_artifacts(
    sql_dir: Path,
    manifest: dict,
    columns: dict[str, dict[str, str]],
) -> list[Path]:
    if manifest.get("database") != "lanverse" or manifest.get("namespace") != "public":
        _fail("database namespace must be lanverse/public")
    tables = list(columns)
    expected = [f"{index:02d}_{table}.sql" for index, table in enumerate(tables, 1)]
    actual = sorted(path.name for path in sql_dir.glob("[0-9][0-9]_*.sql"))
    expected_all = sorted([*expected, "90_foreign_keys.sql", "91_indexes.sql"])
    if actual != expected_all:
        _fail(
            f"SQL file exact-set mismatch: expected={expected_all!r}, actual={actual!r}"
        )

    paths: list[Path] = []
    constraint_names: set[str] = set()
    for filename, table in zip(expected, tables):
        path = sql_dir / filename
        sql = path.read_text(encoding="utf-8")
        paths.append(path)
        if FORBIDDEN.search(sql) or "REFERENCES public." in sql:
            _fail(f"forbidden table-file statement in {filename}")
        created = re.findall(r"CREATE TABLE public\.([a-z_]+)", sql)
        if created != [table] or sql.count(";") != 1:
            _fail(f"{filename} must create exactly public.{table}")
        expected_columns = [
            (name, _sql_type(token)) for name, token in columns[table].items()
        ]
        if _table_columns(sql) != expected_columns:
            _fail(f"column mismatch in {filename}")
        required = {f"pk_{table}", f"ck_{table}_invariants"}
        names = set(re.findall(r"CONSTRAINT ([a-z0-9_]+)", sql))
        if not required <= names:
            _fail(f"missing PK/CHECK name in {filename}")
        if constraint_names & names or any(len(name.encode()) > 63 for name in names):
            _fail(f"duplicate or oversized constraint in {filename}")
        constraint_names |= names

    fk_path = sql_dir / "90_foreign_keys.sql"
    index_path = sql_dir / "91_indexes.sql"
    fk_sql = fk_path.read_text(encoding="utf-8")
    index_sql = index_path.read_text(encoding="utf-8")
    if FORBIDDEN.search(fk_sql + index_sql):
        _fail("forbidden relation/index statement")
    if _sql_fks(fk_sql) != _manifest_fks(manifest):
        _fail("SQL/manifest foreign-key exact-set mismatch")
    if _sql_partials(index_sql, True) != _manifest_partials(
        manifest, "partial_uniques"
    ):
        _fail("SQL/manifest partial-unique exact-set mismatch")
    if _sql_partials(index_sql, False) != _manifest_partials(
        manifest, "partial_indexes"
    ):
        _fail("SQL/manifest partial-index exact-set mismatch")
    index_names = re.findall(r"CREATE (?:UNIQUE )?INDEX ([a-z0-9_]+)", index_sql)
    if len(index_names) != len(set(index_names)) or any(
        len(name.encode()) > 63 for name in index_names
    ):
        _fail("duplicate or oversized index name")
    return [*paths, fk_path, index_path, sql_dir / "README.md"]
