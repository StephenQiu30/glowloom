import asyncio
from uuid import uuid4

import pytest

from app.creation.contract import Command
from app.creation.repository import CommandConflict, Repository
from tests.creation.test_contract import command_payload


def new_command() -> Command:
    payload = command_payload()
    identity = str(uuid4())
    payload.update(
        command_id=identity, run_id=identity, workflow_id=f"lanverse:creation:{identity}"
    )
    return Command.model_validate(payload)


async def test_acceptance_is_atomic_immutable_and_survives_new_repository(
    repository: Repository,
) -> None:
    command = new_command()
    receipts = await asyncio.gather(
        *(repository.accept(command, "creation-text") for _ in range(8))
    )
    assert all(receipt == receipts[0] for receipt in receipts)
    assert await Repository(repository.dsn).get(command.command_id) == receipts[0]
    assert await repository.get(str(uuid4())) is None
    changed = command.model_copy(update={"actor_id": str(uuid4())})
    with pytest.raises(CommandConflict):
        await repository.accept(changed, "another-queue")
    lease = await repository.claim()
    assert lease is not None
    assert lease.task_queue == "creation-text"
    assert lease.command == command
    assert await repository.claim() is None


async def test_acceptance_rolls_back_if_outbox_insert_fails(repository: Repository) -> None:
    command = new_command()
    with pytest.raises(Exception, match="creation_start_outbox"):
        await repository.accept(command, "")
    assert await repository.get(command.command_id) is None


async def test_expired_worker_cannot_complete_new_lease(repository: Repository) -> None:
    command = new_command()
    await repository.accept(command, "creation-text")
    first = await repository.claim()
    assert first is not None
    async with await repository.connect() as conn:
        await conn.execute(
            "UPDATE creation_start_outbox SET lease_until = clock_timestamp() - interval '1 second'"
        )
    second = await repository.claim()
    assert second is not None and second.fence > first.fence
    assert not await repository.finish(first, temporal_run_id=str(uuid4()))
    assert not await repository.fail(first, "temporal_unavailable")
    assert await repository.finish(second, temporal_run_id=str(uuid4()))
    assert await repository.claim() is None


async def test_backoff_and_conflict_are_persistent(repository: Repository) -> None:
    await repository.accept(new_command(), "creation-text")
    lease = await repository.claim()
    assert lease is not None
    assert await repository.fail(lease, "temporal_unavailable")
    assert await repository.claim() is None
    async with await repository.connect() as conn:
        row = await (
            await conn.execute("SELECT state, last_error, attempts FROM creation_start_outbox")
        ).fetchone()
        assert row == {"state": "pending", "last_error": "temporal_unavailable", "attempts": 1}
        await conn.execute("UPDATE creation_start_outbox SET next_attempt_at = clock_timestamp()")
    lease = await repository.claim()
    assert lease is not None
    assert await repository.fail(lease, "workflow_identity_conflict", blocked=True)
    assert await repository.claim() is None
    async with await repository.connect() as conn:
        row = await (await conn.execute("SELECT state FROM creation_start_outbox")).fetchone()
        assert row == {"state": "blocked"}


async def test_migration_is_explicit_and_detects_checksum_drift(repository: Repository) -> None:
    from app.creation.repository import SchemaMismatch

    await repository.ready()
    await repository.migrate()
    async with await repository.connect() as conn:
        await conn.execute("UPDATE creation_schema SET checksum = 'drift'")
    try:
        with pytest.raises(SchemaMismatch):
            await repository.ready()
        with pytest.raises(SchemaMismatch):
            await repository.migrate()
    finally:
        from app.creation.repository import SCHEMA_HASH

        async with await repository.connect() as conn:
            await conn.execute("UPDATE creation_schema SET checksum = %s", (SCHEMA_HASH,))


async def test_initial_migration_refuses_an_existing_business_database(
    repository: Repository,
) -> None:
    from app.creation.repository import SchemaMismatch

    # Only manipulate the dedicated temporary test database supplied by the test runner.
    async with await repository.connect() as conn:
        await conn.execute("ALTER TABLE creation_schema RENAME TO saved_creation_schema")
    try:
        with pytest.raises(SchemaMismatch, match="empty dedicated database"):
            await repository.migrate()
    finally:
        async with await repository.connect() as conn:
            await conn.execute("ALTER TABLE saved_creation_schema RENAME TO creation_schema")


async def test_old_commands_allow_lookup_but_not_a_fresh_start(repository: Repository) -> None:
    command = new_command()
    receipt = await repository.accept(command, "creation-text")
    async with await repository.connect() as conn:
        await conn.execute(
            "UPDATE creation_commands SET accepted_at = clock_timestamp() - interval '13 hours'"
        )
    lease = await repository.claim()
    assert lease is not None and not lease.start_allowed
    assert await repository.get(command.command_id) == receipt
