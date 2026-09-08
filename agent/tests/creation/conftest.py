import os
from collections.abc import AsyncIterator

import pytest

from app.creation.repository import Repository


@pytest.fixture
async def repository() -> AsyncIterator[Repository]:
    dsn = os.getenv("LANVERSE_TEST_CREATION_DATABASE_URL")
    if not dsn:
        pytest.skip("requires an isolated database on the existing PostgreSQL instance")
    store = Repository(dsn)
    await store.migrate()
    async with await store.connect() as conn:
        await conn.execute(
            "TRUNCATE creation_result_outbox, creation_output_bindings, "
            "creation_drafts, creation_attempts, creation_steps, creation_executions, "
            "creation_start_outbox, creation_commands"
        )
    yield store
