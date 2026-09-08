from __future__ import annotations

import asyncio
import time
from dataclasses import replace
from uuid import uuid4

import httpx
import pytest
from psycopg.types.json import Jsonb

from app.creation.execution import ExecutionConflict, ExecutionStore, InvocationLease
from app.creation.repository import Repository
from app.main import create_app
from app.protocol.canonical import canonical_hash
from tests.creation.test_contract import SECRET, authorization
from tests.creation.test_execution import result_for, setup_execution


async def test_concurrent_attempt_and_result_bindings_survive_reconnect(
    repository: Repository,
) -> None:
    store, run, task = await setup_execution(repository)
    results = await asyncio.gather(
        *(store.reserve(run, task) for _ in range(8)), return_exceptions=True
    )
    leases = [result for result in results if isinstance(result, InvocationLease)]
    assert len(leases) == 1
    lease = leases[0]
    history = await store.attempt_history(run, lease.step_id)
    assert history and history["history_origin"] == "recorded"
    assert len(history["attempts"]) == 1
    initial = history["attempts"][0]
    assert initial["attempt_id"] == lease.attempt_id
    assert initial["state"] == "running" and initial["attempt_no"] == 1
    assert initial["fence"] == lease.fence and initial["input_hash"] == lease.input_hash
    assert initial["started_at"] < initial["execution_deadline"] < initial["lease_expires_at"]
    assert initial["finished_at"] is None and initial["result_hash"] is None
    result = await result_for(task)
    await store.finish(lease, result)
    restarted = ExecutionStore(Repository(repository.dsn))
    completed = await restarted.attempt_history(run, lease.step_id)
    assert completed and completed["attempts"][0]["state"] == "succeeded"
    assert completed["attempts"][0]["finished_at"] is not None
    assert await restarted.reserve(run, task) == result
    await restarted.finish(lease, result)
    assert await restarted.attempt_history(run, lease.step_id) == completed
    assert not await restarted.unknown(lease, "harness_response_unknown")
    async with await repository.connect() as conn:
        row = await (
            await conn.execute(
                "SELECT a.id::text, a.result_hash, d.result_hash AS draft_hash, "
                "d.attempt_id::text AS draft_attempt, o.attempt_id::text AS event_attempt "
                "FROM creation_attempts a JOIN creation_drafts d ON d.attempt_id = a.id "
                "JOIN creation_result_outbox o ON o.attempt_id = a.id WHERE a.id = %s",
                (lease.attempt_id,),
            )
        ).fetchone()
        assert row and row["id"] == row["draft_attempt"] == row["event_attempt"] == lease.attempt_id
        assert row["result_hash"] == row["draft_hash"]
    assert (await restarted.snapshot(run))["reserved_calls"] == 1


async def test_forged_attempt_cannot_finish_or_invalidate_owner(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    forged = replace(lease, attempt_id=str(uuid4()))
    with pytest.raises(ExecutionConflict, match="attempt_fence_lost"):
        await store.finish(forged, await result_for(task))
    assert not await store.unknown(forged, "attempt_cancelled")
    await store.finish(lease, await result_for(task))


async def test_result_event_cannot_reference_another_attempts_draft(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    await store.finish(lease, await result_for(task))
    other_store, other_run, other_task = await setup_execution(repository)
    other_lease = await other_store.reserve(other_run, other_task)
    assert isinstance(other_lease, InvocationLease)
    await other_store.finish(other_lease, await result_for(other_task))
    other_draft = (await other_store.snapshot(other_run))["outputs"][0]["draft_id"]
    with pytest.raises(Exception, match="creation_result_draft_attempt"):
        async with await repository.connect() as conn:
            await conn.execute(
                "UPDATE creation_result_outbox SET draft_id = %s WHERE attempt_id = %s",
                (other_draft, lease.attempt_id),
            )


@pytest.mark.parametrize(
    "code", ["harness_response_unknown", "harness_result_invalid", "attempt_cancelled"]
)
async def test_unknown_is_terminal_and_never_renews_budget(
    repository: Repository, code: str
) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    assert await store.unknown(lease, code)
    history = await store.attempt_history(run, lease.step_id)
    assert history
    attempt = history["attempts"][0]
    assert attempt["state"] == "unknown" and attempt["last_error"] == code
    assert attempt["fence"] == lease.fence and attempt["finished_at"] is not None
    assert attempt["usage_status"] == "unknown"
    with pytest.raises(ExecutionConflict, match="invocation_outcome_unknown"):
        await store.reserve(run, task)
    with pytest.raises(ExecutionConflict, match="attempt_fence_lost"):
        await store.finish(lease, await result_for(task))
    assert (await store.snapshot(run))["reserved_calls"] == 1
    assert await store.attempt_history(run, lease.step_id) == history


async def test_expiry_records_original_fence_without_fabricating_retry(
    repository: Repository,
) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    async with await repository.connect() as conn:
        await conn.execute(
            "UPDATE creation_steps SET lease_until = clock_timestamp() - interval '1 second' "
            "WHERE id = %s",
            (lease.step_id,),
        )
    with pytest.raises(ExecutionConflict, match="invocation_outcome_unknown"):
        await store.reserve(run, task)
    history = await store.attempt_history(run, lease.step_id)
    assert history and len(history["attempts"]) == 1
    assert history["attempts"][0]["last_error"] == "attempt_expired"
    assert history["attempts"][0]["fence"] == lease.fence
    assert (await store.snapshot(run))["steps"][0]["fence"] == lease.fence + 1


async def test_outbox_failure_rolls_back_attempt_completion(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    async with await repository.connect() as conn:
        await conn.execute(
            "ALTER TABLE creation_result_outbox ADD CONSTRAINT attempt_test_reject "
            "CHECK (false) NOT VALID"
        )
    try:
        with pytest.raises(Exception, match="attempt_test_reject"):
            await store.finish(lease, await result_for(task))
    finally:
        async with await repository.connect() as conn:
            await conn.execute(
                "ALTER TABLE creation_result_outbox DROP CONSTRAINT attempt_test_reject"
            )
    history = await store.attempt_history(run, lease.step_id)
    assert history and history["attempts"][0]["state"] == "running"
    assert history["attempts"][0]["finished_at"] is None
    assert not (await store.snapshot(run))["outputs"]
    await store.finish(lease, await result_for(task))


async def test_attempt_query_is_signed_scoped_and_omits_content(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    _, other, _ = await setup_execution(repository)
    app = create_app(repository, SECRET, "test")
    path = f"/internal/creation/commands/{run}/steps/{lease.step_id}/attempts"
    headers = {
        "X-Lanverse-Creation-Authorization": authorization(b"", path, "GET", int(time.time()) + 60)
    }
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as client:
        assert (await client.get(path)).status_code == 401
        response = await client.get(path, headers=headers)
        assert response.status_code == 200
        assert response.json()["schema"] == "creation-attempt-history-production"
        assert "candidate" not in response.text and "source" not in response.text
        assert "prompt" not in response.text and "task" not in response.text
        assert (await client.get(path + "?page=1", headers=headers)).status_code == 401
        body = b"{}"
        body_headers = {
            "X-Lanverse-Creation-Authorization": authorization(
                body, path, "GET", int(time.time()) + 60
            )
        }
        assert (
            await client.request("GET", path, content=body, headers=body_headers)
        ).status_code == 422
        other_path = path.replace(run, other)
        assert (await client.get(other_path, headers=headers)).status_code == 401
        headers["X-Lanverse-Creation-Authorization"] = authorization(
            b"", other_path, "GET", int(time.time()) + 60
        )
        assert (await client.get(other_path, headers=headers)).status_code == 404


async def test_attempt_insert_failure_does_not_consume_budget(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    async with await repository.connect() as conn:
        await conn.execute(
            "ALTER TABLE creation_attempts ADD CONSTRAINT attempt_insert_reject "
            "CHECK (false) NOT VALID"
        )
    try:
        with pytest.raises(Exception, match="attempt_insert_reject"):
            await store.reserve(run, task)
    finally:
        async with await repository.connect() as conn:
            await conn.execute(
                "ALTER TABLE creation_attempts DROP CONSTRAINT attempt_insert_reject"
            )
    snapshot = await store.snapshot(run)
    assert snapshot["reserved_calls"] == 0 and snapshot["steps"] == []
    assert isinstance(await store.reserve(run, task), InvocationLease)


async def test_attempt_identity_and_terminal_record_are_immutable(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    with pytest.raises(Exception, match="immutable"):
        async with await repository.connect() as conn:
            await conn.execute(
                "UPDATE creation_attempts SET fence = fence + 1 WHERE id = %s", (lease.attempt_id,)
            )
    await store.finish(lease, await result_for(task))
    with pytest.raises(Exception, match="immutable"):
        async with await repository.connect() as conn:
            await conn.execute(
                "UPDATE creation_attempts SET state = 'running', finished_at = NULL, "
                "result_hash = NULL WHERE id = %s",
                (lease.attempt_id,),
            )


async def test_finish_racing_unknown_has_one_durable_outcome(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    result = await result_for(task)
    outcomes = await asyncio.gather(
        store.finish(lease, result),
        store.unknown(lease, "harness_response_unknown"),
        return_exceptions=True,
    )
    history = await store.attempt_history(run, lease.step_id)
    assert history
    state = history["attempts"][0]["state"]
    if state == "succeeded":
        assert outcomes == [result, False]
        assert len((await store.snapshot(run))["outputs"]) == 1
    else:
        assert state == "unknown"
        assert isinstance(outcomes[0], ExecutionConflict) and outcomes[1] is True
        assert not (await store.snapshot(run))["outputs"]
    assert (await store.snapshot(run))["reserved_calls"] == 1


async def test_upgrade_preserves_legacy_results_without_inventing_attempts(
    repository: Repository,
) -> None:
    store, run, task = await setup_execution(repository)
    step, draft = str(uuid4()), str(uuid4())
    result = await result_for(task)
    # Reconstruct the immediately preceding schema only inside the dedicated test database.
    async with await repository.connect() as conn:
        checksums = await (
            await conn.execute(
                "SELECT name, checksum FROM creation_schema "
                "WHERE name <> 'text-attempts' ORDER BY name"
            )
        ).fetchall()
        await conn.execute("ALTER TABLE creation_result_outbox DROP COLUMN attempt_id")
        await conn.execute("ALTER TABLE creation_drafts DROP COLUMN attempt_id")
        await conn.execute("ALTER TABLE creation_steps DROP COLUMN current_attempt_id")
        await conn.execute("DROP TABLE creation_recoveries, creation_attempt_failures")
        await conn.execute(
            "DROP FUNCTION creation_guard_recovery(), creation_guard_failure_receipt()"
        )
        await conn.execute("DELETE FROM creation_schema WHERE name = 'text-recovery'")
        await conn.execute("DROP TABLE creation_attempts")
        await conn.execute("DROP FUNCTION creation_guard_attempt()")
        await conn.execute("DELETE FROM creation_schema WHERE name = 'text-attempts'")
        await conn.execute(
            "INSERT INTO creation_steps "
            "(id, command_id, step_key, input_hash, task, state, fence) "
            "VALUES (%s, %s, 'map_manuscript', %s, %s, 'needs_review', 1)",
            (
                step,
                run,
                canonical_hash(task.model_dump(mode="json")),
                Jsonb(task.model_dump(mode="json")),
            ),
        )
        await conn.execute(
            "INSERT INTO creation_drafts (id, step_id, candidate_hash, result_hash, result) "
            "VALUES (%s, %s, %s, %s, %s)",
            (draft, step, result["candidate_hash"], canonical_hash(result), Jsonb(result)),
        )
    from app.creation.repository import SchemaMismatch

    with pytest.raises(SchemaMismatch, match="(attempt|recovery) migration"):
        await repository.ready()
    await repository.migrate()
    await repository.migrate()
    await repository.ready()
    history = await store.attempt_history(run, step)
    assert history and history["history_origin"] == "unavailable"
    assert history["current_attempt_id"] is None and history["attempts"] == []
    assert await store.reserve(run, task) == result
    assert await store.draft(run, draft) == result
    async with await repository.connect() as conn:
        assert (
            await (
                await conn.execute(
                    "SELECT name, checksum FROM creation_schema "
                    "WHERE name <> 'text-attempts' ORDER BY name"
                )
            ).fetchall()
            == checksums
        )


async def test_attempt_migration_checksum_drift_is_not_silently_repaired(
    repository: Repository,
) -> None:
    from app.creation.attempt_schema import ATTEMPT_SCHEMA_HASH
    from app.creation.repository import SchemaMismatch

    async with await repository.connect() as conn:
        await conn.execute(
            "UPDATE creation_schema SET checksum = 'drift' WHERE name = 'text-attempts'"
        )
    try:
        with pytest.raises(SchemaMismatch, match="attempt migration"):
            await repository.ready()
        with pytest.raises(SchemaMismatch, match="attempt migration"):
            await repository.migrate()
    finally:
        async with await repository.connect() as conn:
            await conn.execute(
                "UPDATE creation_schema SET checksum = %s WHERE name = 'text-attempts'",
                (ATTEMPT_SCHEMA_HASH,),
            )
