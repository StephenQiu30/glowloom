from typing import Any
from uuid import uuid4

import httpx
import pytest

from app.creation.execution import ExecutionConflict, InvocationLease
from app.creation.platform import HarnessClient
from app.creation.repository import Repository
from app.protocol.canonical import canonical_hash
from tests.creation.test_execution import result_for, setup_execution


async def test_verified_failure_is_distinct_from_unknown_and_preserves_candidate(
    repository: Repository,
) -> None:
    from app.text_contract.failure import HarnessFailed, InvocationFailure

    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    value = InvocationFailure(
        invocation_id=task.invocation_id,
        input_hash=canonical_hash(task.model_dump(mode="json")),
        release_hash=task.release_hash,
        phase="validation",
        code="candidate_contract_invalid",
        diagnostic="evidence quote differs from the source",
        candidate={"synthetic": True},
    )
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(422, json={"detail": value.model_dump(mode="json")})
        )
    ) as http:
        with pytest.raises(HarnessFailed) as error:
            await HarnessClient(http, "http://harness", "independent-secret" * 3).invoke(task)
    assert await store.failed(lease, error.value.failure)
    history = await store.attempt_history(run, lease.step_id)
    assert history and history["attempts"][0]["state"] == "failed"
    async with await repository.connect() as conn:
        row = await (
            await conn.execute(
                "SELECT receipt FROM creation_attempt_failures WHERE attempt_id=%s",
                (lease.attempt_id,),
            )
        ).fetchone()
        assert row and row["receipt"]["candidate"] == {"synthetic": True}
    assert (await store.snapshot(run))["outputs"] == []


async def test_recovery_is_single_use_budgeted_and_never_rewrites_old_attempt(
    repository: Repository,
) -> None:
    from app.creation.recovery import RecoveryStore

    store, run, task = await setup_execution(repository, limit=2)
    old = await store.reserve(run, task)
    assert isinstance(old, InvocationLease)
    await store.unknown(old, "harness_response_unknown")
    await store.progress(run, "blocked", task.stage, "harness_response_unknown")
    recovery = RecoveryStore(repository)
    request_id = str(uuid4())
    args = (
        request_id,
        run,
        old.step_id,
        old.attempt_id,
        str(uuid4()),
        3,
        "operator verified original process ended; result unavailable",
    )
    await recovery.authorize(*args)
    await recovery.authorize(*args)
    new = await store.reserve(run, task)
    assert isinstance(new, InvocationLease)
    assert new.fence > old.fence and new.attempt_id != old.attempt_id
    with pytest.raises(ExecutionConflict, match="invocation_in_progress"):
        await store.reserve(run, task)
    with pytest.raises(ExecutionConflict, match="attempt_fence_lost"):
        await store.finish(old, await result_for(task))
    await store.finish(new, await result_for(task))
    assert isinstance(await store.reserve(run, task), dict)
    history = await store.attempt_history(run, old.step_id)
    assert history and [a["state"] for a in history["attempts"]] == ["unknown", "succeeded"]
    assert (await store.snapshot(run))["reserved_calls"] == 2


@pytest.mark.parametrize("case", ["foreign_input", "malformed", "legacy"])
async def test_unverified_http_failure_never_authorizes_retry(case: str) -> None:
    from app.creation.platform import PlatformUnavailable
    from app.modules.text_storyboard.harness import RELEASE_HASH
    from app.text_contract.task import TextTask
    from tests.unit.text_storyboard_samples import sample

    task = TextTask(
        invocation_id="synthetic",
        stage="map_manuscript",
        source=sample()[0],
        release_hash=RELEASE_HASH,
    )
    detail: Any = "candidate_or_input_contract_invalid"
    if case == "foreign_input":
        from app.text_contract.failure import InvocationFailure

        detail = InvocationFailure(
            invocation_id="foreign",
            input_hash="0" * 64,
            release_hash=RELEASE_HASH,
            phase="preflight",
            code="context_insufficient",
        ).model_dump(mode="json")
    if case == "malformed":
        detail = {"candidate": "untrusted"}
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _: httpx.Response(422, json={"detail": detail}))
    ) as http:
        with pytest.raises(PlatformUnavailable):
            await HarnessClient(http, "http://harness", "secret" * 8).invoke(task)


async def test_native_temporal_reset_recovers_one_attempt_and_preserves_unknown_history(
    repository: Repository,
) -> None:
    import asyncio
    from unittest.mock import AsyncMock

    from temporalio.client import Client
    from temporalio.worker import Worker

    from app.creation.activities import CreationActivities
    from app.creation.execution import ExecutionStore
    from app.creation.platform import PlatformUnavailable
    from app.creation.recover import recover
    from app.creation.temporal import TemporalStarter
    from app.creation.workflow import TextStoryboardWorkflow
    from app.modules.text_storyboard.harness import RELEASE_HASH
    from app.text_contract.task import TextTask
    from tests.creation.test_native_runtime import native_temporal_address
    from tests.creation.test_repository import new_command
    from tests.unit.text_storyboard_samples import sample

    source = sample()[0]
    command = new_command()
    command = command.model_copy(
        update={
            "source": command.source.model_copy(
                update={"revision_id": source.revision_id, "content_hash": source.content_hash}
            )
        }
    )
    queue = "lanverse-recovery-test-" + uuid4().hex
    await repository.accept(command, queue)
    client = await Client.connect(native_temporal_address())
    platform = AsyncMock()
    platform.source.return_value = source
    platform.gate.return_value = {"status": "pending"}
    calls: list[str] = []

    async def invoke(task: TextTask) -> dict[str, Any]:
        calls.append(task.invocation_id)
        if len(calls) == 1:
            raise PlatformUnavailable("lost response")
        return await result_for(task)

    harness = AsyncMock()
    harness.invoke.side_effect = invoke
    store = ExecutionStore(repository)
    activities = CreationActivities(store, platform, harness, RELEASE_HASH, 8)

    async def wait_status(status: str) -> dict[str, Any]:
        async with asyncio.timeout(25):
            while True:
                try:
                    snapshot = await store.snapshot(command.command_id)
                    if snapshot["status"] == status:
                        return snapshot
                except ExecutionConflict:
                    pass
                await asyncio.sleep(0.05)

    handle = client.get_workflow_handle(command.workflow_id)
    try:
        async with Worker(
            client,
            task_queue=queue,
            workflows=[TextStoryboardWorkflow],
            activities=[activities.invoke, activities.gate, activities.progress],
        ):
            old_run = await TemporalStarter(client).ensure_started(command, queue)
            snapshot = await wait_status("blocked")
            step = snapshot["steps"][0]
            history = await store.attempt_history(command.command_id, step["id"])
            assert history
            args = dict(
                request_id=str(uuid4()),
                command_id=command.command_id,
                step_id=step["id"],
                attempt_id=history["current_attempt_id"],
                reason="synthetic original invocation exited without recoverable result",
            )
            new_run = await recover(repository, client, **args)
            assert new_run != old_run
            done = await wait_status("waiting_review")
            assert len(calls) == 2 and calls[0] == calls[1] and done["reserved_calls"] == 2
            assert len(done["outputs"]) == 1
            assert await recover(repository, client, **args) == new_run
            final = await store.attempt_history(command.command_id, step["id"])
            assert final
            assert [a["state"] for a in final["attempts"]] == ["unknown", "succeeded"]
    finally:
        await handle.terminate("synthetic recovery test completed")


async def test_recovery_rejects_exhausted_budget_and_active_attempt(repository: Repository) -> None:
    from app.creation.recovery import RecoveryStore

    store, run, task = await setup_execution(repository, limit=1)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    await store.progress(run, "blocked", task.stage, "harness_response_unknown")
    recovery = RecoveryStore(repository)
    args = (
        str(uuid4()),
        run,
        lease.step_id,
        lease.attempt_id,
        str(uuid4()),
        3,
        "operator verified original process ended",
    )
    with pytest.raises(ExecutionConflict, match="recovery_target_invalid"):
        await recovery.authorize(*args)
    await store.unknown(lease, "harness_response_unknown")
    with pytest.raises(ExecutionConflict, match="invocation_budget_exhausted"):
        await recovery.authorize(*args)


async def test_bound_execution_error_preserves_receipt_but_remains_unknown(
    repository: Repository,
) -> None:
    from app.text_contract.failure import HarnessFailed, InvocationFailure

    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    value = InvocationFailure(
        invocation_id=task.invocation_id,
        input_hash=canonical_hash(task.model_dump(mode="json")),
        release_hash=task.release_hash,
        phase="generation",
        code="reasoning_execution_failed_or_unknown",
        diagnostic="synthetic subprocess failure",
    )
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(502, json={"detail": value.model_dump(mode="json")})
        )
    ) as http:
        with pytest.raises(HarnessFailed) as error:
            await HarnessClient(http, "http://harness", "secret" * 8).invoke(task)
    assert await store.failed(lease, error.value.failure)
    history = await store.attempt_history(run, lease.step_id)
    assert history and history["attempts"][0]["state"] == "unknown"
    assert history["attempts"][0]["last_error"] == "harness_response_unknown"
    async with await repository.connect() as conn:
        row = await (
            await conn.execute(
                "SELECT receipt FROM creation_attempt_failures WHERE attempt_id=%s",
                (lease.attempt_id,),
            )
        ).fetchone()
        assert row and row["receipt"]["diagnostic"] == "synthetic subprocess failure"
    with pytest.raises(ExecutionConflict):
        await store.reserve(run, task)
