from __future__ import annotations

import os
import subprocess
import sys
from collections.abc import AsyncIterator
from pathlib import Path
from uuid import uuid4

import asyncpg
import pytest_asyncio

BACKEND = Path(__file__).resolve().parents[1]


@pytest_asyncio.fixture
async def migrated_database_url() -> AsyncIterator[str]:
    name = f"lanverse_test_{uuid4().hex}"
    assert name.startswith("lanverse_test_")
    admin = await asyncpg.connect(database="postgres")
    await admin.execute(f'CREATE DATABASE "{name}" TEMPLATE template0')
    database_url = f"postgresql:///{name}"
    result = subprocess.run(
        [sys.executable, "-m", "alembic", "-c", "alembic.ini", "upgrade", "head"],
        cwd=BACKEND,
        env={**os.environ, "DATABASE_URL": database_url},
        check=False,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, result.stderr
    try:
        yield database_url
    finally:
        await admin.execute(
            "SELECT pg_terminate_backend(pid) FROM pg_stat_activity "
            "WHERE datname = $1 AND pid <> pg_backend_pid()",
            name,
        )
        await admin.execute(f'DROP DATABASE "{name}"')
        await admin.close()
