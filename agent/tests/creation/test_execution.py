import asyncio
from typing import Any

import pytest

from app.candidate_runtime.canonical import canonical_hash
from app.creation.execution import ExecutionConflict, ExecutionStore, InvocationLease
from app.creation.repository import Repository
from app.modules.text_storyboard.harness import RELEASE_HASH, TextHarness
from app.text_contract.task import TextTask
from tests.creation.test_repository import new_command
from tests.unit.text_storyboard_samples import sample


async def setup_execution(
    repository: Repository, *, limit: int = 2
) -> tuple[ExecutionStore, str, TextTask]:
    command = new_command()
    original = sample()[0]
    command = command.model_copy(
        update={
            "source": command.source.model_copy(
                update={
                    "revision_id": original.revision_id,
                    "content_hash": original.content_hash,
                }
            )
        }
    )
    await repository.accept(command, "text-test")
    store = ExecutionStore(repository)
    await store.freeze(command.command_id, RELEASE_HASH, limit)
    task = TextTask(
        invocation_id=command.run_id + "/map_manuscript",
        stage="map_manuscript",
        source=original,
        release_hash=RELEASE_HASH,
    )
    return store, command.command_id, task


async def result_for(task: TextTask) -> dict[str, Any]:
    async def reason(*_: Any) -> Any:
        return sample()[1]

    return (await TextHarness(reason).execute(task)).model_dump(mode="json")


async def test_concurrent_reservation_replay_and_atomic_output(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    attempts = await asyncio.gather(
        *(store.reserve(run, task) for _ in range(8)), return_exceptions=True
    )
    leases = [item for item in attempts if isinstance(item, InvocationLease)]
    assert len(leases) == 1
    assert all(isinstance(item, (InvocationLease, ExecutionConflict)) for item in attempts)
    lease = leases[0]
    result = await result_for(task)
    saved = await store.finish(lease, result)
    restarted = ExecutionStore(Repository(repository.dsn))
    replay = await restarted.reserve(run, task)
    assert replay == saved
    assert (await restarted.snapshot(run))["reserved_calls"] == 1
    async with await repository.connect() as conn:
        for table in ("creation_drafts", "creation_output_bindings", "creation_result_outbox"):
            row = await (await conn.execute(f"SELECT count(*) AS n FROM {table}")).fetchone()
            assert row and row["n"] == 1
    assert saved["candidate_hash"] == canonical_hash(result["candidate"])


async def test_expired_attempt_is_unknown_and_not_automatically_retried(
    repository: Repository,
) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    async with await repository.connect() as conn:
        await conn.execute(
            "UPDATE creation_steps SET lease_until = clock_timestamp() - interval '1 second'"
        )
    with pytest.raises(ExecutionConflict, match="invocation_outcome_unknown"):
        await ExecutionStore(Repository(repository.dsn)).reserve(run, task)
    with pytest.raises(ExecutionConflict, match="attempt_fence_lost"):
        await store.finish(lease, await result_for(task))
    snapshot = await store.snapshot(run)
    assert snapshot["reserved_calls"] == 1
    assert snapshot["steps"][0]["state"] == "unknown"
    assert snapshot["steps"][0]["usage_status"] == "unknown"


async def test_policy_input_and_result_bindings_are_immutable(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    with pytest.raises(ExecutionConflict, match="execution_policy_conflict"):
        await store.freeze(run, RELEASE_HASH, 3)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    changed = task.model_copy(update={"timeout_seconds": 299})
    with pytest.raises(ExecutionConflict, match="step_input_conflict"):
        await store.reserve(run, changed)
    result = await result_for(task)
    result["input_hash"] = "0" * 64
    with pytest.raises(ValueError, match="result_binding_mismatch"):
        await store.finish(lease, result)
    result = await result_for(task)
    result["candidate"]["episodes"][0]["last_block"] = 99999
    result["candidate_hash"] = canonical_hash(result["candidate"])
    with pytest.raises(ValueError):
        await store.finish(lease, result)
    assert not (await store.snapshot(run))["outputs"]


async def test_budget_persists_and_unknown_failure_retains_reservation(
    repository: Repository,
) -> None:
    store, run, task = await setup_execution(repository, limit=1)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    assert await store.unknown(lease, "harness_response_unknown")
    with pytest.raises(ExecutionConflict, match="invocation_outcome_unknown"):
        await store.reserve(run, task)
    second = TextTask(
        invocation_id=run + "/analyze_episode/episode_1",
        stage="analyze_episode",
        source=task.source,
        release_hash=RELEASE_HASH,
        episode_map=sample()[1],
        episode_key=sample()[1].episodes[0].key,
    )
    with pytest.raises(ExecutionConflict, match="invocation_budget_exhausted"):
        await store.reserve(run, second)
    assert (await store.snapshot(run))["reserved_calls"] == 1


async def test_result_outbox_failure_rolls_back_draft_and_binding(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    async with await repository.connect() as conn:
        await conn.execute(
            "ALTER TABLE creation_result_outbox ADD CONSTRAINT test_reject CHECK (false) NOT VALID"
        )
    try:
        with pytest.raises(Exception, match="test_reject"):
            await store.finish(lease, await result_for(task))
    finally:
        async with await repository.connect() as conn:
            await conn.execute("ALTER TABLE creation_result_outbox DROP CONSTRAINT test_reject")
    assert not (await store.snapshot(run))["outputs"]
    saved = await store.finish(lease, await result_for(task))
    assert saved["status"] == "needs_review"
