from __future__ import annotations

import os
import subprocess
import sys
from collections.abc import AsyncIterator
from pathlib import Path
from uuid import uuid4

import asyncpg
import pytest
import pytest_asyncio

ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
MIGRATION = BACKEND / "migrations" / "versions" / "0001_mvp.py"

TABLES = {
    "adoptions",
    "creative_asset_versions",
    "delivery_versions",
    "episodes",
    "generation_candidates",
    "idempotency_records",
    "media_objects",
    "media_versions",
    "production_attempts",
    "production_tasks",
    "projects",
    "render_snapshots",
    "script_versions",
    "shot_spec_versions",
    "source_revisions",
    "submission_snapshots",
    "subtitle_versions",
    "task_events",
    "task_jobs",
    "task_outputs",
}


@pytest_asyncio.fixture
async def isolated_database() -> AsyncIterator[str]:
    name = f"lanverse_test_{uuid4().hex}"
    assert name.startswith("lanverse_test_")
    admin = await asyncpg.connect(database="postgres")
    await admin.execute(f'CREATE DATABASE "{name}" TEMPLATE template0')
    try:
        yield f"postgresql:///{name}"
    finally:
        await admin.execute(
            "SELECT pg_terminate_backend(pid) FROM pg_stat_activity "
            "WHERE datname = $1 AND pid <> pg_backend_pid()",
            name,
        )
        await admin.execute(f'DROP DATABASE "{name}"')
        await admin.close()


def run_upgrade(database_url: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, "-m", "alembic", "-c", "alembic.ini", "upgrade", "head"],
        cwd=BACKEND,
        env={**os.environ, "DATABASE_URL": database_url},
        check=False,
        capture_output=True,
        text=True,
    )


async def catalog_snapshot(database_url: str) -> tuple[list[asyncpg.Record], ...]:
    connection = await asyncpg.connect(database_url)
    try:
        tables = await connection.fetch(
            "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename"
        )
        columns = await connection.fetch(
            "SELECT table_name,column_name,data_type,is_nullable,column_default "
            "FROM information_schema.columns WHERE table_schema='public' "
            "AND table_name <> 'alembic_version' "
            "ORDER BY table_name,ordinal_position"
        )
        constraints = await connection.fetch(
            "SELECT c.conname,c.contype::text contype,t.relname,"
            "pg_get_constraintdef(c.oid) definition "
            "FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid "
            "JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' "
            "AND t.relname <> 'alembic_version' "
            "ORDER BY t.relname,c.conname"
        )
        indexes = await connection.fetch(
            "SELECT tablename,indexname,indexdef FROM pg_indexes WHERE schemaname='public' "
            "AND tablename <> 'alembic_version' "
            "ORDER BY tablename,indexname"
        )
        return tables, columns, constraints, indexes
    finally:
        await connection.close()


def test_migration_only_executes_the_reviewed_root_sql() -> None:
    assert MIGRATION.is_file()
    text = MIGRATION.read_text()
    assert "CREATE TABLE" not in text.upper()
    assert "sql/[0-9][0-9]_*.sql" in text
    assert "op.create_table" not in text


@pytest.mark.asyncio
async def test_double_upgrade_builds_a_stable_exact_catalog(isolated_database: str) -> None:
    first = run_upgrade(isolated_database)
    assert first.returncode == 0, first.stderr
    initial = await catalog_snapshot(isolated_database)

    second = run_upgrade(isolated_database)
    assert second.returncode == 0, second.stderr
    repeated = await catalog_snapshot(isolated_database)

    assert initial == repeated
    tables, columns, constraints, indexes = initial
    assert {row["tablename"] for row in tables} == TABLES | {"alembic_version"}
    assert len(columns) == 272
    assert sum(row["contype"] == "p" for row in constraints) == 20
    assert sum(row["contype"] == "f" for row in constraints) == 51
    assert sum(row["contype"] == "u" for row in constraints) == 37
    assert sum(row["contype"] == "c" for row in constraints) == 20
    assert len(indexes) == 114


@pytest.mark.asyncio
async def test_database_constraints_reject_invalid_rows(isolated_database: str) -> None:
    result = run_upgrade(isolated_database)
    assert result.returncode == 0, result.stderr
    connection = await asyncpg.connect(isolated_database)
    try:
        with pytest.raises(asyncpg.CheckViolationError):
            await connection.execute(
                "INSERT INTO projects(id,title,width) VALUES($1,$2,$3)", uuid4(), "bad", 1
            )
        with pytest.raises(asyncpg.ForeignKeyViolationError):
            await connection.execute(
                "INSERT INTO episodes(id,project_id) VALUES($1,$2)", uuid4(), uuid4()
            )
    finally:
        await connection.close()
