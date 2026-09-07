from types import SimpleNamespace
from typing import Any
from unittest.mock import AsyncMock
from uuid import uuid4

import pytest
from temporalio.common import WorkflowIDReusePolicy
from temporalio.exceptions import WorkflowAlreadyStartedError
from temporalio.service import RPCError, RPCStatusCode

from app.creation.dispatcher import Dispatcher
from app.creation.repository import Repository
from app.creation.temporal import StartPolicyViolation, TemporalStarter, WorkflowIdentityConflict
from tests.creation.test_repository import new_command


class Description:
    def __init__(self, memo: dict[str, Any], workflow_type: str, task_queue: str) -> None:
        self.values = memo
        self.workflow_type = workflow_type
        self.task_queue = task_queue
        self.run_id = str(uuid4())

    async def memo_value(self, key: str, *, default: Any = None) -> Any:
        return self.values.get(key)


def temporal_double() -> tuple[Any, Any]:
    client = AsyncMock()
    client.namespace = "default"
    client.workflow_service.describe_namespace.return_value = SimpleNamespace(
        config=SimpleNamespace(workflow_execution_retention_ttl=SimpleNamespace(seconds=86400))
    )
    handle = AsyncMock()
    from unittest.mock import Mock

    client.get_workflow_handle = Mock(return_value=handle)
    return client, handle


async def test_existing_identity_is_reconciled_without_second_start() -> None:
    command = new_command()
    client, handle = temporal_double()
    description = Description(TemporalStarter.memo(command), command.flow_type, "creation-text")
    handle.describe.return_value = description
    starter = TemporalStarter(client)
    assert await starter.ensure_started(command, "creation-text") == description.run_id
    client.start_workflow.assert_not_called()
    description.values["creation_payload_hash"] = "b" * 64
    with pytest.raises(WorkflowIdentityConflict):
        await starter.ensure_started(command, "creation-text")
    client.start_workflow.assert_not_called()


async def test_start_race_uses_reject_duplicate_then_checks_original_identity() -> None:
    command = new_command()
    client, handle = temporal_double()
    description = Description(TemporalStarter.memo(command), command.flow_type, "creation-text")
    handle.describe.side_effect = [RPCError("absent", RPCStatusCode.NOT_FOUND, b""), description]
    client.start_workflow.side_effect = WorkflowAlreadyStartedError(
        command.workflow_id, command.flow_type
    )
    assert (
        await TemporalStarter(client).ensure_started(command, "creation-text") == description.run_id
    )
    assert (
        client.start_workflow.call_args.kwargs["id_reuse_policy"]
        == WorkflowIDReusePolicy.REJECT_DUPLICATE
    )
    assert client.start_workflow.call_args.kwargs["id"] == command.workflow_id


async def test_unavailable_lookup_does_not_start() -> None:
    client, handle = temporal_double()
    handle.describe.side_effect = RPCError("unavailable", RPCStatusCode.UNAVAILABLE, b"")
    with pytest.raises(RPCError):
        await TemporalStarter(client).ensure_started(new_command(), "creation-text")
    client.start_workflow.assert_not_called()


async def test_lost_start_reply_is_recovered_from_original_command(repository: Repository) -> None:
    command = new_command()
    await repository.accept(command, "creation-text")
    starter = AsyncMock()
    starter.ensure_started.side_effect = TimeoutError()
    dispatcher = Dispatcher(repository, starter)
    assert await dispatcher.once()
    async with await repository.connect() as conn:
        row = await (
            await conn.execute("SELECT state, last_error FROM creation_start_outbox")
        ).fetchone()
        assert row == {"state": "pending", "last_error": "temporal_outcome_unknown"}
        await conn.execute("UPDATE creation_start_outbox SET next_attempt_at = clock_timestamp()")
    starter.ensure_started.side_effect = None
    starter.ensure_started.return_value = str(uuid4())
    assert await Dispatcher(Repository(repository.dsn), starter).once()
    assert starter.ensure_started.call_args.args == (command, "creation-text")
    assert await repository.claim() is None


async def test_expired_start_window_never_recreates_missing_history() -> None:
    command = new_command()
    client, handle = temporal_double()
    handle.describe.side_effect = RPCError("absent", RPCStatusCode.NOT_FOUND, b"")
    with pytest.raises(StartPolicyViolation):
        await TemporalStarter(client).ensure_started(command, "creation-text", allow_start=False)
    client.start_workflow.assert_not_called()


async def test_short_namespace_retention_blocks_start() -> None:
    client, handle = temporal_double()
    handle.describe.side_effect = RPCError("absent", RPCStatusCode.NOT_FOUND, b"")
    config = client.workflow_service.describe_namespace.return_value.config
    config.workflow_execution_retention_ttl.seconds = 60
    with pytest.raises(StartPolicyViolation):
        await TemporalStarter(client).ensure_started(new_command(), "creation-text")
    client.start_workflow.assert_not_called()


async def test_cancellation_leaves_original_lease_for_recovery(repository: Repository) -> None:
    import asyncio

    command = new_command()
    await repository.accept(command, "creation-text")
    entered = asyncio.Event()

    async def wait_for_cancellation(*args: Any, **kwargs: Any) -> str:
        entered.set()
        await asyncio.Event().wait()
        raise AssertionError("unreachable")

    starter = AsyncMock()
    starter.ensure_started.side_effect = wait_for_cancellation
    task = asyncio.create_task(Dispatcher(repository, starter).run())
    try:
        async with asyncio.timeout(2):
            await entered.wait()
    finally:
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task
    assert await repository.claim() is None
    async with await repository.connect() as conn:
        await conn.execute(
            "UPDATE creation_start_outbox SET lease_until = clock_timestamp() - interval '1 second'"
        )
    recovered = await repository.claim()
    assert recovered is not None and recovered.command == command


async def test_dispatcher_persists_identity_conflict_as_blocked(repository: Repository) -> None:
    await repository.accept(new_command(), "creation-text")
    starter = AsyncMock()
    starter.ensure_started.side_effect = WorkflowIdentityConflict()
    assert await Dispatcher(repository, starter).once()
    async with await repository.connect() as conn:
        row = await (
            await conn.execute("SELECT state, last_error FROM creation_start_outbox")
        ).fetchone()
        assert row == {"state": "blocked", "last_error": "workflow_identity_conflict"}
    assert await repository.claim() is None
